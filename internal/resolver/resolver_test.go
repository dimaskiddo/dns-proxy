package resolver

import (
	"testing"

	"github.com/miekg/dns"

	"github.com/dimaskiddo/dns-proxy/internal/config"
)

// TestLocalResolverCaseInsensitive verifies a mixed-case query matches a
// lowercase-configured static record (RFC 1035: DNS names are
// case-insensitive).
func TestLocalResolverCaseInsensitive(t *testing.T) {
	lr := NewLocalResolver(config.LocalConfig{
		Enable: true,
		StaticRecords: []config.StaticRecord{
			{Domain: "example.com", IP: "10.0.0.1"},
		},
	}, 60)

	q := dns.Question{Name: "EXAMPLE.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}
	resp := lr.Resolve(q)
	if resp == nil {
		t.Fatal("expected a match for a mixed-case query against a lowercase record")
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("expected 1 answer record, got %d", len(resp.Answer))
	}
}

// TestForwarderResolverCaseInsensitive verifies a mixed-case query name
// matches a lowercase-configured forwarder rule.
func TestForwarderResolverCaseInsensitive(t *testing.T) {
	fr := NewForwarderResolver(config.ForwarderConfig{
		Enable: true,
		Rules: []config.ForwarderRule{
			{Domain: "example.com", Upstreams: []string{"10.0.0.53:53"}},
		},
	})

	targets, found := fr.GetUpstream("EXAMPLE.com.")
	if !found {
		t.Fatal("expected a match for a mixed-case query against a lowercase rule")
	}
	if len(targets) != 1 || targets[0] != "10.0.0.53:53" {
		t.Fatalf("unexpected targets: %+v", targets)
	}
}
