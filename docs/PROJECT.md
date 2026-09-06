# DNS-Proxy - Project Information

## Overview

**DNS-Proxy** is a DNSMasq-style DNS proxy/forwarder written in Go, with modern upstream transport support beyond plain UDP/TCP — DNS-over-TLS (DoT) and DNS-over-HTTPS (DoH). It ships as a single `CGO_ENABLED=0` static binary, resolves queries through a local-record → cache → forwarder-rule → upstream chain, and hot-reloads its configuration on `SIGHUP` without dropping in-flight queries.

## Key Features

- **Multiple upstream transports:** plain UDP, plain TCP, DoT, and DoH — selected by `upstream.mode`, with DoH falling back from HTTP/3 to HTTP/2 automatically.
- **Sharded response cache:** power-of-two-sharded LRU keyed by canonical query name + qtype/qclass (plus client ECS subnet when EDNS Client Subnet is enabled), honoring answer TTLs with a configurable floor and a separate, shorter negative-response TTL.
- **Local resolution:** static records, `/etc/hosts`-style file, and `include_files` glob-merged config for managed local zones.
- **Forwarder rules:** per-domain upstream overrides, independent of the global `upstream.mode`, also `include_files`-mergeable.
- **EDNS0 Client Subnet:** optional ECS injection with configurable IPv4/IPv6 mask, for upstreams that use it for geo-aware answers. Enabling it also partitions the response cache by client subnet, so effective entries-per-name multiply by the number of distinct subnets seen — factor this into `cache.size` when turning it on.
- **Bogus-NXDOMAIN filtering:** strips known ISP-hijack/sinkhole IPs from upstream answers when enabled.
- **Zero-downtime reload:** `SIGHUP` rebuilds the entire runtime and swaps it in atomically — no lock held across upstream I/O, no dropped in-flight queries. The superseded runtime's connection pools and DoH transports are explicitly closed after the swap, so a reload doesn't leak connections.

## Security Model

1. **DoT/DoH encrypts the upstream hop** — queries between dns-proxy and its configured upstream are not visible on the wire in plaintext, unlike `udp`/`tcp` mode. `skip_tls_verify` defaults to `false`, so upstream certificates are validated against the system trust store by default; set it to `true` only to knowingly disable that validation (e.g. a self-signed upstream in a lab) — an on-path attacker can otherwise MITM the "encrypted" hop transparently. When `skip_tls_verify` is `true` with mode `dot`/`doh`, dns-proxy logs an operator-visible `WARNING` at startup and on every reload, so the weakened posture can't go unnoticed.
2. **Bogus-NXDOMAIN filtering mitigates response poisoning** — a fixed IP list (`bogus-nxdomain.ips`) is checked against upstream answers, catching ISP/network-level DNS hijacking that redirects NXDOMAIN to an ad or sinkhole page.
3. **EDNS0 Client Subnet is a privacy trade-off, not a security feature** — enabling it leaks a masked portion of the client's IP to the upstream resolver in exchange for geo-aware answers. Leave `edns.enable: false` unless that trade-off is explicitly wanted.
4. **Pooled buffers bound memory under load** — connection pools (`internal/pool`) and the DoH read-buffer `sync.Pool` cap allocation growth during traffic spikes rather than allocating per-query, reducing exposure to memory-exhaustion under query flood.
5. **Upstream transaction-ID verification** — `udp`/`tcp`/`dot` responses whose ID doesn't match the outgoing query are rejected and their connection discarded rather than returned to the pool. Without this, a stale datagram left on a reused pooled connection would be stamped with the right ID by the reply path and be undetectable to the client — effectively a cache/response-confusion vector local to the pool.
6. **UDP response truncation denies an amplification vector** — replies are truncated (TC bit set) to the querying client's advertised EDNS0 buffer size, captured before EDNS Client Subnet injection so a proxy-added OPT never inflates it, rather than sent oversized — so dns-proxy can't be abused to amplify a spoofed-source UDP query into a larger response.
7. **ECS cache-key separation prevents cross-subnet answer leakage** — when EDNS Client Subnet is enabled, the cache key includes the client's masked subnet, so a geo-specific answer computed for one client subnet is never served to a client in a different subnet from cache.
