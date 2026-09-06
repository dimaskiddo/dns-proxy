package resolver

import (
	"net"
	"strings"

	"github.com/miekg/dns"

	"github.com/dimaskiddo/dns-proxy/internal/config"
)

// BogusFilter zeroes out A/AAAA answers that match a configured bogus
// (poisoned) NXDOMAIN response IP.
type BogusFilter struct {
	ips []net.IP
}

// NewBogusFilter builds a BogusFilter from cfg, parsing and skipping any
// invalid IPs. When cfg.Enable is false the filter holds no IPs and Check
// is a no-op.
func NewBogusFilter(cfg config.BogusNXDomainConfig) *BogusFilter {
	bf := &BogusFilter{}

	if !cfg.Enable {
		return bf
	}

	for _, ipStr := range cfg.IPs {
		if ip := net.ParseIP(strings.TrimSpace(ipStr)); ip != nil {
			bf.ips = append(bf.ips, ip)
		}
	}

	return bf
}

// Count returns the number of configured bogus IPs.
func (bf *BogusFilter) Count() int {
	return len(bf.ips)
}

// Check zeroes A/AAAA answers in resp that match a configured bogus IP.
func (bf *BogusFilter) Check(resp *dns.Msg) {
	if resp == nil || len(bf.ips) == 0 {
		return
	}

	hasBogus := false
	for _, rr := range resp.Answer {
		switch v := rr.(type) {
		case *dns.A:
			for _, bip := range bf.ips {
				if v.A.Equal(bip) {
					hasBogus = true
					break
				}
			}

		case *dns.AAAA:
			for _, bip := range bf.ips {
				if v.AAAA.Equal(bip) {
					hasBogus = true
					break
				}
			}
		}

		if hasBogus {
			break
		}
	}

	if hasBogus {
		for _, rr := range resp.Answer {
			if a, ok := rr.(*dns.A); ok {
				a.A = net.IPv4zero
			} else if aaaa, ok := rr.(*dns.AAAA); ok {
				aaaa.AAAA = net.IPv6zero
			}
		}
	}
}
