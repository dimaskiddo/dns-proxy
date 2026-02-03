package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/miekg/dns"
)

type ConnPool struct {
	conns     chan *dns.Conn
	addresses []string
	host      string
	mode      string
	capacity  int
}

func NewPool(size int, addrs []string, host string, mode string) *ConnPool {
	return &ConnPool{
		conns:     make(chan *dns.Conn, size),
		addresses: addrs,
		host:      host,
		mode:      mode,
		capacity:  size,
	}
}

func (p *ConnPool) NewConn() (*dns.Conn, error) {
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

	var errLast error
	for _, addr := range p.addresses {
		conn, err := c.Dial(addr)
		if err == nil {
			if tcpConn, ok := conn.Conn.(*net.TCPConn); ok {
				setTCPOptions(tcpConn)
			}

			return conn, nil
		}

		errLast = err
	}

	return nil, fmt.Errorf("Error Failed to Dial DNS Upstreams: %v", errLast)
}

func (p *ConnPool) Get() (*dns.Conn, bool, error) {
	select {
	case conn := <-p.conns:
		return conn, true, nil
	default:
		c, err := p.NewConn()
		return c, false, err
	}
}

func (p *ConnPool) Return(c *dns.Conn) {
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
