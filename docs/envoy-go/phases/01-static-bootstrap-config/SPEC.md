# Phase 01 — Static Bootstrap Config

**Phase id:** `01`
**Slug:** `01-static-bootstrap-config`
**Status:** `in-progress` (SPEC stage)
**Produced by:** `superpowers:brainstorming` (adapted autonomous mode — see `DECISIONS.md` ADR-0004)
**Depends on:** phase 00 (done)
**Differential surface at end of phase:** existing `0000-tcp-echo` fixture green under a unified real-Envoy-bootstrap config (both proxies consume `envoy.config.bootstrap.v3.Bootstrap`-shaped YAML); admin `/ready` byte-exact equivalence added to that fixture.

---

## 1. Purpose

Phase 01 retires phase 00's ad-hoc minimal subject schema and replaces it with a real bootstrap loader that consumes the exact YAML format upstream Envoy accepts. It also lands the first admin-API surface — the `/ready` endpoint — which is the canonical readiness signal every subsequent phase's differential harness will probe.

Concretely, phase 01 produces:

1. A bootstrap loader under `internal/bootstrap/` that parses YAML into `envoy.config.bootstrap.v3.Bootstrap` proto messages (via the `github.com/envoyproxy/go-control-plane` proto types permitted by doctrine `D-3.2`), validates at skeleton depth, and exposes extractor helpers the subject binary uses to drive its listener and upstream wiring.
2. An admin subsystem under `internal/admin/` that serves one HTTP/1.1 endpoint, `GET /ready`, byte-exact-equivalent to upstream Envoy v1.37.2's response in the ready state.
3. A rewired `cmd/envoy-go/main.go` whose configuration source is `internal/bootstrap` (replacing the phase-00 `Endpoint{Address, Port}×2` schema from ADR-0007) and which starts both the admin server and the phase-00 TCP pump from bootstrap-extracted addresses.
4. An extended fixture `0000-tcp-echo`: `envoy-go.yaml` is rewritten from the phase-00 minimal schema to a real Envoy bootstrap YAML; `expectations.yaml` grows applicable dimensions covering the new admin `/ready` differential surface; the driver probes `/ready` on both proxies and the diff byte-compares the responses.
5. A new `BEHAVIOR_CONTRACT.md` subsection, **Admin API — `/ready`**, codifying the equivalence rule with a justifying ADR.
6. The first production fuzz target in the repo, `internal/bootstrap.FuzzBootstrapLoad`, which satisfies phase-done gate (d) for this phase (the first phase to introduce a parser / codec surface — see `BOOTSTRAP_PROMPT.md` §7.4).

After phase 01, the project has proven its second central engineering claim: *envoy-go ingests upstream Envoy's config format and matches Envoy's admin-ready contract byte-for-byte.* Every later phase works inside that claim and stops needing the phase-00 minimal schema.

## 2. Non-purposes

Phase 01 does **not** do any of the following. Each is explicitly deferred to the phase noted; this list exists to keep scope bounded (see `BOOTSTRAP_PROMPT.md` §6.3).

- **Listener manager.** The subject interprets `static_resources.listeners[0]` at skeleton depth only (reads the listener's `address.socket_address` and nothing else) and drives the phase-00 TCP pump from it. No multi-listener support, no filter_chain dispatch, no per-listener lifecycle. → phase 02.
- **Cluster manager.** The subject reads `static_resources.clusters[0].load_assignment.endpoints[0].lb_endpoints[0].endpoint.address` and uses it as the phase-00 pump's fixed upstream. No load balancing, no DNS resolver, no cluster-type dispatch. → phase 02.
- **Filter chain engine.** `filter_chains[].filters[]` fields are parsed into the proto but their `typed_config` (Any) contents are neither resolved nor dispatched. → phase 02 (TCP proxy filter) / phase 07 (framework).
- **TLS, HTTP/*, HTTP/2, HTTP/3.** Not touched. → phases 03–05.
- **Access log, stats, Prometheus.** Not touched. → phase 06.
- **Admin endpoints other than `/ready`.** `config_dump`, `stats`, `clusters`, `listeners`, `server_info`, drain, hot-restart — none of these are implemented. → phase 08.
- **Dynamic resources / xDS.** `dynamic_resources.{ads_config, cds_config, lds_config}`, `layered_runtime`, `node`'s `id`/`cluster` semantics beyond echo — not implemented. Presence of `dynamic_resources` at load time causes a clean parse error. → xDS family.
- **Runtime layer.** → runtime family.
- **Multiple listeners, multiple clusters, multiple endpoints per cluster.** The extractors are documented as "first-only" and error clearly if the fixture presents more than one. Multi-resource support → phase 02.
- **Pre-initialization state machine for admin `/ready`.** Upstream Envoy returns non-ready responses (e.g., `PRE_INITIALIZING`) during startup; envoy-go's phase-01 admin only serves the ready state. The transient is deferred to the phase that introduces an initialization state machine (expected: xDS family or earlier if filter chain framework forces it). → ADR.
- **`envoy-go.yaml` and `envoy.yaml` unification.** The fixture keeps two files because structural differences remain (subject has no docker bridge, uses loopback addresses, does not need `dns_lookup_family: V4_ONLY`). Unifying to a single rendered template is a later improvement, not phase 01's goal. → deferred, no phase target yet.
- **Graceful drain / SIGTERM handling.** Process exit on SIGINT/SIGKILL without drain; no connection draining. → phase 08.

## 3. Phase-done gates (specialization of §7.5)

Per doctrine `D-3.6`, phase 01 lands only when every gate below is green. The generic `BOOTSTRAP_PROMPT.md` §7.5 gate set is narrowed:

| Gate | Specialization for phase 01 |
|---|---|
| (a) new/changed differential fixtures green | Fixture `test/fixtures/0000-tcp-echo/` passes under its extended `expectations.yaml`: TCP echo byte-exact (phase-00 surface, unchanged) **and** admin `/ready` byte-exact (new surface — status 200, body `LIVE\n`, headers set-equal modulo the `BEHAVIOR_CONTRACT.md` admin allow-list). |
| (b) pre-existing differential fixtures green | The phase-00 TCP echo byte-exact surface on fixture `0000-tcp-echo` remains green with the subject's rewired config pipeline. No regressions. |
| (c) conformance suites at declared threshold | N/A — phase 01 declares threshold 0 (no protocol conformance suites apply; §7.3 suites all land with their respective protocols in phases 05/HTTP-3/gRPC families). |
| (d) new fuzzer clean short-budget run | `internal/bootstrap.FuzzBootstrapLoad` runs clean at the fuzz budget chosen at implementation time (default recommendation: 30s in CI; the planner may adjust and ADR). "Clean" means no panics, no hangs, no resource exhaustion — only structured `error` returns from the loader. |
| (e) `go vet`, `golangci-lint run`, `go test ./...` clean | All three on branch + CI. `go test ./test/differential/...` also green. |
| (f) `REVIEW.md` approved | Per §5 step 5 of the state machine. |

Additional phase-specific exit criteria:

- `docs/envoy-go/BEHAVIOR_CONTRACT.md` contains a populated **Admin API** subsection (not a `_to be filled_` placeholder) with the `/ready` rule and a justifying ADR reference.
- `docs/envoy-go/DECISIONS.md` contains new ADRs for each deferred decision in §10 that the planner settles during implementation (see §10 for the set).
- `docs/envoy-go/ROADMAP.md` row 01 status → `done` on the phase commit.
- `docs/envoy-go/STATE.md` advances `active-phase` to `02-tcp-proxy` with `lifecycle-state: 1` and `next-skill: superpowers:brainstorming`.
- `docs/envoy-go/phases/00-bootstrap/` is not modified by this phase (append-only history); any ADR that supersedes a phase-00 ADR goes in `DECISIONS.md`, not in the phase-00 directory.

## 4. Deliverables (files and directories)

All paths are relative to repo root. Every path listed here is created, modified, or deleted by this phase; no other production paths change.

### 4.1 New files

```
envoy-go/
├── internal/
│   ├── bootstrap/
│   │   ├── bootstrap.go            # Load + extractor helpers
│   │   ├── bootstrap_test.go       # unit tests
│   │   └── fuzz_test.go            # FuzzBootstrapLoad (gate (d))
│   └── admin/
│       ├── admin.go                # /ready server
│       └── admin_test.go           # unit tests
└── docs/envoy-go/phases/01-static-bootstrap-config/
    ├── SPEC.md                     # this file
    ├── PLAN.md                     # produced in the next session
    ├── PROGRESS.md                 # produced during implementation
    └── REVIEW.md                   # produced at review
```

### 4.2 Modified files

```
envoy-go/
├── cmd/envoy-go/
│   ├── main.go                     # rewired: bootstrap.Load + admin.Serve + pump
│   └── main_test.go                # updated for new config source
├── test/
│   ├── differential/
│   │   └── harness.go              # SubjectProxy grows AdminAddr(); see §5.6
│   └── fixtures/0000-tcp-echo/
│       ├── envoy-go.yaml           # rewritten as real Envoy bootstrap (see §5.4)
│       ├── expectations.yaml       # extended: response-status/body/headers applicable (see §5.4)
│       ├── README.md               # updated to reflect phase-01 config pipeline
│       └── driver/driver.go        # ProbeAdmin + SubjectConfig returns real bootstrap (see §5.4)
├── docs/envoy-go/
│   ├── BEHAVIOR_CONTRACT.md        # new "Admin API" subsection
│   ├── DECISIONS.md                # new ADRs for §10 deferred decisions
│   ├── STATE.md                    # advanced to phase 02 on commit
│   └── ROADMAP.md                  # row 01 status → done on commit
├── go.mod                          # add github.com/envoyproxy/go-control-plane + google.golang.org/protobuf (if not transitively present); pinned version per ADR
└── go.sum                          # tidied
```

### 4.3 Deleted files

```
envoy-go/
└── cmd/envoy-go/
    ├── config.go                   # phase-00 minimal schema; superseded by internal/bootstrap (ADR)
    └── config_test.go              # superseded with config.go
```

The deletion is recorded in the phase-01 commit with a superseding-ADR that explicitly names ADR-0007 (phase-00's minimal schema ADR) per `BOOTSTRAP_PROMPT.md` §4.1 invariant 4. ADR-0007 is not edited; it is superseded append-only.

### 4.4 Untouched

Everything else: `.golangci.yml`, `.github/workflows/ci.yml`, `cmd/envoy-go/main.go`'s `netConn`/`halfClose` pump logic, `test/helpers/`, `test/differential/{runner_test.go, diff.go, fixture/}`, `test/fixtures/0000-tcp-echo/envoy.yaml`. Changes to `runner_test.go` and CI are permitted only if §5 requires them; the baseline assumption is they do not (see §5.6 for the harness change scope).

## 5. Architecture and components

### 5.1 Bootstrap loader (`internal/bootstrap/`)

**Purpose:** convert a YAML bootstrap file into an `*envoy_config_bootstrap_v3.Bootstrap` proto message and provide phase-01-scoped extractor helpers for the subject binary.

**Public surface:**

```go
package bootstrap

// Load parses r as YAML, converts to JSON, and unmarshals into the Envoy v3
// Bootstrap proto. The YAML must use the same keys Envoy's YAML loader accepts.
// Unknown fields at the skeleton-depth keys parsed by phase 01 cause an error.
// Phase-01 unsupported features (dynamic_resources, layered_runtime) cause an
// error even though the proto itself defines them.
func Load(r io.Reader) (*bootstrapv3.Bootstrap, error)

// AdminSocket returns the host and port from admin.address.socket_address.
// Errors if admin is missing or the address is not a socket_address.
func AdminSocket(bs *bootstrapv3.Bootstrap) (host string, port uint32, err error)

// FirstListenerSocket returns the host and port of static_resources.listeners[0]
// .address.socket_address. Errors if there are zero or more than one listener,
// or if the listener's address is not a socket_address.
func FirstListenerSocket(bs *bootstrapv3.Bootstrap) (host string, port uint32, err error)

// FirstClusterEndpointSocket returns the host and port of
// static_resources.clusters[0].load_assignment.endpoints[0].lb_endpoints[0]
// .endpoint.address.socket_address. Errors if there are zero or more than one
// cluster/endpoint/lb_endpoint, or if the endpoint address is not a
// socket_address.
func FirstClusterEndpointSocket(bs *bootstrapv3.Bootstrap) (host string, port uint32, err error)
```

**Implementation outline:**

1. `Load` reads r, parses YAML via `gopkg.in/yaml.v3` into a generic `map[string]interface{}` normalized to JSON-compatible shape (string keys, no tagged scalars), marshals to JSON, then `protojson.Unmarshal` into `bootstrapv3.Bootstrap` with `DiscardUnknown: false`. The YAML→JSON pass is necessary because upstream Envoy's YAML conventions (e.g., `{foo: bar}` flow style) map cleanly to JSON and `protojson` is the canonical proto JSON codec.
2. After unmarshal, `Load` rejects the bootstrap if `DynamicResources != nil` or `LayeredRuntime != nil` with an error referencing the specific unsupported field. This is the phase-01 surface guard.
3. Each extractor enforces the "exactly one" invariant for its resource list. `len(listeners) != 1` is an error; same for clusters and nested endpoint collections. The error messages name the fixture constraint, not "TBD": "phase 01: expected exactly one listener, got N" etc.

**Proto type source:** `github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3`. Per doctrine `D-3.2`, this import is permitted as *proto types only*. The phase-01 loader does not import any control-plane helpers, filter helpers, or xDS logic from the go-control-plane package; all parsing, validation, and extraction is written in this repo.

**Validation semantics:**

- Required fields (at phase-01 scope): `admin.address.socket_address`, `static_resources.listeners[0].address.socket_address`, `static_resources.clusters[0].load_assignment.endpoints[0].lb_endpoints[0].endpoint.address.socket_address`. Missing any yields a structured error (not a protojson error leak — the loader wraps).
- Port values 0 are permitted structurally (the harness substitutes them); semantic port validation happens in `cmd/envoy-go/main.go` at bind time (the OS rejects 0 if it reaches bind).
- `node` fields (`id`, `cluster`) are parsed but phase 01 does not enforce their presence or values. The `/ready` response is identical regardless of node contents. ADR-6 below records the deferral.

**Error shape:** every error from `Load` and the extractors is a plain `error` with a human-readable message beginning with `bootstrap: `. No custom error types in phase 01 (YAGNI; phase 08 may introduce structured errors for admin endpoints).

### 5.2 Admin subsystem (`internal/admin/`)

**Purpose:** serve the admin `/ready` endpoint on a configurable address, byte-exact-equivalent to upstream Envoy v1.37.2.

**Public surface:**

```go
package admin

// Server is the admin HTTP/1.1 server. Only /ready is implemented in phase 01;
// other admin endpoints land in phase 08.
type Server struct { /* … */ }

// New returns an admin server bound to addr. The server is not running yet;
// call Start. The Ready gate is initially closed: /ready returns non-200 until
// MarkReady is invoked. The exact non-ready response shape is captured by the
// pre-init decision ADR (see §10); the implementer observes upstream Envoy's
// pre-init response and codifies it, or (per the decision) documents that the
// transient is unobservable by tests and serves 503 with an empty body.
func New(addr string) *Server

// Start begins serving in a goroutine. Returns the bound address (useful when
// addr had port 0). Error only if bind fails.
func (s *Server) Start() (string, error)

// MarkReady flips the /ready gate; subsequent requests receive the ready
// response.
func (s *Server) MarkReady()

// Close performs best-effort shutdown (no graceful drain; phase 01 does not
// implement drain — see phase 08).
func (s *Server) Close() error
```

**`/ready` ready-state response contract (authoritative, codified in `BEHAVIOR_CONTRACT.md` Admin API subsection):**

- Status line: `HTTP/1.1 200 OK`.
- Body: `LIVE\n` (4 ASCII bytes + LF, exact).
- `Content-Type: text/plain`.
- `Content-Length: 5`.
- `Server: envoy` (phase 01 sets this literal value; see ADR recommendation in §10 item 3).
- `Date:` present; value covered by the differential allow-list (non-deterministic).
- `Cache-Control:` and `X-Envoy-*` headers: the phase-01 implementer observes the exact set upstream Envoy v1.37.2 returns and either (a) replicates them in envoy-go's response, or (b) records each divergent header in the `BEHAVIOR_CONTRACT.md` header allow-list under the Admin API subsection. Whichever route the implementer takes, the ADR for this decision names each header explicitly — the spec does not defer this to "observe later."

**Concurrency:** `net/http` serves one goroutine per connection; `/ready` is read-only and stateless after `MarkReady` (an atomic bool). No races.

**HTTP version:** HTTP/1.1 only. HTTP/2 over the admin port is not supported in phase 01; the reference Envoy defaults to HTTP/1.1 on admin too (ALPN is not offered). If empirical observation shows upstream exposes h2c, the planner raises this as a finding and ADRs the decision.

**Lifecycle (in `cmd/envoy-go/main.go`):**

1. Parse bootstrap.
2. `admin.New(adminAddr)`; `admin.Server.Start()` — bound but not ready.
3. Bind TCP listener on `listenerAddr`.
4. Dial `upstreamAddr` once as a sanity check? **No** — fail-fast on upstream is not phase-01's contract; the phase-00 pump lazy-dials per accepted connection. Same behavior retained.
5. `admin.Server.MarkReady()`.
6. Print `envoy-go ready on <listenerAddr>` to stdout (harness readiness sentinel; unchanged from phase 00, see §5.6).
7. Accept loop.

### 5.3 Subject binary rewiring (`cmd/envoy-go/main.go`)

The phase-00 TCP pump logic (`pump`, `halfClose`, `netConn`) is preserved verbatim — its behavior is the phase-00 differential surface and must stay green under gate (b). The config-loading layer is replaced:

- Before: `loadConfig(f)` returns a `*Config{Listener Endpoint, Upstream Endpoint}` via the phase-00 minimal schema.
- After: `bootstrap.Load(f)` returns `*bootstrapv3.Bootstrap`; `cmd/envoy-go/main.go` calls `bootstrap.AdminSocket`, `bootstrap.FirstListenerSocket`, and `bootstrap.FirstClusterEndpointSocket` to get the three `host:port` tuples it needs.

The `-c <path>` flag, stdout ready sentinel format, and `log.Fatalf` exit-1-on-config-error style are unchanged. The `package main` documentation comment is updated to reference `internal/bootstrap` and a new ADR that supersedes ADR-0007.

### 5.4 Fixture `0000-tcp-echo` evolution

The fixture stays two-file (see §2 for why unification is deferred). Concrete changes:

**`envoy.yaml` (reference):** unchanged. Already a real Envoy bootstrap. Phase 01 does not touch it.

**`envoy-go.yaml` (subject):** rewritten from the phase-00 minimal schema to a real Envoy bootstrap YAML. Sketch (ports are placeholders substituted by the harness):

```yaml
node:
  id: envoy-go-subject-0000
  cluster: envoy-go-differential
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }   # harness-allocated
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }   # harness-allocated
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }   # harness-allocated (backend)
```

**Structural differences from the reference `envoy.yaml`** (each justified; none are arbitrary):

1. **Cluster type `STATIC` vs reference's `STRICT_DNS`.** Subject reaches the backend via literal IP; no DNS. Reference reaches via `host.docker.internal` which requires `STRICT_DNS`. The phase-01 subject implements neither `STATIC` nor `STRICT_DNS` cluster semantics (that's phase 02); the declaration is informational for the parser and for a future cluster-manager integration test. Both declarations pass the phase-01 extractor because it only reads the endpoint's `socket_address`.
2. **No `dns_lookup_family` on the subject cluster.** Subject has no DNS resolver in phase 01. Reference keeps `V4_ONLY` per ADR-0010. The phase-01 bootstrap loader does not enforce presence of `dns_lookup_family`; it is parsed into the proto and ignored.
3. **Addresses `127.0.0.1` on the subject.** Subject runs as a subprocess on the host; no docker bridge. Reference uses `0.0.0.0` for bind and `host.docker.internal` for egress per phase-00 and ADR-0010.

Each of these three divergences is recorded inline in `envoy-go.yaml` as a comment block (per doctrine `D-3.4` context isolation: a stranger reading the fixture must understand the differences). The driver's `SubjectConfig` builds this YAML from a template with the three ports interpolated.

**`expectations.yaml`:** the existing file is extended. The three dimensions that were `applicable: false` with reason "pure TCP fixture — no HTTP layer" now become applicable because the fixture now exercises both TCP (for echo) and HTTP/1.1 (for admin `/ready`). The dimensions are multi-surface: the fixture's diff treats them as ordered observations. Concrete post-phase-01 shape:

```yaml
dimensions:
  response-status:
    applicable: true
    rule: exact
    scope: admin-/ready
  response-body:
    applicable: true
    rule: byte-exact
    scope: tcp-echo + admin-/ready
  response-headers:
    applicable: true
    rule: set-equal-modulo-allow-list
    scope: admin-/ready
    allow-list: BEHAVIOR_CONTRACT.md § "Admin API — /ready"
  response-trailers:
    applicable: false
    reason: no trailers on /ready; no HTTP layer on TCP path
  http2-http3-framing:
    applicable: false
    reason: admin is HTTP/1.1; TCP has no framing
  access-log:
    applicable: false
    reason: phase 01 does not emit access logs (phase 06)
  stats:
    applicable: false
    reason: phase 01 does not emit stats (phase 06)
  xds:
    applicable: false
    reason: static config; no xDS
  timing:
    applicable: false
    reason: not opt-in
```

**`README.md`:** two-paragraph update. First paragraph explains the phase-01 addition (now exercises admin `/ready` alongside TCP echo). Second paragraph references the three divergences in §5.4 between `envoy.yaml` and `envoy-go.yaml` and why each is benign.

**`driver/driver.go`:** three changes.

1. `SubjectConfig(refListenerPort, subjListenerPort, backendPort int) string` is replaced by a new signature that also takes `subjAdminPort int`. The returned YAML is the real-bootstrap template above with four port values interpolated. The phase-00 minimal-schema `fmt.Sprintf` goes away.
2. A new method `ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error)` executes `GET /ready` against each admin address and returns the full HTTP responses (status line + headers + body) as byte streams. The response bytes are what the fixture's `response-status`, `response-body`, and `response-headers` diff rules operate on; the harness chooses how to split status/body/headers (phase-01 choice documented in §5.6).
3. The driver is registered once via the existing `init()`; no registry changes.

### 5.5 `BEHAVIOR_CONTRACT.md` — new "Admin API" subsection

The subsection is introduced under a new top-level heading `## Admin API`, structurally parallel to `## Test harness host networking`. It defines:

- **`/ready` equivalence (introduced by phase 01, justified by ADR-NNNN — number assigned at landing time per §4.1 invariant 4).**
  - **Ready-state response (authoritative):** status `200 OK`; body `LIVE\n` (5 bytes including LF); `Content-Type: text/plain`; `Content-Length: 5`. Byte-exact on body; exact on status.
  - **Headers (required equivalence):** `Content-Type` and `Content-Length` exact. `Server: envoy` exact on envoy-go's response. `Date` present on both; value is on the admin header allow-list (non-deterministic).
  - **Admin header allow-list (phase 01 entries):** `Date` — value allowed to differ (non-deterministic). Any additional header the implementer observes upstream v1.37.2 emit on `/ready` is either replicated in envoy-go's response or added to this allow-list with an entry in the `## Header allow-list` section above (linking back here).
  - **Applies to:** ready-state responses only. Pre-init responses are not contracted by phase 01 — the planner records the decision as an ADR per §10 item 4.
  - **Does not yet apply to:** HTTP/2 over admin (phase 01 is HTTP/1.1 only); additional admin endpoints (phase 08).

The phase-01 implementer is responsible for running upstream Envoy v1.37.2 once, issuing `curl -i http://<admin>/ready`, capturing the raw wire response, and reconciling this subsection against the observed bytes before the fuzz and differential gates run. If reality differs from the authoritative response above (e.g., body is not literally `LIVE\n`, or an extra header is always present), the implementer updates this subsection via the same ADR rather than shipping a divergent implementation.

### 5.6 Harness changes (`test/differential/harness.go`)

**Goal: minimize the delta.** The phase-00 harness already orchestrates reference and subject proxies, allocates a listener port for the subject, and substitutes ports into fixture YAML. Phase 01 adds admin port allocation and exposure.

Concrete changes:

1. **`SubjectProxy` gains an `adminAddr string` field and `AdminAddr() string` getter.** The subject config now contains `admin.address.socket_address`; the harness allocates a free TCP port for it (same `freeTCPPort(t)` helper the phase-00 listener uses), interpolates it into the subject's `envoy-go.yaml`, and records the `host:port` on the `SubjectProxy`. No changes to `StartSubjectProxy`'s signature except `cfg` now embeds the admin port; the harness wraps this.

2. **Reference admin address is already exposed** via `ReferenceProxy.AdminAddr()` (wired in phase 00). Phase 01 uses it as-is.

3. **Readiness detection:** unchanged from phase 00. The subject still writes `envoy-go ready on <listener>` to stdout after `MarkReady()`; the harness still scans stdout. The admin `/ready` endpoint is a *differential surface*, not the harness's readiness signal. Rationale: keeping the harness's readiness detection orthogonal to the surface under test avoids circular reasoning ("the harness couldn't detect readiness because the surface we're testing is broken"). A future phase may switch to `/ready`-based harness detection; ADR required if so.

4. **Driver invocation:** the runner calls `d.Drive(ctx, refAddr, subjAddr)` today. Phase 01 adds a second call `d.ProbeAdmin(ctx, refAdminAddr, subjAdminAddr)` and the runner feeds both results into the diff. The diff's existing `CompareBytes` helper is reused; the fixture's `expectations.yaml` tells the diff which dimensions apply per observation.

   Alternative: extend `Drive` to return a structured `ObservationSet`. The planner may prefer this and ADR the choice. The spec-level requirement is: **the runner must produce two independent byte-pair observations per fixture — one for the TCP echo stream, one for the admin `/ready` response — and both must diff green for the fixture to pass.** How the driver API exposes these is an implementation detail.

5. **No changes to `StartReferenceProxy`.** The reference admin port is already exposed.

The harness does not become "HTTP aware" in the sense of understanding framing; the admin response bytes are diffed as opaque byte streams. That is sufficient for byte-exact body equivalence and set-equal headers via a small HTTP response parser the diff subsystem can apply. The planner decides whether that parser lives in `test/differential/` or in a small helper under `test/helpers/`; ADR if cross-cutting.

## 6. Data flow

End-to-end for the extended `0000-tcp-echo` fixture:

```
  ┌──────────────┐   ping payload            ┌─────────────────────┐
  │   driver     │ ────────────────────────► │ upstream Envoy       │ ──► echo backend
  │  (Drive)     │                           │ (testcontainers)     │ ◄──
  └──────────────┘ ◄────── echo bytes ────── └─────────────────────┘

  ┌──────────────┐   ping payload            ┌─────────────────────┐
  │   driver     │ ────────────────────────► │    envoy-go          │ ──► echo backend
  │  (Drive)     │                           │    (subprocess)      │ ◄──
  └──────────────┘ ◄────── echo bytes ────── └─────────────────────┘

  ┌──────────────┐   GET /ready              ┌─────────────────────┐
  │   driver     │ ────────────────────────► │ upstream Envoy admin │
  │ (ProbeAdmin) │ ◄── 200 LIVE\n + hdrs ─── └─────────────────────┘
  └──────────────┘

  ┌──────────────┐   GET /ready              ┌─────────────────────┐
  │   driver     │ ────────────────────────► │ envoy-go admin       │
  │ (ProbeAdmin) │ ◄── 200 LIVE\n + hdrs ─── └─────────────────────┘
  └──────────────┘

  ┌──────────────┐
  │    diff      │  byte-exact TCP echo + byte-exact admin body +
  │  (runner)    │  set-equal admin headers (allow-listed per contract)
  └──────────────┘
```

The two surfaces are independent observations. The diff passes iff both pass.

## 7. Error handling and failure modes

| Failure | Handling |
|---|---|
| Bootstrap YAML parse error (subject) | `cmd/envoy-go/main.go` logs `bootstrap: parse: <err>` and exits 1. Harness's `StartSubjectProxy` observes the non-zero exit before the ready sentinel, captures stderr, and returns a descriptive error to the test. |
| Missing required bootstrap field (subject) | Same as parse error; loader returns a `bootstrap: missing admin.address.socket_address` (or equivalent) error. |
| `dynamic_resources` or `layered_runtime` present | Loader rejects at Load time with `bootstrap: dynamic_resources not supported in phase 01`; subject exits 1. |
| More than one listener / cluster / endpoint | Extractor returns `phase 01: expected exactly one listener, got N`; subject exits 1. |
| Admin port bind failure | `admin.Server.Start()` returns error; subject exits 1; harness sees non-zero exit. |
| Listener port bind failure | `net.Listen` fails; main exits 1. (Unchanged from phase 00.) |
| Admin `/ready` returns non-200 or wrong body | `ProbeAdmin` returns response bytes; the diff reports a divergence; fixture fails with a hex-dump window (existing `CompareBytes` formatter from phase 00). |
| Admin `/ready` times out on either proxy | HTTP client's context deadline; driver returns `admin probe: context deadline exceeded`. Treated as a fixture failure, not a flake. |
| Fuzz target `FuzzBootstrapLoad` panics | Fuzz engine records the crashing input; CI fails. The loader's invariant is "no panic on any bytes; only structured errors." A panic is a bug to fix, not to retry. |

Non-goals for error handling (same as phase 00): no structured error types; no error wrapping beyond `fmt.Errorf("%w", ...)`; no metrics/alerts (no stats subsystem yet).

## 8. Testing scope for phase 01

Phase 01 produces:

- **Unit tests** for `internal/bootstrap`:
  - Happy path: the fixture's `envoy-go.yaml` template (with stable port values) round-trips through `Load` into a `*bootstrapv3.Bootstrap` and the three extractors return the expected tuples.
  - Error paths: missing `admin`, missing listener, missing cluster, missing endpoint; `dynamic_resources` present; two listeners; two clusters; two endpoints; YAML syntax error; unknown top-level field.
  - Subtle: `port_value: 0` is accepted structurally (harness substitutes later).
- **Unit tests** for `internal/admin`:
  - `/ready` before `MarkReady`: returns non-200 per the pre-init decision ADR (§10 item 4). Body and status match the ADR's contract.
  - `/ready` after `MarkReady`: returns 200 with body `LIVE\n`, `Content-Type: text/plain`, `Content-Length: 5`, `Server: envoy`.
  - Concurrent requests after `MarkReady`: all return the same response; no data races under `go test -race`.
  - `Close` is idempotent and returns cleanly when the server was never started and when it was running.
- **Unit test** for `cmd/envoy-go/main.go`: either the existing `main_test.go` is updated to exercise the new bootstrap-based startup path, or it is converted into a subprocess-based integration test invoking the built binary. Planner decision; ADR if the choice is non-obvious.
- **Fuzz target** `internal/bootstrap/fuzz_test.go` with function `FuzzBootstrapLoad`:
  - Seed corpus: the fixture's `envoy-go.yaml` and `envoy.yaml`, plus 3–5 hand-crafted degenerate inputs (empty, only whitespace, deeply nested, malformed UTF-8).
  - Invariant: `Load` never panics; it returns either `(*Bootstrap, nil)` or `(nil, err)`.
  - CI budget: default 30s (planner may adjust and ADR). Longer budget nightly (out of phase scope; scheduled by the phase that introduces the nightly CI lane, if different).
- **Differential test**: the existing `go test ./test/differential/...` continues to exercise fixture `0000-tcp-echo`. The harness changes in §5.6 wire the admin probe into the runner; the diff asserts both surfaces green.

No conformance suites run. No HTTP/2, HTTP/3, gRPC conformance driver scaffolding is added (those remain out-of-scope for phase 01 per §2).

`go test -short ./...` skips the differential (existing `testing.Short()` gate) but does **not** skip the unit tests or the fuzz target's seed-only replay. The fuzz target's evolving-corpus run is only invoked under the separate fuzz job the planner wires per gate (d).

## 9. Out-of-scope (explicitly deferred)

| Feature | Lands in |
|---|---|
| Multiple listeners / clusters / endpoints | Phase 02 |
| Filter chain typed_config dispatch (TCP proxy) | Phase 02 |
| Real cluster type dispatch (STATIC, STRICT_DNS, LOGICAL_DNS, EDS) | Phase 02 and xDS family |
| Round-robin / other load balancing | Phase 02 |
| TLS bootstrap fields (TransportSockets) | Phase 03 |
| HTTP connection manager | Phase 04 |
| HTTP/2 (downstream or upstream) | Phase 05 |
| Access log bootstrap integration | Phase 06 |
| Stats bootstrap integration | Phase 06 |
| Admin endpoints beyond `/ready` | Phase 08 |
| Pre-init admin state machine | Later (xDS or filter-chain framework, whichever introduces init gates first) |
| Graceful drain | Phase 08 |
| `dynamic_resources`, xDS wire | xDS family |
| `layered_runtime`, RTDS consumption | Runtime family |
| Hot restart | Runtime family |
| `envoy-go.yaml` ↔ `envoy.yaml` unification | Not scheduled; deferred pending a driver of the unification (e.g., when structural differences vanish in later phases) |

If reality during implementation pushes phase 01 toward any of these, the planner must either (a) re-scope the fixture or the subject to stay within phase 01 and write an ADR explaining the choice, or (b) split phase 01 into `01.1`, `01.2`, … per `BOOTSTRAP_PROMPT.md` §6.

## 10. Deferred decisions (the planner / implementer settles these)

Intentionally not decided at SPEC time. The PLAN (next session) settles each, records an ADR where the decision has cross-phase impact, and proceeds.

1. **YAML→proto pipeline exact shape.** The recommended three-stage pipeline is YAML → JSON → `protojson.Unmarshal`. Alternatives: a direct YAML-to-proto library (`sigs.k8s.io/yaml`, `ghodss/yaml`, `gopkg.in/yaml.v3` with custom proto reflection). The planner picks and ADRs. Criterion: must handle Envoy bootstrap YAML's mix of flow-style objects, `@type` Any tags, and bytes/duration types correctly.
2. **`github.com/envoyproxy/go-control-plane` version pin.** The planner picks a concrete version (current recommendation: latest tag at planning time, as long as it includes `envoy.config.bootstrap.v3`; `v0.13.x` is representative). ADR pins it, just as ADR-0008 pins Envoy. Refresh is its own future phase.
3. **`Server:` header value on envoy-go admin responses.** Recommendation: literal string `envoy` (matches upstream byte-exact; the identity header value is part of what a downstream consumer observes). Alternative: `envoy-go` plus an allow-list entry for that header. The planner picks and ADRs. Rationale the planner must address: does any phase-01 or later consumer of admin responses encode logic against this header value?
4. **Pre-init admin response contract.** Upstream Envoy's pre-init `/ready` response shape on v1.37.2 must be observed empirically during implementation. The planner then decides: (a) match it byte-for-byte in envoy-go; or (b) declare pre-init unobservable by the phase-01 differential test (subject never exposes the pre-init window to the harness because `MarkReady` fires before the stdout sentinel), and serve a documented but test-irrelevant pre-init response. ADR records the choice; `BEHAVIOR_CONTRACT.md` Admin API subsection is extended accordingly.
5. **Unknown-field handling.** `protojson.UnmarshalOptions.DiscardUnknown` is `false` by recommendation — reject unknown top-level and nested fields at phase-01 scope, so fixture authors get immediate feedback. Exception: `typed_config` fields of type `google.protobuf.Any` legitimately carry implementation-specific bytes and the loader must preserve them without resolving. The planner ADRs if any nuance (e.g., "accept unknown fields under `static_resources.listeners[].*` for forward-compat") is needed.
6. **Node field semantics.** Phase 01 parses `node` but does not enforce any value. The planner may choose to (a) keep the parsed proto as-is and ignore `node` entirely in phase 01, or (b) require `node.id` and `node.cluster` to be non-empty strings at load time (defensive; catches user typos early). Recommendation: (a), YAGNI. ADR records the choice.
7. **Fuzz budget for gate (d).** Default recommendation: 30 seconds per CI run. The planner may adjust and ADR. Budget must be short enough to not dominate CI wall-clock.
8. **Admin response parser location.** The diff needs to split an HTTP response into status-line / headers / body to apply the three dimensions separately. Helper location options: `test/differential/` (harness-local), `test/helpers/` (shared helper), or inline in the driver (if the fixture is the only consumer). Recommendation: `test/helpers/` under a new `http_response.go` — anticipating fixtures 0002+ will probe HTTP surfaces. ADR if the planner disagrees.
9. **Main_test.go rewrite vs replacement.** Either rewrite the existing unit test to exercise the new bootstrap-based startup, or replace with a subprocess-based integration test. Recommendation: rewrite (keeps cmd-level unit coverage lightweight). ADR if replaced.

Per doctrine `D-3.5`, none of these require human consultation; each is a standard engineering call the planner resolves and records.

## 11. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Phase-00 differential surface breaks during the main.go rewrite | The phase-00 TCP pump code path (`pump`, `halfClose`, `netConn`) is preserved byte-for-byte; only the config source changes. Gate (b) (pre-existing fixtures green) catches any regression before REVIEW. |
| `go-control-plane` proto import pulls in large transitive dependency tree | Acceptable cost for proto types — the ADR-pinned version is vendored via Go modules; no runtime control-plane code imported. CI build time impact is measurable and bounded. |
| Envoy v1.37.2's `/ready` ready-state bytes differ from the spec's authoritative contract (e.g., body is not literally `LIVE\n`) | The implementer verifies empirically before writing the admin server; any divergence causes a spec update via ADR, not a shipped divergent envoy-go. |
| `protojson` strict mode rejects `@type` handling around `typed_config` | The loader configures `protojson.UnmarshalOptions` to permit `Any` resolution; the typed_config bytes pass through untouched in phase 01 (not resolved). The `internal/bootstrap` unit tests lock this in. |
| Fuzz target finds a panic late in the phase | Landing without gate (d) green is forbidden. If fuzz finds a bug, the planner fixes it before proceeding; there is no deferral path. |
| Fixture `envoy-go.yaml` interpolation diverges between test runs (non-deterministic ports in the compared observation) | Admin response bodies do not include port values; status lines do not include headers. The YAML itself is re-rendered per run but is not part of the diff. Listener/backend ports are substituted into the YAML, never into the diffed observations. |
| Admin response's `Date` header fails the allow-list because the differential harness compares it too strictly | The Admin API subsection in `BEHAVIOR_CONTRACT.md` explicitly allow-lists `Date`. The diff helper reads the allow-list from `expectations.yaml` → `BEHAVIOR_CONTRACT.md` reference. Missing allow-list wiring would be caught by the spec-document-reviewer subagent loop. |
| `internal/bootstrap` error messages leak `protojson` internals, becoming unstable across go-control-plane bumps | Every error is wrapped to start with `bootstrap: ` and uses a bounded vocabulary documented in `bootstrap.go`. Unit tests assert on the prefix, not the full message. |

## 12. Acceptance checklist (for the reviewer of this phase's final state)

When phase 01's REVIEW is written, the reviewer confirms:

- [ ] All §4.1 paths exist with the described contents.
- [ ] All §4.2 files reflect the described modifications.
- [ ] All §4.3 files are deleted in the phase commit.
- [ ] `internal/bootstrap` parses the fixture's `envoy-go.yaml` and `envoy.yaml` without error, and all three extractors return the expected tuples.
- [ ] `internal/bootstrap` rejects the error inputs listed in §8 with `bootstrap: ` prefixed errors.
- [ ] `internal/admin` serves `/ready` with the authoritative response in §5.2 / BEHAVIOR_CONTRACT Admin API subsection, byte-exact on the ready path.
- [ ] `cmd/envoy-go` starts from the fixture's `envoy-go.yaml`, binds admin + listener, marks admin ready, and prints the readiness sentinel in that order.
- [ ] Fixture `0000-tcp-echo` is green under extended `expectations.yaml`: TCP byte-exact echo + admin `/ready` byte-exact body, exact status, set-equal headers modulo allow-list.
- [ ] Phase-00 TCP echo surface remains green under the rewired subject (gate (b)).
- [ ] `go vet ./...`, `golangci-lint run`, `go test ./...`, `go test ./test/differential/...` all green locally and on CI.
- [ ] `FuzzBootstrapLoad` runs clean at the chosen budget; no panics; no hangs.
- [ ] `BEHAVIOR_CONTRACT.md` has a populated **Admin API** section with a `/ready` rule and the justifying ADR reference.
- [ ] ADRs for every settled §10 decision are landed in `DECISIONS.md` (each as a new `ADR-NNNN`; none edit prior landed ADRs; ADR-0007 is superseded by an ADR that explicitly names it).
- [ ] `STATE.md` advances to phase 02 and `ROADMAP.md` row 01 is set to `done`.
- [ ] `PROGRESS.md` contains a full log and the gate outputs from §3 quoted verbatim (per state machine §4).
- [ ] `REVIEW.md` is approved.
