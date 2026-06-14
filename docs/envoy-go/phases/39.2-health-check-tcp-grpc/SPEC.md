# Phase 39.2 SPEC — `tcp_health_check` (connect-only) + `grpc_health_check` (`grpc.health.v1.Health/Check` over H2): the TCP + gRPC checker codecs reusing the 39.1 substrate — the pre-authorized SECOND leg of the phase-39 by-codec split

> **For agentic workers:** the NEXT lifecycle step is `superpowers:writing-plans` (the 39.2 PLAN; SKILL_ROUTING state 2 → 3). This SPEC is the input to that PLAN. Steps are NOT checkboxes here — the PLAN decomposes §10 into bite-sized TDD tasks. Phase 39 is a flat Upstream-robustness-family row WITH a PRE-AUTHORIZED 39.1/39.2 by-codec split (ADR-0106 + ADR-0045; the phase-39 SPEC §3.0). 39.1 (the keystone + the HTTP checker) LANDED at the phase-39.1 IMPL (squash `6714575`); this SPEC scopes **39.2** (the TCP + gRPC checker codecs — the pre-authorized second leg, the 36.1/36.2 + 38.1/38.2 precedent). Phase 39 keeps the Upstream-robustness family OPEN; 4 candidates remain after 39.2 (outlier detection, circuit breakers, retries + hedging, per-protocol connection pooling).

**Goal:** Land the TWO remaining active-HC checker codecs over the UNCHANGED 39.1 substrate (the host health-state registry + the cluster-level checker runtime + the health-aware LB pick): (1) `tcp_health_check` — a **connect-only** TCP probe (AMEND-TG2: empty `tcp_health_check{}` = a bare TCP connect; success ⇒ healthy, connect-refused ⇒ `failure` + `network_failure`); (2) `grpc_health_check` — a unary `grpc.health.v1.Health/Check` RPC over H2 (AMEND-TG3: `SERVING` ⇒ healthy; `NOT_SERVING`/non-SERVING ⇒ `failure`; a transport/connect failure ⇒ `failure` + `network_failure` — the application-vs-network discriminator). In ONE flat 39.2 leg: the 39.1 `parseHealthChecks` + `healthChecker` generalize from HTTP-only to a **kind-tagged `prober` dispatch** (`httpProber`/`tcpProber`/`grpcProber`), the runtime + transition model + the six-LB health view stay BYTE-STABLE, and the gRPC checker reuses the EXISTING `google.golang.org/grpc v1.70.0` direct dep (`google.golang.org/grpc/health/grpc_health_v1` + the `internal/grpcclient` dial path) — **ZERO new go.mod module** (AMEND-TG1, `go mod tidy -diff` empty). The differential proof is two cross-side poll-to-convergence fixtures (`0067-health-check-tcp` reusing the HTTP backends — NO new BackendKind — and `0068-health-check-grpc` with a +1 gRPC SERVING responder BackendKind), each following the 39.1 `0066` warmup-gate + delta-counter protocol.

**Architecture:** The 39.1 `healthChecker` (`internal/cluster/health.go`) hard-binds `cfg httpHealthCheckCfg` + calls the package-level `probeHTTP`. 39.2 introduces a small `prober` interface — `probe(addr string) (ok, networkFailure bool)` — and three implementations: `httpProber` (the 39.1 `probeHTTP` body, unchanged behavior), `tcpProber` (a `net.Dialer{Timeout}` `Dial("tcp", addr)` then immediate close — connect-only), and `grpcProber` (a `grpc.health.v1.Health/Check` unary call: dial the addr over H2 with insecure transport, `Check(ctx, &HealthCheckRequest{Service: serviceName})`, classify `err != nil` ⇒ network_failure, else `resp.Status == SERVING` ⇒ ok). The common timing/threshold envelope (`interval`/`timeout`/`unhealthy`/`healthy`) lifts out of `httpHealthCheckCfg` into the `healthChecker`/a shared `checkerEnvelope`; the prober holds only codec-specific config. `parseHealthChecks` returns a kind-tagged `[]checkerSpec{ envelope, prober }` (the oneof arm chooses the prober) — the 39.1 `only http_health_check is supported` DEPARTURE-reject is LIFTED. `buildCluster` (`manager.go:322`; the `parseHealthChecks` call at `manager.go:364`) is otherwise UNCHANGED (it already loops `hcCfgs` into `newHealthChecker`); the only new wiring is the gRPC **cluster-must-be-H2** reject (AMEND-TG5 — checked against the cluster's parsed protocol via the existing `extractH2Mode`, `manager.go:582`/called at `:414`). The `clusterHealth`/`hostHealth` registry, the `recordResult` transition model, `probeOnce`/`applyResult`/`run`, `StartHealthChecks`/`Drain`, the +7 stats, and the build-time-injected health view on all six LB constructs are REUSED VERBATIM — 39.2 is a codec-only widening (the thrift-33-over-the-ADR-0230-runtime reuse precedent, INVERTED: 39.1 built the runtime, 39.2 is its second + third consumer).

**Tech stack:** Go 1.26.x / golangci-lint 1.64.8 (ADR-0009); reference Envoy **`envoyproxy/envoy:contrib-v1.37.2`** (ADR-0227, @ `sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`). go-control-plane **`/envoy v1.32.4`** (ADR-0008 — `HealthCheck_TcpHealthCheck` {`send`/`receive`/`proxy_protocol_config`} + `HealthCheck_GrpcHealthCheck` {`service_name`/`authority`/`initial_metadata`} + `HealthCheck_Payload` {`text`/`binary`} already in the pinned module). **`google.golang.org/grpc v1.70.0`** ALREADY a direct dep (imported by `internal/grpcclient`, `internal/filter/http/{extauthz,ratelimit}`) → `google.golang.org/grpc/health/grpc_health_v1` is a subpackage of an existing module; **ZERO new go.mod dep**, `go mod tidy -diff` empty (AMEND-TG1). Reuses `internal/cluster/health.go` (the 39.1 `clusterHealth`/`hostHealth`/`healthChecker`/`probeHTTP`/`parseHealthChecks` — generalized, NOT replaced) + `manager.go` (`buildCluster`/`StartHealthChecks`/`Drain` — UNCHANGED except the gRPC-H2 reject) + the six LB constructs (UNCHANGED — the health view is codec-agnostic) + `internal/grpcclient` (the gRPC dial path — a reuse candidate) + the differential harness (the `0066` poll-to-converge + warmup-gate + delta-counter precedent + the `HTTPEcho` backend) + upstream Envoy v1.37.2 source (`source/common/upstream/health_checker_impl.{h,cc}` [`TcpHealthCheckerImpl` / `GrpcHealthCheckerImpl`] + `source/server/config_validation/server.cc` [the gRPC-H2 + hex-decode rejects]) for the codec pins. ZERO new packages.

**Authored:** 2026-06-14. **Empirical-pin probe date:** 2026-06-14 (parallel-subagent live probes against `contrib-v1.37.2` on a Docker bridge network per `reference_docker_probe_bridge_network`).

---

## 1. Purpose / Mission

Phase 39.2 lands the **TCP + gRPC active-HC checker codecs** — the pre-authorized SECOND leg of the phase-39 by-codec split (the phase-39 BRAINSTORM §1.4/§2.2; the phase-39 SPEC §3.0). Where 39.1 built the family keystone (the host health-state dimension + the cluster-level background runtime + the health-aware LB pick) and exercised it with the HTTP checker, 39.2 adds the two remaining checker mechanisms over that substrate UNCHANGED. It is therefore (i) the FIRST consumers of the 39.1 checker runtime beyond HTTP (the codec-dispatch generalization — `httpProber`/`tcpProber`/`grpcProber`), (ii) the project's FIRST connect-only (payload-free) probe (TCP), (iii) the project's FIRST outbound gRPC RPC issued by the cluster subsystem (the `grpc.health.v1.Health/Check` unary call, reusing the existing `google.golang.org/grpc` dep), and (iv) a CODEC-ONLY widening — the runtime, the transition model, the stat surface, the panic threshold, and the six-LB health view are byte-stable (the framework-zero-touch second-consumer posture, the thrift-33 reuse precedent inverted onto the 39.1 runtime).

This SPEC refines the phase-39 BRAINSTORM (`docs/envoy-go/phases/39-upstream-health-check/BRAINSTORM.md`, §1.4/§2.2 — the pre-authorized 39.1/39.2 by-codec split; §8 the TCP/gRPC deferrals) and the phase-39 SPEC (`docs/envoy-go/phases/39-upstream-health-check/SPEC.md`, AMEND-HC2 [TCP parse-accepts; gRPC needs H2] + AMEND-HC8 [grpc v1.70.0 already a direct dep]) against the AS-BUILT 39.1 substrate (`internal/cluster/health.go` + `manager.go` + the six LB files) + the §11 D-TG1..D-TG7 empirical pins EXECUTED IN-SESSION (parallel-subagent fan-out) against (1) the live contrib reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (a connect-only TCP HC + a SERVING/NOT_SERVING gRPC HC over H2 + a `--mode validate` reject matrix + a `/stats` name-set probe on a docker BRIDGE network per `reference_docker_probe_bridge_network`), (2) go-control-plane `/envoy v1.32.4` bindings, and (3) `google.golang.org/grpc v1.70.0` (the `grpc_health_v1` package + the turnkey `health.NewServer()` for the test backend). It anchors the ADR-0244 §Context DRAFT (§13) and CONSUMES the pre-authorized split (the second leg goes straight to its own SPEC per the 36.1/36.2 + 38.1/38.2 precedent).

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 D-TG1..D-TG7 live probes CONFIRMED the BRAINSTORM's "codec-only widening over the 39.1 substrate" framing and refined six anticipations: (a) TCP connect-only is the right MVP — payload send/receive parse-accepts but is a clean deferral; (b) the gRPC checker reads the `ServingStatus` (NOT_SERVING ≠ unreachable — the `network_failure` discriminator), and the `grpc_health_v1` client is turnkey (no hand-roll → no new fuzzer); (c) BOTH codecs add ZERO new stat surface (the 39.1 `health_check.*`/`membership_*`/`lb_healthy_panic` set is codec-agnostic); (d) the only genuinely-new reject is the gRPC cluster-must-be-H2 config-init check; (e) the dep is already present (ZERO new module). The load-bearing pins, each carried into the relevant §§ below:

- **AMEND-TG1 (D-TG1 — the v1.32.4 TCP/gRPC surface re-pinned; ZERO new dep).** `HealthCheck_TcpHealthCheck`: `Send *HealthCheck_Payload` (field 1 — "empty payloads imply a connect-only health check"), `Receive []*HealthCheck_Payload` (field 2 — fuzzy ordered match), `ProxyProtocolConfig` (field 3, DEFERRED). `HealthCheck_Payload` = oneof {`Text` (hex string), `Binary` (bytes)}. `HealthCheck_GrpcHealthCheck`: `ServiceName string` (field 1), `Authority string` (field 2 — defaults to the cluster name), `InitialMetadata []*HeaderValueOption` (field 3, DEFERRED). `core.HealthStatus` MVP stays HEALTHY/UNHEALTHY (39.1). `grpc.health.v1`: `HealthCheckRequest{Service string}`, `HealthCheckResponse{Status ServingStatus}`, `ServingStatus` {UNKNOWN=0, SERVING=1, NOT_SERVING=2, SERVICE_UNKNOWN=3}, full method `/grpc.health.v1.Health/Check`. **`google.golang.org/grpc v1.70.0` is ALREADY a direct dep** (`internal/grpcclient`/extauthz/ratelimit import it) → `grpc_health_v1` is a subpackage of an existing module; `go mod tidy -diff` → exit 0, EMPTY — **ZERO new go.mod dep**. See §5 / §11.1.
- **AMEND-TG2 (D-TG2 SPEC-BLOCKING — TCP connect-only is the MVP; behaves like the 39.1 HTTP checker; payload DEFERRED).** Empty `tcp_health_check{}` = a bare TCP connect (live probe: `:80` open ⇒ `health_flags::healthy`, `:81` refused ⇒ `/failed_active_hc`). A connect refusal increments BOTH `health_check.failure` AND `health_check.network_failure` (identical classification to the HTTP checker's dial failure — the `tcpProber` returns `(ok=false, networkFailure=true)`). The transition model + thresholds + first-check-immediate are UNCHANGED (the 39.1 `recordResult`). **Payload `send`/`receive` matching is DEFERRED** (the live probe confirmed it parse-accepts text + binary, but fuzzy-ordered receive matching needs an echo backend and is orthogonal to the connect liveness signal) — 39.2 DEPARTURE-rejects a `tcp_health_check` that sets `send`/`receive` (the deferred-feature fail-fast boundary, the 38.2 `cluster_header` precedent; §6). See §3.1 / §11.2.
- **AMEND-TG3 (D-TG3 SPEC-BLOCKING — gRPC reads the ServingStatus; the network-vs-application discriminator; grpc_health_v1 used directly).** A `grpc.health.v1.Health/Check` unary RPC over H2. Live probe: a SERVING backend ⇒ `health_flags::healthy`; a dead/refused port ⇒ `/failed_active_hc` with `failure++` AND `network_failure++`; a reachable backend reporting `NOT_SERVING` (via `service_name`) ⇒ `/failed_active_hc` with `failure++` but **`network_failure` stays 0** — the clean discriminator between "can't connect" (network) and "connected but unhealthy" (application). The `grpcProber` classifies: a transport/RPC error (`err != nil` — connection refused surfaces as `codes.Unavailable`) ⇒ `(false, networkFailure=true)`; a completed `Check` with `resp.Status != SERVING` ⇒ `(false, networkFailure=false)`. `service_name` is SUPPORTED (empty ⇒ the overall `""` service); `authority` + `initial_metadata` are DEFERRED (the MVP issues the Check with the default authority + no extra metadata). The probe uses `google.golang.org/grpc` + `grpc_health_v1` DIRECTLY (the turnkey client; NO hand-rolled wire decoder → NO new fuzzer — §8.3). See §3.2 / §11.3.
- **AMEND-TG4 (D-TG4 — ZERO new stat surface; both codecs reuse the 39.1 set; surface 1132 → 1132).** Live `/stats` scrapes under BOTH a TCP HC and a gRPC HC emit EXACTLY the 39.1 set — `health_check.{attempt,success,failure,network_failure}` (counters) + `health_check.healthy` (gauge) + `membership_healthy` (gauge) + `lb_healthy_panic` (counter) — with NO codec-specific stat name. The checker codec changes the PROBE MECHANISM, not the observable surface. **39.2 adds ZERO new stat names; surface STAYS 1132.** The deferred-feature cluster-wide names (`health_check.{degraded,passive_failure,verify_cluster}`, `membership_{change,degraded,excluded}`) remain recorded departures (39.1 §7). See §7 / §11.4.
- **AMEND-TG5 (D-TG5 — the reject roster; the 39.1 oneof-reject LIFTED; the NEW gRPC-H2 reject is config-init not PGV; the TCP hex distinction).** 39.2 LIFTS the 39.1 `health_check: only http_health_check is supported` DEPARTURE-reject (tcp + grpc now accepted). The genuinely-NEW reject: **`grpc_health_check` on a non-H2 cluster** → reference (config-init, NOT PGV) `<cluster_name> cluster must support HTTP/2 for gRPC healthchecking`; envoy-go authors the byte-stable house equivalent (checked against the cluster's parsed protocol at build). The TCP payload-hex pins (recorded, relevant only if payload is later un-deferred): empty `Payload.text` → PGV `PayloadValidationError.Text: value length must be at least 1 characters`; a NON-hex `Payload.text` → a RUNTIME `invalid hex string '<value>'` (a different error class — do NOT lump them). The 39.1 envelope rejects (interval/timeout/thresholds/the checker oneof required) are UNCHANGED and apply to all codecs. The TCP `send`/`receive` DEPARTURE-reject (AMEND-TG2) is the deferred-feature boundary. See §6 / §11.5.
- **AMEND-TG6 (D-TG6 — two fixtures; +1 BackendKind for gRPC; single flat 39.2 leg; ONE ADR; no new fuzzer).** `0067-health-check-tcp` = a cross-side connect-only TCP HC over {2 LIVE HTTP backends (a bare TCP connect to their port succeeds — `HTTPEcho` REUSED, NO new BackendKind), 1 DEAD unbound port}, 66% healthy (> 50% — asserts FILTERING not panic), poll-to-converge + warmup gate + delta counters (the `0066` protocol). `0068-health-check-grpc` = a cross-side gRPC HC over an H2 cluster {2 LIVE gRPC SERVING responders, 1 DEAD unbound port} — the SERVING responder is a **+1 BackendKind** (an h2c server that answers `grpc.health.v1.Health/Check` ⇒ SERVING AND returns 200 to plain-H2 data-plane requests, so the load-phase filtering assertion holds). BackendKind tail **33 → 34**. The 39.2 production footprint (the `prober` interface + the 3 probers + the kind-tagged parse + the gRPC-H2 reject ≈ **~180–290 prod LoC / ~13–16 tasks**) sits UNDER the ADR-0045 gate (`> ~25 tasks OR > ~1500 LoC`) → **single flat 39.2 leg; NO TCP-vs-gRPC sub-split** (the BRAINSTORM's "may split at its SPEC" → resolved NO). **NO new fuzzer** (the gRPC response is decoded by the `grpc_health_v1` library, not a hand-rolled wire parser; the TCP probe decodes nothing — fuzzers STAY 42; the BRAINSTORM's candidate `FuzzGrpcHealthResponse` is NOT warranted). ONE ADR anticipated (ADR-0244 — the multi-codec checker prober dispatch; §13). See §3.0 / §8 / §11.6.
- **AMEND-TG7 (D-TG7 — no-traffic deactivation; gate on convergence + warmup, not raw `attempt` deltas).** Live probe: with no data-plane traffic the HC counters froze at `attempt:N` after settling host state (the default `no_traffic_interval` 60s back-off). So a differential MUST gate on `membership_healthy` convergence + the warmup-until-K-consecutive-200s pattern (`reference_health_check_propagation_warmup`), NEVER on a monotonically-growing `attempt` delta (which stalls). The per-request counters are asserted as DELTAs over a post-warmup baseline. See §8.1 / §11.7.

### 1.2 ADR continuity + D-disposition at SPEC commit

DECISIONS.md tail at master tip is **ADR-0243** (the health-aware LB pick, ACCEPTED); next-free **ADR-0244**. Per the phase-39.2 routing, the DECISIONS tail **STAYS ADR-0243 at this SPEC** (counts UNCHANGED at the SPEC); the ADR-0244 (the multi-codec checker prober dispatch — the TCP connect-only + gRPC-over-H2 codecs over the 39.1 runtime) §Context draft is anchored in §13 and the full DECISIONS.md entry (§Context + §Decision + §Consequences) lands at the phase-39.2 IMPL per ADR-0044 (DECISIONS tail → ADR-0244; next-free after phase 39.2 ≈ ADR-0245). All seven D-TG pins are RESOLVED this session (§11); the remaining open items are PLAN/IMPL D-questions (§12).

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §11 amendments)

- **TCP payload `send`/`receive` matching** — the fuzzy-ordered receive-payload match (`HealthCheck_Payload` text/binary). The live probe confirmed it parse-accepts, but the connect liveness signal is the MVP; payload matching needs an echo backend and is orthogonal. DEFERRED — a `tcp_health_check` with `send`/`receive` set DEPARTURE-rejects (§6; the fail-fast deferred boundary). `proxy_protocol_config` (field 3) likewise DEFERRED.
- **gRPC `authority` + `initial_metadata`** — the custom `:authority` header (defaults to the cluster name / endpoint addr) + the per-call request metadata (auth headers etc.). DEFERRED — the MVP issues `Check` with the default authority and no extra metadata; `service_name` (field 1) IS supported. (The PLAN confirms silent-ignore vs reject for a set `authority`/`initial_metadata`; anticipated silent-ignore — they are additive and orthogonal to the SERVING check.)
- **The `grpc.health.v1.Health/Watch` streaming variant** — Envoy's active gRPC HC uses unary `Check`, not the streaming `Watch`. `Watch` is out of scope entirely.
- **gRPC non-SERVING-status granularity** — the MVP maps `Status == SERVING` ⇒ healthy and everything else (NOT_SERVING / SERVICE_UNKNOWN / UNKNOWN / a non-OK RPC status) ⇒ unhealthy. The DEGRADED/DRAINING host states stay deferred (39.1 §2); a gRPC `SERVICE_UNKNOWN` is treated as unhealthy (not a distinct state).
- **The rich interval/jitter variants, event logging, HC-specific TLS, `alt_port`, `reuse_connection`, the EDS `health_status` initial-health input, the per-host degraded/draining/timeout states** — all REMAIN deferred from 39.1 (39.1 §2). 39.2 is a codec-only widening; it does NOT revisit the 39.1 deferral set.
- **A new stat surface** — 39.2 adds NO new stats (AMEND-TG4); the codec is observable through the codec-agnostic 39.1 `health_check.*`/`membership_*` set.
- **Reusing the cluster data-plane connection pool for the probe** — both the 39.1 HTTP probe and the 39.2 TCP/gRPC probes use a FRESH connection per probe (`reuse_connection` deferred). The gRPC prober dials its own `grpc.ClientConn` (or reuses `internal/grpcclient`'s dial), NOT the cluster's H2 data-plane pool.

---

## 3. The TCP + gRPC checker codecs over the 39.1 substrate (ADR-0244)

### 3.0 Split disposition — D-TG6 RESOLVED (single flat 39.2 leg)

ADR-0045 split-gate fires at `> ~25 tasks OR > ~1500 production LoC`. 39.2 is the pre-authorized SECOND leg of the phase-39 by-codec split (BRAINSTORM §1.4; the phase-39 SPEC §3.0) — chartered because the three checker codecs are independent probe mechanisms and the keystone substrate landed once with HTTP (39.1). 39.2's own surface:

| Unit | Anticipated production LoC |
|---|---|
| The `prober` interface + the dispatch generalization of `healthChecker`/`parseHealthChecks` (lift the `interval`/`timeout`/`unhealthy`/`healthy` envelope out of `httpHealthCheckCfg`; the kind-tagged `[]checkerSpec`); `httpProber` = the 39.1 `probeHTTP` body unchanged | ~50–80 |
| `tcpProber` (connect-only: `net.Dialer{Timeout}.Dial` + close) + the `tcp_health_check` parse arm + the `send`/`receive` DEPARTURE-reject | ~30–50 |
| `grpcProber` (the `grpc.health.v1.Health/Check` unary call + the network-vs-application classification) + the `grpc_health_check` parse arm (`service_name`) | ~50–90 |
| The gRPC **cluster-must-be-H2** reject (detect the cluster's parsed protocol at build; the byte-stable house wording) | ~20–40 |
| The `0067`/`0068` fixture drivers + asserters + the +1 gRPC BackendKind + the prober unit tests | test-side LoC, NOT counted |

Net production **~150–260 LoC, ~13–16 tasks** — BOTH axes UNDER the gate. **Single flat 39.2 leg — NO further split** (the BRAINSTORM's "39.2 may split TCP vs gRPC at its SPEC" → resolved NO: the two probers share the parse/dispatch refactor and the runtime, so splitting would duplicate the refactor across two legs). The PLAN re-checks the gate per ADR-0045 (anticipated NO further split). The 39.2 leg adds its sub-leg record to ROADMAP row 39 (NO parent rollup per ADR-0106 — the flat family row); the Upstream-robustness family STAYS OPEN (4 candidates remain after 39.2). With 39.2 the active-HC checker roster (HTTP + TCP + gRPC) is COMPLETE — the phase-39 by-codec split fully consumed.

### 3.1 The TCP connect-only prober (ADR-0244; AMEND-TG2)

`tcpProber` is the simplest codec: a bare TCP connect is the liveness signal (`tcp_health_check{}` empty = connect-only — AMEND-TG2). Indicative shape (the PLAN/IMPL finalizes):

```go
// tcpProber is the connect-only TCP health-check codec (ADR-0244). An empty
// tcp_health_check means "a successful TCP connect proves liveness"; a dial
// failure (connect refused / timeout) is a network failure, exactly as the HTTP
// probe classifies a dial error. send/receive payload matching is DEFERRED (a
// tcp_health_check that sets send/receive DEPARTURE-rejects at parse — §6).
type tcpProber struct{ timeout time.Duration }

func (p tcpProber) probe(addr string) (ok, networkFailure bool) {
    d := net.Dialer{Timeout: p.timeout}
    conn, err := d.Dial("tcp", addr)
    if err != nil {
        return false, true // connect refused / timeout = network failure (failure++ AND network_failure++)
    }
    _ = conn.Close()
    return true, false
}
```

The result flows through the UNCHANGED `applyResult` (`health.go:194`): a `(false, true)` increments BOTH `health_check.failure` and `health_check.network_failure` (AMEND-TG2 — matching the reference's connect-refused classification); a `(true, false)` increments `health_check.success`; the `recordResult` first-check-immediate + threshold transitions are byte-stable (the 39.1 model). The connect-only probe is a strict subset of the HTTP probe's dial step (the HTTP probe ALSO connects then additionally sends `GET` + reads the status), so the classification is identical by construction.

### 3.2 The gRPC prober (ADR-0244; AMEND-TG3) — the project's first cluster-issued outbound RPC

`grpcProber` issues a unary `grpc.health.v1.Health/Check` over H2 and reads the `ServingStatus` (AMEND-TG3). It reuses the EXISTING `google.golang.org/grpc` dep (NO hand-roll, NO new module — AMEND-TG1). Indicative shape:

```go
// grpcProber is the grpc.health.v1.Health/Check codec (ADR-0244). It reads the
// ServingStatus, not mere reachability: a reachable backend reporting NOT_SERVING
// is an APPLICATION failure (failure++, network_failure stays 0); a transport/
// connect failure is a NETWORK failure (failure++ AND network_failure++). The
// MVP supports service_name (empty = the overall "" service); authority +
// initial_metadata are DEFERRED. A fresh ClientConn per probe (reuse_connection
// deferred); the dial reuses internal/grpcclient or grpc.NewClient directly.
type grpcProber struct {
    serviceName string
    timeout     time.Duration
}

func (p grpcProber) probe(addr string) (ok, networkFailure bool) {
    ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
    defer cancel()
    conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        return false, true
    }
    defer func() { _ = conn.Close() }()
    resp, err := grpc_health_v1.NewHealthClient(conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: p.serviceName})
    if err != nil {
        // MVP SIMPLIFICATION: any RPC error => network failure. A transport error
        // (codes.Unavailable — refused/dead port) IS a network failure, but a
        // returned gRPC status on a REACHABLE backend (e.g. codes.Unimplemented —
        // no health service) is an APPLICATION failure. The IMPL must split these
        // per D-S39.2-3 (transport/Unavailable => network; a returned status =>
        // application, networkFailure=false). The 0068 fixture exercises only
        // refused + SERVING, so this snippet is the MVP contract, NOT the final one.
        return false, true
    }
    return resp.GetStatus() == grpc_health_v1.HealthCheckResponse_SERVING, false // NOT_SERVING/other = application failure (network_failure stays 0)
}
```

The classification (AMEND-TG3): `err != nil` ⇒ `(false, networkFailure=true)` (a refused/dead port surfaces as `codes.Unavailable` — verified: both `failure` and `network_failure` increment); a completed `Check` returning `resp.Status != SERVING` ⇒ `(false, networkFailure=false)` (verified: `failure` increments, `network_failure` stays 0). **Refinement (D-S39.2-3):** a reachable backend that returns a non-OK gRPC STATUS for `Check` itself (e.g. `codes.Unimplemented` — the cluster does not run the health service) currently maps to `networkFailure=true` via the `err != nil` arm; the reference treats an Unimplemented `Check` as an application failure. The `0068` fixture exercises only connect-refused + SERVING (+ optionally NOT_SERVING via `service_name`), so the Unimplemented edge is a recorded IMPL refinement (classify `codes.Unavailable`/transport ⇒ network; a returned gRPC status ⇒ application), not an MVP blocker. The gRPC probe is ALWAYS H2 (gRPC mandates H2) regardless of the cluster's data-plane protocol — hence the cluster-must-be-H2 reject (§6) is a config-consistency guard, not a probe-transport requirement.

### 3.3 The dispatch generalization (the substrate stays byte-stable)

`parseHealthChecks` (`health.go:219`) widens from returning `[]httpHealthCheckCfg` to a kind-tagged `[]checkerSpec{ envelope checkerEnvelope, prober prober }` where `checkerEnvelope` holds the common `interval`/`timeout`/`unhealthy`/`healthy` (lifted out of `httpHealthCheckCfg`) and the oneof arm selects the prober: `GetHttpHealthCheck()` ⇒ `httpProber`, `GetTcpHealthCheck()` ⇒ `tcpProber` (reject `send`/`receive`), `GetGrpcHealthCheck()` ⇒ `grpcProber` (require cluster H2). `healthChecker` (`health.go:159`) holds the `envelope` + a `prober` instead of `cfg httpHealthCheckCfg`; `probeOnce`/`applyResult`/`run`/`registerStats` are UNCHANGED except they call `hc.prober.probe(addr)` instead of `probeHTTP(addr, hc.cfg)`. The `manager.go:394` loop (`for _, cfg := range hcCfgs { cl.checkers = append(..., newHealthChecker(...)) }`, guarded by `if health != nil {` at `:393`) is structurally UNCHANGED (it iterates the kind-tagged specs). **Everything else is byte-stable:** the `clusterHealth`/`hostHealth` registry, the transition model, `StartHealthChecks`/`Drain`, the +7 stats, the build-time-injected health view on all six LB constructs, and the panic threshold. A cluster with no `health_checks` still gets `health == nil` and the LB fast path (byte-identical to pre-39.1). The full 68-dir differential is the real guard that the generalization is behavior-preserving for HTTP + the existing fixtures.

---

## 4. Framework primitives — a codec dispatch over the 39.1 runtime + 0 new packages + 0 new go.mod deps

- **A `prober` interface + three implementations** (`httpProber`/`tcpProber`/`grpcProber`) — the FIRST checker-codec dispatch; `httpProber` is the 39.1 `probeHTTP` body unchanged.
- **The project's FIRST connect-only probe** (TCP) and **FIRST cluster-issued outbound gRPC RPC** (`grpc.health.v1.Health/Check`).
- **The 39.1 runtime + transition model + six-LB health view REUSED VERBATIM** — `clusterHealth`/`hostHealth`/`recordResult`/`healthChecker.run`/`StartHealthChecks`/`Drain` + the LB health view are codec-agnostic; 39.2 is a probe-mechanism widening, not a runtime change.
- **ONE new reject** — `grpc_health_check` requires the cluster to support HTTP/2 (config-init; §6). The 39.1 `only http_health_check is supported` reject is LIFTED.
- **ZERO new packages** — the probers live in the existing `internal/cluster/health.go`.
- **ZERO new go.mod deps** — `google.golang.org/grpc v1.70.0` is already a direct dep; `grpc_health_v1` is a subpackage (`go mod tidy -diff` empty — AMEND-TG1).
- **ZERO new stat surface** — the codec-agnostic 39.1 set (AMEND-TG4).

---

## 5. Proto-field roster (per §11.1 D-TG1)

### 5.1 `HealthCheck_TcpHealthCheck` (`config/core/v3/health_check.pb.go:813`)

| Field | Wire | Go type | 39.2 disposition |
|---|---|---|---|
| `send` | field 1, `*HealthCheck_Payload` | `Send` | DEFERRED — empty = connect-only (MVP); a SET `send` DEPARTURE-rejects (§6) |
| `receive` | field 2, `[]*HealthCheck_Payload` | `Receive` | DEFERRED — a SET `receive` DEPARTURE-rejects (§6) |
| `proxy_protocol_config` | field 3, `*ProxyProtocolConfig` | `ProxyProtocolConfig` | DEFERRED |

`HealthCheck_Payload` = oneof {`Text` (hex string; PGV `min_len:1`), `Binary` (bytes)} — relevant only if payload is un-deferred (§6 records the hex pins).

### 5.2 `HealthCheck_GrpcHealthCheck` (`config/core/v3/health_check.pb.go:940`)

| Field | Wire | Go type | 39.2 disposition |
|---|---|---|---|
| `service_name` | field 1, `string` | `ServiceName` | **CONSUMED** — the `HealthCheckRequest.Service`; empty = the overall `""` service; no PGV constraint |
| `authority` | field 2, `string` | `Authority` | DEFERRED — defaults to the cluster name; no PGV constraint |
| `initial_metadata` | field 3, `[]*HeaderValueOption` | `InitialMetadata` | DEFERRED — each entry's `HeaderValue.Key` carries PGV `min_len:1` |

### 5.3 `grpc.health.v1` (`google.golang.org/grpc@v1.70.0/health/grpc_health_v1`)

| Symbol | Shape | 39.2 use |
|---|---|---|
| `Health_Check_FullMethodName` | `"/grpc.health.v1.Health/Check"` | the unary RPC the prober invokes |
| `HealthCheckRequest` | `{ Service string }` | built from `grpc_health_check.service_name` |
| `HealthCheckResponse` | `{ Status ServingStatus }` | `Status == SERVING` ⇒ healthy |
| `HealthCheckResponse_ServingStatus` | UNKNOWN=0, **SERVING=1**, NOT_SERVING=2, SERVICE_UNKNOWN=3 | the MVP maps SERVING ⇒ healthy, all else ⇒ unhealthy |

The 39.1 `HealthCheck` envelope (`timeout`/`interval`/`unhealthy_threshold`/`healthy_threshold` + the `HealthChecker` oneof) is UNCHANGED; 39.2 adds the `tcp`/`grpc` oneof arms.

---

## 6. PARSE-REJECT roster (per §11.5 + ADR-0080)

Per ADR-0080: envoy-go pins its OWN byte-stable house wording (parity in OUTCOME with the reference's text — envoy-go does NOT run PGV). The 39.1 envelope rejects (interval/timeout/thresholds/checker-oneof required — `health.go:222-243`) are UNCHANGED and apply to all codecs. 39.2's reject delta:

| # | Condition | Anticipated envoy-go wording | Reference (parity in outcome) |
|---|---|---|---|
| (L) | `tcp_health_check`/`grpc_health_check` checker | **ACCEPT** (the 39.1 `only http_health_check is supported` reject is **LIFTED**) | accepts (TCP parse-accepts; gRPC needs H2) |
| 1 | `grpc_health_check` on a cluster WITHOUT HTTP/2 | `cluster: %q: grpc_health_check requires the cluster to support HTTP/2` | `<cluster> cluster must support HTTP/2 for gRPC healthchecking` (config-init, NOT PGV) |
| 2 | `tcp_health_check` sets `send` or `receive` | `cluster: %q: tcp_health_check send/receive payload matching is not supported (connect-only)` | (accepts upstream — an envoy-go-strict DEPARTURE-reject, the deferred-feature boundary; the 38.2 `cluster_header` precedent) |
| (3) | `grpc_health_check` sets `authority`/`initial_metadata` | (anticipated SILENT-IGNORE; the PLAN confirms ignore vs reject — D-S39.2-2) | accepts upstream |

**Recorded TCP payload-hex pins** (apply only if `send`/`receive` is later un-deferred — they distinguish two error CLASSES that must NOT be lumped): an empty `Payload.text` → PGV `PayloadValidationError.Text: value length must be at least 1 characters`; a NON-hex `Payload.text` (e.g. `"ZZZZ"`) → a RUNTIME `invalid hex string '<value>'` (the config-build hex decode, a different code path than PGV). **NON-rejects** (envoy-go must NOT over-reject): an empty `tcp_health_check{}` (connect-only — ACCEPTS); an empty `grpc_health_check{}` with the envelope + H2 (the overall `""` service — ACCEPTS); a `grpc_health_check.service_name` with a dotted name (`svc.Bad`) or a custom `authority` (free-form strings, no PGV). The gRPC-H2 reject is config-init (it fires under `--mode validate` WITHOUT the `Proto constraint validation failed` PGV envelope — a boot-reject fixture, if warranted, matches the plain string). All 39.2 rejects are config-build (boot) — anticipated UNIT-level config-build reject tests (the 39.1 §8.2 precedent; a cross-side boot-reject dir only if the IMPL finds it warranted, per `reference_differential_fixture_dispatch_constraint`).

---

## 7. Stat surface — ZERO new stats (per §11.4 D-TG4 + AMEND-TG4)

- **ZERO new stat names.** Surface **1132 → 1132** (UNCHANGED at the IMPL). Live `/stats` scrapes under BOTH a TCP HC and a gRPC HC emit EXACTLY the 39.1 codec-agnostic set: `health_check.{attempt,success,failure,network_failure}` (counters) + `health_check.healthy` (gauge) + `membership_healthy` (gauge) + `lb_healthy_panic` (counter). The checker codec is a different probe MECHANISM behind the same `applyResult`/`registerStats` — no codec-specific stat exists.
- **The `0067`/`0068` `StatsAsserter`/delta set:** the cross-equal converged subset {`membership_healthy`, `health_check.attempt/success/failure`} + the per-request `upstream_rq_*` DELTAs (post-warmup baseline — AMEND-TG7), identical to the `0066` posture. For `0068` additionally pin `health_check.network_failure` separately (the dead-port arm increments it; a NOT_SERVING arm, if included, does NOT — the AMEND-TG3 discriminator).
- The deferred-feature cluster-wide names (`health_check.{degraded,passive_failure,verify_cluster}`, `membership_{change,degraded,excluded}`, `update_no_rebuild`) remain recorded departures (39.1 §7) — 39.2 does not implement them.

---

## 8. Differential fixture taxonomy (+2: `0067` TCP + `0068` gRPC cross-side poll-to-convergence)

Per `reference_differential_fixture_dispatch_constraint`: two cross-side dirs (NO boot-reject dir — §8.2). Per `reference_differential_asserter_dispatch`: the cross-side health assertions run via `StatsAsserter`. Per `reference_health_check_propagation_warmup`: each fixture polls `membership_healthy` to convergence THEN runs a warmup-until-K-consecutive-200s gate THEN asserts per-request counters as DELTAs (the `0066` protocol — gate on convergence + warmup, NOT raw `attempt` deltas, AMEND-TG7). Per `reference_differential_run_selector`: targeted runs use `-run 'TestDifferential/0067'` / `'TestDifferential/0068'` (NOT `-run '0067'`). Per `reference_fixture_workload_constant_desync`: the host counts / thresholds / N / warmup-K DERIVE from named constants (never literals). Numbering continues from `0066`; re-pinned at IMPL Task 1.

### 8.1 `0067-health-check-tcp` (cross-side; connect-only TCP HC; NO new BackendKind)

An HTTP listener routing to a cluster `c_tcp` with `health_checks: [{tcp_health_check{}, interval, timeout, unhealthy_threshold, healthy_threshold}]` over endpoints = {2 LIVE HTTP backends (a bare TCP connect to their listen port succeeds — `HTTPEcho` REUSED; the live host answers BOTH the TCP HC connect AND the data-plane HTTP load), 1 DEAD unbound port} on BOTH sides (subject STATIC / `127.0.0.1`; reference STRICT_DNS / bridge-network hostname — the `0066` shape). 66% healthy (> 50% strict ⇒ asserts FILTERING not panic — 39.1 AMEND-HC5). The driver: (1) **poll phase** — scrape `/stats` on BOTH sides until `membership_healthy == 2` (no fixed sleep); (2) **warmup gate** — 503-tolerant `GET /` until K consecutive 200s (the worker-rotation propagation window); (3) **measured phase** — N requests; assert 100% land on the LIVE backends + the per-request `upstream_rq_*` DELTAs (post-warmup baseline); (4) cross-side `health_check.*`/`membership_healthy` parity. **NO new BackendKind** (tail STAYS 33 here — the connect-only probe needs no special backend; the dead host is an unbound address). Deliberate-breaks (`-count=1`): (i) `tcpProber` always returns healthy ⇒ the dead host never drops ⇒ convergence times out; (ii) `Pick` ignores health ⇒ warmup never stabilizes (traffic leaks to the dead host).

### 8.2 `0068-health-check-grpc` (cross-side; gRPC HC over H2; +1 BackendKind)

An H2 cluster `c_grpc` (`http2_protocol_options`/`HttpProtocolOptions.explicit_http_config.http2_protocol_options`) with `health_checks: [{grpc_health_check{}, …}]` over endpoints = {2 LIVE gRPC SERVING responders, 1 DEAD unbound port} on BOTH sides. The **+1 BackendKind** — an **h2c server** that (a) answers `grpc.health.v1.Health/Check` ⇒ `SERVING` (via the turnkey `health.NewServer()` + `grpc_health_v1`, the probe-confirmed pattern) for the HC probe AND (b) returns HTTP 200 to plain-H2 data-plane requests (so the load-phase 100%-live assertion holds — a `grpc.Server.ServeHTTP` / `http.Handler` mux: `application/grpc` content-type ⇒ the gRPC server, else ⇒ 200). BackendKind tail **33 → 34** (anticipated name `GRPCHealthResponder`). The driver follows the `0067`/`0066` protocol (poll `membership_healthy == 2` → warmup → measured-phase 100%-live + delta counters → cross-side parity); the data-plane load is HTTP/2 (the cluster is H2). Deliberate-breaks (`-count=1`): (i) `grpcProber` ignores `resp.Status` (always healthy) ⇒ convergence times out against the dead host; (ii) `Pick` ignores health ⇒ warmup never stabilizes. **Candidate finer arm (D-S39.2-4):** a NOT_SERVING responder host (via a `service_name` the backend reports NOT_SERVING) to assert the application-vs-network discriminator (`failure++`, `network_failure` stays 0) — included if it does not complicate the keystone fixture; else a unit-level prober test covers it.

### 8.3 NO new fuzzer (family expectations)

Fuzzers STAY **42**. The gRPC `Check` response is decoded by the `grpc_health_v1` library (NOT a hand-rolled wire parser — there is no untrusted-wire decoder in envoy-go's code to fuzz); the TCP connect-only probe decodes nothing. A prober-classification PROPERTY test (probe-result sequences → correct healthy/unhealthy transitions; the network-vs-application classification) is unit-level, FOLDED into `health_test.go`. The BRAINSTORM's candidate `FuzzGrpcHealthResponse` is **NOT warranted** (it presupposed a hand-rolled decoder; using the library client removes the attack surface). No new conformance harness; h2spec 53/53 + proxy-wasm 10/10 re-run asserted-unaffected at the six-gate (39.2 touches only the cluster checker codec + the parse path — the wire path is byte-identical when no `health_checks` is configured; the full 70-dir differential is the real guard).

---

## 9. Behavior-contract delta (the 39.2 bundle; ADR-0052 atomic landing)

The existing `### Cluster — active health checks` BEHAVIOR_CONTRACT subsection (added at 39.1) gains: the TCP connect-only codec (success/refused classification; payload deferred-reject) + the gRPC codec (SERVING ⇒ healthy; the application-vs-network `network_failure` discriminator; service_name; the cluster-must-be-H2 reject) + the LIFT of the 39.1 `only http_health_check is supported` reject. The stat-surface block STAYS 1132 (AMEND-TG4 — no new stats). DECISIONS gains ADR-0244 (body at IMPL). STATE/ROADMAP/the parent phase-39 README/the 39.2 PROGRESS updated; ROADMAP row 39 gains the 39.2 sub-leg `done` note at the IMPL six-gate (NO parent rollup per ADR-0106). BackendKind tail 33 → 34 (`GRPCHealthResponder`).

---

## 10. Per-task structure (~13–16 tasks; PLAN decomposes)

Anticipated spine (the PLAN bite-sizes; the FINAL ADR-0045 re-check at the PLAN): (1) baselines/anchors + PROGRESS.md; (2) the `prober` interface + the dispatch generalization of `healthChecker`/`parseHealthChecks` (`httpProber` = the 39.1 body unchanged; the kind-tagged `[]checkerSpec`) — the full 68-dir differential (the pre-39.2 baseline; `0067`/`0068` raise it to 70 in later tasks) must stay GREEN after this refactor (byte-stable HTTP); (3) `tcpProber` (connect-only) + the `tcp_health_check` parse arm + the `send`/`receive` DEPARTURE-reject + unit tests; (4) `0067-health-check-tcp` fixture; (5) `0067` deliberate-break + flake; (6) `grpcProber` (the `grpc.health.v1.Health/Check` call + the network-vs-application classification) + the `grpc_health_check` parse arm (`service_name`) + unit tests; (7) the gRPC cluster-must-be-H2 reject (the build-time protocol detection) + unit test; (8) the +1 `GRPCHealthResponder` BackendKind (the h2c gRPC-SERVING + 200 responder) in the harness; (9) `0068-health-check-grpc` fixture; (10) `0068` deliberate-break + flake; (11) full differential re-verify gate (70-dir) + six-gate; (12) ADR-0244 body + BEHAVIOR_CONTRACT 39.2 delta; (13) completion bundle (STATE/ROADMAP/README/PROGRESS + counts).

---

## 11. SPEC-time empirical-pin block (D-TG1..D-TG7 — executed IN-SESSION 2026-06-14)

Probed live against `envoyproxy/envoy:contrib-v1.37.2` (@ `sha256:7edd5b0f…`, ADR-0227) on Docker bridge networks (`reference_docker_probe_bridge_network`): a STRICT_DNS TCP cluster (live `:80` open + dead `:81` refused, `tcp_health_check{}`), an H2 STRICT_DNS gRPC cluster (a purpose-built `grpc.health.v1` SERVING server `:50051` + dead `:50052`, `grpc_health_check{}` + a `service_name` NOT_SERVING arm), `--mode validate` reject probes, and `/stats` name-set scrapes. Traffic-ran proof via `health_check.attempt` ≥ 1 + host `health_flags` transitions.

### Summary disposition table (7 pins)

| Pin | Disposition |
|---|---|
| D-TG1 proto surface + deps | RESOLVED — Tcp/Grpc fields re-pinned; grpc v1.70.0 already direct dep; `go mod tidy -diff` empty → ZERO new module |
| D-TG2 TCP connect-only behavior | RESOLVED — empty tcp = connect-only; refused ⇒ failure++ AND network_failure++; behaves like HTTP; payload parse-accepts but DEFERRED |
| D-TG3 gRPC behavior | RESOLVED — SERVING ⇒ healthy; NOT_SERVING ⇒ failure (network_failure 0); dead-port ⇒ both++; service_name supported; grpc_health_v1 used directly (no hand-roll, no fuzzer) |
| D-TG4 stat surface | RESOLVED — ZERO new stat names (both codecs reuse the 39.1 set); surface 1132 → 1132 |
| D-TG5 reject roster | RESOLVED — 39.1 oneof-reject LIFTED; NEW gRPC-must-be-H2 (config-init); TCP empty-hex PGV vs non-hex runtime distinction |
| D-TG6 fixtures + split | RESOLVED — 0067 (no new BackendKind) + 0068 (+1 BackendKind); single flat 39.2 leg; ONE ADR; no new fuzzer |
| D-TG7 no-traffic deactivation | RESOLVED — counters freeze; gate on membership_healthy + warmup, not raw attempt deltas |

### 11.1 D-TG1 — proto surface + deps: CONFIRMED LIVE
`HealthCheck_TcpHealthCheck{ send (f1, empty=connect-only), receive (f2, fuzzy ordered), proxy_protocol_config (f3) }`; `HealthCheck_Payload` = oneof {text (hex), binary}. `HealthCheck_GrpcHealthCheck{ service_name (f1), authority (f2), initial_metadata (f3) }`. `grpc_health_v1` (v1.70.0): `HealthCheckRequest{Service}`, `HealthCheckResponse{Status}`, `ServingStatus`{UNKNOWN=0,SERVING=1,NOT_SERVING=2,SERVICE_UNKNOWN=3}, `/grpc.health.v1.Health/Check`. `google.golang.org/grpc v1.70.0` is ALREADY a direct dep (imported by `internal/grpcclient`/extauthz/ratelimit); `go mod tidy -diff` exit 0, EMPTY → ZERO new module. The gRPC probe build (a static `grpc_health_v1` server under bare alpine) pulled only the already-cached grpc/protobuf/genproto/x-sys/x-text and ran turnkey via `health.NewServer()`.

### 11.2 D-TG2 — TCP connect-only: CONFIRMED LIVE
Empty `tcp_health_check{}` over live `:80` (nginx, TCP connect succeeds) + dead `:81` (connect-refused — verified `80=OPEN`/`81=refused`): `:80 → health_flags::healthy`, `:81 → /failed_active_hc`. Stats: `attempt:2, success:1, failure:1, network_failure:1, health_check.healthy:1, membership_healthy:1, lb_healthy_panic:0`. **A connect refusal increments BOTH `failure` and `network_failure`** (identical to the HTTP dial-failure classification). Payload `send:{text:"504F4E47"}`/`receive:[{text}]` and the binary variant both parse-accept (`configuration OK`); `receive` WITHOUT `send` accepts. Connect-only is the MVP; payload matching DEFERRED. The counters froze at `attempt:2` (no-traffic deactivation — D-TG7).

### 11.3 D-TG3 — gRPC behavior: CONFIRMED LIVE
H2 cluster over a live `grpc.health.v1` SERVING server `:50051` + dead `:50052`, `grpc_health_check{}` (overall `""`): `:50051 → healthy`, `:50052 → /failed_active_hc` with `failure++` AND `network_failure++`. A `service_name: "svc.Bad"` (server reports NOT_SERVING) against the reachable live host: `→ /failed_active_hc` with `failure:1` but **`network_failure:0`** — the application-vs-network discriminator (connected-but-unhealthy ≠ can't-connect). Stats otherwise the 39.1 set. The `grpc_health_v1` client + `health.NewServer()` were turnkey → the prober uses the library directly (no hand-roll).

### 11.4 D-TG4 — stat surface: CONFIRMED LIVE
Both the TCP and gRPC `/stats` scrapes emit EXACTLY `health_check.{attempt,success,failure,network_failure}` + `health_check.healthy` + `membership_healthy` + `lb_healthy_panic` (the 39.1 set) plus the always-present deferred-feature cluster-wide names (`health_check.{degraded,passive_failure,verify_cluster}`, `membership_{change,degraded,excluded,total}`). NO codec-specific stat. **ZERO new stat names; surface 1132 → 1132.**

### 11.5 D-TG5 — reject roster: PINNED LIVE
`grpc_health_check{}` on a non-H2 cluster (`--mode validate`) → `<cluster> cluster must support HTTP/2 for gRPC healthchecking` (config-init, no PGV envelope). No-interval → the generic `HealthCheckValidationError.Interval: value is required` (39.1 envelope, unchanged). Empty `grpc_health_check{}` + H2 + envelope → `OK`. `service_name`/`authority` are unconstrained free-form strings (no PGV). `initial_metadata` entries inherit `HeaderValueValidationError.Key: value length must be at least 1 characters` (deferred field). TCP empty-hex `Payload.text` → PGV `PayloadValidationError.Text: value length must be at least 1 characters`; NON-hex text → RUNTIME `invalid hex string 'ZZZZ'` (distinct error class — recorded for the deferred payload path).

### 11.6 D-TG6 — fixtures + envelope + split: RESOLVED
`0067-health-check-tcp` (2-live/1-dead, connect-only, NO new BackendKind — the live HTTP backends answer the TCP connect) + `0068-health-check-grpc` (2-live/1-dead, +1 `GRPCHealthResponder` BackendKind — an h2c gRPC-SERVING + 200 responder). The 39.2 envelope (~150–260 prod LoC / ~13–16 tasks) holds as a single leg under the ADR-0045 gate; the PLAN re-checks. ONE ADR (ADR-0244). NO new fuzzer (library-decoded response). The 39.1/39.2 by-codec split is now FULLY consumed (HTTP + TCP + gRPC complete).

### 11.7 D-TG7 — no-traffic deactivation: CONFIRMED LIVE
HC counters froze at `attempt:N` after settling host state (default `no_traffic_interval` 60s back-off) with no data-plane traffic; HC connections did NOT count toward host `cx_total`. The `0067`/`0068` drivers therefore gate on `membership_healthy` convergence + the warmup-until-K-consecutive-200s pattern (`reference_health_check_propagation_warmup`) and assert per-request counters as post-warmup DELTAs — never on a growing `attempt` delta (which stalls).

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-S39.2-1** the exact `prober` interface placement + the `checkerEnvelope`/`checkerSpec` shape (lift `interval`/`timeout`/`unhealthy`/`healthy` from `httpHealthCheckCfg` into a shared envelope; keep `httpProber` behavior byte-stable; the 68-dir differential GREEN after the refactor).
- **D-S39.2-2** the gRPC `authority`/`initial_metadata` disposition (silent-ignore vs reject — anticipated silent-ignore) + whether to reuse `internal/grpcclient`'s dial vs `grpc.NewClient` directly for the prober.
- **D-S39.2-3** the gRPC classification edge: a reachable backend returning a non-OK gRPC STATUS for `Check` itself (e.g. `codes.Unimplemented`) — classify transport/`codes.Unavailable` ⇒ network_failure vs a returned status ⇒ application failure; pin against the reference if the `0068` fixture exercises it.
- **D-S39.2-4** the `0068` fixture constants (host counts, interval/timeout/threshold, N, warmup-K, the poll predicate + deadline) + whether the NOT_SERVING discriminator arm is in `0068` or a unit test + the `GRPCHealthResponder` BackendKind shape (the h2c gRPC-SERVING + 200 mux) + the `fixture_workload_constant_desync` guard.
- **D-S39.2-5** the gRPC-must-be-H2 detection site (the cluster's parsed protocol at `buildCluster` — reuse the existing H2-detection used by `dial_h2.go`) + the exact byte-stable reject wording + whether it is a unit-level config-build reject vs a cross-side boot-reject dir.
- **D-S39.2-6** the FINAL ADR-0045 re-check (single 39.2 leg vs a TCP/gRPC sub-split — anticipated single leg).

---

## 13. ADR continuity — the ADR-0244 §Context DRAFT (anchored here; full entry lands at the 39.2 IMPL)

**ADR-0244 (§Context draft) — the multi-codec active-HC checker prober dispatch (TCP connect-only + gRPC-over-H2).** The 39.1 checker runtime hard-bound the HTTP probe (`healthChecker.cfg httpHealthCheckCfg` + `probeHTTP`). The phase-39 by-codec split pre-authorized the TCP + gRPC codecs as the second leg. Decision (to be ratified at IMPL): generalize the runtime to a small `prober` interface (`probe(addr) (ok, networkFailure bool)`) with three implementations — `httpProber` (the 39.1 body unchanged), `tcpProber` (connect-only; a dial failure ⇒ network_failure, matching the HTTP dial-failure classification; payload send/receive DEPARTURE-deferred), `grpcProber` (a `grpc.health.v1.Health/Check` unary RPC over H2 reusing the existing `google.golang.org/grpc` dep; the application-vs-network discriminator — a returned non-SERVING status ⇒ failure-only, a transport error ⇒ network_failure); `parseHealthChecks` returns a kind-tagged `[]checkerSpec` (the 39.1 `only http_health_check is supported` reject LIFTED) and the gRPC arm requires the cluster to support HTTP/2 (the byte-stable config-init reject). The `clusterHealth`/`hostHealth` registry, the transition model, `StartHealthChecks`/`Drain`, the +7 stats, and the build-time-injected six-LB health view are REUSED VERBATIM (the codec-only widening — the thrift-33-over-the-ADR-0230-runtime reuse precedent inverted onto the 39.1 runtime; ADR-0242/0243 unamended). Consequences: the active-HC checker roster (HTTP + TCP + gRPC) is COMPLETE; ZERO new packages, ZERO new go.mod modules, ZERO new stats, +1 BackendKind (the gRPC SERVING responder); the prober dispatch is the extension point for any future codec (e.g. a Redis/MySQL liveness probe).

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

ALL counts UNCHANGED at this SPEC (stat surface 1132 / fixtures 68 / fuzzers 42 / BackendKind 33 / DECISIONS tail ADR-0243, next-free ADR-0244). Anticipated at the 39.2 IMPL: fixtures 68 → **70** (`0067-health-check-tcp` + `0068-health-check-grpc`), BackendKind tail 33 → **34** (`GRPCHealthResponder`), DECISIONS tail ADR-0243 → **ADR-0244** (next-free ADR-0245), **stat surface 1132 → 1132** (ZERO new — AMEND-TG4), fuzzers 42 → 42 (ZERO new — AMEND-TG6/§8.3), ZERO new packages + ZERO new go.mod modules. Row 39 STAYS `in-progress` at the SPEC commit (the 39.2 sub-leg flips `done` at the IMPL six-gate — NO parent rollup per ADR-0106; with 39.2 the phase-39 by-codec split is fully consumed). Next → the phase-39.2 PLAN (`superpowers:writing-plans` — decompose §10 into ~13–16 bite-sized TDD tasks; the FINAL ADR-0045 split-gate re-check).
