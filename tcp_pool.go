package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/miekg/dns"
)

type TCPPool struct {
	conns     chan *dns.Conn
	addresses []string
	host      string
	mode      string
	capacity  int
}

func NewTCPPool(size int, addrs []string, host string, mode string) *TCPPool {
	return &TCPPool{
		conns:     make(chan *dns.Conn, size),
		addresses: addrs,
		host:      host,
		mode:      mode,
		capacity:  size,
	}
}

func (p *TCPPool) NewConn() (*dns.Conn, error) {
	var lastErr error

	for _, addr := range p.addresses {
		conn, err := p.Dial(addr)
		if err == nil {
			if tcpConn, ok := conn.Conn.(*net.TCPConn); ok {
				setTCPOptions(tcpConn)
			}

			return conn, nil
		}

		lastErr = err
	}

	return nil, fmt.Errorf("Error Failed to Dial DNS Upstreams: %v", lastErr)
}

func (p *TCPPool) Dial(addr string) (*dns.Conn, error) {
	c := new(dns.Client)
	c.Net = "tcp"

	if p.mode == "dot" {
		c.Net = "tcp-tls"
		c.TLSConfig = &tls.Config{
			ServerName:         p.host,
			InsecureSkipVerify: config.Upstream.SkipTLSVerify,
			ClientSessionCache: tls.NewLRUClientSessionCache(128),
		}
	}

	c.Dialer = &net.Dialer{
		Timeout:   time.Duration(config.Upstream.Timeout) * time.Second,
		KeepAlive: time.Duration(config.Upstream.KeepAlive) * time.Second,
		Control:   setSocketOptions,
	}

	return c.Dial(addr)
}

func (p *TCPPool) Get() (*dns.Conn, bool, error) {
	select {
	case conn := <-p.conns:
		if conn == nil {
			c, err := p.NewConn()
			return c, false, err
		}

		return conn, true, nil

	default:
		c, err := p.NewConn()
		return c, false, err
	}
}

func (p *TCPPool) Return(c *dns.Conn) {
	if c == nil {
		return
	}

	if c.Conn == nil {
		return
	}

	select {
	case p.conns <- c:
	default:
		c.Close()
	}
}
