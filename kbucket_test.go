package bittorrentdht

import (
	"encoding/hex"
	"errors"
	"net/netip"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/attilabuti/eventemitter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type KBucketTestSuite struct {
	suite.Suite
	kbucket *KBucket
}

func (s *KBucketTestSuite) SetupTest() {
	options := KBucketOptions{
		LocalNodeId:     []byte("test"),
		NodesPerKBucket: 20,
		NodesToPing:     3,
	}

	s.kbucket, _ = NewKBucket(options, eventemitter.New())
}

func TestKBucketTestSuite(t *testing.T) {
	suite.Run(t, new(KBucketTestSuite))
}

func clearTime(c *Contact) {
	c.SeenAt = time.Time{}
}

func TestNewKBucket(t *testing.T) {
	kbucket, err := NewKBucket(KBucketOptions{
		LocalNodeId:     []byte("test"),
		NodesPerKBucket: 20,
		NodesToPing:     3,
	}, eventemitter.New())

	root := createNode()

	if assert.NoError(t, err) {
		assert.NotEmpty(t, kbucket)
		assert.Exactly(t, root, kbucket.root)
		assert.Exactly(t, map[string]any{}, kbucket.Metadata)

		kbucket.Metadata["lastChanged"] = time.Now()
		assert.NotEmpty(t, kbucket.Metadata)
	}
}

func TestNewKBuckeErrors(t *testing.T) {
	kbucket, err := NewKBucket(KBucketOptions{
		LocalNodeId:     []byte{},
		NodesPerKBucket: 20,
		NodesToPing:     3,
	}, eventemitter.New())
	if assert.Error(t, err) {
		assert.Exactly(t, &KBucket{}, kbucket)
		assert.Equal(t, errors.New("Local node ID cannot be empty"), err)
	}

	kbucket, err = NewKBucket(KBucketOptions{
		LocalNodeId:     []byte("test"),
		NodesPerKBucket: 0,
		NodesToPing:     3,
	}, eventemitter.New())
	if assert.Error(t, err) {
		assert.Exactly(t, &KBucket{}, kbucket)
		assert.Equal(t, errors.New("Nodes per k-bucket must be at least 1"), err)
	}

	kbucket, err = NewKBucket(KBucketOptions{
		LocalNodeId:     []byte("test"),
		NodesPerKBucket: 20,
		NodesToPing:     0,
	}, eventemitter.New())
	if assert.Error(t, err) {
		assert.Exactly(t, &KBucket{}, kbucket)
		assert.Equal(t, errors.New("Nodes to ping must be at least 1"), err)
	}
}

func TestCreateNode(t *testing.T) {
	expected := &node{
		contacts:  []Contact{},
		dontSplit: false,
		left:      nil,
		right:     nil,
	}

	assert.Exactly(t, expected, createNode())
}

// Adding a contact places it in root node.
func (s *KBucketTestSuite) TestAddContact() {
	c := Contact{Id: []byte("a")}

	s.kbucket.Add(c)
	s.False(s.kbucket.root.contacts[0].SeenAt.IsZero())
	s.Empty(s.kbucket.root.contacts[0].Metadata)

	clearTime(&s.kbucket.root.contacts[0])
	s.Exactly(Contacts{c}, s.kbucket.root.contacts)

	c2 := Contact{Id: []byte("b"), Metadata: map[string]any{"test": "metadata_test"}}
	s.kbucket.Add(c2)
	s.False(s.kbucket.root.contacts[1].SeenAt.IsZero())
	s.NotEmpty(s.kbucket.root.contacts[1].Metadata)
	s.Equal("metadata_test", s.kbucket.root.contacts[1].Metadata["test"])
}

// Adding an existing contact does not increase number of contacts in root node.
func (s *KBucketTestSuite) TestAddExistingContact() {
	s.kbucket.Add(Contact{Id: []byte("a")})
	s.kbucket.Add(Contact{Id: []byte("a")})

	s.Len(s.kbucket.root.contacts, 1)
}

// Adding same contact moves it to the end of the root node (most-recently-contacted end).
func (s *KBucketTestSuite) TestAddSameContact() {
	c := Contact{Id: []byte("a")}

	s.kbucket.Add(c)
	s.Len(s.kbucket.root.contacts, 1)

	clearTime(&s.kbucket.root.contacts[0])
	s.kbucket.Add(Contact{Id: []byte("b")})
	s.Len(s.kbucket.root.contacts, 2)
	s.Exactly(c, s.kbucket.root.contacts[0]) // Least-recently-contacted end

	s.kbucket.Add(c)
	s.Len(s.kbucket.root.contacts, 2)
	s.Exactly(c, s.kbucket.root.contacts[1]) // Most-recently-contacted end
}

// Adding contact to bucket that can't be split results in calling "ping" callback.
func TestAddEventPing(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	kbucket, _ := NewKBucket(KBucketOptions{
		LocalNodeId:     []byte{0x00, 0x00},
		NodesPerKBucket: 20,
		NodesToPing:     3,
	}, eventemitter.New())

	counter := 0
	kbucket.emitter.On("kbucket.ping", func(old Contacts, new Contact) {
		defer wg.Done()

		assert.Len(t, old, kbucket.ping)
		for i := 0; i < kbucket.ping; i++ {
			// The least recently contacted end of the node should be pinged.
			assert.Exactly(t, kbucket.root.right.contacts[i], old[i])
			counter++
		}
		assert.Exactly(t, Contact{Id: []byte{0x80, byte(kbucket.size)}}, new)
		assert.Equal(t, kbucket.ping, counter)
	})

	for j := 0; j < kbucket.size+1; j++ {
		// Make sure all go into "far away" node
		kbucket.Add(Contact{Id: []byte{0x80, byte(j)}})
	}

	wg.Wait()
}

// Should emit event "added" once.
func TestAddEventEmitAdd(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	kbucket, _ := NewKBucket(KBucketOptions{
		LocalNodeId:     []byte("test"),
		NodesPerKBucket: 20,
		NodesToPing:     3,
	}, eventemitter.New())
	c := Contact{Id: []byte("a")}

	counter := 0
	kbucket.emitter.On("kbucket.added", func(added Contact) {
		defer wg.Done()

		clearTime(&added)
		assert.Exactly(t, c, added)

		counter++
	})

	kbucket.Add(c)
	kbucket.Add(c)

	wg.Wait()

	// Should emit event "added" once.
	assert.Equal(t, 1, counter)
}

// Should emit event "added" when adding to a split node.
func TestAddSplitEventEmitAdd(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	kbucket, _ := NewKBucket(KBucketOptions{
		LocalNodeId:     []byte("test"),
		NodesPerKBucket: 20,
		NodesToPing:     3,
	}, eventemitter.New())
	c := Contact{Id: []byte("a")}

	for i := 0; i < kbucket.size+1; i++ {
		kbucket.Add(Contact{Id: []byte{byte(i)}})
	}

	assert.Len(t, kbucket.root.contacts, 0)

	kbucket.emitter.On("kbucket.added", func(added Contact) {
		defer wg.Done()

		clearTime(&added)
		assert.Exactly(t, c, added)
	})

	kbucket.Add(c)

	wg.Wait()
}

// Closest nodes are returned.
func (s *KBucketTestSuite) TestClosestNodes() {
	for i := 0; i < 0x12; i++ {
		s.kbucket.Add(Contact{Id: []byte{byte(i)}})
	}

	c := s.kbucket.Closest([]byte{byte(0x15)}, 3)
	s.Len(c, 3)

	tests := []struct {
		expected []byte
		input    []byte
	}{
		{[]byte{byte(0x11)}, c[0].Id},
		{[]byte{byte(0x10)}, c[1].Id},
		{[]byte{byte(0x05)}, c[2].Id},
	}

	for _, test := range tests {
		s.Exactly(test.input, test.expected)
	}
}

// All closest nodes are returned.
func TestClosestAll(t *testing.T) {
	kbucket, _ := NewKBucket(KBucketOptions{
		LocalNodeId:     []byte{byte(0x00), byte(0x00)},
		NodesPerKBucket: 20,
		NodesToPing:     3,
	}, eventemitter.New())

	for i := 0; i < 1e3; i++ {
		kbucket.Add(Contact{Id: []byte{byte(i / 256), byte(i % 256)}})
	}

	assert.True(t, len(kbucket.Closest([]byte{byte(0x00), byte(0x00)}, 5000)) > 100)
}

// Closest nodes are returned (including exact match).
func (s *KBucketTestSuite) TestClosestIncludingExactMatch() {
	for i := 0; i < 0x12; i++ {
		s.kbucket.Add(Contact{Id: []byte{byte(i)}})
	}

	c := s.kbucket.Closest([]byte{byte(0x11)}, 3)

	tests := []struct {
		expected []byte
		input    []byte
	}{
		{[]byte{byte(0x11)}, c[0].Id},
		{[]byte{byte(0x10)}, c[1].Id},
		{[]byte{byte(0x01)}, c[2].Id},
	}

	for _, test := range tests {
		s.Exactly(test.input, test.expected)
	}
}

// Closest nodes are returned even if there isn't enough in one bucket.
func TestClosestNotEnough(t *testing.T) {
	kbucket, _ := NewKBucket(KBucketOptions{
		LocalNodeId:     []byte{byte(0x00), byte(0x00)},
		NodesPerKBucket: 20,
		NodesToPing:     3,
	}, eventemitter.New())

	for i := 0; i < kbucket.size; i++ {
		kbucket.Add(Contact{Id: []byte{byte(0x80), byte(i)}})
		kbucket.Add(Contact{Id: []byte{byte(0x01), byte(i)}})
	}

	kbucket.Add(Contact{Id: []byte{byte(0x00), byte(0x01)}})

	c := kbucket.Closest([]byte{byte(0x00), byte(0x03)}, 22)

	tests := [][]byte{
		{byte(0x00), byte(0x01)}, // distance: 0000000000000010
		{byte(0x01), byte(0x03)}, // distance: 0000000100000000
		{byte(0x01), byte(0x02)}, // distance: 0000000100000010
		{byte(0x01), byte(0x01)},
		{byte(0x01), byte(0x00)},
		{byte(0x01), byte(0x07)},
		{byte(0x01), byte(0x06)},
		{byte(0x01), byte(0x05)},
		{byte(0x01), byte(0x04)},
		{byte(0x01), byte(0x0b)},
		{byte(0x01), byte(0x0a)},
		{byte(0x01), byte(0x09)},
		{byte(0x01), byte(0x08)},
		{byte(0x01), byte(0x0f)},
		{byte(0x01), byte(0x0e)},
		{byte(0x01), byte(0x0d)},
		{byte(0x01), byte(0x0c)},
		{byte(0x01), byte(0x13)},
		{byte(0x01), byte(0x12)},
		{byte(0x01), byte(0x11)},
		{byte(0x01), byte(0x10)},
		{byte(0x80), byte(0x03)}, // distance: 1000000000000000
	}

	for i, test := range tests {
		assert.Exactly(t, test, c[i].Id)
	}
}

// Count returns 0 when no contacts in bucket.
func (s *KBucketTestSuite) TestCountReturnsZero() {
	s.Equal(0, s.kbucket.Count())
}

// Count returns 1 when 1 contact in bucket.
func (s *KBucketTestSuite) TestCountReturnsOne() {
	s.kbucket.Add(Contact{Id: []byte("a")})
	s.Equal(1, s.kbucket.Count())
}

// Count returns 1 when same contact added to bucket twice.
func (s *KBucketTestSuite) TestCountReturnsOneSameContact() {
	c := Contact{Id: []byte("a")}
	s.kbucket.Add(c).Add(c)

	s.Equal(1, s.kbucket.Count())
}

// Count returns number of added unique contacts.
func (s *KBucketTestSuite) TestCountReturnsUnique() {
	for _, v := range []string{"a", "a", "b", "b", "c", "d", "c", "d", "e", "f"} {
		s.kbucket.Add(Contact{Id: []byte(v)})
	}

	s.Equal(6, s.kbucket.Count())
}

func (s *KBucketTestSuite) TestToSliceEmpty() {
	s.Len(s.kbucket.ToSlice(), 0)
}

func (s *KBucketTestSuite) TestToSlice() {
	var expectedIds []int
	var i int

	for i = 0; i < s.kbucket.size; i++ {
		s.kbucket.Add(Contact{Id: []byte{byte(0x80), byte(i)}}) // Make sure all go into "far away" bucket.
		expectedIds = append(expectedIds, 0x80*256+i)
	}

	// Cause a split to happen.
	s.kbucket.Add(Contact{Id: []byte{byte(0x00), byte(0x80), byte(i - 1)}})
	contacts := s.kbucket.ToSlice()

	s.Len(contacts, s.kbucket.size+1)
	nodeId, _ := strconv.ParseInt(hex.EncodeToString(contacts[0].Id), 16, 0)
	s.Equal(0x80*256+i-1, int(nodeId))

	contacts = contacts[1:] // Get rid of low bucket contact.
	for i = 0; i < s.kbucket.size; i++ {
		nodeId, _ := strconv.ParseInt(hex.EncodeToString(contacts[i].Id), 16, 0)
		s.Exactly(expectedIds[i], int(nodeId))
	}
}

func (s *KBucketTestSuite) TestDistance() {
	s.Equal(0, s.kbucket.Distance([]byte{byte(0x00)}, []byte{byte(0x00)}))                             // Distance between 00000000 and 00000000 is 00000000
	s.Equal(1, s.kbucket.Distance([]byte{byte(0x00)}, []byte{byte(0x01)}))                             // Distance between 00000000 and 00000001 is 00000001
	s.Equal(3, s.kbucket.Distance([]byte{byte(0x02)}, []byte{byte(0x01)}))                             // Distance between 00000010 and 00000001 is 00000011
	s.Equal(255, s.kbucket.Distance([]byte{byte(0x00)}, []byte{byte(0x00), byte(0x00)}))               // Distance between 00000000 and 0000000000000000 is 0000000011111111
	s.Equal(16640, s.kbucket.Distance([]byte{byte(0x01), byte(0x24)}, []byte{byte(0x40), byte(0x24)})) // Distance between 0000000100100100 and 0100000000100100 is 0100000100000000
}

func TestDetermineNode(t *testing.T) {
	leftNode := createNode()
	rightNode := createNode()

	leftNode.contacts = append(leftNode.contacts, Contact{Id: []byte{byte(0x24), byte(0x25)}})
	rightNode.contacts = append(rightNode.contacts, Contact{Id: []byte{byte(0x34), byte(0x35)}})

	rootNode := node{left: leftNode, right: rightNode}

	assert.Equal(t, leftNode, rootNode.determine([]byte{byte(0x00)}, 0))                           // ID 00000000, bitIndex 0, should be low
	assert.Equal(t, leftNode, rootNode.determine([]byte{byte(0x40)}, 0))                           // ID 01000000, bitIndex 0, should be low
	assert.Equal(t, rightNode, rootNode.determine([]byte{byte(0x40)}, 1))                          // ID 01000000, bitIndex 1, should be high
	assert.Equal(t, leftNode, rootNode.determine([]byte{byte(0x40)}, 2))                           // ID 01000000, bitIndex 2, should be low
	assert.Equal(t, leftNode, rootNode.determine([]byte{byte(0x40)}, 9))                           // ID 01000000, bitIndex 9, should be low
	assert.Equal(t, rightNode, rootNode.determine([]byte{byte(0x41)}, 7))                          // ID 01000001, bitIndex 7, should be high
	assert.Equal(t, rightNode, rootNode.determine([]byte{byte(0x41), byte(0x00)}, 7))              // ID 0100000100000000, bitIndex 7, should be high
	assert.Equal(t, rightNode, rootNode.determine([]byte{byte(0x00), byte(0x41), byte(0x00)}, 15)) // ID 000000000100000100000000, bitIndex 15, should be high
}

// Get retrieves null if no contacts.
func (s *KBucketTestSuite) TestGetRetrievesNull() {
	s.Exactly(Contact{}, s.kbucket.Get([]byte("a")))
}

// Get retrieves a contact that was added.
func (s *KBucketTestSuite) TestGetRetrievesContact() {
	c := Contact{Id: []byte("a")}
	s.kbucket.Add(c)

	clearTime(&s.kbucket.root.contacts[0])
	s.Exactly(c, s.kbucket.Get([]byte("a")))
}

// Get retrieves most recently added contact if same id.
func (s *KBucketTestSuite) TestGetRetrievesMostRecently() {
	addr1, _ := netip.ParseAddrPort("1.1.1.1:6881")
	s.kbucket.Add(Contact{Id: []byte("a"), VectorClock: 0, AddrPort: addr1})

	addr2, _ := netip.ParseAddrPort("127.0.0.1:6881")
	s.kbucket.Add(Contact{Id: []byte("a"), VectorClock: 1, AddrPort: addr2})

	s.Exactly([]byte("a"), s.kbucket.Get([]byte("a")).Id)
	s.Equal(1, s.kbucket.Get([]byte("a")).VectorClock)
	s.Exactly(addr2, s.kbucket.Get([]byte("a")).AddrPort)
}

// Get retrieves contact from nested leaf node.
func (s *KBucketTestSuite) TestGetRetrievesFromNestedLeaf() {
	i := 0
	for ; i < s.kbucket.size; i++ {
		s.kbucket.Add(Contact{Id: []byte{byte(0x80), byte(i)}})
	}

	addr, _ := netip.ParseAddrPort("127.0.0.1:6881")
	s.kbucket.Add(Contact{Id: []byte{byte(0x00), byte(i)}, AddrPort: addr}) // Cause a split to happen.

	s.Exactly(addr, s.kbucket.Get([]byte{byte(0x00), byte(i)}).AddrPort)
}

func (s *KBucketTestSuite) TestGet() {
	c := Contact{Id: []byte("a")}
	s.kbucket.Add(c)
	clearTime(&s.kbucket.root.contacts[0])

	i, contact := s.kbucket.get([]byte("a"))
	s.Exactly(c, *contact)
	s.Equal(0, i)

	i, contact = s.kbucket.get([]byte("b"))
	s.Exactly(Contact{}, *contact)
	s.Equal(-1, i)
}

func (s *KBucketTestSuite) TestHas() {
	c := Contact{Id: []byte("a")}
	s.kbucket.Add(c)

	s.True(s.kbucket.Has([]byte("a")))
	s.False(s.kbucket.Has([]byte("b")))
}

func (s *KBucketTestSuite) TestHasNestedLeaf() {
	i := 0
	for ; i < s.kbucket.size; i++ {
		s.kbucket.Add(Contact{Id: []byte{byte(0x80), byte(i)}})
	}

	s.kbucket.Add(Contact{Id: []byte{byte(0x00), byte(i)}}) // Cause a split to happen.

	s.True(s.kbucket.Has([]byte{byte(0x00), byte(i)}))
}

func (s *KBucketTestSuite) TestSeen() {
	c := Contact{Id: []byte("a")}
	s.kbucket.Add(c)
	s.False(s.kbucket.root.contacts[0].SeenAt.IsZero())

	time.Sleep(time.Microsecond)
	previousTime := s.kbucket.root.contacts[0].SeenAt
	testA := s.kbucket.Seen([]byte("a"))
	testB := s.kbucket.Seen([]byte("b"))
	s.NotEqual(previousTime, s.kbucket.root.contacts[0].SeenAt)
	s.True(testA)
	s.False(testB)
}

// indexOf returns a contact with id that contains the same byte sequence as the test contact.
func (s *KBucketTestSuite) TestIndexOfReturnsContact() {
	c := []byte("a")
	s.kbucket.Add(Contact{Id: c})

	s.Equal(0, s.kbucket.root.contacts.indexOf(c))
}

// indexOf returns -1 if contact is not found
func (s *KBucketTestSuite) TestIndexOfReturnsMinusOne() {
	s.kbucket.Add(Contact{Id: []byte("a")})
	s.Equal(-1, s.kbucket.root.contacts.indexOf([]byte("b")))
}

// Removing a contact should remove contact from nested buckets.
func TestRemoveContactNested(t *testing.T) {
	kbucket, _ := NewKBucket(KBucketOptions{
		LocalNodeId:     []byte{byte(0x00), byte(0x00)},
		NodesPerKBucket: 20,
		NodesToPing:     3,
	}, eventemitter.New())
	i := 0

	for ; i < kbucket.size; i++ {
		kbucket.Add(Contact{Id: []byte{byte(0x80), byte(i)}}) // Make sure all go into "far away" bucket.
	}

	// Cause a split to happen.
	kbucket.Add(Contact{Id: []byte{byte(0x00), byte(i)}})

	contactToDelete := Contact{Id: []byte{byte(0x80), byte(0x00)}}
	assert.Equal(t, 0, kbucket.root.right.contacts.indexOf(contactToDelete.Id))

	kbucket.Remove(contactToDelete.Id)
	assert.Equal(t, -1, kbucket.root.right.contacts.indexOf(contactToDelete.Id))
}

// Should emit "removed" event.
func (s *KBucketTestSuite) TestRemoveEmitEventRemoved() {
	var wg sync.WaitGroup
	wg.Add(1)

	c := Contact{Id: []byte("a")}

	s.kbucket.emitter.On("kbucket.removed", func(removed Contact) {
		clearTime(&removed)
		s.Exactly(c, removed)

		wg.Done()
	})

	s.kbucket.Add(c)
	s.kbucket.Remove(c.Id)

	wg.Wait()
}

// Should emit event "removed" when removing from a split bucket.
func TestRemoveEmitEventRemovedWhenSplit(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	kbucket, _ := NewKBucket(KBucketOptions{
		LocalNodeId:     []byte{byte(0x00)}, // Need non-random LocalNodeID for deterministic splits.
		NodesPerKBucket: 20,
		NodesToPing:     3,
	}, eventemitter.New()) // Need non-random LocalNodeID for deterministic splits.

	for i := 0; i < kbucket.size+1; i++ {
		kbucket.Add(Contact{Id: []byte{byte(i)}})
	}

	c := Contact{Id: []byte("a")}
	kbucket.emitter.On("kbucket.removed", func(removed Contact) {
		clearTime(&removed)
		assert.Exactly(t, c, removed)

		wg.Done()
	})

	kbucket.Add(c)
	kbucket.Remove(c.Id)

	wg.Wait()
}

// Adding a contact does not split node.
func (s *KBucketTestSuite) TestSplitDoesNotSplit() {
	s.kbucket.Add(Contact{Id: []byte("a")})

	s.Nil(s.kbucket.root.left)
	s.Nil(s.kbucket.root.right)
	s.True(len(s.kbucket.root.contacts) > 0)
}

// Adding maximum number of contacts (per node) [20] into node does not split node.
func (s *KBucketTestSuite) TestSplitMaxDoesNotSplit() {
	for i := 0; i < s.kbucket.size; i++ {
		s.kbucket.Add(Contact{Id: []byte{byte(i)}})
	}

	s.Nil(s.kbucket.root.left)
	s.Nil(s.kbucket.root.right)
	s.True(len(s.kbucket.root.contacts) > 0)
}

// Adding maximum number of contacts (per node) + 1 [21] into node splits the node.
func (s *KBucketTestSuite) TestSplitMaxSplit() {
	for i := 0; i < s.kbucket.size+1; i++ {
		s.kbucket.Add(Contact{Id: []byte{byte(i)}})
	}

	s.Len(s.kbucket.root.left.contacts, 0)
	s.Len(s.kbucket.root.right.contacts, 0)
	s.Len(s.kbucket.root.contacts, 0)
}

// Split nodes contain all added contacts.
func TestSplitContain(t *testing.T) {
	kbucket, _ := NewKBucket(KBucketOptions{
		LocalNodeId:     []byte{byte(0x00)},
		NodesPerKBucket: 20,
		NodesToPing:     3,
	}, eventemitter.New())

	foundContact := make(map[string]bool)

	for i := 0; i < kbucket.size+1; i++ {
		kbucket.Add(Contact{Id: []byte{byte(i)}})
		foundContact[string(byte(i))] = false
	}

	var traverse func(n node)
	traverse = func(n node) {
		if n.contacts == nil {
			traverse(*n.left)
			traverse(*n.right)
		} else {
			for _, c := range n.contacts {
				foundContact[string(c.Id)] = true
			}
		}
	}
	traverse(*kbucket.root)

	for k := range foundContact {
		assert.True(t, foundContact[k])
	}

	assert.Nil(t, kbucket.root.contacts)
}

// When splitting nodes the "far away" node should be marked to prevent splitting "far away" node.
func TestSplitFarAway(t *testing.T) {
	kbucket, _ := NewKBucket(KBucketOptions{
		LocalNodeId:     []byte{byte(0x00)},
		NodesPerKBucket: 20,
		NodesToPing:     3,
	}, eventemitter.New())

	for i := 0; i < kbucket.size+1; i++ {
		kbucket.Add(Contact{Id: []byte{byte(i)}})
	}

	// Above algorithm will split left node 4 times and put 0x00 through 0x0f in
	// the left node, and put 0x10 through 0x14 in right node.
	// Since localNodeId is 0x00, we expect every right node to be "far" and
	// therefore marked as "dontSplit = true"
	// There will be one "left" node and four "right" nodes.
	conuter := 0
	var traverse func(n node, dontSplit bool)
	traverse = func(n node, dontSplit bool) {
		if n.contacts == nil {
			traverse(*n.left, false)
			traverse(*n.right, true)
		} else {
			if dontSplit {
				assert.True(t, n.dontSplit)
			} else {
				assert.False(t, n.dontSplit)
			}

			conuter++
		}
	}
	traverse(*kbucket.root, false)

	assert.Equal(t, 5, conuter)
}

func (s *KBucketTestSuite) TestUpdateNotEqualID() {
	s.kbucket.Add(Contact{Id: []byte("a"), VectorClock: 3})
	s.kbucket.update(s.kbucket.root, 0, Contact{Id: []byte("d"), VectorClock: 2})

	s.Equal(s.kbucket.root.contacts[0].VectorClock, 3)
}

// Deprecated vectorClock results in contact drop.
func (s *KBucketTestSuite) TestUpdateDepractedDrop() {
	s.kbucket.Add(Contact{Id: []byte("a"), VectorClock: 3})
	s.kbucket.update(s.kbucket.root, 0, Contact{Id: []byte("a"), VectorClock: 2})

	s.Equal(s.kbucket.root.contacts[0].VectorClock, 3)
}

// Equal vectorClock results in contact marked as most recent.
func (s *KBucketTestSuite) TestUpdateEqalVector() {
	c := Contact{Id: []byte("a"), VectorClock: 3}
	s.kbucket.Add(c)
	s.kbucket.Add(Contact{Id: []byte("b")})
	s.kbucket.update(s.kbucket.root, 0, c)

	s.Exactly(s.kbucket.root.contacts[1], c)
}

// More recent vectorClock results in contact update and contact being marked as most recent.
func (s *KBucketTestSuite) TestUpdateMoreRecent() {
	addr1, _ := netip.ParseAddrPort("127.0.0.1:6881")
	addr2, _ := netip.ParseAddrPort("1.1.1.1:6881")

	c := Contact{Id: []byte("a"), VectorClock: 3, AddrPort: addr1}

	s.kbucket.Add(c)
	s.kbucket.Add(Contact{Id: []byte("b")})
	s.kbucket.update(s.kbucket.root, 0, Contact{Id: []byte("a"), VectorClock: 4, AddrPort: addr2})

	s.Exactly(c.Id, s.kbucket.root.contacts[1].Id)
	s.Equal(s.kbucket.root.contacts[1].VectorClock, 4)
	s.Exactly(s.kbucket.root.contacts[1].AddrPort, addr2)
}

// Should emit event "updated".
func TestUpdateEmitEvent(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	kbucket, _ := NewKBucket(KBucketOptions{
		LocalNodeId:     []byte("test"),
		NodesPerKBucket: 20,
		NodesToPing:     3,
	}, eventemitter.New())

	c1 := Contact{Id: []byte("a"), VectorClock: 1}
	c2 := Contact{Id: []byte("a"), VectorClock: 2}

	kbucket.emitter.On("kbucket.updated", func(old Contact, new Contact) {
		defer wg.Done()

		clearTime(&old)
		clearTime(&new)
		assert.Exactly(t, c1, old)
		assert.Exactly(t, c2, new)
	})

	kbucket.Add(c1)
	kbucket.Add(c2)

	wg.Wait()
}

// Should emit event "updated" when updating a split node.
func TestUpdateSplitEmitUpdateEvent(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	kbucket, _ := NewKBucket(KBucketOptions{
		LocalNodeId:     []byte("test"),
		NodesPerKBucket: 20,
		NodesToPing:     3,
	}, eventemitter.New())

	c1 := Contact{Id: []byte("a"), VectorClock: 1}
	c2 := Contact{Id: []byte("a"), VectorClock: 2}

	kbucket.emitter.On("kbucket.updated", func(old Contact, new Contact) {
		defer wg.Done()

		clearTime(&old)
		clearTime(&new)
		assert.Exactly(t, c1, old)
		assert.Exactly(t, c2, new)
	})

	for i := 0; i < kbucket.size+1; i++ {
		kbucket.Add(Contact{Id: []byte{byte(i)}})
	}

	kbucket.Add(c1)
	kbucket.Add(c2)

	wg.Wait()
}

func TestSetDistanceFn(t *testing.T) {
	kbucket, _ := NewKBucket(KBucketOptions{
		LocalNodeId:     []byte("test"),
		NodesPerKBucket: 20,
		NodesToPing:     3,
	}, eventemitter.New())

	kbucket.SetDistanceFn(func(fid, sid []byte) int {
		return len(fid) + len(sid)
	})

	fid := []byte("first_id")
	sid := []byte("second_id")
	assert.Equal(t, len(fid)+len(sid), kbucket.Distance(fid, sid))
}

func TestSetArbiterFn(t *testing.T) {
	kbucket, _ := NewKBucket(KBucketOptions{
		LocalNodeId:     []byte("test"),
		NodesPerKBucket: 20,
		NodesToPing:     3,
	}, eventemitter.New())

	kbucket.SetArbiterFn(func(c1, c2 Contact) Contact {
		return c2
	})

	c1 := Contact{Id: []byte("a"), VectorClock: 1}
	c2 := Contact{Id: []byte("b"), VectorClock: 2}
	assert.Equal(t, c2, kbucket.arbiter(c1, c2))
}

func TestCompareContacts(t *testing.T) {
	addr1, _ := netip.ParseAddrPort("127.0.0.1:6881")
	addr2, _ := netip.ParseAddrPort("1.1.1.1:6881")

	c1 := Contact{Id: []byte("a"), VectorClock: 1, AddrPort: addr1}
	c2 := Contact{Id: []byte("b"), VectorClock: 2, AddrPort: addr2}
	assert.False(t, c1.compare(c2))

	c1 = Contact{Id: []byte("a")}
	c2 = Contact{Id: []byte("b")}
	assert.False(t, c1.compare(c2))

	c1 = Contact{Id: []byte("a"), VectorClock: 1}
	c2 = Contact{Id: []byte("a"), VectorClock: 2}
	assert.False(t, c1.compare(c2))

	c1 = Contact{Id: []byte("a"), AddrPort: addr1}
	c2 = Contact{Id: []byte("a"), AddrPort: addr2}
	assert.False(t, c1.compare(c2))

	c1 = Contact{Id: []byte("a"), VectorClock: 1, AddrPort: addr1}
	c2 = Contact{Id: []byte("a"), VectorClock: 1, AddrPort: addr1}
	assert.True(t, c1.compare(c2))
}

func setupBenchmark() *KBucket {
	localNodeId, _ := GenerateId()
	emitter := eventemitter.New()
	options := KBucketOptions{
		LocalNodeId:     localNodeId,
		NodesPerKBucket: 20,
		NodesToPing:     3,
	}
	kbucket, _ := NewKBucket(options, emitter)

	return kbucket
}

func BenchmarkKBucketAdd(b *testing.B) {
	kbucket := setupBenchmark()

	ids := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		ids[i], _ = GenerateId()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		kbucket.Add(Contact{Id: ids[i]})
	}
}

func BenchmarkKBucketGet(b *testing.B) {
	kbucket := setupBenchmark()

	var id []byte
	for i := 0; i < b.N; i++ {
		id, _ = GenerateId()
		kbucket.Add(Contact{Id: id})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		kbucket.Get(id)
	}
}

func BenchmarkKBucketClosest(b *testing.B) {
	kbucket := setupBenchmark()

	var id []byte
	for i := 0; i < b.N; i++ {
		id, _ = GenerateId()
		kbucket.Add(Contact{Id: id})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		kbucket.Closest(id, 10)
	}
}

func BenchmarkKBucketDistance(b *testing.B) {
	kbucket := setupBenchmark()

	_0000000100100100, _ := hex.DecodeString("0124")
	_0100000000100100, _ := hex.DecodeString("4024")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		kbucket.Distance(_0000000100100100, _0100000000100100)
	}
}
