// Package server wires config, cache, resolvers, and the upstream client
// into a running DNS proxy, and supports hot config reload.
package server

import (
	"sync/atomic"

	"github.com/dimaskiddo/dns-proxy/internal/cache"
	"github.com/dimaskiddo/dns-proxy/internal/config"
	"github.com/dimaskiddo/dns-proxy/internal/resolver"
	"github.com/dimaskiddo/dns-proxy/internal/upstream"
)

// Runtime bundles everything built from one loaded Config. A reload builds
// a new Runtime and atomically swaps it in — in-flight requests keep using
// the Runtime they started with.
type Runtime struct {
	Config    *config.Config
	Upstream  *upstream.Client
	Cache     *cache.DNSCache
	Local     *resolver.LocalResolver
	Forwarder *resolver.ForwarderResolver
	EDNS      *resolver.EDNSHandler
	Bogus     *resolver.BogusFilter
}

// NewRuntime constructs a Runtime from cfg.
func NewRuntime(cfg *config.Config) (*Runtime, error) {
	upstreamClient, err := upstream.NewClient(cfg.Upstream)
	if err != nil {
		return nil, err
	}

	return &Runtime{
		Config:    cfg,
		Upstream:  upstreamClient,
		Cache:     cache.New(cfg.Cache.Size, cfg.Cache.Shards, cfg.Cache.MinTTL, cfg.Cache.NegTTL),
		Local:     resolver.NewLocalResolver(cfg.Local, cfg.Cache.MinTTL),
		Forwarder: resolver.NewForwarderResolver(cfg.Forwarder),
		EDNS:      resolver.NewEDNSHandler(cfg.EDNS),
		Bogus:     resolver.NewBogusFilter(cfg.BogusNXDomain),
	}, nil
}

// Stop releases background resources: the cache's cleanup goroutine and the
// upstream client's pooled/idle connections.
func (rt *Runtime) Stop() {
	rt.Cache.Stop()
	rt.Upstream.Close()
}

// Server holds the currently active Runtime and swaps it in on reload,
// without ever blocking request handling on a lock.
type Server struct {
	rt atomic.Pointer[Runtime]
}

// New returns an empty Server; call SetRuntime before handling requests.
func New() *Server {
	return &Server{}
}

// SetRuntime installs rt as the active Runtime, stopping the previous one
// (if any) after the swap.
func (s *Server) SetRuntime(rt *Runtime) {
	old := s.rt.Swap(rt)
	if old != nil {
		old.Stop()
	}
}

// Runtime returns the currently active Runtime.
func (s *Server) Runtime() *Runtime {
	return s.rt.Load()
}
