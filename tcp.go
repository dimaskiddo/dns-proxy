package main

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/miekg/dns"
)

func forwardTCP(m *dns.Msg) (*dns.Msg, error) {
	const maxRetries = 3
	var lastErr error

	// Retry Loop
	for i := 0; i < maxRetries; i++ {
		conn, err := tcpPool.Get()
		if err != nil {
			return nil, err
		}

		ctxTimeout := time.Now().Add(time.Duration(config.Upstream.Timeout) * time.Second)

		conn.SetWriteDeadline(ctxTimeout)
		conn.SetReadDeadline(ctxTimeout)

		if err := conn.WriteMsg(m); err != nil {
			conn.Close()
			tcpPool.Return(nil)

			return nil, fmt.Errorf("Error Failed to Write: %w", err)
		}

		resp, err := conn.ReadMsg()
		if err != nil {
			if err == io.EOF || isNetworkError(err) {
				conn.Close()
				tcpPool.Return(nil)

				lastErr = fmt.Errorf("Error Failed to Read (Stale Connection): %w", err)
				continue
			}

			conn.Close()
			tcpPool.Return(nil)

			return nil, fmt.Errorf("Error Failed to Read: %w", err)
		}

		conn.SetWriteDeadline(time.Time{})
		conn.SetReadDeadline(time.Time{})

		tcpPool.Return(conn)

		return resp, nil
	}

	return nil, fmt.Errorf("Error DNS Upstream Failed After %d Retries: %v", maxRetries, lastErr)
}

func isNetworkError(err error) bool {
	if _, ok := err.(net.Error); ok {
		return true
	}

	return false
}
