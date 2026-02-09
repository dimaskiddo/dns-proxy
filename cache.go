package main

import (
	"hash/fnv"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type CacheEntry struct {
	Msg     *dns.Msg
	Expires time.Time
}

type CacheShard struct {
	store    map[string]CacheEntry
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
			store:    make(map[string]CacheEntry, shardCapacity),
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

	shard.mu.RLock()
	entry, found := shard.store[k]
	shard.mu.RUnlock()

	if !found {
		return nil
	}

	if time.Now().After(entry.Expires) {
		return nil
	}

	return entry.Msg.Copy()
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
	entry := CacheEntry{
		Msg:     r.Copy(),
		Expires: time.Now().Add(ttl),
	}

	shard := c.getShard(k)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	if len(shard.store) >= shard.capacity {
		// Random Eviction Strategy
		for k := range shard.store {
			delete(shard.store, k)
			break
		}
	}

	shard.store[k] = entry
}

func (c *DNSCache) cleanupRoutine() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()

			// Loop Through Shard
			for i := 0; i < int(c.shardCount); i++ {
				shard := c.shards[i]

				// Loop Through Capacity in Shard
				shard.mu.Lock()
				for k, v := range shard.store {
					if now.After(v.Expires) {
						// Delete Expired Cache
						delete(shard.store, k)
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
