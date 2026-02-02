package main

import (
	"fmt"

	"github.com/miekg/dns"
)

func forwardUDP(m *dns.Msg, addrs []string) (*dns.Msg, error) {
	var errLast error

	for _, addr := range addrs {
		r, _, err := udpClient.Exchange(m, addr)
		if err == nil {
			return r, nil
		}

		errLast = err
	}

	return nil, fmt.Errorf("Error Failed to Dial DNS Upstreams: %v", errLast)
}
