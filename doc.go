/*
# KBucket

Kademlia DHT K-bucket implementation as a binary tree.
KBucket was ported from Tristan Slominski's k-bucket: github.com/tristanls/k-bucket

A Distributed Hash Table (DHT) is a decentralized distributed system that
provides a lookup table similar to a hash table.

KBucket is an implementation of a storage mechanism for keys within a DHT.
It stores Contact objects which represent locations and addresses of nodes in
the decentralized distributed system. Contact objects are typically identified
by a SHA-1 hash, however this restriction is lifted in this implementation.
Additionally, node ids of different lengths can be compared.

This Kademlia DHT k-bucket implementation is meant to be as minimal as possible.
It assumes that Contact objects consist only of Id. It is useful, and necessary,
to attach other properties to a Contact. For example, one may want to attach
ip and port properties, which allow an application to send IP traffic to the
Contact. However, this information is extraneous and irrelevant to the operation
of a k-bucket.

KBucket events:
	kbucket.added
		  	newContact Contact: The new contact that was added.
		Emitted only when "newContact" was added to bucket and it was not stored
		in the bucket before.

	kbucket.ping
		  	old Contacts: The slice of contacts to ping.
			new Contact: The new contact to be added if one of old contacts does not respond.
		Emitted every time a contact is added that would exceed the capacity of a
		"don't split" k-bucket it belongs to.

	kbucket.removed
		  	contact Contact: The contact that was removed.
		Emitted when "contact" was removed from the bucket.

	kbucket.updated
		  	old Contact: The contact that was stored prior to the update.
			new Contact: The new contact that is now stored after the update.
		Emitted when a previously existing ("previously existing" means "oldContact.id"
		equals "newContact.id") contact was added to the bucket and it was replaced with
		"newContact".
*/
package bittorrentdht
