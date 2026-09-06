package resolver

import (
	"net"

	"github.com/miekg/dns"

	"github.com/dimaskiddo/dns-proxy/internal/config"
)

// EDNSHandler adds EDNS0 Client Subnet (ECS) options to outgoing queries.
type EDNSHandler struct {
	v4Mask uint8
	v6Mask uint8
}

// NewEDNSHandler returns an EDNSHandler for cfg, or nil if EDNS is disabled.
func NewEDNSHandler(cfg config.EDNSConfig) *EDNSHandler {
	if !cfg.Enable {
		return nil
	}

	v4 := cfg.IPv4Mask
	if v4 < 0 {
		v4 = 0
	}
	if v4 > 32 {
		v4 = 32
	}

	v6 := cfg.IPv6Mask
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

// AddECS attaches an EDNS0 Client Subnet option derived from clientAddr to
// r, unless one is already present.
func (e *EDNSHandler) AddECS(r *dns.Msg, clientAddr string) {
	host, _, err := net.SplitHostPort(clientAddr)
	if err != nil {
		host = clientAddr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return
	}

	opt := r.IsEdns0()
	if opt != nil {
		for _, o := range opt.Option {
			if o.Option() == dns.EDNS0SUBNET {
				// Option already exists, do not overwrite.
				return
			}
		}
	} else {
		opt = new(dns.OPT)
		opt.Hdr.Name = "."
		opt.Hdr.Rrtype = dns.TypeOPT

		r.Extra = append(r.Extra, opt)
	}

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

	opt.Option = append(opt.Option, ecs)
}
