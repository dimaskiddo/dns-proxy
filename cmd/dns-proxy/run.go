package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/miekg/dns"
	"github.com/spf13/cobra"

	"github.com/dimaskiddo/dns-proxy/internal/config"
	"github.com/dimaskiddo/dns-proxy/internal/server"
)

// runCmd is a hidden alias for the root command's default action. The pre-refactor
// binary had no subcommands at all — starting the server is the root command's job
// (see main.go) — but this stays registered, undocumented, for anything already
// scripted against the refactored `dns-proxy run --config ...` form.
func runCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "run",
		Short:  "Run the DNS Proxy server",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("DNS-Proxy v" + version + "~" + commit)
			fmt.Println("By Dimas Restu H <drh.dimasrestu@gmail.com>")
			fmt.Println("-------------------------------------------")

			return runServer(cfgFile)
		},
	}
}

func runServer(configFile string) error {
	mgr, err := config.NewManager(configFile)
	if err != nil {
		log.Fatalf("Error Failed to Load Configuration: %v", err)
	}

	srv := server.New()
	if err := reload(srv, mgr); err != nil {
		log.Fatalf("Error Initial Configuration Load: %v", err)
	}

	dns.HandleFunc(".", srv.HandleRequest)

	cfg := mgr.GetConfig()
	for _, addr := range cfg.Server.Listen {
		go server.StartListener("udp", addr, cfg.Upstream.BufferSize)
		go server.StartListener("tcp", addr, cfg.Upstream.BufferSize)

		log.Printf("DNS Proxy Listening on %s -> %v [%s]", addr, cfg.Upstream.Addresses, strings.ToUpper(cfg.Upstream.Mode))
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		s := <-sig
		if s != syscall.SIGHUP {
			break
		}

		log.Println("Reloading Configuration...")

		if err := mgr.Reload(); err != nil {
			log.Printf("Error Reloading Configuration: %v", err)
			continue
		}

		if err := reload(srv, mgr); err != nil {
			log.Printf("Error Reloading Configuration: %v", err)
		}
	}

	fmt.Println("")
	log.Println("Shutdown Complete")

	return nil
}

// reload builds a Runtime from mgr's current config and installs it on srv,
// logging what got (re)initialized.
func reload(srv *server.Server, mgr *config.Manager) error {
	cfg := mgr.GetConfig()

	rt, err := server.NewRuntime(cfg)
	if err != nil {
		return err
	}

	srv.SetRuntime(rt)

	log.Printf("Initialized: Connection UDP Pool (Size: %d)", cfg.Upstream.PoolSize)
	if cfg.Upstream.Mode == "tcp" || cfg.Upstream.Mode == "dot" {
		log.Printf("Initialized: Connection TCP Pool (Size: %d)", cfg.Upstream.PoolSize)
	}

	if cfg.BogusNXDomain.Enable {
		log.Printf("Initialized: Bogus NXDomain Filtering (Total IPs: %d)", rt.Bogus.Count())
	}

	if cfg.EDNS.Enable {
		log.Printf("Initialized: EDNS0 Client Subnet (IPv4 Mask: /%d, IPv6 Mask: /%d)", cfg.EDNS.IPv4Mask, cfg.EDNS.IPv6Mask)
	}

	if cfg.Cache.Size > 0 {
		log.Printf("Initialized: DNS Cache (Size: %d, Shards: %d, Minimum TTL: %ds, Negative TTL: %ds)", cfg.Cache.Size, cfg.Cache.Shards, cfg.Cache.MinTTL, cfg.Cache.NegTTL)
	}

	if cfg.Local.Enable {
		log.Printf("Initialized: Local Resolver (Hosts File: %v, Static: %d)", cfg.Local.UseHostsFile, len(cfg.Local.StaticRecords))
	}

	if cfg.Forwarder.Enable {
		log.Printf("Initialized: Forwarder Resolver (Rules: %d)", len(cfg.Forwarder.Rules))
	}

	return nil
}
