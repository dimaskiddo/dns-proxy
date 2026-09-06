package upstream

import (
	"fmt"
	"io"
	"time"

	"github.com/miekg/dns"

	"github.com/dimaskiddo/dns-proxy/internal/util"
)

// ForwardUDP sends m over a pooled UDP connection, retrying up to
// MaxAttempts times. When overrides is non-empty, each attempt dials
// (uncached) one of those addresses round-robin instead of using the pool.
func (c *Client) ForwardUDP(m *dns.Msg, overrides []string) (*dns.Msg, error) {
	var conn *dns.Conn
	var reused bool

	var err error
	var lastErr error

	attempts := 0
	maxAttempts := c.cfg.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for attempts < maxAttempts {
		deadline := time.Now().Add(time.Duration(c.cfg.Timeout) * time.Second)

		if len(overrides) > 0 {
			addr := overrides[attempts%len(overrides)]

			reused = false
			conn, err = c.udpPool.Dial(addr)
			if err != nil {
				lastErr = err
				attempts++

				continue
			}
		} else {
			conn, reused, err = c.udpPool.Get()
			if err != nil {
				return nil, err
			}
		}

		conn.SetWriteDeadline(deadline)
		conn.SetReadDeadline(deadline)

		if err := conn.WriteMsg(m); err != nil {
			conn.Close()
			c.udpPool.Return(nil)

			if reused {
				continue
			}

			lastErr = err
			attempts++
			continue
		}

		resp, err := conn.ReadMsg()
		if err != nil {
			conn.Close()
			c.udpPool.Return(nil)

			if reused && (err == io.EOF || util.IsNetworkError(err)) {
				continue
			}

			lastErr = err
			attempts++
			continue
		}

		conn.SetWriteDeadline(time.Time{})
		conn.SetReadDeadline(time.Time{})

		if len(overrides) > 0 {
			conn.Close()
		} else {
			c.udpPool.Return(conn)
		}

		return resp, nil
	}

	return nil, fmt.Errorf("[UDP] DNS upstream failed after %d attempts: %w", attempts, lastErr)
}
