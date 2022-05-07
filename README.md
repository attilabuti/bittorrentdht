# BitTorrent DHT

[![build status](https://img.shields.io/github/workflow/status/attilabuti/bittorrentdht/CI/main?style=flat-square)](https://github.com/attilabuti/bittorrentdht/actions)
[![codecov](https://img.shields.io/codecov/c/github/attilabuti/bittorrentdht?style=flat-square)](https://codecov.io/gh/attilabuti/bittorrentdht)
[![Go Report Card](https://goreportcard.com/badge/github.com/attilabuti/bittorrentdht?style=flat-square)](https://goreportcard.com/report/github.com/attilabuti/bittorrentdht)
[![license](https://img.shields.io/github/license/attilabuti/bittorrentdht?style=flat-square)](https://raw.githubusercontent.com/attilabuti/bittorrentdht/main/LICENSE)

Go implementation of the [BitTorrent DHT protocol](http://www.bittorrent.org/beps/bep_0005.html).
BitTorrent DHT is the main peer discovery layer for BitTorrent, which allows for trackerless torrents.

## Installation

```bash
$ go get
```

## Usage

**For more information, please see the [Package Docs](#).**

```go
package main

import (
	"fmt"
)

func main() {
}
```

## KBucket

Ported from Tristan Slominski's [k-bucket](https://github.com/tristanls/k-bucket)

### Overview

A [*Distributed Hash Table (DHT)*](http://en.wikipedia.org/wiki/Distributed_hash_table) is a decentralized distributed system that provides a lookup table similar to a hash table.

*k-bucket* is an implementation of a storage mechanism for keys within a DHT. It stores `contact` objects which represent locations and addresses of nodes in the decentralized distributed system. `contact` objects are typically identified by a SHA-1 hash, however this restriction is lifted in this implementation. Additionally, node ids of different lengths can be compared.

This Kademlia DHT k-bucket implementation is meant to be as minimal as possible. It assumes that `contact` objects consist only of `id`. It is useful, and necessary, to attach other properties to a `contact`. For example, one may want to attach `ip` and `port` properties, which allow an application to send IP traffic to the `contact`. However, this information is extraneous and irrelevant to the operation of a k-bucket.

### arbiter function

This *k-bucket* implementation implements a conflict resolution mechanism using an `arbiter` function. The purpose of the `arbiter` is to choose between two `contact` objects with the same `id` but perhaps different properties and determine which one should be stored.  As the `arbiter` function returns the actual object to be stored, it does not need to make an either/or choice, but instead could perform some sort of operation and return the result as a new object that would then be stored. See [kBucket.update(node, index, contact)](https://github.com/attilabuti/bittorrentdht/blob/bab49c2e7119333be260b77474f0ec235c93eed1/kbucket.go#L414) for detailed semantics of which `contact` (`incumbent` or `candidate`) is selected.

For example, an `arbiter` function implementing a `VectorClock` mechanism would look something like:

```go
// Contact example
contact := Contact{
    Id: []byte("contactId"),
    VectorClock: 0
};

func arbiter(incumbent Contact, candidate Contact) Contact {
	if incumbent.VectorClock > candidate.VectorClock {
		return incumbent
	}

	return candidate
}
```

### Documentation

**For more information, please see the [Package Docs](#).**

Implementation of a Kademlia DHT k-bucket used for storing contact (peer node) information.

For a step by step example of k-bucket operation you may find the following slideshow useful: [Distribute All The Things](https://docs.google.com/presentation/d/11qGZlPWu6vEAhA7p3qsQaQtWH7KofEC9dMeBFZ1gYeA/edit#slide=id.g1718cc2bc_0661).

KBucket starts off as a single k-bucket with capacity of _k_. As contacts are added, once the _k+1_ contact is added, the k-bucket is split into two k-buckets. The split happens according to the first bit of the contact node id. The k-bucket that would contain the local node id is the "near" k-bucket, and the other one is the "far" k-bucket. The "far" k-bucket is marked as _don't split_ in order to prevent further splitting. The contact nodes that existed are then redistributed along the two new k-buckets and the old k-bucket becomes an inner node within a tree data structure.

As even more contacts are added to the "near" k-bucket, the "near" k-bucket will split again as it becomes full. However, this time it is split along the second bit of the contact node id. Again, the two newly created k-buckets are marked "near" and "far" and the "far" k-bucket is marked as _don't split_. Again, the contact nodes that existed in the old bucket are redistributed. This continues as long as nodes are being added to the "near" k-bucket, until the number of splits reaches the length of the local node id.

As more contacts are added to the "far" k-bucket and it reaches its capacity, it does not split. Instead, the k-bucket emits a "ping" event (register a listener: `emitter.On("kbucket.ping", function (old Contacts, new Contact) {...});` and includes a slice of old contact nodes that it hasn't heard from in a while and requires you to confirm that those contact nodes still respond (literally respond to a PING RPC). If an old contact node still responds, it should be re-added (`kBucket.Add(old Contact)`) back to the k-bucket. This puts the old contact on the "recently heard from" end of the list of nodes in the k-bucket. If the old contact does not respond, it should be removed (`kBucket.Remove(oldContact.Id []byte)`) and the new contact being added now has room to be stored (`kBucket.Add(new Contact)`).

#### Events

* kbucket.added
    * `newContact Contact`: The new contact that was added.
	* Emitted only when "newContact" was added to bucket and it was not stored in the bucket before.

* kbucket.ping
    * `old Contacts`: The slice of contacts to ping.
	* `new Contact`: The new contact to be added if one of old contacts does not respond.
    * Emitted every time a contact is added that would exceed the capacity of a "don't split" k-bucket it belongs to.

* kbucket.removed
	* `contact Contact`: The contact that was removed.
	* Emitted when "contact" was removed from the bucket.

* kbucket.updated
	* `old Contact`: The contact that was stored prior to the update.
	* `new Contact`: The new contact that is now stored after the update.
	* Emitted when a previously existing ("previously existing" means "oldContact.id" equals "newContact.id") contact was added to the bucket and it was replaced with "newContact".

## Further reading

- [DHT Protocol](https://www.bittorrent.org/beps/bep_0005.html)
- [Peer ID Conventions](https://www.bittorrent.org/beps/bep_0020.html)
- [BitTorrent DHT Extensions for IPv6](https://www.bittorrent.org/beps/bep_0032.html)
- [DHT Security extension](https://www.bittorrent.org/beps/bep_0042.html)
- [Storing arbitrary data in the DHT](https://www.bittorrent.org/beps/bep_0044.html)
- [Distributed Hash Table (DHT)](http://en.wikipedia.org/wiki/Distributed_hash_table)
- [A formal specification of the Kademlia distributed hash table](http://maude.sip.ucm.es/kademlia/files/pita_kademlia.pdf)
- [Distributed Hash Tables (part 2)](https://web.archive.org/web/20140217064545/http://offthelip.org/?p=157)
- [DHT Walkthrough Notes](https://gist.github.com/gubatron/cd9cfa66839e18e49846)
- [Distribute All The Things](https://docs.google.com/presentation/d/11qGZlPWu6vEAhA7p3qsQaQtWH7KofEC9dMeBFZ1gYeA/edit#slide=id.g1718cc2bc_0661)

## Issues

Submit the [issues](https://github.com/attilabuti/bittorrentdht/issues) if you find any bug or have any suggestion.

## Contribution

Fork the [repo](https://github.com/attilabuti/bittorrentdht) and submit pull requests.

## License

This extension is licensed under the [MIT License](https://github.com/attilabuti/bittorrentdht/blob/main/LICENSE).