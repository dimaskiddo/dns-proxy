package main

import (
	"net"
	"path/filepath"
	"time"

	"github.com/miekg/dns"
)

func nextPowerOfTwo(v int) int {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v++

	return v
}

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

func parseIncludeFiles(baseDir string, patterns []string) []string {
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

func filterIPv6Records(rrs []dns.RR) []dns.RR {
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
