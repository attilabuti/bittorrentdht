package bittorrentdht

// Kademlia DHT K-bucket implementation as a binary tree.
// KBucket was ported from Tristan Slominski's k-bucket (https://github.com/tristanls/k-bucket)
//
// A Distributed Hash Table (DHT) is a decentralized distributed system that
// provides a lookup table similar to a hash table.
//
// k-bucket is an implementation of a storage mechanism for keys within a DHT.
// It stores Contact objects which represent locations and addresses of nodes in
// the decentralized distributed system. Contact objects are typically identified
// by a SHA-1 hash, however this restriction is lifted in this implementation.
// Additionally, node ids of different lengths can be compared.
//
// This Kademlia DHT k-bucket implementation is meant to be as minimal as possible.
// It assumes that Contact objects consist only of Id. It is useful, and necessary,
// to attach other properties to a Contact. For example, one may want to attach
// ip and port properties, which allow an application to send IP traffic to the
// Contact. However, this information is extraneous and irrelevant to the operation
// of a k-bucket.
//
//
// The MIT License (MIT)
//
// Copyright (c) 2022 Attila Buti
// Copyright (c) 2013-2022 Tristan Slominski
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

import (
	"bytes"
	"errors"
	"math"
	"net/netip"
	"sort"
	"sync"
	"time"

	"github.com/attilabuti/eventemitter"
)

type KBucketOptions struct {
	LocalNodeId     []byte // Local node ID.
	NodesPerKBucket int    // The number of nodes that a KBucket can contain before being full or split.
	NodesToPing     int    // The number of nodes to ping when a bucket that should not be split becomes full.
}

type KBucket struct {
	id         []byte                         // The local node ID.
	size       int                            // The number of nodes that a KBucket can contain before being full or split.
	ping       int                            // The number of nodes to ping when a bucket that should not be split becomes full.
	root       *node                          // The root node of the KBucket.
	distanceFn func([]byte, []byte) int       // An optional distance function that gets two id and return distance (as number) between them.
	arbiterFn  func(Contact, Contact) Contact // An optional arbiter function that given two contact objects with the same id returns the desired object to be used for updating the KBucket.
	emitter    *eventemitter.Emitter          // The emitter to use for emitting events.
	mutex      sync.RWMutex

	// Optional satellite data to include with the KBucket. Metadata property is
	// guaranteed not be altered, it is provided as an explicit container for users
	// of KBucket to store implementation-specific data.
	Metadata map[string]any
}

type node struct {
	contacts  Contacts
	dontSplit bool
	left      *node
	right     *node
}

type Contact struct {
	Id          []byte         // The node id.
	AddrPort    netip.AddrPort // The address and port of the node.
	SeenAt      time.Time      // SeenAt is the time this node was last seen.
	VectorClock int
	Metadata    map[string]any // Optional satellite data to include with the Contact.
	distance    int
}

type Contacts []Contact

// Implementation of a Kademlia DHT k-bucket used for storing contact (peer node) information.
// NewKBucket creates a new KBucket with the given options.
func NewKBucket(options KBucketOptions, emitter *eventemitter.Emitter) (*KBucket, error) {
	if len(options.LocalNodeId) == 0 {
		return &KBucket{}, errors.New("Local node ID cannot be empty")
	}

	if options.NodesPerKBucket < 1 {
		return &KBucket{}, errors.New("Nodes per k-bucket must be at least 1")
	}

	if options.NodesToPing < 1 {
		return &KBucket{}, errors.New("Nodes to ping must be at least 1")
	}

	return &KBucket{
		id:       options.LocalNodeId,
		size:     options.NodesPerKBucket,
		ping:     options.NodesToPing,
		root:     createNode(),
		emitter:  emitter,
		Metadata: map[string]any{},
	}, nil
}

// Adds a contact to the kBucket.
func (b *KBucket) Add(contact Contact) *KBucket {
	b.mutex.Lock()

	bitIndex := 0
	node := b.root

	for node.contacts == nil {
		// This is not a leaf node but an inner node with "low" and "high" branches;
		// we will check the appropriate bit of the identifier and delegate to the
		// appropriate node for further processing.
		node = node.determine(contact.Id, bitIndex)
		bitIndex++
	}

	// Check if the contact already exists.
	if i := node.contacts.indexOf(contact.Id); i >= 0 {
		old, new, ok := b.update(node, i, contact)

		if ok {
			// Event kbucket.updated emitted when a previously existing ("previously
			// existing" means oldContact.Id equals newContact.Id) contact was added to
			// the bucket and it was replaced with newContact.
			b.emitter.Emit("kbucket.updated", old, new)
		}

		b.mutex.Unlock()
		return b
	}

	if len(node.contacts) < b.size {
		contact.SeenAt = time.Now()

		node.contacts = append(node.contacts, contact)

		// Event kbucket.added emitted only when newContact was added to bucket
		// and it was not stored in the bucket before.
		b.emitter.Emit("kbucket.added", contact)

		b.mutex.Unlock()
		return b
	}

	// The bucket is full.
	if node.dontSplit {
		// We are not allowed to split the bucket.
		// We need to ping the first kBucket.Ping in order to determine if they
		// are alive.
		// Only if one of the pinged nodes does not respond can the new contact
		// be added (this prevents DDoS flooding with new invalid contacts).
		b.emitter.Emit("kbucket.ping", node.contacts[0:b.ping], contact)

		b.mutex.Unlock()
		return b
	} else {
		// The bucket is full and we are allowed to split it.
		node.split(bitIndex, b.id)
	}

	b.mutex.Unlock()
	return b.Add(contact)
}

// Get a contact by its exact id. Returns an empty Contact if the contact is not found.
func (b *KBucket) Get(id []byte) Contact {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	i, contact := b.get(id)

	if i == -1 {
		return Contact{}
	}

	return *contact
}

// Has returns true if the contact with the given id is in the kBucket, false otherwise.
func (b *KBucket) Has(id []byte) bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	i, _ := b.get(id)

	return i != -1
}

// Seen updates the contact's last seen time.
// Returns true if the contact was found, false otherwise.
func (b *KBucket) Seen(id []byte) bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	i, contact := b.get(id)
	if i < 0 {
		return false
	}

	contact.SeenAt = time.Now()

	return true
}

// Removes contact with the provided id.
func (b *KBucket) Remove(id []byte) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	node := b.root

	for bitIndex := 0; node.contacts == nil; {
		node = node.determine(id, bitIndex)
		bitIndex++
	}

	if i := node.contacts.indexOf(id); i >= 0 {
		// Event kbucket.removed emitted when contact was removed from the bucket.
		b.emitter.Emit("kbucket.removed", node.contacts[i])
		node.contacts = append(node.contacts[:i], node.contacts[i+1:]...)
	}
}

// Get the n closest contacts to the provided node id. "Closest" here means:
// closest according to the XOR metric of the contact node id.
// Return the maximum of n closest contacts to the node id.
func (b *KBucket) Closest(id []byte, n uint) Contacts {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	contacts := Contacts{}

	for bitIndex, nodes := 0, []node{*b.root}; len(nodes) > 0 && len(contacts) < int(n); {
		node := nodes[len(nodes)-1]
		nodes = nodes[:len(nodes)-1]

		if node.contacts == nil {
			detNode := node.determine(id, bitIndex)
			bitIndex++

			if node.left == detNode {
				nodes = append(nodes, *node.right)
			} else {
				nodes = append(nodes, *node.left)
			}

			nodes = append(nodes, *detNode)
		} else {
			contacts = append(contacts, node.contacts...)
		}
	}

	for i, v := range contacts {
		contacts[i].distance = b.Distance(v.Id, id)
	}

	sort.Slice(contacts, func(i, j int) bool {
		return contacts[i].distance < contacts[j].distance
	})

	if int(n) > len(contacts) {
		return contacts
	}

	return contacts[0:n]
}

// SetDistanceFn overrides the default distance function.
func (b *KBucket) SetDistanceFn(distanceFn func([]byte, []byte) int) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	b.distanceFn = distanceFn
}

// Default distance function. Finds the XOR distance between first id and second id.
// Returns the XOR distance between first id and second id.
func (b *KBucket) Distance(fid, sid []byte) int {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if b.distanceFn != nil {
		return b.distanceFn(fid, sid)
	}

	dist := 0
	i := 0

	for i < int(math.Min(float64(len(fid)), float64(len(sid)))) {
		dist = dist*256 + (int(fid[i]) ^ int(sid[i]))
		i++
	}

	for i < int(math.Max(float64(len(fid)), float64(len(sid)))) {
		dist = dist*256 + 255
		i++
	}

	return dist
}

// Count returns the number of contacts in the KBucket.
func (b *KBucket) Count() int {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	count, _ := b.getAll(true)

	return count
}

// ToSlice returns a slice with all contacts in the KBucket.
func (b *KBucket) ToSlice() Contacts {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	_, contacts := b.getAll(false)

	return contacts
}

// SetArbiterFn overrides the default arbiter function.
func (b *KBucket) SetArbiterFn(arbiterFn func(Contact, Contact) Contact) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	b.arbiterFn = arbiterFn
}

// Default arbiter function for contacts with the same id.
// Uses Contact.VectorClock to select which contact to update the kBucket with.
// Contact with larger VectorClock field will be selected. If VectorClock is the
// same, candidat will be selected.
func (b *KBucket) arbiter(incumbent Contact, candidate Contact) Contact {
	if b.arbiterFn != nil {
		return b.arbiterFn(incumbent, candidate)
	}

	if incumbent.VectorClock > candidate.VectorClock {
		return incumbent
	}

	return candidate
}

// Get the index of the contact and the contact itself by its exact id.
// Returns -1 and an empty contact if the contact is not found.
func (b *KBucket) get(id []byte) (int, *Contact) {
	node := b.root

	for bitIndex := 0; node.contacts == nil; {
		node = node.determine(id, bitIndex)
		bitIndex++
	}

	if i := node.contacts.indexOf(id); i >= 0 {
		return i, &node.contacts[i]
	}

	return -1, &Contact{}
}

// getAll returns the number of contacts or a slice with all contacts.
func (b *KBucket) getAll(justCount bool) (int, Contacts) {
	var count int
	var contacts Contacts

	for nodes := []node{*b.root}; len(nodes) > 0; {
		node := nodes[len(nodes)-1]  // Get the last node.
		nodes = nodes[:len(nodes)-1] // Remove the last node from the list.

		if node.contacts == nil {
			nodes = append(nodes, *node.right, *node.left)
		} else {
			if justCount {
				count += len(node.contacts)
			} else {
				contacts = append(contacts, node.contacts...)
			}
		}
	}

	return count, contacts
}

// Updates the contact by using the arbiter function to compare the incumbent and
// the candidate. If arbiter function selects the old contact but the candidate is
// some new contact, then the new contact is abandoned. If arbiter function selects
// the old contact and the candidate is that same old contact, the contact is marked
// as most recently contacted (by being moved to the right/end of the bucket array).
// If arbiter function selects the new contact, the old contact is removed and the
// new contact is marked as most recently contacted.
func (b *KBucket) update(n *node, i int, contact Contact) (Contact, Contact, bool) {
	if bytes.Compare(n.contacts[i].Id, contact.Id) != 0 {
		return Contact{}, Contact{}, false
	}

	incumbent := n.contacts[i]
	selection := b.arbiter(incumbent, contact)

	// If the selection is our old contact and the candidate is some new contact,
	// then there is nothing to do.
	if selection.compare(incumbent) && !incumbent.compare(contact) {
		return Contact{}, Contact{}, false
	}

	n.contacts = append(n.contacts[:i], n.contacts[i+1:]...) // Remove old contact
	n.contacts = append(n.contacts, selection)               // Add more recent contact version

	return incumbent, selection, true
}

// Determines whether the id at the bitIndex is 0 or 1.
// Return left leaf if "id" at "bitIndex" is 0, right leaf otherwise.
func (n *node) determine(id []byte, bitIndex int) *node {
	// Id's that are too short are put in low bucket (1 byte = 8 bits).
	// (bitIndex >> 3) finds how many bytes the bitIndex describes.
	// bitIndex % 8 checks if we have extra bits beyond byte multiples.
	// If number of bytes is <= no. of bytes described by bitIndex and there are
	// extra bits to consider, this means id has less bits than what bitIndex
	// describes, id therefore is too short, and will be put in low bucket.
	bytesDescribedByBitIndex := bitIndex >> 3
	bitIndexWithinByte := bitIndex % 8
	if len(id) <= bytesDescribedByBitIndex && bitIndexWithinByte != 0 {
		return n.left
	}

	// byteUnderConsideration is an integer from 0 to 255 represented by 8 bits
	// where 255 is 11111111 and 0 is 00000000.
	// In order to find out whether the bit at bitIndexWithinByte is set we
	// construct (1 << (7 - bitIndexWithinByte)) which will consist of all bits
	// being 0, with only one bit set to 1.
	// For example, if bitIndexWithinByte is 3, we will construct 00010000 by
	// (1 << (7 - 3)) -> (1 << 4) -> 16
	if byteUnderConsideration := id[bytesDescribedByBitIndex]; (byteUnderConsideration & (1 << (7 - bitIndexWithinByte))) > 0 {
		return n.right
	}

	return n.left
}

// Splits the node, redistributes contacts to the new nodes, and marks the node
// that was split as an inner node of the binary tree of nodes by setting
// node.Contacts = nil. Also, marks the "far away" node as dontSplit.
func (n *node) split(bitIndex int, localNodeid []byte) {
	n.left = createNode()
	n.right = createNode()

	// Redistribute existing contacts amongst the two newly created nodes.
	for _, contact := range n.contacts {
		tmp := n.determine(contact.Id, bitIndex)
		tmp.contacts = append(tmp.contacts, contact)
	}

	n.contacts = nil // Mark as inner tree node

	// Don't split the "far away" node.
	// We check where the local node would end up and mark the other one as
	// dontSplit (i.e. "far away").
	if detNode := n.determine(localNodeid, bitIndex); n.left == detNode {
		n.right.dontSplit = true
	} else {
		n.left.dontSplit = true
	}
}

// Returns the index of the contact with provided id if it exists, returns -1 otherwise.
func (c Contacts) indexOf(id []byte) int {
	for i, v := range c {
		if bytes.Compare(v.Id, id) == 0 {
			return i
		}
	}

	return -1
}

// Compare two contacts. The result will be true if a == b, false otherwise.
func (a Contact) compare(b Contact) bool {
	if bytes.Compare(a.Id, b.Id) != 0 {
		return false
	}

	if a.VectorClock != b.VectorClock {
		return false
	}

	if !CompareAddrPorts(a.AddrPort, b.AddrPort) {
		return false
	}

	return true
}

// createNode creates a new node.
func createNode() *node {
	return &node{
		contacts:  Contacts{},
		dontSplit: false,
		left:      nil,
		right:     nil,
	}
}
