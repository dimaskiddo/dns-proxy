// Package pool provides a generic channel-based dns.Conn pool. TCP/UDP/DoT
// each supply their own Dial function; the pooling logic itself (Get/Return)
// is protocol-agnostic.
package pool

import (
	"fmt"

	"github.com/miekg/dns"
)

// Pool is a fixed-capacity channel of reusable *dns.Conn, falling back to
// Dial when empty.
type Pool struct {
	conns     chan *dns.Conn
	addresses []string

	// Dial opens a new connection to addr. Set by the caller; also used
	// directly (bypassing the pool) for per-request upstream overrides.
	Dial func(addr string) (*dns.Conn, error)
}

// New creates a Pool with the given capacity, upstream addresses, and dial
// function.
func New(size int, addresses []string, dial func(addr string) (*dns.Conn, error)) *Pool {
	return &Pool{
		conns:     make(chan *dns.Conn, size),
		addresses: addresses,
		Dial:      dial,
	}
}

// NewConn dials each configured address in turn, returning the first
// successful connection.
func (p *Pool) NewConn() (*dns.Conn, error) {
	var lastErr error

	for _, addr := range p.addresses {
		conn, err := p.Dial(addr)
		if err == nil {
			return conn, nil
		}

		lastErr = err
	}

	return nil, fmt.Errorf("failed to dial DNS upstreams: %w", lastErr)
}

// Get returns a pooled connection if one is available, otherwise dials a
// new one. The bool reports whether the connection was reused from the pool.
func (p *Pool) Get() (*dns.Conn, bool, error) {
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

// Return gives a connection back to the pool, closing it if the pool is full.
func (p *Pool) Return(c *dns.Conn) {
	if c == nil || c.Conn == nil {
		return
	}

	select {
	case p.conns <- c:
	default:
		c.Close()
	}
}

// Cap returns the pool's connection capacity (its configured pool_size).
func (p *Pool) Cap() int {
	return cap(p.conns)
}

// Close drains and closes every pooled connection. Callers already holding
// checked-out connections are unaffected; this only reclaims what is idle
// in the pool at the moment of the call.
func (p *Pool) Close() {
	for {
		select {
		case c := <-p.conns:
			if c != nil {
				c.Close()
			}
		default:
			return
		}
	}
}
