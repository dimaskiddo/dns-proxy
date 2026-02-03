package main

import (
	"net"
	"time"
)

func setTCPOptions(conn *net.TCPConn) {
	conn.SetNoDelay(true)

	conn.SetKeepAlive(true)
	conn.SetKeepAlivePeriod(time.Duration(config.Upstream.KeepAlive) * time.Second)

	conn.SetReadBuffer(config.Upstream.BufferSize)
	conn.SetWriteBuffer(config.Upstream.BufferSize)
}

func isNetworkError(err error) bool {
	if _, ok := err.(net.Error); ok {
		return true
	}

	return false
}
