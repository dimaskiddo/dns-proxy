# DNS-Proxy Project Information

## Overview

**DNS-Proxy** is a DNSMasq-style DNS proxy/forwarder written in Go, with modern upstream transport support beyond plain UDP/TCP — DNS-over-TLS (DoT) and DNS-over-HTTPS (DoH). It ships as a single `CGO_ENABLED=0` static binary, resolves queries through a local-record → cache → forwarder-rule → upstream chain, and hot-reloads its configuration on `SIGHUP` without dropping in-flight queries.

## Key Features

- **Multiple upstream transports:** plain UDP, plain TCP, DoT, and DoH — selected by `upstream.mode`, with DoH falling back from HTTP/3 to HTTP/2 automatically.
- **Sharded response cache:** power-of-two-sharded LRU keyed by question, honoring answer TTLs with a configurable floor and a separate, shorter negative-response TTL.
- **Local resolution:** static records, `/etc/hosts`-style file, and `include_files` glob-merged config for managed local zones.
- **Forwarder rules:** per-domain upstream overrides, independent of the global `upstream.mode`, also `include_files`-mergeable.
- **EDNS0 Client Subnet:** optional ECS injection with configurable IPv4/IPv6 mask, for upstreams that use it for geo-aware answers.
- **Bogus-NXDOMAIN filtering:** strips known ISP-hijack/sinkhole IPs from upstream answers when enabled.
- **Zero-downtime reload:** `SIGHUP` rebuilds the entire runtime and swaps it in atomically — no lock held across upstream I/O, no dropped in-flight queries.

## Security Model

1. **DoT/DoH encrypts the upstream hop** — queries between dns-proxy and its configured upstream are not visible on the wire in plaintext, unlike `udp`/`tcp` mode. **Caveat:** the shipped `dns-proxy.yaml.example` sets `skip_tls_verify: true`, which disables upstream certificate validation. Do not treat the example config as authenticated — set `skip_tls_verify: false` and supply a proper trust store for that guarantee to hold.
2. **Bogus-NXDOMAIN filtering mitigates response poisoning** — a fixed IP list (`bogus-nxdomain.ips`) is checked against upstream answers, catching ISP/network-level DNS hijacking that redirects NXDOMAIN to an ad or sinkhole page.
3. **EDNS0 Client Subnet is a privacy trade-off, not a security feature** — enabling it leaks a masked portion of the client's IP to the upstream resolver in exchange for geo-aware answers. Leave `edns.enable: false` unless that trade-off is explicitly wanted.
4. **Pooled buffers bound memory under load** — connection pools (`internal/pool`) and the DoH read-buffer `sync.Pool` cap allocation growth during traffic spikes rather than allocating per-query, reducing exposure to memory-exhaustion under query flood.
