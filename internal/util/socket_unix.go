//go:build linux || darwin

package util

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// SocketControl returns a net.Dialer/net.ListenConfig Control function that
// tunes the receive buffer (scaled by bufferSize) and enables address/port
// reuse.
func SocketControl(bufferSize int) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		return c.Control(func(fd uintptr) {
			unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_RCVBUF, bufferSize*1024)

			unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
			unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
		})
	}
}
