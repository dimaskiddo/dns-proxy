package main

import (
	"fmt"
	"net"
	"time"

	"github.com/miekg/dns"
)

type UDPPool struct {
	conns     chan *dns.Conn
	addresses []string
	capacity  int
}

func NewUDPPool(size int, addrs []string) *UDPPool {
	return &UDPPool{
		conns:     make(chan *dns.Conn, size),
		addresses: addrs,
		capacity:  size,
	}
}

func (p *UDPPool) NewConn() (*dns.Conn, error) {
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

func (p *UDPPool) Dial(addr string) (*dns.Conn, error) {
	c := new(dns.Client)
	c.Net = "udp"

	c.Dialer = &net.Dialer{
		Timeout:   time.Duration(config.Upstream.Timeout) * time.Second,
		KeepAlive: time.Duration(config.Upstream.KeepAlive) * time.Second,
		Control:   setSocketOptions,
	}

	return c.Dial(addr)
}

func (p *UDPPool) Get() (*dns.Conn, bool, error) {
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

func (p *UDPPool) Return(c *dns.Conn) {
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
