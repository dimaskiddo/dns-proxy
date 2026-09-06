package cache

import (
	"container/list"
	"hash/fnv"
	"sync"
	"time"

	"github.com/miekg/dns"

	"github.com/dimaskiddo/dns-proxy/internal/util"
)

type cacheItem struct {
	Key     string
	Msg     *dns.Msg
	Expires time.Time
}

type shard struct {
	store    map[string]*list.Element
	ll       *list.List
	mu       sync.RWMutex
	capacity int
}

// DNSCache is a sharded, TTL-aware LRU cache of DNS responses.
type DNSCache struct {
	enabled    bool
	shards     []*shard
	shardCount uint64
	shardMask  uint64
	defaultTTL time.Duration
	minTTL     time.Duration
	negTTL     time.Duration
	stop       chan struct{}
}

// New creates a DNSCache with the given total size (0 disables caching),
// shard count, minimum positive TTL, and negative-response TTL (all in
// seconds except size/shards).
func New(size int, shards int, minTTL int, negTTL int) *DNSCache {
	if shards < 1 {
		shards = 256
	}

	shardsCount := util.NextPowerOfTwo(shards)

	c := &DNSCache{
		enabled:    size > 0,
		shards:     make([]*shard, shardsCount),
		shardCount: uint64(shardsCount),
		shardMask:  uint64(shardsCount - 1),
		defaultTTL: 60 * time.Second,
		minTTL:     time.Duration(minTTL) * time.Second,
		negTTL:     time.Duration(negTTL) * time.Second,
		stop:       make(chan struct{}),
	}

	shardCapacity := size / shardsCount
	if shardCapacity < 1 {
		shardCapacity = 1
	}

	for i := 0; i < shardsCount; i++ {
		c.shards[i] = &shard{
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

func (c *DNSCache) getShard(key string) *shard {
	h := fnv.New64a()
	h.Write([]byte(key))

	return c.shards[h.Sum64()&c.shardMask]
}

// Get returns a cached response for r, or nil on a miss/expiry.
func (c *DNSCache) Get(r *dns.Msg) *dns.Msg {
	if !c.enabled || len(r.Question) == 0 {
		return nil
	}

	k := key(r.Question[0])
	s := c.getShard(k)

	s.mu.Lock()
	defer s.mu.Unlock()

	elem, found := s.store[k]
	if !found {
		return nil
	}

	item := elem.Value.(*cacheItem)
	if time.Now().After(item.Expires) {
		s.ll.Remove(elem)
		delete(s.store, k)

		return nil
	}

	s.ll.MoveToFront(elem)

	return item.Msg.Copy()
}

// Set stores r, deriving TTL from its answer records (or the negative TTL
// for NXDOMAIN/SERVFAIL).
func (c *DNSCache) Set(r *dns.Msg) {
	if !c.enabled || len(r.Question) == 0 {
		return
	}

	ttl := c.defaultTTL

	if r.Rcode == dns.RcodeNameError || r.Rcode == dns.RcodeServerFailure {
		ttl = c.negTTL
	} else {
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
	newItem := &cacheItem{
		Key:     k,
		Msg:     r.Copy(),
		Expires: time.Now().Add(ttl),
	}

	s := c.getShard(k)

	s.mu.Lock()
	defer s.mu.Unlock()

	if elem, found := s.store[k]; found {
		elem.Value = newItem
		s.ll.MoveToFront(elem)
		return
	}

	if s.ll.Len() >= s.capacity {
		oldest := s.ll.Back()
		if oldest != nil {
			s.ll.Remove(oldest)
			delete(s.store, oldest.Value.(*cacheItem).Key)
		}
	}

	elem := s.ll.PushFront(newItem)
	s.store[k] = elem
}

func (c *DNSCache) cleanupRoutine() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()

			for i := 0; i < int(c.shardCount); i++ {
				s := c.shards[i]

				s.mu.Lock()

				var next *list.Element
				for e := s.ll.Front(); e != nil; e = next {
					next = e.Next()
					item := e.Value.(*cacheItem)

					if now.After(item.Expires) {
						s.ll.Remove(e)
						delete(s.store, item.Key)
					}
				}

				s.mu.Unlock()
			}

		case <-c.stop:
			return
		}
	}
}

// Stop terminates the background cleanup goroutine.
func (c *DNSCache) Stop() {
	if c.enabled {
		close(c.stop)
	}
}
