package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type CacheEntry struct {
	Msg     *dns.Msg
	Expires time.Time
}

type DNSCache struct {
	store      map[string]CacheEntry
	mu         sync.RWMutex
	capacity   int
	enabled    bool
	defaultTTL time.Duration
	minTTL     time.Duration
	negTTL     time.Duration
}

func NewCache(size int, minTTL, negTTL int) *DNSCache {
	c := &DNSCache{
		store:      make(map[string]CacheEntry, size),
		capacity:   size,
		enabled:    size > 0,
		defaultTTL: 60 * time.Second,
		minTTL:     time.Duration(minTTL) * time.Second,
		negTTL:     time.Duration(negTTL) * time.Second,
	}

	if c.enabled {
		go c.cleanupRoutine()
	}

	return c
}

func key(q dns.Question) string {
	return fmt.Sprintf("%s:%d:%d", q.Name, q.Qtype, q.Qclass)
}

func (c *DNSCache) Get(r *dns.Msg) *dns.Msg {
	if !c.enabled || len(r.Question) == 0 {
		return nil
	}

	k := key(r.Question[0])

	c.mu.RLock()
	entry, found := c.store[k]
	c.mu.RUnlock()

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

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.store) >= c.capacity {
		for k := range c.store {
			delete(c.store, k)
			break
		}
	}

	c.store[k] = entry
}

func (c *DNSCache) cleanupRoutine() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()

		now := time.Now()
		for k, v := range c.store {
			if now.After(v.Expires) {
				delete(c.store, k)
			}
		}

		c.mu.Unlock()
	}
}
