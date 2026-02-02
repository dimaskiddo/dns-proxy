package main

import (
	"strings"
	"sync"

	"github.com/miekg/dns"
)

type ForwarderResolver struct {
	rules map[string][]string
	mu    sync.RWMutex
}

func NewForwarderResolver(cfg ForwarderConfig) *ForwarderResolver {
	fr := &ForwarderResolver{
		rules: make(map[string][]string),
	}

	if !cfg.Enable {
		return fr
	}

	for _, rule := range cfg.Rules {
		domain := dns.Fqdn(rule.Domain)
		fr.rules[domain] = rule.Upstreams
	}

	return fr
}

func (fr *ForwarderResolver) GetUpstream(qName string) ([]string, bool) {
	fr.mu.RLock()
	defer fr.mu.RUnlock()

	var bestLen int

	var bestMatch []string
	found := false

	for domain, upstreams := range fr.rules {
		if strings.HasSuffix(qName, "."+domain) || qName == domain {
			if len(domain) > bestLen {
				bestLen = len(domain)

				bestMatch = upstreams
				found = true
			}
		}
	}

	return bestMatch, found
}
