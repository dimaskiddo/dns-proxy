package main

import (
	"fmt"
	"io"
	"time"

	"github.com/miekg/dns"
)

func forwardTCP(m *dns.Msg) (*dns.Msg, error) {
	var conn *dns.Conn
	var reused bool

	var err error
	var lastErr error

	attempts := 0
	maxAttempts := config.Upstream.MaxAttempts

	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for attempts < maxAttempts {
		ctxTimeout := time.Now().Add(time.Duration(config.Upstream.Timeout) * time.Second)

		conn, reused, err = tcpPool.Get()
		if err != nil {
			return nil, err
		}

		conn.SetWriteDeadline(ctxTimeout)
		conn.SetReadDeadline(ctxTimeout)

		if err := conn.WriteMsg(m); err != nil {
			conn.Close()
			tcpPool.Return(nil)

			if reused {
				continue
			}

			lastErr = fmt.Errorf("Error Failed to Write: %w", err)
			attempts++

			continue
		}

		resp, err := conn.ReadMsg()
		if err != nil {
			conn.Close()
			tcpPool.Return(nil)

			if reused && (err == io.EOF || isNetworkError(err)) {
				continue
			}

			lastErr = fmt.Errorf("Error Failed to Read: %w", err)
			attempts++

			continue
		}

		conn.SetWriteDeadline(time.Time{})
		conn.SetReadDeadline(time.Time{})

		tcpPool.Return(conn)

		return resp, nil
	}

	return nil, fmt.Errorf("Error DNS Upstream Failed After %d Attempts: %v", attempts, lastErr)
}
