package server

import (
	"net"
	"strings"
	"testing"

	"github.com/miekg/dns"

	"github.com/dimaskiddo/dns-proxy/internal/config"
)

// TestReplyToPreservesRcode verifies replyTo does not let dns.Msg.SetReply's
// unconditional Rcode reset erase an upstream NXDOMAIN/REFUSED, while still
// copying the request's ID and question section as a normal reply would.
func TestReplyToPreservesRcode(t *testing.T) {
	cases := []struct {
		name  string
		rcode int
	}{
		{"NXDOMAIN", dns.RcodeNameError},
		{"REFUSED", dns.RcodeRefused},
		{"SERVFAIL", dns.RcodeServerFailure},
		{"NOERROR", dns.RcodeSuccess},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := new(dns.Msg)
			req.SetQuestion("example.com.", dns.TypeA)
			req.Id = 42

			resp := new(dns.Msg)
			resp.Rcode = tc.rcode

			out := replyTo(resp, req, true)

			if out.Rcode != tc.rcode {
				t.Fatalf("Rcode = %d, want %d", out.Rcode, tc.rcode)
			}
			if out.Id != req.Id {
				t.Fatalf("Id = %d, want %d", out.Id, req.Id)
			}
			if !out.Response {
				t.Fatal("Response flag not set by SetReply")
			}
			if len(out.Question) != 1 || out.Question[0].Name != "example.com." {
				t.Fatalf("question not copied from request: %+v", out.Question)
			}
			if !out.Compress {
				t.Fatal("Compress not set")
			}
		})
	}
}

// fakeResponseWriter captures the message written by HandleRequest, reporting
// a UDP remote address so the truncation branch runs.
type fakeResponseWriter struct {
	written *dns.Msg
}

func (f *fakeResponseWriter) LocalAddr() net.Addr  { return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5353} }
func (f *fakeResponseWriter) RemoteAddr() net.Addr { return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 40000} }
func (f *fakeResponseWriter) WriteMsg(m *dns.Msg) error {
	f.written = m
	return nil
}
func (f *fakeResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (f *fakeResponseWriter) Close() error                { return nil }
func (f *fakeResponseWriter) TsigStatus() error            { return nil }
func (f *fakeResponseWriter) TsigTimersOnly(bool)           {}
func (f *fakeResponseWriter) Hijack()                       {}

// startFakeUpstream runs a UDP DNS server that replies to any query with an
// EDNS0 OPT (4096) and enough TXT records to exceed 512 bytes, mimicking a
// real EDNS-capable upstream. Returns its address and a stop func.
func startFakeUpstream(t *testing.T) string {
	t.Helper()

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}

	mux := dns.NewServeMux()
	mux.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)

		for i := 0; i < 20; i++ {
			m.Answer = append(m.Answer, &dns.TXT{
				Hdr: dns.RR_Header{Name: r.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 60},
				Txt: []string{strings.Repeat("x", 50)},
			})
		}

		opt := new(dns.OPT)
		opt.Hdr.Name = "."
		opt.Hdr.Rrtype = dns.TypeOPT
		opt.SetUDPSize(4096)
		m.Extra = append(m.Extra, opt)

		w.WriteMsg(m)
	})

	srv := &dns.Server{PacketConn: pc, Handler: mux}
	go srv.ActivateAndServe()

	t.Cleanup(func() { srv.Shutdown() })

	return pc.LocalAddr().String()
}

func newTestRuntime(t *testing.T, upstreamAddr string) *Runtime {
	t.Helper()

	cfg := &config.Config{
		Server: config.ServerConfig{Compress: true},
		Upstream: config.UpstreamConfig{
			Mode: "udp", Timeout: 2, KeepAlive: 5, BufferSize: 4096,
			PoolSize: 2, MaxAttempts: 1, Addresses: []string{upstreamAddr},
		},
		EDNS:      config.EDNSConfig{Enable: true, IPv4Mask: 24, IPv6Mask: 56},
		Forwarder: config.ForwarderConfig{Enable: true, Rules: []config.ForwarderRule{{Domain: "test.example.", Upstreams: []string{upstreamAddr}}}},
	}

	rt, err := NewRuntime(cfg)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	t.Cleanup(rt.Stop)

	return rt
}

// TestHandleRequestNonEDNSClientGetsNoOPTAnd512Truncation pins §1.1/§1.2: a
// non-EDNS UDP client must get a reply sized to 512 bytes with no OPT record,
// even though edns.enable adds one to the outgoing upstream query.
func TestHandleRequestNonEDNSClientGetsNoOPTAnd512Truncation(t *testing.T) {
	upstreamAddr := startFakeUpstream(t)
	rt := newTestRuntime(t, upstreamAddr)

	s := New()
	s.SetRuntime(rt)

	req := new(dns.Msg)
	req.SetQuestion("test.example.", dns.TypeTXT)

	w := &fakeResponseWriter{}
	s.HandleRequest(w, req)

	if w.written == nil {
		t.Fatal("no response written")
	}
	if !w.written.Truncated {
		t.Fatal("expected Truncated flag set for oversized reply to a 512-byte client")
	}
	for _, rr := range w.written.Extra {
		if rr.Header().Rrtype == dns.TypeOPT {
			t.Fatalf("OPT record present in reply to a non-EDNS client: %+v", rr)
		}
	}
}

// TestHandleRequestEDNSClientKeepsOPTAndIsNotTruncated is the control case:
// a client that advertised a 4096-byte buffer keeps the OPT record and isn't
// truncated.
func TestHandleRequestEDNSClientKeepsOPTAndIsNotTruncated(t *testing.T) {
	upstreamAddr := startFakeUpstream(t)
	rt := newTestRuntime(t, upstreamAddr)

	s := New()
	s.SetRuntime(rt)

	req := new(dns.Msg)
	req.SetQuestion("test.example.", dns.TypeTXT)
	req.SetEdns0(4096, false)

	w := &fakeResponseWriter{}
	s.HandleRequest(w, req)

	if w.written == nil {
		t.Fatal("no response written")
	}
	if w.written.Truncated {
		t.Fatal("unexpected Truncated flag for a 4096-byte EDNS client")
	}

	foundOPT := false
	for _, rr := range w.written.Extra {
		if rr.Header().Rrtype == dns.TypeOPT {
			foundOPT = true
		}
	}
	if !foundOPT {
		t.Fatal("expected OPT record preserved in reply to an EDNS client")
	}
}
