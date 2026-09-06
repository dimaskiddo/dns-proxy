package util

import (
	"net"
	"path/filepath"
	"time"

	"github.com/miekg/dns"
)

// NextPowerOfTwo rounds v up to the next power of two.
func NextPowerOfTwo(v int) int {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v++

	return v
}

// TCPOptions returns a function that tunes a *net.TCPConn using the given
// keep-alive interval (seconds) and buffer size (bytes).
func TCPOptions(keepAlive, bufferSize int) func(*net.TCPConn) {
	return func(conn *net.TCPConn) {
		conn.SetNoDelay(true)

		conn.SetKeepAlive(true)
		conn.SetKeepAlivePeriod(time.Duration(keepAlive) * time.Second)

		conn.SetReadBuffer(bufferSize)
		conn.SetWriteBuffer(bufferSize)
	}
}

// IsNetworkError reports whether err is a net.Error.
func IsNetworkError(err error) bool {
	_, ok := err.(net.Error)
	return ok
}

// ParseIncludeFiles expands glob patterns (relative to baseDir when not
// absolute) into a deduplicated list of matching file paths.
func ParseIncludeFiles(baseDir string, patterns []string) []string {
	var files []string

	seen := make(map[string]bool)

	for _, pattern := range patterns {
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(baseDir, pattern)
		}

		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}

		for _, match := range matches {
			if !seen[match] {
				files = append(files, match)
				seen[match] = true
			}
		}
	}

	return files
}

// FilterIPv6Records drops AAAA records from rrs.
func FilterIPv6Records(rrs []dns.RR) []dns.RR {
	if len(rrs) == 0 {
		return rrs
	}

	var filtered []dns.RR
	for _, rr := range rrs {
		if rr.Header().Rrtype != dns.TypeAAAA {
			filtered = append(filtered, rr)
		}
	}

	return filtered
}
