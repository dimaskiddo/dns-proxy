package cache

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

func newQuery(name string) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), dns.TypeA)
	return m
}

func newAnswer(query *dns.Msg, ip string) *dns.Msg {
	resp := new(dns.Msg)
	resp.SetReply(query)
	resp.Answer = append(resp.Answer, &dns.A{
		Hdr: dns.RR_Header{Name: query.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
		A:   net.ParseIP(ip),
	})
	return resp
}

// TestCaseInsensitiveKey verifies a query differing only by case hits the
// same cache entry (RFC 1035: DNS names are case-insensitive).
func TestCaseInsensitiveKey(t *testing.T) {
	c := New(100, 4, 0, 30)
	defer c.Stop()

	q1 := newQuery("EXAMPLE.com")
	c.Set(q1, newAnswer(q1, "1.2.3.4"))

	q2 := newQuery("example.com")
	got := c.Get(q2)
	if got == nil {
		t.Fatal("expected cache hit for differently-cased name, got miss")
	}
	if len(got.Answer) != 1 {
		t.Fatalf("expected 1 answer record, got %d", len(got.Answer))
	}
}

// TestECSKeySeparation verifies two queries for the same name but different
// ECS subnets are cached separately, and a non-ECS query has an unrelated
// key shape (no "|ecs:" suffix collision).
func TestECSKeySeparation(t *testing.T) {
	c := New(100, 4, 0, 30)
	defer c.Stop()

	withECS := func(subnet string) *dns.Msg {
		m := newQuery("geo.example.com")
		opt := new(dns.OPT)
		opt.Hdr.Name = "."
		opt.Hdr.Rrtype = dns.TypeOPT
		ecs := &dns.EDNS0_SUBNET{
			Code:          dns.EDNS0SUBNET,
			Family:        1,
			SourceNetmask: 24,
			Address:       net.ParseIP(subnet).To4(),
		}
		opt.Option = append(opt.Option, ecs)
		m.Extra = append(m.Extra, opt)
		return m
	}

	qA := withECS("10.0.0.1")
	c.Set(qA, newAnswer(qA, "1.1.1.1"))

	qB := withECS("192.168.1.1")
	if got := c.Get(qB); got != nil {
		t.Fatal("expected miss for a different ECS subnet, got a hit")
	}

	if got := c.Get(qA); got == nil {
		t.Fatal("expected hit for the original ECS subnet")
	}

	// A plain query for the same name (no ECS) must be a distinct entry too.
	plain := newQuery("geo.example.com")
	if got := c.Get(plain); got != nil {
		t.Fatal("expected miss for a non-ECS query against an ECS-keyed name")
	}
}
