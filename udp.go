package main

import (
	"fmt"
	"time"

	"github.com/miekg/dns"
)

func forwardUDP(m *dns.Msg, overrides []string) (*dns.Msg, error) {
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

		if len(overrides) > 0 {
			addr := overrides[attempts%len(overrides)]

			reused = false
			conn, err = udpPool.Dial(addr)
			if err != nil {
				lastErr = err
				attempts++

				continue
			}
		} else {
			conn, reused, err = udpPool.Get()
			if err != nil {
				return nil, err
			}
		}

		conn.SetWriteDeadline(ctxTimeout)
		conn.SetReadDeadline(ctxTimeout)

		resp, _, err := udpClient.ExchangeWithConn(m, conn)
		if err != nil {
			conn.Close()
			udpPool.Return(nil)

			if reused {
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
			udpPool.Return(conn)
		}

		return resp, nil
	}

	return nil, fmt.Errorf("Error DNS Upstream Failed After %d Attempts: %v", attempts, lastErr)
}
