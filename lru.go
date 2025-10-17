package main

import "fmt"

// Node represents a doubly linked list node
type Node struct {
	key        string
	value      interface{}
	prev, next *Node
}

// LRUCache implements a Least Recently Used cache
type LRUCache struct {
	capacity int
	cache    map[string]*Node
	head     *Node // most recently used
	tail     *Node // least recently used
}

func NewLRUCache(capacity int) *LRUCache {
	lru := &LRUCache{
		capacity: capacity,
		cache:    make(map[string]*Node),
		head:     &Node{},
		tail:     &Node{},
	}
	lru.head.next = lru.tail
	lru.tail.prev = lru.head
	return lru
}

// Get retrieves a value from the cache
func (lru *LRUCache) Get(key string) interface{} {
	if node, exists := lru.cache[key]; exists {
		lru.moveToFront(node)
		return node.value
	}
	return nil
}

// Put adds or updates a key-value pair
func (lru *LRUCache) Put(key string, value interface{}) {
	if node, exists := lru.cache[key]; exists {
		node.value = value
		lru.moveToFront(node)
		return
	}

	// Create new node
	node := &Node{key: key, value: value}
	lru.cache[key] = node
	lru.addToFront(node)

	// Evict if over capacity
	if len(lru.cache) > lru.capacity {
		lru.removeLRU()
	}
}

// moveToFront moves an existing node to the front (most recently used)
func (lru *LRUCache) moveToFront(node *Node) {
	lru.removeNode(node)
	lru.addToFront(node)
}

// addToFront adds a node right after the head
func (lru *LRUCache) addToFront(node *Node) {
	node.next = lru.head.next
	node.prev = lru.head
	lru.head.next.prev = node
	lru.head.next = node
}

// removeNode removes a node from the list
func (lru *LRUCache) removeNode(node *Node) {
	node.prev.next = node.next
	node.next.prev = node.prev
}

// removeLRU removes the least recently used item
func (lru *LRUCache) removeLRU() {
	lruNode := lru.tail.prev
	lru.removeNode(lruNode)
	delete(lru.cache, lruNode.key)
}

// Display shows the current cache state
func (lru *LRUCache) Display() {
	fmt.Print("Cache (MRU -> LRU): ")
	current := lru.head.next
	for current != lru.tail {
		fmt.Printf("[%s] ", current.key)
		current = current.next
	}
	fmt.Println()
}

// Size returns the current number of items in the cache
func (lru *LRUCache) Size() int {
	return len(lru.cache)
}

// Remove removes a specific key from the cache
func (lru *LRUCache) Remove(key string) {
	if node, exists := lru.cache[key]; exists {
		lru.removeNode(node)
		delete(lru.cache, key)
	}
}
