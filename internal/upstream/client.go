// Package upstream forwards DNS queries to remote resolvers over UDP, TCP,
// DNS-over-TLS, or DNS-over-HTTPS.
package upstream

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	quic "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	"github.com/dimaskiddo/dns-proxy/internal/config"
	"github.com/dimaskiddo/dns-proxy/internal/pool"
	"github.com/dimaskiddo/dns-proxy/internal/util"
)

// Client forwards queries to the configured upstream(s) using the mode
// (udp/tcp/dot/doh) selected in config.
type Client struct {
	cfg config.UpstreamConfig

	Addresses []string
	DoHURLs   []string

	bufPool   *sync.Pool
	dohClient *http.Client

	udpPool *pool.Pool
	tcpPool *pool.Pool // nil unless mode is "tcp" or "dot"
}

// NewClient builds a Client from cfg, dialing no connections eagerly (pools
// dial lazily/on demand).
func NewClient(cfg config.UpstreamConfig) (*Client, error) {
	var addresses, dohURLs []string
	for _, addr := range cfg.Addresses {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}

		addresses = append(addresses, addr)
		dohURLs = append(dohURLs, fmt.Sprintf("https://%s%s", addr, cfg.DoH.QueryPath))
	}

	if len(addresses) == 0 {
		return nil, fmt.Errorf("no valid remote addresses configured")
	}

	c := &Client{
		cfg:       cfg,
		Addresses: addresses,
		DoHURLs:   dohURLs,
		bufPool: &sync.Pool{
			New: func() interface{} {
				b := make([]byte, cfg.BufferSize+1024)
				return &b
			},
		},
	}

	c.udpPool = pool.New(cfg.PoolSize, addresses, c.dialUDP)

	if cfg.Mode == "tcp" || cfg.Mode == "dot" {
		c.tcpPool = pool.New(cfg.PoolSize, addresses, c.dialTCP)
	}

	if cfg.Mode == "doh" {
		c.dohClient = newDOHClient(cfg)
	}

	return c, nil
}

func newDOHClient(cfg config.UpstreamConfig) *http.Client {
	dialer := &net.Dialer{
		Timeout:   time.Duration(cfg.Timeout) * time.Second,
		KeepAlive: time.Duration(cfg.KeepAlive) * time.Second,
		Control:   util.SocketControl(cfg.BufferSize),
	}

	tcpOptions := util.TCPOptions(cfg.KeepAlive, cfg.BufferSize)

	transportH2 := &http.Transport{
		DialContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}

			if tcpConn, ok := conn.(*net.TCPConn); ok {
				tcpOptions(tcpConn)
			}

			return conn, nil
		},
		MaxIdleConns:          cfg.DoH.Idle.MaxConnection,
		MaxIdleConnsPerHost:   cfg.DoH.Idle.MaxConnectionPerHost,
		IdleConnTimeout:       time.Duration(cfg.KeepAlive) * time.Second,
		ResponseHeaderTimeout: time.Duration(cfg.Timeout) * time.Second,
		ForceAttemptHTTP2:     true,
		TLSClientConfig: &tls.Config{
			ServerName:         cfg.Domain,
			InsecureSkipVerify: cfg.SkipTLSVerify,
			ClientSessionCache: tls.NewLRUClientSessionCache(cfg.PoolSize),
		},
	}

	transportH3 := &http3.Transport{
		EnableDatagrams: true,
		QUICConfig: &quic.Config{
			KeepAlivePeriod: time.Duration(cfg.KeepAlive) * time.Second,
			MaxIdleTimeout:  time.Duration(cfg.Timeout) * time.Second,
		},
		TLSClientConfig: &tls.Config{
			ServerName:         cfg.Domain,
			InsecureSkipVerify: cfg.SkipTLSVerify,
			ClientSessionCache: tls.NewLRUClientSessionCache(cfg.PoolSize),
		},
	}

	return &http.Client{
		Transport: &hybridRoundTripper{
			H2Transport: transportH2,
			H3Transport: transportH3,
		},
		Timeout: time.Duration(cfg.Timeout) * time.Second,
	}
}

func (c *Client) dialUDP(addr string) (*dns.Conn, error) {
	dc := new(dns.Client)
	dc.Net = "udp"
	dc.Dialer = &net.Dialer{
		Timeout:   time.Duration(c.cfg.Timeout) * time.Second,
		KeepAlive: time.Duration(c.cfg.KeepAlive) * time.Second,
		Control:   util.SocketControl(c.cfg.BufferSize),
	}

	conn, err := dc.Dial(addr)
	if err != nil {
		return nil, err
	}

	conn.UDPSize = uint16(c.cfg.BufferSize)

	return conn, nil
}

func (c *Client) dialTCP(addr string) (*dns.Conn, error) {
	dc := new(dns.Client)
	dc.Net = "tcp"

	if c.cfg.Mode == "dot" {
		host := c.cfg.Domain
		if host == "" && len(c.Addresses) > 0 {
			host = c.Addresses[0]
		}

		dc.Net = "tcp-tls"
		dc.TLSConfig = &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: c.cfg.SkipTLSVerify,
			ClientSessionCache: tls.NewLRUClientSessionCache(128),
		}
	}

	dc.Dialer = &net.Dialer{
		Timeout:   time.Duration(c.cfg.Timeout) * time.Second,
		KeepAlive: time.Duration(c.cfg.KeepAlive) * time.Second,
		Control:   util.SocketControl(c.cfg.BufferSize),
	}

	conn, err := dc.Dial(addr)
	if err != nil {
		return nil, err
	}

	if tcpConn, ok := conn.Conn.(*net.TCPConn); ok {
		util.TCPOptions(c.cfg.KeepAlive, c.cfg.BufferSize)(tcpConn)
	}

	return conn, nil
}

// Close stops any background resources held by the client (currently none;
// pools close connections lazily as they're returned/evicted).
func (c *Client) Close() {}
