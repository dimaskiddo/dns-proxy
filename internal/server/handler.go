package server

import (
	"fmt"
	"log"
	"net"

	"github.com/miekg/dns"

	"github.com/dimaskiddo/dns-proxy/internal/util"
)

// replyTo prepares resp as the reply to r. dns.Msg.SetReply resets Rcode to
// NOERROR, which would erase an upstream NXDOMAIN/REFUSED/SERVFAIL — so
// restore the original Rcode after SetReply runs.
func replyTo(resp *dns.Msg, r *dns.Msg, compress bool) *dns.Msg {
	rcode := resp.Rcode
	resp.SetReply(r)
	resp.Rcode = rcode
	resp.Compress = compress
	return resp
}

// HandleRequest is the dns.HandleFunc for all queries: it loads the current
// Runtime atomically (no lock held across upstream I/O — a SIGHUP reload no
// longer blocks behind in-flight queries), then resolves via local records,
// cache, forwarder rules, or the configured upstream mode in turn.
func (s *Server) HandleRequest(w dns.ResponseWriter, r *dns.Msg) {
	rt := s.Runtime()
	cfg := rt.Config

	var err error
	var resp *dns.Msg

	if cfg.Upstream.DisableIPv6 && len(r.Question) > 0 && r.Question[0].Qtype == dns.TypeAAAA {
		empty := new(dns.Msg)
		w.WriteMsg(replyTo(empty, r, cfg.Server.Compress))
		return
	}

	if len(r.Question) == 0 {
		fail := new(dns.Msg)
		fail.SetRcode(r, dns.RcodeFormatError)

		w.WriteMsg(fail)
		return
	}

	if localResp := rt.Local.Resolve(r.Question[0]); localResp != nil {
		w.WriteMsg(replyTo(localResp, r, cfg.Server.Compress))
		return
	}

	// Captured before AddECS, which synthesizes its own OPT for a client that
	// sent none — reading r.IsEdns0() after that point would see the proxy's
	// OPT instead of the client's real (or absent) advertisement.
	clientOPT := r.IsEdns0()
	clientHadEDNS := clientOPT != nil
	clientUDPSize := dns.MinMsgSize
	if clientHadEDNS {
		clientUDPSize = int(clientOPT.UDPSize())
	}

	// ECS must be injected before the cache lookup: the cache key folds in
	// the ECS subnet (see cache.key), so a geo-specific answer for one
	// client is never served to a client from a different subnet.
	if rt.EDNS != nil {
		rt.EDNS.AddECS(r, w.RemoteAddr().String())
	}

	if cachedResp := rt.Cache.Get(r); cachedResp != nil {
		w.WriteMsg(replyTo(cachedResp, r, cfg.Server.Compress))
		return
	}

	forwardFound := false
	if cfg.Forwarder.Enable {
		if targets, found := rt.Forwarder.GetUpstream(r.Question[0].Name); found {
			resp, err = rt.Upstream.ForwardUDP(r, targets)
			forwardFound = true
		}
	}

	if !forwardFound {
		switch cfg.Upstream.Mode {
		case "doh":
			resp, err = rt.Upstream.ForwardDoH(r)
		case "tcp", "dot":
			resp, err = rt.Upstream.ForwardTCP(r)
		case "udp":
			resp, err = rt.Upstream.ForwardUDP(r, nil)
		default:
			// Defense in depth — config.Manager.Reload is the primary guard
			// against an unrecognized mode reaching this switch.
			err = fmt.Errorf("unsupported upstream mode %q", cfg.Upstream.Mode)
		}
	}

	if err != nil {
		log.Printf("Error DNS Upstream Server: %v", err)

		failMsg := new(dns.Msg)
		failMsg.SetRcode(r, dns.RcodeServerFailure)
		failMsg.Compress = cfg.Server.Compress

		w.WriteMsg(failMsg)
		return
	}

	if resp == nil {
		failMsg := new(dns.Msg)
		failMsg.SetRcode(r, dns.RcodeServerFailure)
		failMsg.Compress = cfg.Server.Compress

		w.WriteMsg(failMsg)
		return
	}

	if cfg.BogusNXDomain.Enable {
		rt.Bogus.Check(resp)
	}

	if cfg.Upstream.DisableIPv6 {
		resp.Ns = util.FilterIPv6Records(resp.Ns)
		resp.Answer = util.FilterIPv6Records(resp.Answer)
		resp.Extra = util.FilterIPv6Records(resp.Extra)
	}

	rt.Cache.Set(r, resp)
	replyTo(resp, r, cfg.Server.Compress)

	if !clientHadEDNS {
		resp.Extra = util.StripOPT(resp.Extra)
	}

	// Truncate only for datagram clients — an oversized UDP reply is both
	// IP-fragmented and a usable amplification vector for spoofed-source
	// queries.
	if _, isUDP := w.RemoteAddr().(*net.UDPAddr); isUDP {
		resp.Truncate(clientUDPSize)
	}

	w.WriteMsg(resp)
}
