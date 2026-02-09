package main

import (
	"net"

	"github.com/miekg/dns"
)

type EDNSHandler struct {
	v4Mask uint8
	v6Mask uint8
}

func NewEDNSHandler(config EDNSConfig) *EDNSHandler {
	if !config.Enable {
		return nil
	}

	// Sanity Check for IPv4 Mask
	v4 := config.IPv4Mask
	if v4 < 0 {
		v4 = 0
	}
	if v4 > 32 {
		v4 = 32
	}

	// Sanity Check for IPv6 Mask
	v6 := config.IPv6Mask
	if v6 < 0 {
		v6 = 0
	}
	if v6 > 128 {
		v6 = 128
	}

	return &EDNSHandler{
		v4Mask: uint8(v4),
		v6Mask: uint8(v6),
	}
}

func (e *EDNSHandler) AddECS(r *dns.Msg, clientAddr string) {
	// Parse Real Client-IP
	host, _, err := net.SplitHostPort(clientAddr)
	if err != nil {
		host = clientAddr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return
	}

	// Check if OPT RR exists
	// And if ECS option is already present
	opt := r.IsEdns0()
	if opt != nil {
		for _, o := range opt.Option {
			if o.Option() == dns.EDNS0SUBNET {
				// Option already exists, do not overwrite
				return
			}
		}
	} else {
		// Create OPT RR
		opt = new(dns.OPT)
		opt.Hdr.Name = "."
		opt.Hdr.Rrtype = dns.TypeOPT

		// Append OPT RR to Extra
		r.Extra = append(r.Extra, opt)
	}

	// Create ECS Option
	ecs := new(dns.EDNS0_SUBNET)

	ecs.Code = dns.EDNS0SUBNET
	ecs.SourceScope = 0
	ecs.Address = ip

	if ip4 := ip.To4(); ip4 != nil {
		ecs.Family = 1
		ecs.SourceNetmask = e.v4Mask
		ecs.Address = ip4
	} else {
		ecs.Family = 2
		ecs.SourceNetmask = e.v6Mask
	}

	// Append Option to OPT RR
	opt.Option = append(opt.Option, ecs)
}
