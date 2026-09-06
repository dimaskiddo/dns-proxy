# DNS-Proxy — Agent Instructions

DNS proxy/forwarder in Go with UDP/TCP/DoT/DoH upstream support, sharded caching, and local/forwarder resolution. Single static binary (`CGO_ENABLED=0`). Never guess DNS protocol or config semantics — ask when ambiguous.

---

## Workflow Rules

1. Read `TASKS.md` before every session to orient to current state.
2. Never rework items marked `[x]` in `TASKS.md` unless explicitly instructed.
3. Update `TASKS.md` immediately after completing a task.
4. Never attempt to write the entire codebase in a single response.

## Skills & Caveman Mode

- **GLOBAL:** All prompts processed as if `"Use caveman mode full"` is injected.
- Before ANY coding task, invoke and read: `using-superpowers`, `karpathy-guidelines`, `caveman`.
- Use `using-superpowers` to route to other relevant skills per task.

---

## Architecture

| Component | Role |
|---|---|
| **Server** | Request handling + hot reload — `atomic.Pointer[Runtime]` swap, no lock held during upstream I/O |
| **Upstream** | One `Client`, four transports (udp/tcp/dot/doh) selected by `upstream.mode` — DoH does H3→H2 fallback |
| **Pool** | Generic `*dns.Conn` channel pool — TCP/UDP/DoT share `Get`/`Return`, differ only by injected `Dial` func. `Cap()`/`Close()` bound retries and drain on reload |
| **Cache** | Sharded LRU (power-of-two shard count, FNV hash of canonical name+qtype/qclass+ECS suffix), TTL from answer records, separate negative-response TTL |
| **Resolver** | Local (hosts file + static + wildcard), Forwarder (longest-suffix domain match), EDNS0 Client Subnet, Bogus NXDOMAIN filter |
| **Config** | YAML via viper. `include_files` glob-merge is hand-rolled (`yaml.v3`) — not a viper feature |
| **Util** | Socket buffer/keepalive tuning, build-tagged (`socket_unix.go` / `socket_windows.go`) |

## CLI

```
dns-proxy [-config|--config <path>]   # Start the DNS proxy (UDP + TCP listeners, SIGHUP hot-reload)
dns-proxy -version | --version | version   # Print version + commit hash
```

Root command starts the server directly — no subcommand required, matching the
pre-cobra production binary. `run` still exists as a hidden alias for anything scripted
against the interim `dns-proxy run --config ...` form. `-config`/`-version` (single-dash)
are normalized to `--config`/`--version` in `main.go` before cobra parses — production
predates the cobra migration and still invokes the binary with stdlib `flag` syntax,
which pflag would otherwise read as a shorthand cluster and reject. Default config path
`./dns-proxy.yaml`, resolved relative to CWD.

---

## Critical Constraints

### Build — Pure Go
- `CGO_ENABLED=0` always. No C/C++ toolchains. No `hack/` build wrappers.

### DNS Protocol
- All message parsing/serialization via `miekg/dns`. Never hand-roll wire format.
- Always guard `len(r.Question) == 0` before indexing `r.Question[0]` — malformed queries reach the handler.

### Upstream Modes
- `udp`, `tcp`, `dot`, `doh` — one at a time, set by `upstream.mode`. DoT reuses the TCP pool with `tcp-tls`. DoH never touches the pool; it goes through `http.Client` + `hybridRoundTripper`.
- Any other mode value is rejected at config load/reload (`config.Manager.Reload`); a second guard in the handler returns SERVFAIL rather than nil-deref'ing if an unrecognized mode ever reaches it.
- Per-query forwarder overrides (`forwarder.rules`) always go over UDP (`ForwardUDP` with explicit targets), regardless of the configured mode.

### Configuration
- YAML via `spf13/viper`. Every `Config` field needs both `yaml` and `mapstructure` tags — viper unmarshals via mapstructure.
- Defaults live in `config.setDefaultConfig`, applied before `viper.Unmarshal` overwrites them.
- `local.include_files` / `forwarder.include_files` glob patterns merge via `config.mergeIncludeFiles`, tested in `internal/config/manager_test.go` alongside `upstream.mode` validation. Other covered paths: `cmd/dns-proxy/args_test.go` (flag normalization), `internal/cache/cache_test.go` (key derivation/TTL), `internal/resolver/resolver_test.go` (case-insensitive matching), `internal/server/handler_test.go` (`replyTo` Rcode preservation). Touch these carefully.
- Default path: `--config` flag, relative to **CWD** (not `os.Executable()`).

### CLI
- `spf13/cobra`. Root command's `RunE` starts the server directly — no subcommand needed. `run` (hidden) and `version` are the only subcommands, kept for compatibility with the interim cobra-only invocation form.
- `main.go`'s `normalizeLegacyFlags` rewrites single-dash long flags (`-config`, `-version`) to double-dash before `rootCmd.Execute()`. Do not remove — production still invokes with the pre-cobra stdlib-`flag` syntax.

### Cross-Platform
- `filepath.Join()` for any filesystem path.
- Socket options split by build tag: `internal/util/socket_unix.go` (`//go:build linux || darwin`), `internal/util/socket_windows.go` (no-op stub).

### Logging
- Stdlib `log` only. No structured logging framework — do not introduce `log/slog` or a rotation library without being asked.

### Context & Concurrency
- No lock held across upstream network I/O — that was a real bug (`configLock.RLock()` blocking SIGHUP reloads behind every in-flight query) fixed by the `Runtime` atomic-swap pattern. Do not reintroduce a mutex on the request hot path.
- Reload builds a full new `Runtime` and swaps it in one atomic op; the old `Runtime.Stop()` only after the swap — it now also calls `Upstream.Close()`, so a reload doesn't leak the superseded pools/DoH transports.

### Memory Efficiency
- `sync.Pool` for DoH read buffers (`upstream.Client.bufPool`). Connection pools (`internal/pool`) reuse `*dns.Conn` instead of dialing per query. `Pool.Close()` is called explicitly on reload teardown so pooled connections don't leak across `Runtime` generations.

### Error Handling
- `fmt.Errorf("context: %w", err)` wrapping. Upstream/dial failures return SERVFAIL to the client, never panic.
- Two hot-path guards exist specifically to prevent panics and must not be removed as "redundant" with config validation: the handler's `default:` case on `upstream.mode` (SERVFAIL instead of nil-deref if an invalid mode ever reaches it), and the `resp == nil` check after a nil-error upstream call (SERVFAIL instead of a nil-pointer dereference on the response).

### No Stubbing
- Every function complete and production-ready. No `// TODO`, no placeholder logic.

### Dependencies
- Stdlib first. Approved third-party: `miekg/dns`, `quic-go` (+ `quic-go/http3`), `cobra`, `viper`, `yaml.v3`, `golang.org/x/sys`.

### Build Artifacts
- Output binary via Makefile (`make build`). `make vendor` regenerates `vendor/` + `go.sum` after any dependency change.

---

## Non-Negotiable Rules

1. **No stubs.** Every file complete, production-ready.
2. **No guessing** on DNS protocol behavior or config semantics. Pause, state ambiguity, ask.
3. **Never auto-run pipeline.** Provide exact command + expected output, wait for user.
4. **No system temp dirs.** Runtime files (cache, config) in configured paths only.
5. **Minimal comments.** Comments describe WHY, never WHAT or HOW.
   - No step-by-step process descriptions, algorithm blueprints, or protection-mechanism detail.
   - No inline comments restating code (`// increment counter` on `counter++`).
   - No section separator comments (`// --- Section ---`).
   - Go doc comments on exported symbols: one line for simple functions, max three for complex ones.
   - If a comment would help a malware author bypass a detection mechanism, delete it.

---

## Directory Tree

```
dns-proxy/
├── cmd/dns-proxy/            # Entry: cobra CLI (main, run, version)
│   └── args_test.go          # Flag normalization test
├── internal/
│   ├── cache/                # Sharded TTL-aware LRU (internal/cache/cache.go)
│   │   └── cache_test.go     # Key derivation / TTL tests
│   ├── config/                # Viper YAML config, include_files glob-merge
│   │   └── manager_test.go   # include_files merge + upstream.mode validation
│   ├── pool/                  # Generic *dns.Conn channel pool
│   ├── resolver/               # Local, Forwarder, EDNS, Bogus NXDOMAIN
│   │   └── resolver_test.go  # Case-insensitive matching
│   ├── server/                 # Runtime + atomic swap, request handler, listeners
│   │   └── handler_test.go   # replyTo Rcode preservation
│   ├── upstream/               # Client: UDP/TCP/DoT/DoH forwarding
│   └── util/                   # Socket options (build-tagged), misc helpers
├── docs/
│   ├── ARCHITECTURE.md       # Module map, component details, request pipeline
│   ├── WORKFLOWS.md          # Startup, query flow, hot-reload, error recovery
│   └── PROJECT.md            # Project overview, features, security model
├── vendor/                   # Vendored dependencies
├── dns-proxy.yaml.example    # Recommended production config profile
├── AGENTS.md                 # Agent instructions (also via symlinks CLAUDE.md, GEMINI.md)
├── TASKS.md                  # Local task tracker — gitignored, not committed
├── Makefile                  # Build targets
├── .goreleaser.yml           # Cross-platform release config
└── Dockerfile                # Multi-stage build (CGO_ENABLED=0)
```

---

## References

| File | Purpose |
|---|---|
| `dns-proxy.yaml.example` | Full annotated config reference |
| `docs/ARCHITECTURE.md` | Module map, component internals, request pipeline |
| `docs/WORKFLOWS.md` | Startup, query flow, hot-reload, error recovery |
| `docs/PROJECT.md` | Project overview, features, security model |
| `TASKS.md` | Current project state — read before every session |
| `Makefile` | Build targets and vendoring |
| `.goreleaser.yml` | Release configuration (darwin/linux/windows × 386/amd64/arm64) |
| `internal/server/` | Runtime construction, atomic hot-swap, request handler, listeners |
| `internal/upstream/` | UDP/TCP/DoT/DoH forwarding client |
| `internal/pool/` | Generic pooled `*dns.Conn` |
| `internal/cache/` | Sharded TTL-aware LRU cache |
| `internal/resolver/` | Local, forwarder, EDNS, bogus NXDOMAIN |
| `internal/config/` | Viper config types, manager, include_files merge |
| `internal/util/` | Socket tuning, build-tagged platform code |
