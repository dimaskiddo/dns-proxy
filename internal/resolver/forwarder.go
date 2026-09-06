package resolver

import (
	"strings"
	"sync"

	"github.com/miekg/dns"

	"github.com/dimaskiddo/dns-proxy/internal/config"
)

// ForwarderResolver maps query domains to per-domain upstream address
// overrides, using longest-suffix-match.
type ForwarderResolver struct {
	rules map[string][]string
	mu    sync.RWMutex
}

// NewForwarderResolver builds a ForwarderResolver from cfg.
func NewForwarderResolver(cfg config.ForwarderConfig) *ForwarderResolver {
	fr := &ForwarderResolver{
		rules: make(map[string][]string),
	}

	if !cfg.Enable {
		return fr
	}

	for _, rule := range cfg.Rules {
		// RFC 1035 names are case-insensitive; CanonicalName normalizes case
		// here so a mixed-case query still matches a rule entered any-case.
		domain := dns.CanonicalName(rule.Domain)
		fr.rules[domain] = rule.Upstreams
	}

	return fr
}

// GetUpstream returns the configured upstream addresses for qName, if any
// rule matches.
func (fr *ForwarderResolver) GetUpstream(qName string) ([]string, bool) {
	qName = dns.CanonicalName(qName)

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
