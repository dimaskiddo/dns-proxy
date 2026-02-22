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
	version = "dev"
	commit  = "none"
)

var (
	config     *Config
	configLock sync.RWMutex
	configFile string
)

var (
	tcpPool      *TCPPool
	udpPool      *UDPPool
	bufPool      *sync.Pool
	dohClient    *http.Client
	dnsLocal     *LocalResolver
	dnsForwarder *ForwarderResolver
	dnsCache     *DNSCache
	dnsEDNS      *EDNSHandler
)

var (
	dnsAddreses    []string
	dohURLs        []string
	bogusNXDomains []net.IP
)

func init() {
	var showVersion bool

	flag.StringVar(&configFile, "config", "./dns-proxy.yaml", "Path to YAML configuration file")
	flag.BoolVar(&showVersion, "version", false, "Show DNS-Proxy version")
	flag.Parse()

	if showVersion {
		fmt.Println("DNS-Proxy v" + version + "~" + commit)
		fmt.Println("By Dimas Restu H <drh.dimasrestu@gmail.com>")
		os.Exit(0)
	}
}

func parseConfig() error {
	var err error

	newConfig, err := LoadConfig(configFile)
	if err != nil {
		log.Fatalf("Error Failed to Load Configuration: %v", err)
	}

	if len(newConfig.Server.Listen) == 0 {
		log.Fatal("Error No Listen Addresses Configured")
	}

	newBufPool := &sync.Pool{
		New: func() interface{} {
			b := make([]byte, newConfig.Upstream.BufferSize+1024)
			return &b
		},
	}

	newDOHDialer := &net.Dialer{
		Timeout:   time.Duration(newConfig.Upstream.Timeout) * time.Second,
		KeepAlive: time.Duration(newConfig.Upstream.KeepAlive) * time.Second,
		Control:   setSocketOptions,
	}

	newDOHClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
				conn, err := newDOHDialer.DialContext(ctx, network, addr)
				if err != nil {
					return nil, err
				}

				if tcpConn, ok := conn.(*net.TCPConn); ok {
					setTCPOptions(tcpConn)
				}

				return conn, nil
			},
			MaxIdleConns:          newConfig.Upstream.DoH.Idle.MaxConnection,
			MaxIdleConnsPerHost:   newConfig.Upstream.DoH.Idle.MaxConnectionPerHost,
			IdleConnTimeout:       time.Duration(newConfig.Upstream.KeepAlive) * time.Second,
			ResponseHeaderTimeout: time.Duration(newConfig.Upstream.Timeout) * time.Second,
			ForceAttemptHTTP2:     true,
			DisableKeepAlives:     false,
			DisableCompression:    false,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: newConfig.Upstream.SkipTLSVerify,
			},
		},
	}

	var newDnsAddresses, newDOHURLs []string
	for _, remoteAddr := range newConfig.Upstream.Addresses {
		remoteAddr = strings.TrimSpace(remoteAddr)
		if remoteAddr == "" {
			continue
		}

		// Populate DNS Upstream Targets
		newDnsAddresses = append(newDnsAddresses, remoteAddr)

		// Populate DNS-Over-HTTPS URLs
		url := fmt.Sprintf("https://%s%s", remoteAddr, newConfig.Upstream.DoH.QueryPath)
		newDOHURLs = append(newDOHURLs, url)
	}

	if len(newDnsAddresses) == 0 {
		log.Fatal("Error No Valid Remote Addresses Provided")
	}

	newUDPPool := NewUDPPool(newConfig.Upstream.PoolSize, newDnsAddresses)
	log.Printf("Initialized: Connection UDP Pool (Size: %d)", newConfig.Upstream.PoolSize)

	var newTCPPool *TCPPool
	if newConfig.Upstream.Mode == "tcp" || newConfig.Upstream.Mode == "dot" {
		hostSNI := newConfig.Upstream.Domain
		if hostSNI == "" && len(newDnsAddresses) > 0 {
			hostSNI = newDnsAddresses[0]
		}

		newTCPPool = NewTCPPool(newConfig.Upstream.PoolSize, newDnsAddresses, hostSNI, newConfig.Upstream.Mode)
		log.Printf("Initialized: Connection TCP Pool (Size: %d)", newConfig.Upstream.PoolSize)
	}

	newDNSCache := NewCache(newConfig.Cache.Size, newConfig.Cache.Shards, newConfig.Cache.MinTTL, newConfig.Cache.NegTTL)
	if newConfig.Cache.Size > 0 {
		log.Printf("Initialized: DNS Cache (Size: %d, Shards: %d, Minimum TTL: %ds, Negative TTL: %ds)", newConfig.Cache.Size, newConfig.Cache.Shards, newConfig.Cache.MinTTL, newConfig.Cache.NegTTL)
	}

	var newBogusNXDomains []net.IP
	if newConfig.BogusNXDomain.Enable {
		for _, ipStr := range newConfig.BogusNXDomain.IPs {
			ipStr = strings.TrimSpace(ipStr)
			if ip := net.ParseIP(ipStr); ip != nil {
				newBogusNXDomains = append(newBogusNXDomains, ip)
			}
		}

		log.Printf("Initialized: Bogus NXDomain Filtering (Total IPs: %d)", len(newBogusNXDomains))
	}

	newDNSEDNS := NewEDNSHandler(newConfig.EDNS)
	if newConfig.EDNS.Enable {
		log.Printf("Initialized: EDNS0 Client Subnet (IPv4 Mask: /%d, IPv6 Mask: /%d)", newConfig.EDNS.IPv4Mask, newConfig.EDNS.IPv6Mask)
	}

	newDNSLocal := NewLocalResolver(newConfig.Local, newConfig.Cache.MinTTL)
	if newConfig.Local.Enable {
		log.Printf("Initialized: Local Resolver (Hosts File: %v, Static: %d)", newConfig.Local.UseHostsFile, len(newConfig.Local.StaticRecords))
	}

	newDNSForwarder := NewForwarderResolver(newConfig.Forwarder)
	if newConfig.Forwarder.Enable {
		log.Printf("Initialized: Forwarder Resolver (Rules: %d)", len(newConfig.Forwarder.Rules))
	}

	configLock.Lock()
	defer configLock.Unlock()

	if dnsCache != nil {
		dnsCache.Stop()
	}

	config = newConfig

	bufPool = newBufPool
	dohClient = newDOHClient

	dnsAddreses = newDnsAddresses
	dohURLs = newDOHURLs
	bogusNXDomains = newBogusNXDomains

	udpPool = newUDPPool
	tcpPool = newTCPPool

	dnsCache = newDNSCache
	dnsEDNS = newDNSEDNS
	dnsLocal = newDNSLocal
	dnsForwarder = newDNSForwarder

	return nil
}

func main() {
	if err := parseConfig(); err != nil {
		log.Fatalf("Error Initial Configuration Load: %v", err)
	}

	dns.HandleFunc(".", handleRequest)

	for _, addr := range config.Server.Listen {
		go startListener("udp", addr)
		go startListener("tcp", addr)

		log.Printf("DNS Proxy Listening on %s -> %v [%s]", addr, dnsAddreses, strings.ToUpper(config.Upstream.Mode))
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		s := <-sig
		if s == syscall.SIGHUP {
			log.Println("Reloading Configuration...")

			if err := parseConfig(); err != nil {
				log.Printf("Error Reloading Configuration: %v", err)
			}
		} else {
			// Exit Loop and Continue to Shutdown
			break
		}
	}

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
	configLock.RLock()
	defer configLock.RUnlock()

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

	if dnsEDNS != nil {
		dnsEDNS.AddECS(r, w.RemoteAddr().String())
	}

	forwardFound := false
	if config.Forwarder.Enable {
		if targets, found := dnsForwarder.GetUpstream(r.Question[0].Name); found {
			resp, err = forwardUDP(r, targets)
			forwardFound = true
		}
	}

	if !forwardFound {
		switch config.Upstream.Mode {
		case "doh":
			resp, err = forwardDoH(r, dohURLs)
		case "tcp", "dot":
			resp, err = forwardTCP(r)
		case "udp":
			resp, err = forwardUDP(r, nil)
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
		if config.BogusNXDomain.Enable {
			checkBogusNXDomain(resp)
		}

		dnsCache.Set(resp)
	}

	resp.SetReply(r)
	resp.Compress = config.Server.Compress

	w.WriteMsg(resp)
}
