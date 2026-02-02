package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/miekg/dns"
)

var (
	config     *Config
	configFile string
)

var (
	tcpPool      *ConnPool
	bufPool      *sync.Pool
	udpClient    *dns.Client
	dohClient    *http.Client
	dnsLocal     *LocalResolver
	dnsForwarder *ForwarderResolver
	dnsCache     *DNSCache
)

var (
	dnsAddreses []string
	dohURL      []string
)

func init() {
	var err error

	flag.StringVar(&configFile, "config", "./dns-proxy.yaml", "Path to YAML configuration file")
	flag.Parse()

	config, err = LoadConfig(configFile)
	if err != nil {
		log.Fatalf("Error Failed to Load Configuration: %v", err)
	}
}

func main() {
	bufPool = &sync.Pool{
		New: func() interface{} {
			b := make([]byte, config.Upstream.BufferSize+1024)
			return &b
		},
	}

	udpClient = &dns.Client{
		Net:            "udp",
		DialTimeout:    time.Duration(config.Upstream.Timeout) * time.Second,
		Timeout:        time.Duration(config.Upstream.Timeout) * time.Second,
		SingleInflight: true,
		UDPSize:        uint16(config.Upstream.BufferSize),
	}

	dohClient = &http.Client{
		Timeout: time.Duration(config.Upstream.Timeout) * time.Second,
		Transport: &http.Transport{
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        config.Upstream.DoH.Idle.MaxConnection,
			MaxIdleConnsPerHost: config.Upstream.DoH.Idle.MaxConnectionPerHost,
			IdleConnTimeout:     time.Duration(config.Upstream.KeepAlive) * time.Second,
			DisableKeepAlives:   false,
			DisableCompression:  false,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: config.Upstream.SkipTLSVerify,
			},
		},
	}

	for _, remoteAddr := range config.Upstream.Addresses {
		remoteAddr = strings.TrimSpace(remoteAddr)
		if remoteAddr == "" {
			continue
		}

		// Populate DNS Upstream Targets
		dnsAddreses = append(dnsAddreses, remoteAddr)

		// Populate DNS-Over-HTTPS URLs
		url := fmt.Sprintf("https://%s%s", remoteAddr, config.Upstream.DoH.QueryPath)
		dohURL = append(dohURL, url)
	}

	if len(dnsAddreses) == 0 {
		log.Fatal("Error No Valid Remote Addresses Provided")
	}

	if config.Upstream.Mode == "tcp" || config.Upstream.Mode == "dot" {
		hostSNI := config.Upstream.Domain
		if hostSNI == "" && len(dnsAddreses) > 0 {
			hostSNI = dnsAddreses[0]
		}

		tcpPool = NewPool(config.Upstream.PoolSize, dnsAddreses, hostSNI, config.Upstream.Mode)
		log.Printf("Initialized: Connection Pool (Size: %d)", config.Upstream.PoolSize)
	}

	dnsLocal = NewLocalResolver(config.Local)
	if config.Local.Enable {
		log.Printf("Initialized: Local Resolver (Hosts File: %v, Static: %d)", config.Local.UseHostsFile, len(config.Local.StaticRecords))
	}

	dnsForwarder = NewForwarderResolver(config.Forwarder)
	if config.Forwarder.Enable {
		log.Printf("Initialized: Forwarder Resolver (Rules: %d)", len(config.Forwarder.Rules))
	}

	dnsCache = NewCache(config.Cache.Size, config.Cache.MinTTL, config.Cache.NegTTL)
	if config.Cache.Size > 0 {
		log.Printf("Initialized: DNS Cache (Size: %d, Minimum TTL: %ds, Negative TTL: %ds)", config.Cache.Size, config.Cache.MinTTL, config.Cache.NegTTL)
	}

	dns.HandleFunc(".", handleRequest)

	go startListener("udp", fmt.Sprintf("%s:%s", config.Server.Address, config.Server.Port))
	go startListener("tcp", fmt.Sprintf("%s:%s", config.Server.Address, config.Server.Port))

	log.Printf("DNS Proxy Listening on %s:%s -> %v [%s]", config.Server.Address, config.Server.Port, dnsAddreses, strings.ToUpper(config.Upstream.Mode))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	fmt.Println("")
	log.Println("Shutdown Complete")
}

func startListener(netType string, addr string) {
	var server *dns.Server

	lc := net.ListenConfig{
		Control: setSocketOptions,
	}

	switch netType {
	case "udp":
		l, err := lc.ListenPacket(context.Background(), netType, addr)
		if err != nil {
			log.Fatalf("Failed to Listen on '%s': %v", strings.ToUpper(netType), err)
		}

		server = &dns.Server{PacketConn: l, Net: netType}

	case "tcp":
		l, err := lc.Listen(context.Background(), netType, addr)
		if err != nil {
			log.Fatalf("Failed to Listen on '%s': %v", strings.ToUpper(netType), err)
		}

		server = &dns.Server{Listener: l, Net: netType}
	}

	if err := server.ActivateAndServe(); err != nil {
		log.Fatalf("Failed to Start '%s' Listener: %s", netType, err.Error())
	}
}

func handleRequest(w dns.ResponseWriter, r *dns.Msg) {
	var err error
	var resp *dns.Msg

	if localResp := dnsLocal.Resolve(r.Question[0]); localResp != nil {
		localResp.SetReply(r)
		localResp.Compress = config.Server.Compress

		w.WriteMsg(localResp)
		return
	}

	cachedResp := dnsCache.Get(r)
	if cachedResp != nil {
		cachedResp.SetReply(r)
		cachedResp.Compress = config.Server.Compress

		w.WriteMsg(cachedResp)
		return
	}

	forwardFound := false
	if config.Forwarder.Enable {
		if target, found := dnsForwarder.GetUpstream(r.Question[0].Name); found {
			resp, err = forwardUDP(r, []string{target})
			forwardFound = true
		}
	}

	if !forwardFound {
		switch config.Upstream.Mode {
		case "doh":
			resp, err = forwardDoH(r, dohURL)
		case "tcp", "dot":
			resp, err = forwardTCP(r)
		case "udp":
			resp, err = forwardUDP(r, dnsAddreses)
		}
	}

	if err != nil {
		log.Printf("Error DNS Upstream Server: %v", err)

		failMsg := new(dns.Msg)
		failMsg.SetRcode(r, dns.RcodeServerFailure)
		failMsg.Compress = config.Server.Compress

		w.WriteMsg(failMsg)
		return
	}

	if resp != nil {
		dnsCache.Set(resp)
	}

	resp.SetReply(r)
	resp.Compress = config.Server.Compress

	w.WriteMsg(resp)
}
