package server

import (
	"log"

	"github.com/miekg/dns"

	"github.com/dimaskiddo/dns-proxy/internal/util"
)

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

		empty.SetReply(r)
		empty.Compress = cfg.Server.Compress

		w.WriteMsg(empty)
		return
	}

	if len(r.Question) == 0 {
		fail := new(dns.Msg)
		fail.SetRcode(r, dns.RcodeFormatError)

		w.WriteMsg(fail)
		return
	}

	if localResp := rt.Local.Resolve(r.Question[0]); localResp != nil {
		localResp.SetReply(r)
		localResp.Compress = cfg.Server.Compress

		w.WriteMsg(localResp)
		return
	}

	if cachedResp := rt.Cache.Get(r); cachedResp != nil {
		cachedResp.SetReply(r)
		cachedResp.Compress = cfg.Server.Compress

		w.WriteMsg(cachedResp)
		return
	}

	if rt.EDNS != nil {
		rt.EDNS.AddECS(r, w.RemoteAddr().String())
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

	if resp != nil {
		if cfg.BogusNXDomain.Enable {
			rt.Bogus.Check(resp)
		}

		if cfg.Upstream.DisableIPv6 {
			resp.Ns = util.FilterIPv6Records(resp.Ns)
			resp.Answer = util.FilterIPv6Records(resp.Answer)
			resp.Extra = util.FilterIPv6Records(resp.Extra)
		}

		rt.Cache.Set(resp)
	}

	resp.SetReply(r)
	resp.Compress = cfg.Server.Compress

	w.WriteMsg(resp)
}
