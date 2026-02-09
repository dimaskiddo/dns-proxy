package main

import (
	"container/list"
	"hash/fnv"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type CacheItem struct {
	Key     string
	Msg     *dns.Msg
	Expires time.Time
}

type CacheShard struct {
	store    map[string]*list.Element
	ll       *list.List
	mu       sync.RWMutex
	capacity int
}

type DNSCache struct {
	enabled    bool
	shards     []*CacheShard
	shardCount uint64
	shardMask  uint64
	defaultTTL time.Duration
	minTTL     time.Duration
	negTTL     time.Duration
	stop       chan struct{}
}

func NewCache(size int, shards int, minTTL int, negTTL int) *DNSCache {
	if shards < 1 {
		shards = 256
	}

	shardsCount := nextPowerOfTwo(shards)

	c := &DNSCache{
		enabled:    size > 0,
		shards:     make([]*CacheShard, shardsCount),
		shardCount: uint64(shardsCount),
		shardMask:  uint64(shardsCount - 1),
		defaultTTL: 60 * time.Second,
		minTTL:     time.Duration(minTTL) * time.Second,
		negTTL:     time.Duration(negTTL) * time.Second,
		stop:       make(chan struct{}),
	}

	// Count Cache Capacity Per-Shard
	shardCapacity := size / shardsCount
	if shardCapacity < 1 {
		shardCapacity = 1
	}

	// Initialize Cache Shard with Calculated Capacity
	for i := 0; i < shardsCount; i++ {
		c.shards[i] = &CacheShard{
			store:    make(map[string]*list.Element, shardCapacity),
			ll:       list.New(),
			capacity: shardCapacity,
		}
	}

	if c.enabled {
		go c.cleanupRoutine()
	}

	return c
}

func key(q dns.Question) string {
	return q.Name + string(rune(q.Qtype)) + string(rune(q.Qclass))
}

func (c *DNSCache) getShard(key string) *CacheShard {
	h := fnv.New64a()
	h.Write([]byte(key))

	return c.shards[h.Sum64()&c.shardMask]
}

func (c *DNSCache) Get(r *dns.Msg) *dns.Msg {
	if !c.enabled || len(r.Question) == 0 {
		return nil
	}

	k := key(r.Question[0])
	shard := c.getShard(k)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	elem, found := shard.store[k]
	if !found {
		return nil
	}

	item := elem.Value.(*CacheItem)
	if time.Now().After(item.Expires) {
		shard.ll.Remove(elem)
		delete(shard.store, k)

		return nil
	}

	shard.ll.MoveToFront(elem)

	return item.Msg.Copy()
}

func (c *DNSCache) Set(r *dns.Msg) {
	if !c.enabled || len(r.Question) == 0 {
		return
	}

	ttl := c.defaultTTL

	if r.Rcode == dns.RcodeNameError || r.Rcode == dns.RcodeServerFailure {
		// Negative Caching
		ttl = c.negTTL
	} else {
		// Positive Caching
		minFound := uint32(0)
		for _, rr := range r.Answer {
			if minFound == 0 || rr.Header().Ttl < minFound {
				minFound = rr.Header().Ttl
			}
		}

		if minFound > 0 {
			ttl = time.Duration(minFound) * time.Second
		}

		if ttl < c.minTTL {
			ttl = c.minTTL
		}
	}

	k := key(r.Question[0])
	newItem := &CacheItem{
		Key:     k,
		Msg:     r.Copy(),
		Expires: time.Now().Add(ttl),
	}

	shard := c.getShard(k)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Check if Cache Item Already Exist
	// If Exist Update it in Linked List
	if elem, found := shard.store[k]; found {
		elem.Value = newItem
		shard.ll.MoveToFront(elem)
		return
	}

	// If Shard Capacity is Reached Then
	// Remove the Back (Least Recently Used (LRU))
	if shard.ll.Len() >= shard.capacity {
		oldest := shard.ll.Back()
		if oldest != nil {
			shard.ll.Remove(oldest)
			delete(shard.store, oldest.Value.(*CacheItem).Key)
		}
	}

	// Add Cache Item in to Linked List
	elem := shard.ll.PushFront(newItem)
	shard.store[k] = elem
}

func (c *DNSCache) cleanupRoutine() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()

			// Loop Through Shards
			for i := 0; i < int(c.shardCount); i++ {
				shard := c.shards[i]

				shard.mu.Lock()

				// Loop Through Linked List in Shard
				var next *list.Element
				for e := shard.ll.Front(); e != nil; e = next {
					next = e.Next()
					item := e.Value.(*CacheItem)

					if now.After(item.Expires) {
						shard.ll.Remove(e)
						delete(shard.store, item.Key)
					}
				}

				shard.mu.Unlock()
			}

		case <-c.stop:
			// Stop Routine when Stop Signal Recieved
			return
		}
	}
}

func (c *DNSCache) Stop() {
	if c.enabled {
		close(c.stop)
	}
}
