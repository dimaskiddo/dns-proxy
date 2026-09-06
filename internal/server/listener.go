package server

import (
	"context"
	"log"
	"net"
	"strings"

	"github.com/miekg/dns"

	"github.com/dimaskiddo/dns-proxy/internal/util"
)

// StartListener starts a blocking UDP or TCP DNS listener on addr, tuning
// the listen socket using bufferSize. It calls log.Fatalf on failure.
func StartListener(netType string, addr string, bufferSize int) {
	var srv *dns.Server

	lc := net.ListenConfig{
		Control: util.SocketControl(bufferSize),
	}

	switch netType {
	case "udp":
		l, err := lc.ListenPacket(context.Background(), netType, addr)
		if err != nil {
			log.Fatalf("Failed to Listen on '%s': %v", strings.ToUpper(netType), err)
		}

		// UDPSize defaults to dns.MinMsgSize (512) when left zero, clipping
		// any larger EDNS query (cookies, ECS, DNSSEC, long names) to a read
		// that fails to unpack and FORMERRs. Size it from the configured
		// buffer instead.
		srv = &dns.Server{PacketConn: l, Net: netType, UDPSize: int(util.ClampUDPSize(bufferSize))}

	case "tcp":
		l, err := lc.Listen(context.Background(), netType, addr)
		if err != nil {
			log.Fatalf("Failed to Listen on '%s': %v", strings.ToUpper(netType), err)
		}

		srv = &dns.Server{Listener: l, Net: netType}

	default:
		log.Fatalf("Unsupported listener type %q", netType)
	}

	if err := srv.ActivateAndServe(); err != nil {
		log.Fatalf("Failed to Start '%s' Listener: %s", netType, err.Error())
	}
}
