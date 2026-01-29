// Package dsa provides data structure implementations for ResultStore.
// Uses go-radix for compressed prefix tree (radix tree).
package dsa

import (
	"github.com/armon/go-radix"
)

// Trie wraps go-radix for a compressed prefix tree (radix tree).
// Much more memory-efficient than standard trie for file paths.
//
// Standard trie: /Users/richard → 14 nodes (one per character)
// Radix tree:    /Users/richard → 1 node (compressed path)
//
// Time Complexity: O(k) where k is key length
// Space Complexity: O(n * avg_key_len) instead of O(n * alphabet * max_key_len)
type Trie[V any] struct {
	tree *radix.Tree
	size int
}

// NewTrie creates a new empty radix tree.
func NewTrie[V any]() *Trie[V] {
	return &Trie[V]{
		tree: radix.New(),
	}
}

// Insert adds a key-value pair to the tree.
// Time Complexity: O(k) where k is key length.
func (t *Trie[V]) Insert(key string, value V) {
	_, updated := t.tree.Insert(key, value)
	if !updated {
		t.size++
	}
}

// Search looks up a key in the tree.
// Time Complexity: O(k) where k is key length.
func (t *Trie[V]) Search(key string) (V, bool) {
	val, found := t.tree.Get(key)
	if !found {
		var zero V
		return zero, false
	}
	v, ok := val.(V)
	if !ok {
		var zero V
		return zero, false
	}
	return v, true
}

// HasPrefix checks if any key starts with the given prefix.
// Time Complexity: O(k) where k is prefix length.
func (t *Trie[V]) HasPrefix(prefix string) bool {
	_, _, found := t.tree.LongestPrefix(prefix)
	if found {
		return true
	}
	// Also check if any keys have this prefix
	found = false
	t.tree.WalkPrefix(prefix, func(k string, v interface{}) bool {
		found = true
		return true // stop after first match
	})
	return found
}

// StartsWith returns all keys that start with the given prefix.
// Time Complexity: O(k + m) where k is prefix length, m is number of matches.
func (t *Trie[V]) StartsWith(prefix string) []string {
	var results []string
	t.tree.WalkPrefix(prefix, func(k string, v interface{}) bool {
		results = append(results, k)
		return false // continue walking
	})
	return results
}

// Delete removes a key from the tree.
// Returns true if the key was found and deleted.
func (t *Trie[V]) Delete(key string) bool {
	_, deleted := t.tree.Delete(key)
	if deleted {
		t.size--
	}
	return deleted
}

// Contains checks if a key exists in the tree.
func (t *Trie[V]) Contains(key string) bool {
	_, found := t.tree.Get(key)
	return found
}

// Size returns the number of keys in the tree.
func (t *Trie[V]) Size() int {
	return t.size
}

// IsEmpty returns true if the tree has no keys.
func (t *Trie[V]) IsEmpty() bool {
	return t.size == 0
}

// Keys returns all keys in the tree.
func (t *Trie[V]) Keys() []string {
	return t.StartsWith("")
}

// Clear removes all keys from the tree.
func (t *Trie[V]) Clear() {
	t.tree = radix.New()
	t.size = 0
}

// ForEach calls the given function for each key-value pair.
func (t *Trie[V]) ForEach(fn func(key string, value V)) {
	t.tree.Walk(func(k string, v interface{}) bool {
		if val, ok := v.(V); ok {
			fn(k, val)
		}
		return false // continue walking
	})
}

// LongestPrefix returns the longest key that is a prefix of the query.
func (t *Trie[V]) LongestPrefix(query string) (string, V, bool) {
	key, val, found := t.tree.LongestPrefix(query)
	if !found {
		var zero V
		return "", zero, false
	}
	v, ok := val.(V)
	if !ok {
		var zero V
		return "", zero, false
	}
	return key, v, true
}
