package main

import (
	"bufio"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

type LocalResolver struct {
	records         map[string][]net.IP
	recordWildcards map[string][]net.IP
	minTTL          uint32
	mu              sync.RWMutex
}

func NewLocalResolver(cfg LocalConfig, minTTL int) *LocalResolver {
	lr := &LocalResolver{
		records:         make(map[string][]net.IP),
		recordWildcards: make(map[string][]net.IP),
		minTTL:          uint32(minTTL),
	}

	if !cfg.Enable {
		return lr
	}

	if config.Local.UseHostsFile {
		path := "/etc/hosts"
		if runtime.GOOS == "windows" {
			path = "C:\\Windows\\System32\\drivers\\etc\\hosts"
		}

		lr.loadHostsFile(path)
	}

	for _, rec := range cfg.StaticRecords {
		lr.addRecord(rec.Domain, rec.IP)
	}

	return lr
}

func (lr *LocalResolver) loadHostsFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		ip := net.ParseIP(parts[0])
		if ip == nil {
			continue
		}

		for _, domain := range parts[1:] {
			lr.addRecordIP(domain, ip)
		}
	}
}

func (lr *LocalResolver) addRecord(domain string, ipStr string) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return
	}

	lr.addRecordIP(domain, ip)
}

func (lr *LocalResolver) addRecordIP(domain string, ip net.IP) {
	isWildcard := false
	domain = dns.Fqdn(domain)

	if strings.HasPrefix(domain, "*.") {
		domain = domain[2:]
		isWildcard = true
	}

	lr.mu.Lock()
	defer lr.mu.Unlock()

	if isWildcard {
		lr.recordWildcards[domain] = append(lr.recordWildcards[domain], ip)
	} else {
		lr.records[domain] = append(lr.records[domain], ip)
	}
}

func (lr *LocalResolver) Resolve(q dns.Question) *dns.Msg {
	lr.mu.RLock()

	ips, found := lr.records[q.Name]
	if !found {
		var bestMatchLen int = -1
		for domain, ipsWildcard := range lr.recordWildcards {
			if strings.HasSuffix(q.Name, "."+domain) || q.Name == domain {
				// Logic: If this domain is longer than the previous best match, pick this one
				if len(domain) > bestMatchLen {
					bestMatchLen = len(domain)

					ips = ipsWildcard
					found = true
				}
			}
		}
	}

	lr.mu.RUnlock()

	if !found {
		return nil
	}

	m := new(dns.Msg)
	m.SetReply(&dns.Msg{Question: []dns.Question{q}})
	m.Authoritative = true

	for _, ip := range ips {
		var rr dns.RR

		if ip.To4() != nil && q.Qtype == dns.TypeA {
			rr = &dns.A{
				Hdr: dns.RR_Header{
					Name:   q.Name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    lr.minTTL,
				},
				A: ip,
			}
		} else if ip.To4() == nil && q.Qtype == dns.TypeAAAA {
			rr = &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   q.Name,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    lr.minTTL,
				},
				AAAA: ip,
			}
		}

		if rr != nil {
			m.Answer = append(m.Answer, rr)
		}
	}

	return m
}
