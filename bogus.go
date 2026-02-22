package main

import (
	"net"

	"github.com/miekg/dns"
)

func checkBogusNXDomain(resp *dns.Msg) {
	if resp == nil || len(bogusNXDomains) == 0 {
		return
	}

	hasBogus := false
	for _, rr := range resp.Answer {
		switch v := rr.(type) {
		case *dns.A:
			for _, bip := range bogusNXDomains {
				if v.A.Equal(bip) {
					hasBogus = true
					break
				}
			}

		case *dns.AAAA:
			for _, bip := range bogusNXDomains {
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
