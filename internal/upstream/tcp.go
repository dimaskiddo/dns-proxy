package upstream

import (
	"fmt"
	"io"
	"time"

	"github.com/miekg/dns"

	"github.com/dimaskiddo/dns-proxy/internal/util"
)

// ForwardTCP sends m over a pooled TCP (or DoT) connection, retrying up to
// MaxAttempts times.
func (c *Client) ForwardTCP(m *dns.Msg) (*dns.Msg, error) {
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

		conn, reused, err = c.tcpPool.Get()
		if err != nil {
			return nil, err
		}

		conn.SetWriteDeadline(deadline)
		conn.SetReadDeadline(deadline)

		if err := conn.WriteMsg(m); err != nil {
			conn.Close()
			c.tcpPool.Return(nil)

			if reused {
				continue
			}

			lastErr = fmt.Errorf("[TCP] failed to write: %w", err)
			attempts++

			continue
		}

		resp, err := conn.ReadMsg()
		if err != nil {
			conn.Close()
			c.tcpPool.Return(nil)

			if reused && (err == io.EOF || util.IsNetworkError(err)) {
				continue
			}

			lastErr = fmt.Errorf("[TCP] failed to read: %w", err)
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
