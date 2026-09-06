//go:build windows

package util

import "syscall"

// SocketControl returns a net.Dialer/net.ListenConfig Control function.
// No Windows-specific socket options are applied.
func SocketControl(bufferSize int) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		return c.Control(func(fd uintptr) {
			// Windows-specific options
		})
	}
}
