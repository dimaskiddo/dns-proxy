package upstream

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/miekg/dns"

	"github.com/dimaskiddo/dns-proxy/internal/util"
)

// ForwardTCP sends m over a pooled TCP (or DoT) connection, retrying up to
// MaxAttempts times.
func (c *Client) ForwardTCP(m *dns.Msg) (*dns.Msg, error) {
	if c.tcpPool == nil {
		return nil, fmt.Errorf("[TCP] pool not initialized for mode %q", c.cfg.Mode)
	}

	var conn *dns.Conn
	var reused bool

	var err error
	var lastErr error

	attempts := 0
	maxAttempts := c.cfg.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	// See the matching comment in ForwardUDP: bounds the loop independent of
	// attempts, since a reused connection's failure doesn't consume one.
	iterations := 0
	maxIterations := maxAttempts + c.tcpPool.Cap()

	for attempts < maxAttempts && iterations < maxIterations {
		iterations++
		deadline := time.Now().Add(time.Duration(c.cfg.Timeout) * time.Second)

		conn, reused, err = c.tcpPool.Get()
		if err != nil {
			return nil, err
		}

		conn.SetWriteDeadline(deadline)
		conn.SetReadDeadline(deadline)

		if err := conn.WriteMsg(m); err != nil {
			conn.Close()
			lastErr = fmt.Errorf("[TCP] failed to write: %w", err)

			if reused {
				continue
			}

			attempts++
			continue
		}

		resp, err := conn.ReadMsg()
		if err != nil {
			conn.Close()
			lastErr = fmt.Errorf("[TCP] failed to read: %w", err)

			if reused && (errors.Is(err, io.EOF) || util.IsNetworkError(err)) {
				continue
			}

			attempts++
			continue
		}

		if resp.Id != m.Id {
			// See ForwardUDP's matching check.
			conn.Close()
			lastErr = fmt.Errorf("[TCP] id mismatch: got %d, want %d", resp.Id, m.Id)
			attempts++

			continue
		}

		conn.SetWriteDeadline(time.Time{})
		conn.SetReadDeadline(time.Time{})

		c.tcpPool.Return(conn)

		return resp, nil
	}

	return nil, fmt.Errorf("[TCP] DNS upstream failed after %d attempts: %w", attempts, lastErr)
}
