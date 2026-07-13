# Phase 61.3 Implementation Plan — `http3-downstream-get-differential`: the FIRST cross-side HTTP/3 differential fixture (`0104-http3-downstream-get`) + the differential harness's FIRST non-TCP transport surgery (UDP port exposure on the reference container-starters + a shared quic-go `test/helpers/h3.go` `H3RoundTrip` client) + the deferred writeH3Reply `server`/`content-length` synthesis for cross-side response fidelity. The THIRD and FINAL leg of the confirmed 61.1/61.2/61.3 split (SPEC-61 §3.0); **flips ROADMAP row 61 → `done`** (ALL THREE legs landed, ADR-0106); ANCHORS **ADR-0282**.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`). Subagents auto-commit per CLAUDE.md — the controller verifies each commit, cleans leak files, squashes at stage-close, re-runs the suite on the frozen HEAD (`feedback_subagent_autocommit_claudemd`).

**Goal:** Prove envoy-go's HTTP/3 downstream listener (61.1 substrate + 61.2 codec/HCM arm) serves a GET → 200 IDENTICALLY to the reference Envoy (`envoyproxy/envoy:contrib-v1.37.2`) — the first cross-side HTTP/3 differential. A new fixture `0104-http3-downstream-get` boots BOTH a reference-Envoy QUIC/H3 container and the subject envoy-go with a QUIC listener (`udp_listener_config.quic_options` + an `envoy.transport_sockets.quic` transport socket + HCM `codec_type: HTTP3` + a `/health → direct_response 200 "OK\n"` route), drives an HTTP/3 GET against each with a shared quic-go `http3.Transport` client, asserts a byte-stable observable (status 200 + body `"OK\n"` + a NAMED downstream-stat subset). The differential harness — TCP-only through 60 phases — gains its FIRST UDP/QUIC port exposure. This is the leg that lifts row 61 from `in-progress` to `done`.

**Architecture:** The harness starts the reference Envoy in a Docker container and the subject envoy-go as a subprocess; a shared HTTP/3 client (running on the host) drives BOTH. Three surgery seams: (1) `test/helpers/h3.go` `H3RoundTrip` — a quic-go `http3.Transport` client that PINS the dialed UDP `host:port` (ignoring Alt-Svc / server-preferred-address, exactly as `test/helpers/h2.go`'s `H2RoundTrip` pins its TCP dial), reusable by BOTH the reference dial and the subject dial (test code — quic-go in `test/helpers` is ALLOWED; the production import-hygiene gate confines quic-go to `internal/listener/quic.go` only). (2) `test/differential/harness.go` UDP exposure — the reference `ReferenceProxy` gains a `udpAddrs map[int]string` beside `tcpAddrs` (`harness.go:100`), a transport-aware `/udp` exposure + `MappedPort(…/udp)` in the two mapping starters, and a `ListenerUDPAddr(containerPort int)` accessor beside `ListenerAddr` (`harness.go:227`); the runner passes the reference's UDP-mapped addr (not the TCP one) to a QUIC fixture's `DriveReference` (a new marker interface gates this). The subject side needs NO Docker change — envoy-go binds the UDP socket directly and reports its addr through the existing `\S+`-accepting ready-sentinel (`harness.go:69`). (3) A small production fidelity pickup — `writeH3Reply`/`runH3` (`internal/filter/hcm/h3dispatch.go`) synthesizes `server: envoy` + `content-length` on the H3 response (the 61.2 arm was minimal — SPEC-61 §3.5 / ADR-0281 §Consequences deferred this to 61.3), so the subject's H3 response carries the same fidelity headers the reference emits. The fixture driver follows the `0003-http11-routing` template shape (Drive hooks issue the client request + return the body for `CompareBytes`; stats assertion in `StatsAsserter.AssertStats`), NOT the TCP-only `HTTPExpectations` path.

**Tech Stack:** Go; `github.com/quic-go/quic-go v0.54.1`'s `http3.Transport` (the H3 CLIENT — used ONLY in `test/helpers/h3.go`, test code); the EXISTING differential harness (`testcontainers-go` reference-container start, the subject-subprocess ready-sentinel, `CompareBytes`, `StatsAsserter`); `test/differential/fixture` Driver interface; `envoyproxy/envoy:contrib-v1.37.2` (`reference_envoy_contrib_image_tagging` — the `contrib-`-prefixed tag in the standard `envoyproxy/envoy` repo). +1 fixture (`0104`, 105 → 106); +0 production Go packages; +0 go.mod modules (quic-go + qpack landed at 61.1/61.2); +0 fuzzers.

---

## Global Constraints

- **One stage = leg 61.3 only (the differential fixture + harness UDP surgery + the writeH3Reply fidelity pickup).** This PLAN is the 61.3 IMPL decomposition. Leg 61.3 is the LAST of the 61.1/61.2/61.3 split — its six-gate **flips ROADMAP row 61 → `done`** (ADR-0106 / `reference_roadmap_split_phase_row_done`: a split-phase row flips `done` only when ALL legs land — 61.1 substrate + 61.2 codec/HCM + THIS 61.3). The HTTP/3 + QUIC FAMILY STAYS OPEN after phase-done (the §8 deferred candidates remain — upstream H3, alt-svc, 0-RTT, h3spec, QuicProtocolOptions tuning, full transport-socket options, QUIC robustness).
- **Fixture number is `0104`, NOT `0102` (SPEC/router citation corrected — `feedback_brief_citations_not_evidence`).** SPEC-61 §8 and the router named the fixture `0102-http3-downstream-get`; that slot was FREE at the SPEC's master tip (`cbda648b`, fixtures 103, tail `0101-stats-sink-graphite`). Since then `0102-tracing-custom-tags-literal` (phase 59) and `0103-xds-sds-server-cert` (phase 60.2) LANDED — the tail is now `0103`, so the next free number is **`0104`**. RE-VERIFY at IMPL start (`ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | sort | tail -3`); if a parallel row consumed `0104`, take the next free number and record it. Fixtures **105 → 106**.
- **Cross-side dispatch — put subject assertions in `StatsAsserter.AssertStats`, NOT `SubjectAsserter` (`reference_differential_asserter_dispatch`).** `0104` is a CROSS-SIDE fixture (the default runner branch: `DriveReference` + `DriveSubject` + `CompareBytes`). On that branch the runner calls `StatsAsserter.AssertStats(t, refAdminAddr, subjAdminAddr)` (`runner_test.go:1319`) but NEVER `SubjectAsserter.AssertSubject` (that fires only on the reference-less branch). A stat/structural assertion placed in `AssertSubject` is SILENTLY DROPPED (vacuous pass). Every subject-side assertion (the named-stat subset) lives in `AssertStats`.
- **DO NOT implement `HTTPExpectations` (TCP-only, unusable for H3).** The runner enforces `HTTPExpectations` by re-driving each request with its OWN internal HTTP/1-over-TCP client (`runner_test.go:1273-1291`, `refResp.StatusCode`/`subjResp.StatusCode`). Against a QUIC/UDP listener that TCP client cannot connect — it would fail or hang. So the `0104` driver does NOT implement `HTTPExpectations`. Status is asserted INSIDE the Drive hooks (`H3RoundTrip` → if `status != 200` return an error, failing the fixture); body parity via the runner's `CompareBytes` (both Drive hooks return the `"OK\n"` body). This mirrors `0003-http11-routing`'s Drive-returns-body shape but WITHOUT its (TCP-usable) `HTTPExpectations`.
- **One fixture dir = ONE runner branch (`reference_differential_fixture_dispatch_constraint`).** `0104` is a single cross-side directory; it does NOT also host a boot-reject scenario (the HTTP3-on-non-QUIC / QUIC-without-transport-socket boot-rejects are already covered at the unit layer in `internal/filter/hcm/config_test.go` [61.2] and `internal/listener/manager_test.go` [61.1] — do NOT re-cover them as a `BootRejectFixture`).
- **`BackendCount() >= 1` even for a backend-less `direct_response` fixture (`reference_differential_backendcount_min_one`).** The runner rejects `BackendCount()==0` (`runner_test.go:227`). `0104` serves a `/health → direct_response` route (no upstream dial), so it returns `BackendCount() 1` and IGNORES the throwaway backend port. BackendKind stays **38** (+0 — `direct_response` needs no `test/helpers` responder).
- **quic-go stays confined to `internal/listener/quic.go` in PRODUCTION; `test/helpers/h3.go` (TEST code) MAY import quic-go.** The 61.1/61.2 import-hygiene gate (`go list -deps ./internal/filter/hcm | grep -i quic-go` → NOTHING; quic-go only in `./internal/listener` deps) is a PRODUCTION invariant. `test/helpers/h3.go` is test code — its quic-go `http3.Transport` import does NOT touch production deps. Re-verify the production gate is UNCHANGED at Task 9 (61.3 adds NO production quic-go import — the writeH3Reply fidelity pickup at Task 6 stays stdlib-only `net/http`).
- **Docker UDP-reachability discipline (`reference_docker_probe_bridge_network`, `reference_host_gateway_ip_docker_desktop`).** The reference container's QUIC listener is reached from the HOST over a PUBLISHED `-p host:container/udp` port (host→container UDP publishing — the direction SPEC-61 §8 flagged as the untested one; on Linux bridge the kernel NATs it and it works, PROVEN by the SPEC-61 §11 live probe which drove an H3 GET over exactly a published UDP port against `contrib-v1.37.2` on this machine; Docker Desktop's userland UDP proxy is a residual risk). The `0104` fixture uses `direct_response` (NO backend dial), so the reference container does NOT need to reach a host backend — `HostGatewayIP`/shared-bridge concerns do NOT apply on the backend side. VERIFY NON-VACUOUSLY (Task 4 de-risk + Task 8): the H3 client actually completes a request and the admin `/stats` shows `downstream_rq_2xx: 1` — never trust a green that never decoded.
- **Contrib image tag (`reference_envoy_contrib_image_tagging`).** The reference H3 listener needs the CONTRIB build (QUIC is a contrib-compiled path in some Envoy builds; the SPEC-61 probe used `contrib-v1.37.2`). Use the pin the harness already boots (`test/differential` `EnvoyPin` — RE-DERIVE the current pin; it is `contrib-v1.37.2` per the SPEC probe) — the `contrib-`-prefixed tag in the STANDARD `envoyproxy/envoy` repo, NOT the stale `envoyproxy/envoy-contrib` repo. Confirm the harness's default pin is a contrib tag; if the differential runner defaults to the non-contrib `v1.37.2` image, `0104` must select the contrib pin (RE-DERIVE how a fixture selects its image pin, if at all — most fixtures inherit the runner's single pin).
- **TDD (`superpowers:test-driven-development`):** every code task is failing-test → run-fail → minimal-impl → run-pass → commit. The `H3RoundTrip` helper (Task 2) and the `writeH3Reply` fidelity pickup (Task 6) are unit-tested in isolation; the harness UDP surgery (Task 3) has a harness-level test; the fixture (Task 7) is proven by the full cross-side run (Task 8) with per-assertion liveness breaks.
- **Break protocol for the cross-side fixture (`reference_differential_break_protocol_count1`, `reference_deliberate_break_wrong_assertion`, `reference_differential_run_selector`).** Prove EACH load-bearing assertion is LIVE with a `-count=1` deliberate break (go-test caches a stale PASS otherwise), and CONFIRM WHICH assertion fires (a break can abort earlier and MASK the intended one). Select the fixture with the FULL subtest path `-run 'TestDifferential/0104-http3-downstream-get'` (a bare `-run '0104'` matches ZERO subtests → vacuous green).
- **`reference_fatalf_makes_assertions_unreachable`:** in `AssertStats`, use `Errorf` per independent stat property (each named counter is its own assertion); `Fatalf` ONLY for a broken precondition (admin scrape failed, the H3 load errored).
- **`reference_stats_sink_emits_used_only`:** assert a NAMED SUBSET of stats, never the whole registry. The reference emits `downstream_cx_http3_total`/`downstream_rq_http3_total`/the `http3.*` family that envoy-go does NOT register (deferred, ADR-0281). Assert only the counters BOTH sides emit: `http.<prefix>.downstream_rq_2xx`, `http.<prefix>.downstream_rq_total` (and, if present on both, `listener.<addr>.downstream_cx_total`) — NOT the http3-specific counters (which envoy-go omits) and NOT a whole-map equality.
- **Per-task gates (`feedback_pertask_gofmt_lint`):** every code task ends with `gofmt -l` (empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`. Do NOT skip gofmt.
- **Worktree hygiene (`feedback_subagent_worktree_detach` / `feedback_subagent_worktree_path_targeting`):** subagents write to the WORKTREE path (`.worktrees/phase-61.3-impl/…`); the controller verifies `git -C <main-checkout> status` stays clean after each task and the worktree branch is unchanged (no detached HEAD). Pin worktree-relative paths in every dispatch.
- **RE-DERIVE every `file:line` against the master tip at IMPL start (`feedback_brief_citations_not_evidence`).** This PLAN's citations were RE-DERIVED against master tip `eaef171a` (the phase-61.2 IMPL) this PLAN session by reading the ACTUAL source (`test/differential/harness.go`, `test/differential/runner_test.go`, `test/differential/fixture/fixture.go`, `test/helpers/h2.go`, `internal/filter/hcm/h3dispatch.go`, `internal/listener/manager_test.go`, `internal/listener/quic_test.go`). The `test/` line numbers are especially load-bearing (the harness is intricate) — RE-CONFIRM before editing.
- **ADR body lands at THIS IMPL (ADR-0044):** ADR-0282 §Context/§Decision/§Consequences are authored at this 61.3 IMPL. DECISIONS tail **ADR-0281 → ADR-0282** (next-free after this IMPL is **ADR-0283**). The PLAN RECOMMENDS a new per-leg ADR-0282 (the harness's first non-TCP transport is a distinct, reusable test-infra seam); the IMPL MAY instead fold into ADR-0281 (documented alternative) if it judges the surgery a mere completion of the codec-arm ADR.
- **Counts at 61.3 exit:** fixtures **106** (+1 — `0104-http3-downstream-get`) · fuzzers **55** (+0 — quic-go owns H3 framing/QPACK; no new hand-rolled parse) · BackendKind **38** (+0 — `direct_response`) · stat surface **1201** (+0 RECOMMENDED — the H3 path already Inc's the codec-agnostic `downstream_rq_*` counters; 61.3 asserts them cross-side, registers no new counter; the IMPL MAY pin +2 `downstream_{cx,rq}_http3_total` if it decides to match the reference's http3-specific surface, but the RECOMMENDATION is +0 + a named-subset assertion of the shared counters) · new production Go packages **+0** · new go.mod modules **+0** · DECISIONS tail **ADR-0282** · **ROADMAP row 61 → `done`**.

---

## Orientation — read before Task 1 (the zero-context brief)

You are extending a Go reimplementation of Envoy. The project proves fidelity with a **differential test harness** (`test/differential/`): for each fixture directory, it boots the REFERENCE Envoy (`envoyproxy/envoy:contrib-v1.37.2`) in a Docker container AND the SUBJECT envoy-go as a subprocess, drives IDENTICAL traffic against both, and asserts the observable outputs match. Through 60 phases the harness has been **TCP-only** — every listener/backend port is exposed and mapped as `/tcp`.

Leg **61.1** (LANDED, ADR-0279) stood up envoy-go's FIRST UDP/QUIC downstream listen path (binds `net.ListenUDP`, stands a quic-go listener over it, completes the QUIC/TLS-1.3 handshake negotiating ALPN `h3`). Leg **61.2** (LANDED, ADR-0281) made it SERVE HTTP/3: a stdlib-typed dispatch arm (`ServeH3`→`runH3`→`writeH3Reply` in `internal/filter/hcm/h3dispatch.go`) reached via a `network.H3Terminal` bridge, wiring `serveQUICConnection` into quic-go's `http3.Server.ServeQUICConn`. 61.2 proved the subject serves a GET→200 over H3 with a LOCAL quic-go client (`internal/listener/quic_test.go` `TestQUICListener_ServesH3GET`) — but there is **no CROSS-SIDE proof yet**: no reference-Envoy comparison, and the harness cannot drive H3 (no UDP exposure, no H3 client).

Leg **61.3** (THIS plan) closes the gap: the first cross-side HTTP/3 differential + the harness surgery that makes it possible. When it lands, ROADMAP row 61 flips `done` (all three legs in).

**The KEY facts that shape the leg (RE-DERIVED against master tip `eaef171a` — RE-CONFIRM at IMPL):**

- **The harness is TCP-only in three container-starters** (`test/differential/harness.go`): `StartReferenceProxy` (`:107`), `StartReferenceProxyWithMounts` (`:170`), `tryStartReferenceProxy` (`:422`, the boot-reject path — no `MappedPort`, no address map). Every `/tcp` site: `exposed` lists (`:108/:110`, `:171/:173`, `:423/:425`), readiness `wait.ForHTTP("/ready").WithPort("9901/tcp")` (`:121`, `:184`, `:445`), admin `MappedPort(ctx, "9901/tcp")` (`:138`, `:202`), per-listener `MappedPort(ctx, nat.Port(fmt.Sprintf("%d/tcp", p)))` (`:145`, `:209`). `nat` (`github.com/docker/go-connections/nat`) is imported (`:19`) and accepts the `/udp` form; testcontainers-go maps `/udp` ports natively — NO new hook needed.
- **The reference state struct** `ReferenceProxy` (`harness.go:97-101`): `container testcontainers.Container` (`:98`), `adminAddr string` (`:99`), `tcpAddrs map[int]string` (`:100`, in-container listener port → host `host:port`). Accessors: `AdminAddr()` (`:224`), `ListenerAddr(containerPort int)` (`:227`, returns `r.tcpAddrs[containerPort]`), `Stop(ctx)` (`:230`). The surgery adds a parallel `udpAddrs map[int]string` + `ListenerUDPAddr(containerPort int)`.
- **The subject side needs NO Docker change.** The subject envoy-go binds its UDP socket directly and reports the bound addr through the ready-sentinel regex `^envoy-go listener (\S+) ready on (\S+)$` (`harness.go:69`, in `readyListenerAddrs` at `:63`; capture 1 = name, capture 2 = addr, stored at `:78`). The `\S+` addr token accepts a UDP `host:port` unchanged. `SubjectProxy.ListenerAddr(name)` (`harness.go:294`) returns whatever the sentinel captured — the subject's UDP addr for a QUIC listener. **UNVERIFIED (IMPL confirms):** (a) that the subject BINARY emits the ready-sentinel for a QUIC listener with the UDP addr (61.1/61.2 proved the manager binds; whether the `cmd/…` entrypoint prints the sentinel for `kindQUIC` is untested — RE-DERIVE the sentinel-emission site and confirm it fires for a UDP listener); (b) the subject's UDP free-port allocation — the runner allocates `subjPort := freeTCPPortBlock(t)` (`runner_test.go:207`, helper `harness_test.go:194`) and passes it to `SubjectConfig`; envoy-go binds UDP on that port. A TCP-free port is almost always UDP-free too, but not guaranteed — the IMPL decides between reusing the TCP block (minimal; tiny flake risk) or adding a `freeUDPPortBlock` helper (`harness_test.go`). Recommend reuse + a flag; escalate only if a UDP-bind conflict actually flakes.
- **The runner** (`test/differential/runner_test.go`, 3444 lines — there is NO `runner.go`): `TestDifferential` (`:165`) discovers fixtures, looks each up in `fixture.DriverRegistry[fx]` (`:178`), and calls `runFixture` (`:186`, main path `:221`). The cross-side branch spawns the reference (`StartReferenceProxyWithMounts`/`StartReferenceProxy` at `:1173`/`:1175` with the reference ports from `refPorts` — single-port fallback `refPorts = []int{d.ReferenceListenerPort()}` at `:1146`), spawns the subject (`:1185`), computes `refAddr := ref.ListenerAddr(d.ReferenceListenerPort())` (`:1181` — TCP today), drives `d.DriveReference(ctx, refAddr)` (`:1217`) + `d.DriveSubject(ctx, subj.ListenerAddr(d.SubjectListenerName()))` (`:1243`), and `CompareBytes`. `StatsAsserter.AssertStats(t, ref.AdminAddr(), subj.AdminAddr())` at `:1319`. **The QUIC-fixture surgery** makes the runner pass the UDP addr to `DriveReference` for a QUIC-marked fixture (`refAddr = ref.ListenerUDPAddr(port)` instead of `ListenerAddr`).
- **The Driver interface** (`test/differential/fixture/fixture.go:15-52`): `BackendCount() int` (`:20`), `SubjectListenerName() string`, `ReferenceBootstrap([]int) string`, `SubjectConfig(refListenerPort, subjListenerPort int, backendPorts []int, subjAdminPort int) string`, `ReferenceListenerPort() int` (`:38`), `DriveReference(ctx, addr) ([]byte, error)`, `DriveSubject(ctx, addr) ([]byte, error)`, `ProbeAdmin(ctx, refAdminAddr, subjAdminAddr) (refBytes, subjBytes []byte, err error)`. Optional: `StatsAsserter.AssertStats(t TB, refAdminAddr, subjAdminAddr string)` (`:75`), `DistributionAsserter` (`:57`), `HTTPExpectations` (`:118`, TCP-only — DO NOT USE), `ReferenceLessFixture.RequiresReference() bool` (`:685`, the escape hatch if the reference H3 container proves unreachable).
- **Fixture registration is two-part:** the driver's `init()` calls `fixture.RegisterFixture(name, driver)` (`fixture.go:84`); a blank-import line in `runner_test.go` (the contiguous block `:26-130`, last entry `:130` = `0103-…/driver`) pulls the `init()` in. A new fixture adds BOTH.
- **The fixture-driver template** is `0003-http11-routing/driver/driver.go`: `const fixtureName` + `init()` register (`:15-19`); `BackendCount()` (`:23`); `BackendKind()` → `fixture.HTTPEcho` (`:24`); `SubjectListenerName()` (`:25`); `ReferenceListenerPort()` (`:26`); `ReferenceBootstrap`/`SubjectConfig` `fmt.Sprintf` backend ports into two `const` YAML templates; `drive()` (`:54`) issues the client request via `helpers.HTTPRoundTrip` and returns the body; `DriveReference`/`DriveSubject` (`:36-41`) both call `drive`. The `/health` route is a `direct_response` 200 `inline_string: "OK\n"` (`:153-156` ref, `:203-206` subject) — the exact shape `0104` reuses over H3.
- **`H2RoundTrip`** (`test/helpers/h2.go:33`) is the client model: it PINS the dial by explicitly `d.DialContext(ctx, "tcp", addr)` (`:41-42`) and building the transport over the pre-dialed conn (`:55-56`), so the request never re-resolves the URL host; a FRESH transport per call (no caching) for determinism (`:26-29`). `H3RoundTrip` mirrors this over quic-go's `http3.Transport` with a pinned UDP dial.
- **`AssertStats` + `scrapeStats` precedent** (`0067-health-check-tcp/driver/driver.go`): `AssertStats(t, refAdminAddr, subjAdminAddr)` (`:419`) scrapes both admin `/stats` (a LOCAL `scrapeStats(adminAddr)` helper) and asserts named counters. The `0104` `AssertStats` scrapes both sides' `/stats` and asserts the named downstream-stat subset. (RE-DERIVE the exact `scrapeStats` helper — copy the `0067` local copy; per `reference_host_gateway_ip_docker_desktop`, driver-side helpers are DUPLICATED locally, not imported from `test/differential`, to avoid an import cycle with the blank-import.)
- **The 61.2 `writeH3Reply` is MINIMAL** (`internal/filter/hcm/h3dispatch.go:33-48`): it copies the router action's `OrderedHeaders` (skipping `:`-pseudo-headers), `w.WriteHeader(status)`, `w.Write(body)` — it does NOT synthesize `server`/`date`/`content-length`. The reference Envoy's H3 `direct_response` emits `server: envoy` + `content-length`. Task 6 adds those to envoy-go's H3 response for cross-side fidelity. `emitAccessLogH3` (`accesslog_emit.go:147-198`) sets `Protocol: "HTTP/3"` (span `:166` + record `:186`); `runH3` seeds `chain.SetDownstreamProtocol("HTTP/3")` (`h3dispatch.go:171`).
- **NO http3-specific stat is registered** (grep `downstream_cx_http3|downstream_rq_http3|http3_total` over `internal/` → nothing). The H3 path Inc's the codec-agnostic `f.downstreamRqTotal.Inc()` (`h3dispatch.go:100`) + `f.downstreamStatusClassCounter(status).Inc()` (per-response) + the per-listener `listener.<addr>.downstream_cx_total` (asserted in `quic_test.go:95`). Those are the counters `0104` asserts cross-side.

### Discipline (honor on EVERY task) — the memory traps that bite this row
- **`feedback_brief_citations_not_evidence`** — RE-DERIVE every `test/` `file:line`; the harness is intricate and the numbers above are from `eaef171a`.
- **`reference_differential_asserter_dispatch`** — subject assertions in `AssertStats`, NEVER `AssertSubject` (dead on the cross-side branch).
- **`reference_differential_break_protocol_count1` / `reference_deliberate_break_wrong_assertion` / `reference_differential_run_selector`** — `-count=1` breaks; confirm WHICH fires; `-run 'TestDifferential/0104-http3-downstream-get'`.
- **`reference_stats_sink_emits_used_only`** — named-subset stat assertion; never the whole registry, never the http3-specific counters envoy-go omits.
- **`reference_docker_probe_bridge_network` / `reference_host_gateway_ip_docker_desktop` / `reference_envoy_contrib_image_tagging`** — the Docker UDP/contrib-image trio; verify decode RAN.
- **`reference_differential_backendcount_min_one`** — `BackendCount() 1` for the backend-less `direct_response` fixture.
- **`reference_fatalf_makes_assertions_unreachable`** — `Errorf` per stat property in `AssertStats`.
- **`reference_roadmap_split_phase_row_done`** — row 61 flips `done` at THIS (final-leg) six-gate, not before.
- **`reference_sentinel_deferred_sentence_live_vs_historical`** — after 61.3 the HTTP/3 deferred sentence STAYS exactly one live "candidates:" match (the family stays open).
- **`reference_next_prompt_tracked_despite_gitignore`** — edit `next-prompt.txt` in the worktree; fold into the squash; locate commits by SUBJECT, never position.

---

## Design pins settled here (the 61.3 D-question resolutions over SPEC-61 §8 + ADR-0281 §Consequences)

**FIXTURE NUMBER = `0104-http3-downstream-get` (SPEC/router `0102` corrected).** `0102`/`0103` were consumed by phases 59/60.2 after the SPEC was written; the tail is `0103`, next free is `0104`. Fixtures 105 → 106. (RE-VERIFY at IMPL start; take the next free if a parallel row raced.)

**OBSERVABLE = status 200 + body `"OK\n"` (byte-stable) + a NAMED downstream-stat subset — NOT full response-header byte-parity, NOT `HTTPExpectations`.** The H3 wire is QUIC-encrypted + QPACK-compressed — raw wire bytes are NOT cross-side-comparable (each codec frames differently below the decoded response). The comparable observable is the DECODED response: status + body. The `0104` driver's `DriveReference`/`DriveSubject` call `H3RoundTrip`, return an error on `status != 200` (fails the fixture — the status assertion), and return the `"OK\n"` body for the runner's `CompareBytes` (the body-parity assertion). The named-stat subset (`http.<prefix>.downstream_rq_2xx == 1`, `downstream_rq_total == 1`, both sides) is asserted in `AssertStats`. `HTTPExpectations` is NOT implemented (its runner-internal HTTP/1-over-TCP client cannot reach a QUIC listener). Rejected alternative: full response-header byte-parity via a normalized serialization — fragile (`date` is a timestamp; header ordering + casing vary by codec) and unnecessary for the minimal GET→200 proof; the header FIDELITY (`server`/`content-length`) is verified subject-side (Task 6's unit test) + optionally asserted as a named header subset in `AssertStats`.

**HARNESS SURGERY = a transport-aware `/udp` exposure + a `udpAddrs` map + `ListenerUDPAddr` + a runner marker interface.** Add `udpAddrs map[int]string` beside `tcpAddrs` (`harness.go:100`) on `ReferenceProxy`; in the two MAPPING starters (`StartReferenceProxy` `:107`, `StartReferenceProxyWithMounts` `:170` — NOT the boot-reject `tryStartReferenceProxy`, which maps nothing), expose the listener port as `/udp` and `MappedPort(…/udp)` into `udpAddrs`. **Transport-awareness:** a QUIC listener port must be exposed as `/udp` (its container publishes UDP, not TCP — a `MappedPort(…/tcp)` on it would FAIL). The minimal, backward-compatible seam: a new optional driver method `ReferenceListenerIsUDP() bool` (default absent/false → TCP, the existing behavior for all 105 fixtures), consulted by the runner to (a) tell the starter to expose that port as `/udp` and (b) pass `ref.ListenerUDPAddr(port)` to `DriveReference`. RE-DERIVE the CLEANEST plumbing at IMPL (the starters take `listenerPorts ...int`; threading a per-port transport may need a small signature/adjacent-map change OR a parallel `StartReferenceProxyUDP`-style path — the IMPL decides; keep the 105 existing fixtures byte-identical). Rejected alternative: exposing EVERY listener port as both `/tcp` and `/udp` unconditionally — a `MappedPort` on the unpublished transport errors, so exposure must be transport-selective. The SUBJECT side needs NO change (binds UDP directly, reports via the `\S+` sentinel).

**H3 CLIENT = `test/helpers/h3.go` `H3RoundTrip`, one client drives BOTH sides.** A quic-go `http3.Transport` client modeled on `H2RoundTrip` (`h2.go:33`): PIN the dialed UDP `host:port` (the container advertises its INTERNAL port via Alt-Svc / server-preferred-address — the client MUST ignore that and dial the host-mapped addr), fresh transport per call, `InsecureSkipVerify` + `NextProtos: ["h3"]` (self-signed test cert). quic-go's `http3.Transport` pins the dial via its `Dial` func field (RE-DERIVE the exact v0.54.1 `Dial func(ctx, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error)` signature via `go doc github.com/quic-go/quic-go/http3.Transport` at IMPL) — the closure dials the PINNED addr regardless of the request URL authority. Reused by BOTH `DriveReference` (against `ref.ListenerUDPAddr(...)`) and `DriveSubject` (against the subject's sentinel UDP addr). This is TEST code — quic-go in `test/helpers` does NOT breach the production import-hygiene gate.

**writeH3Reply FIDELITY PICKUP = synthesize `server: envoy` + `content-length` (the ADR-0281 §Consequences deferral).** The 61.2 arm was minimal (SPEC-61 §3.5). For cross-side response fidelity the subject's H3 `direct_response` should carry the same non-body-affecting fidelity headers the reference emits: `server: envoy` and `content-length: <len(body)>`. Add these in `writeH3Reply` (or `runH3` before it) — RE-DERIVE how the H1 path (`connection.go`/`codec.go`) synthesizes `server`/`content-length` and MIRROR it (envoy-go emits `server: envoy` deliberately as a faithful reimplementation). `date` is a timestamp (varies cross-side) — the H3 path MAY emit it (quic-go/`http3.Server` may add it) but the differential does NOT byte-compare headers, so `date` needs no synthesis or normalization here. Rejected alternative: leaving writeH3Reply minimal and asserting only body/stats — acceptable for the GET→200 proof, but ADR-0281 §Consequences explicitly pinned the synthesis as a 61.3 pickup, and the fidelity is cheap + unit-testable subject-side. Verify via a Task-6 unit test (`httptest.NewRecorder` — no quic-go) asserting the H3 response carries `server`/`content-length`; the differential (Task 8) is the cross-side confirmation.

**ACCESS-LOG/SPAN Protocol STRING — verified subject-side, UNasserted cross-side (deferred confirmation).** envoy-go emits `Protocol: "HTTP/3"` (`accesslog_emit.go:166/:186`). ADR-0281 §Consequences asked 61.3 to "confirm the reference's exact wire string." Capturing the reference container's ACCESS LOG cross-side in the harness is involved (the access-log asserter path + a log-capture mount) and out of proportion to the minimal GET proof; the reference's `%PROTOCOL%` for H3 is `"HTTP/3"` (SPEC-61 §8 — the Lua `:protocol()` string), which MATCHES. Pin `"HTTP/3"` as VERIFIED-BY-INSPECTION and UNasserted cross-side in the fixture (per the `reference_tracing_upstream_cluster_framework_gap` UNassert-until-proven discipline). If the IMPL finds a cheap `AccessLogAsserter` path, it MAY assert it; otherwise record the deferral in ADR-0282 §Consequences. Do NOT block row 61 → `done` on it.

**ADR-0282 = a NEW per-leg ADR (the harness's first non-TCP transport).** The UDP/QUIC harness surgery + the reusable `H3RoundTrip` client is a distinct, reusable test-infra seam (every future QUIC/UDP fixture — upstream H3, alt-svc, QUIC robustness — reuses it), warranting its own ADR rather than folding into ADR-0281 (the codec-arm ADR). §Context + §Decision + §Consequences land at this IMPL (ADR-0044). DECISIONS tail **ADR-0281 → ADR-0282** (next-free **ADR-0283**). Documented alternative: fold into ADR-0281 (the IMPL may choose this if it judges the surgery a mere completion of the codec-arm leg — record the choice either way).

**REFERENCE-CONTAINER RISK + the de-risk-first ordering.** Host→container UDP publishing for QUIC is the direction SPEC-61 §8 flagged untested — though the SPEC-61 §11 live probe DID drive an H3 GET over a published UDP port against `contrib-v1.37.2` on this machine (so it is de-risked on the CI substrate). Task 4 PROVES the reference H3 container path (a harness-level test: boot a reference QUIC listener, publish `/udp`, `H3RoundTrip` GET→200 non-vacuously) BEFORE the full fixture is built on top — an early failure signal. If it fails under the local Docker (Desktop's userland UDP proxy), the fallback is a `ReferenceLessFixture` (`RequiresReference()==false`) subject-only `0104` (loses the cross-side proof — an escalation, record it) OR gating the fixture on a Linux-bridge CI. Recommend proving the reference path first; escalate only on a real failure.

**Deferred-maintenance dispositions (carried from 61.1/61.2 — fold when the relevant file reopens, per PROGRESS):** 61.1 M6-1 (`quicAcceptLoop` no backoff), M-FB1 (`default_filter_chain` QUIC decode gap), M-FB2 (`quicTLSConfig`/`quicChain` map-nondeterminism) — all THREE are QUIC-robustness/SNI-multi-chain items, NOT exercised by the single-chain `0104` fixture; RE-DEFER to a dedicated QUIC-robustness row (record in PROGRESS + ADR-0282 §Consequences). 61.2 T5-M1/T5-M2/T5-B1/T7-M1 (runH3 encode-error counter gap [WriteH2 parity], POST-body test depth, `SetDownstreamLocalAddr` nil, `quicChain`/`quicTLSConfig` divergence-only-under-multi-chain) — none is exercised or worsened by `0104`; RE-DEFER unchanged. None is a security/resource/crash risk within the deferred scope.

---

## File structure (decomposition locked here)

**Production (modified — the ONLY production change; stdlib-only, no quic-go):**
- `internal/filter/hcm/h3dispatch.go` — MODIFY `writeH3Reply` (or add a helper called from `runH3`) to synthesize `server: envoy` + `content-length` on the H3 response (Task 6). Stays stdlib `net/http` — ZERO quic-go.

**Test infrastructure (created / modified):**
- `test/helpers/h3.go` — CREATE: `H3RoundTrip(ctx, addr, tlsConf, method, path, headers, body) (status int, respHeaders http.Header, respBody []byte, err error)` — a quic-go `http3.Transport` client pinning the UDP dial (Task 2).
- `test/differential/harness.go` — MODIFY: `ReferenceProxy` gains `udpAddrs map[int]string` (`:100`); `StartReferenceProxy` (`:107`) + `StartReferenceProxyWithMounts` (`:170`) gain transport-aware `/udp` exposure + `MappedPort(…/udp)` → `udpAddrs`; ADD `ListenerUDPAddr(containerPort int)` accessor beside `:227` (Task 3).
- `test/differential/runner_test.go` — MODIFY: for a QUIC-marked fixture (the new `fixture.ReferenceListenerIsUDP` marker), expose the ref listener port as `/udp` and pass `ref.ListenerUDPAddr(port)` to `DriveReference` (`:1181`/`:1217` region); ADD the `0104` blank-import (after `:130`) (Tasks 4/5/7).
- `test/differential/fixture/fixture.go` — ADD the optional marker interface `ReferenceListenerIsUDP interface { ReferenceListenerIsUDP() bool }` (Task 5).

**Fixture (created):**
- `test/fixtures/0104-http3-downstream-get/driver/driver.go` — CREATE: the driver (register, config templates for BOTH sides, `DriveReference`/`DriveSubject` via `H3RoundTrip`, `AssertStats` named-subset, `ReferenceListenerIsUDP() bool { return true }`, `BackendCount() 1`) (Task 7).
- `test/fixtures/0104-http3-downstream-get/expectations.yaml` + `README.md` — CREATE: prose docs (ADR-0019, not machine-evaluated) (Task 7).
- (Cert: an INLINE self-signed PEM `const` in `driver.go`, `fmt.Sprintf`'d into both config templates as `inline_string` — no container mount, mirroring the listener unit tests' inline-cert approach; RE-DERIVE / reuse the `internal/listener/manager_test.go` `testAlphaCertPEM`/`testAlphaKeyPEM` PEM values, or generate a fresh self-signed cert `const`.)

**Docs (this IMPL — the final-leg close):**
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the HTTP/3 section: cross-side-proven (Task 9).
- `docs/envoy-go/DECISIONS.md` — ADR-0282 §Context/§Decision/§Consequences (Task 9).
- `docs/envoy-go/phases/61-http3-downstream-listener/PROGRESS-61.3.md` — the checklist (updated each task).
- `docs/envoy-go/ROADMAP.md` — **row 61 → `done`** (ALL legs landed); the LIVE HTTP/3 deferred sentence UNCHANGED (Task 9).
- `docs/envoy-go/STATE.md` — active-phase header → phase 61.3 IMPL done / row 61 done (Task 9, controller).
- `next-prompt.txt` — the router roll (self-pick the next subject per the STANDING DIRECTIVE; sentinel re-checked — does NOT fire) (Task 9, folded into the squash).

---

## Task 1: PROGRESS scaffold + baselines + design pins + the SPEC/harness corrections

**Files:**
- Create: `docs/envoy-go/phases/61-http3-downstream-listener/PROGRESS-61.3.md`

- [ ] **Step 1: Author `PROGRESS-61.3.md`** — the baseline-counts table (fixtures 105 → **106**, all else +0 except DECISIONS tail → ADR-0282 and ROADMAP row 61 → **done**), the import-hygiene note (quic-go stays confined to `internal/listener/quic.go` in PRODUCTION; `test/helpers/h3.go` is test code and MAY import quic-go), the 61.3 design pins (copied from "Design pins settled here"), the two RE-DERIVATION CORRECTIONS recorded prominently (per `feedback_brief_citations_not_evidence`): **(a) the fixture is `0104`, not the SPEC/router `0102`** (0102/0103 consumed by phases 59/60.2); **(b) `HTTPExpectations` is TCP-only and UNUSABLE for H3** (runner_test.go:1273-1291 re-drives with an internal HTTP/1 client) — the `0104` driver asserts status in the Drive hooks + body via `CompareBytes` + stats via `AssertStats`, NOT via `HTTPExpectations`. Also record the carried 61.1/61.2 deferred-maintenance dispositions (M6-1/M-FB1/M-FB2/T5-*/T7-M1 — none exercised by `0104`, RE-DEFERRED). Model it on `PROGRESS-61.2.md`. (This step is folded into the PLAN commit — no separate code.)

- [ ] **Step 2: Commit** (folded into the PLAN stage commit by the controller).

---

## Task 2: `test/helpers/h3.go` — the shared `H3RoundTrip` client (pinned UDP dial)

**Files:**
- Create: `test/helpers/h3.go`
- Test: `test/helpers/h3_test.go`

**Interfaces:**
- Produces: `func H3RoundTrip(ctx context.Context, addr string, tlsConf *tls.Config, method, path string, headers http.Header, body []byte) (status int, respHeaders http.Header, respBody []byte, err error)` — a single-shot quic-go `http3.Transport` GET/POST that PINS the dialed UDP `addr` (ignores Alt-Svc), fresh transport per call. Used by BOTH sides of the `0104` fixture (Task 7).
- Consumes: `github.com/quic-go/quic-go/http3` + `github.com/quic-go/quic-go` (test code — allowed).

- [ ] **Step 1: Write the failing test.** In `test/helpers/h3_test.go` — stand up a LOCAL in-process quic-go `http3.Server` on a UDP socket serving `/health → 200 "OK\n"`, then drive it with `H3RoundTrip` against the bound UDP addr:

```go
package helpers

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
)

// TestH3RoundTrip_GET stands up a local http3.Server and confirms H3RoundTrip
// completes a pinned-addr GET returning status + body over HTTP/3.
func TestH3RoundTrip_GET(t *testing.T) {
	cert := testSelfSignedTLS(t) // RE-DERIVE: an existing test-cert helper in test/helpers,
	                             // or generate a self-signed cert here (ALPN "h3").
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	defer udpConn.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("OK\n"))
	})
	srv := &http3.Server{Handler: mux, TLSConfig: cert}
	go func() { _ = srv.Serve(udpConn) }()
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	clientTLS := &tls.Config{NextProtos: []string{"h3"}, InsecureSkipVerify: true} //nolint:gosec // local test
	status, _, body, err := H3RoundTrip(ctx, udpConn.LocalAddr().String(), clientTLS, http.MethodGet, "/health", nil, nil)
	if err != nil {
		t.Fatalf("H3RoundTrip: %v", err)
	}
	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}
	if string(body) != "OK\n" {
		t.Errorf("body = %q, want OK\\n", string(body))
	}
}
```

*(RE-DERIVE `testSelfSignedTLS` — check `test/helpers` for an existing self-signed-cert helper [there is inline cert plumbing in the listener tests; `test/helpers` may have its own]. If none, generate an ECDSA P-256 self-signed cert in the test with `NextProtos: ["h3"]`. The server-side `http3.Server.Serve(net.PacketConn)` v0.54.1 signature is RE-DERIVED via `go doc`.)*

- [ ] **Step 2: Run to verify RED.** `go test ./test/helpers/ -run 'TestH3RoundTrip' -count=1 -v` — FAIL (`H3RoundTrip` undefined).

- [ ] **Step 3: Implement `H3RoundTrip`.** In `test/helpers/h3.go` — mirror `h2.go`'s pinned-dial discipline over `http3.Transport`:

```go
package helpers

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net/http"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

// H3RoundTrip issues a single HTTP/3 request to the QUIC listener at the pinned
// UDP addr and returns the decoded status/headers/body. Like H2RoundTrip
// (test/helpers/h2.go), it PINS the dialed address — the http3.Transport.Dial
// closure dials `addr` regardless of the request URL authority and IGNORES any
// Alt-Svc / server-preferred-address the server advertises (a reference Envoy
// container advertises its INTERNAL port, not the host-mapped one). A fresh
// Transport per call (no connection caching) keeps cross-side runs deterministic.
// Test-only helper — quic-go here does NOT touch the production import-hygiene
// gate (quic-go is production-confined to internal/listener/quic.go).
func H3RoundTrip(ctx context.Context, addr string, tlsConf *tls.Config, method, path string, headers http.Header, body []byte) (int, http.Header, []byte, error) {
	rt := &http3.Transport{
		TLSClientConfig: tlsConf,
		QUICConfig:      &quic.Config{},
		// Dial pins the UDP dial to `addr` regardless of the request URL host.
		// RE-DERIVE the EXACT v0.54.1 Dial signature via `go doc http3.Transport`
		// — it is approximately:
		//   Dial func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error)
		Dial: func(ctx context.Context, _ string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
			return quic.DialAddr(ctx, addr, tlsCfg, cfg)
		},
	}
	defer rt.Close()

	var rdr io.Reader
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	// The URL host is cosmetic (the Dial closure pins the real dial); use `addr`
	// so req.Host is well-formed.
	req, err := http.NewRequestWithContext(ctx, method, "https://"+addr+path, rdr)
	if err != nil {
		return 0, nil, nil, err
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, resp.Header, nil, err
	}
	return resp.StatusCode, resp.Header, respBody, nil
}
```

*(RE-DERIVE at IMPL: (a) the EXACT `http3.Transport.Dial` field signature + `quic.DialAddr` signature for v0.54.1 [`go doc github.com/quic-go/quic-go.DialAddr` / `go doc github.com/quic-go/quic-go/http3.Transport`] — the closure above is the shape, confirm the types. (b) Whether `http3.Transport` even needs a custom `Dial` to pin, or whether dialing the URL host `addr` directly already pins it [it may — `H2RoundTrip` pins by pre-dialing; `http3.Transport` derives the dial from the URL host, which IS `addr` here, so the Alt-Svc concern is the real reason for the explicit `Dial`]. Confirm the client does NOT follow Alt-Svc to the container's internal port. (c) Confirm `resp.Header` carries the H3 response headers [it does — quic-go decodes QPACK into `http.Header`].)*

- [ ] **Step 4: Run to verify GREEN.** `go test ./test/helpers/ -run 'TestH3RoundTrip' -count=1 -v` — PASS (200, body `OK\n`).

- [ ] **Step 5: Per-task gates.** `gofmt -l test/helpers/` · `golangci-lint run ./test/helpers/...` · `go vet ./...` · `go build ./...`.

- [ ] **Step 6: Commit** — `phase 61.3: test/helpers/h3.go H3RoundTrip (pinned-UDP-dial quic-go http3.Transport client)`.

---

## Task 3: Harness UDP exposure — `udpAddrs` + transport-aware `/udp` + `ListenerUDPAddr`

**Files:**
- Modify: `test/differential/harness.go:100` (`ReferenceProxy.udpAddrs`), `:107-155` (`StartReferenceProxy`), `:170-219` (`StartReferenceProxyWithMounts`), `:227` (add `ListenerUDPAddr`)
- Test: `test/differential/harness_test.go` (a harness-level UDP-exposure test)

**Interfaces:**
- Produces: `ReferenceProxy.udpAddrs map[int]string`; `func (r *ReferenceProxy) ListenerUDPAddr(containerPort int) string` (returns `r.udpAddrs[containerPort]`); transport-aware exposure — a listener port exposed as `/udp` maps into `udpAddrs`, as `/tcp` into `tcpAddrs` (existing).
- Consumes: `nat.Port` (`harness.go:19`), testcontainers `MappedPort`.

- [ ] **Step 1: Write the failing test.** In `harness_test.go`, boot a minimal reference container with a UDP-exposed port and assert `ListenerUDPAddr` returns a mapped `host:port`. The cheapest non-flaky harness test may just assert the accessor + map plumbing compile and return the mapped addr for a known-published `/udp` port. RE-DERIVE the existing `harness_test.go` container-start test shape (there are `freeTCPPort`/`freeTCPPortBlock` helpers at `:162`/`:194` and harness tests already boot containers). A focused test:

```go
// TestReferenceProxy_UDPExposure verifies a QUIC/UDP listener port is exposed as
// /udp and ListenerUDPAddr returns its host-mapped host:port. (RE-DERIVE the
// minimal reference bootstrap that binds a UDP/QUIC listener — reuse the 0104
// reference template once Task 7 lands, or a minimal quic_options listener here.)
func TestReferenceProxy_UDPExposure(t *testing.T) {
	// boot a reference QUIC listener on container port P via the UDP-aware
	// start path; assert ref.ListenerUDPAddr(P) != "" and is a valid host:port.
}
```

*(This test is genuinely integration-heavy [it boots a container]. If a full container boot is too slow/flaky for a unit gate, the IMPL MAY prove the plumbing indirectly: a table-level assertion that the exposure list contains the `/udp` form + `udpAddrs` is populated, deferring the true end-to-end proof to Task 4's de-risk test. RE-DERIVE the lightest sufficient test; do NOT ship untested map plumbing.)*

- [ ] **Step 2: Run to verify RED.** `go test ./test/differential/ -run 'TestReferenceProxy_UDPExposure' -count=1 -v` — FAIL (`ListenerUDPAddr` undefined / `udpAddrs` unpopulated).

- [ ] **Step 3: Implement the UDP exposure.**
  (a) `harness.go:100` — add to `ReferenceProxy`:
```go
	udpAddrs  map[int]string // in-container UDP/QUIC listener port → host host:port (phase 61.3)
```
  (b) In `StartReferenceProxy` (`:107`) + `StartReferenceProxyWithMounts` (`:170`): make the per-listener-port exposure + mapping TRANSPORT-AWARE. The starters receive `listenerPorts ...int`; the caller (the runner) knows which are UDP. RE-DERIVE the cleanest signal — the recommended seam is a parallel `udpListenerPorts []int` param (or a `map[int]string` port→transport) threaded from the runner. For each UDP port `p`: `exposed = append(exposed, fmt.Sprintf("%d/udp", p))` and `mapped, err := c.MappedPort(ctx, nat.Port(fmt.Sprintf("%d/udp", p)))` → `udp[p] = "127.0.0.1:" + mapped.Port()`; assign `udpAddrs: udp` on the struct (beside the existing `tcpAddrs: tcp` at `:155`/`:219`). TCP ports keep the existing `/tcp` path. Admin stays `9901/tcp` (`:138`/`:202`).
  (c) Add the accessor beside `:227`:
```go
// ListenerUDPAddr returns the host host:port that maps the given in-container
// UDP/QUIC listener port (phase 61.3 — the harness's first non-TCP transport).
func (r *ReferenceProxy) ListenerUDPAddr(containerPort int) string {
	return r.udpAddrs[containerPort]
}
```

*(RE-DERIVE the EXACT `MappedPort` return shape [`nat.Port`; `.Port()` yields the number] and how `tcpAddrs` values are formatted at `:150`/`:214` — MIRROR it for `udpAddrs`. Confirm testcontainers publishes `/udp` exposed ports [it does — the `ExposedPorts`/`nat.Port` machinery is transport-generic]. If threading `udpListenerPorts` through both starters is invasive, an acceptable minimal alternative is a dedicated UDP-only start helper the runner calls for QUIC fixtures — the IMPL decides; keep the 105 TCP fixtures byte-identical.)*

- [ ] **Step 4: Run to verify GREEN.** `go test ./test/differential/ -run 'TestReferenceProxy_UDPExposure' -count=1 -v` — PASS. Then `go test ./test/differential/ -run 'TestReferenceProxy' -count=1` (no regression to existing TCP harness tests).

- [ ] **Step 5: Per-task gates.** `gofmt -l test/differential/` · `golangci-lint run ./test/differential/...` · `go vet ./...` · `go build ./...`.

- [ ] **Step 6: Commit** — `phase 61.3: harness UDP exposure (udpAddrs + transport-aware /udp + ListenerUDPAddr)`.

---

## Task 4: Reference-container H3 de-risk — prove host→container UDP publishing serves an H3 GET

**Files:**
- Test: `test/differential/harness_test.go` (a reference-H3 integration proof) OR a throwaway `internal/…` probe (RE-DERIVE the cleanest home; it needs `H3RoundTrip` [Task 2] + the UDP-aware start [Task 3])

**Interfaces:**
- Consumes: `H3RoundTrip` (Task 2), the UDP-aware reference start + `ListenerUDPAddr` (Task 3), a reference QUIC bootstrap (a minimal `udp_listener_config` + quic transport socket + HCM `codec_type HTTP3` + `/health → direct_response 200 "OK\n"` — the same shape Task 7's reference template uses; RE-DERIVE/prototype it here).
- Produces: the NON-VACUOUS proof that a reference contrib-Envoy H3 container is reachable from the host over a published `/udp` port and serves a GET→200 — the load-bearing de-risk for the whole leg.

- [ ] **Step 1: Write the de-risk test.** Boot a reference `contrib-v1.37.2` container with a QUIC/H3 listener on an exposed `/udp` port; `H3RoundTrip` a GET `/health` against `ref.ListenerUDPAddr(port)`; assert status 200 + body `"OK\n"`; then scrape the reference admin `/stats` and assert `http.<prefix>.downstream_rq_2xx == 1` (the NON-VACUOUS decode signal, per `reference_docker_probe_bridge_network` — never trust a green that never decoded).

```go
// TestReferenceH3_ServesGET is the leg-61.3 de-risk: it proves the reference
// contrib-Envoy H3 container is reachable from the host over a published /udp
// port and serves a GET→200 (host→container UDP publishing — the SPEC-61 §8
// untested direction; PROVEN on this machine by the SPEC-61 §11 probe, re-proven
// here in the harness). If this fails under the local Docker (Desktop userland
// UDP proxy), ESCALATE — the fallback is a ReferenceLess subject-only 0104.
func TestReferenceH3_ServesGET(t *testing.T) {
	// boot reference QUIC listener (port P, /udp exposed); H3RoundTrip GET /health;
	// assert 200 + "OK\n"; scrape admin /stats; assert downstream_rq_2xx == 1.
}
```

- [ ] **Step 2: Run it.** `go test ./test/differential/ -run 'TestReferenceH3_ServesGET' -count=1 -v` (this boots Docker — it runs in the same environment the full differential runs). Expect PASS (200 + a non-zero decode). **If it FAILS with a UDP-reachability error**, STOP and escalate to the controller with the exact failure (per the reference-container-risk pin) — do NOT paper over it; the whole cross-side leg depends on this path.

- [ ] **Step 3: Record the de-risk evidence** in PROGRESS: the observed status, body, `downstream_rq_2xx` value, and the Docker substrate (Linux bridge vs Desktop). This is the non-vacuous-verify record.

- [ ] **Step 4: Per-task gates.** `gofmt -l test/differential/` · `golangci-lint run ./test/differential/...` · `go vet ./...` · `go build ./...`.

- [ ] **Step 5: Commit** — `phase 61.3: reference contrib-Envoy H3 container de-risk (host→container UDP GET→200, non-vacuous)`.

*(NOTE: Task 4's test may be SUPERSEDED by Task 8's full fixture run — it is a de-risk scaffold. The IMPL MAY keep it as a fast focused smoke test OR fold it into the fixture once `0104` passes. RE-DERIVE and record which.)*

---

## Task 5: Runner surgery — the `ReferenceListenerIsUDP` marker + UDP-addr dispatch to `DriveReference`

**Files:**
- Modify: `test/differential/fixture/fixture.go` (add the optional marker interface)
- Modify: `test/differential/runner_test.go:1146,1173-1181,1217` (expose the ref port as `/udp` + pass the UDP addr to `DriveReference` for a QUIC fixture)
- Test: covered end-to-end by Task 8 (the runner change is exercised only by a QUIC fixture, which lands at Task 7) — a focused unit test of the marker dispatch is OPTIONAL (RE-DERIVE if the runner logic is unit-isolable)

**Interfaces:**
- Produces: `type ReferenceListenerIsUDP interface { ReferenceListenerIsUDP() bool }` (`fixture.go`); the runner, when `d.(ReferenceListenerIsUDP)` reports true, exposes the reference listener port as `/udp` and passes `ref.ListenerUDPAddr(d.ReferenceListenerPort())` to `d.DriveReference` (instead of the TCP `ref.ListenerAddr(...)`).
- Consumes: `ListenerUDPAddr` (Task 3), the UDP-aware start (Task 3).

- [ ] **Step 1: Add the marker interface.** In `fixture/fixture.go` (beside the other optional interfaces, e.g. after `StatsAsserter` `:75`):
```go
// ReferenceListenerIsUDP marks a fixture whose reference listener is a UDP/QUIC
// (HTTP/3) listener: the runner exposes its port as /udp (not /tcp) and passes
// the UDP-mapped host addr to DriveReference. Default absent → TCP (all pre-61.3
// fixtures). Phase 61.3.
type ReferenceListenerIsUDP interface {
	ReferenceListenerIsUDP() bool
}
```

- [ ] **Step 2: Thread it through the runner.** In `runner_test.go` (RE-DERIVE the EXACT lines against the master tip — the region around `:1140-1181`/`:1217`):
  (a) Detect the marker early (near `refPorts` at `:1142-1146`): `refIsUDP := false; if u, ok := d.(fixture.ReferenceListenerIsUDP); ok { refIsUDP = u.ReferenceListenerIsUDP() }`.
  (b) When `refIsUDP`, tell the starter to expose `refPorts` as `/udp` (thread the transport signal into `StartReferenceProxy*` per Task 3's chosen seam).
  (c) When `refIsUDP`, compute `refAddr := ref.ListenerUDPAddr(d.ReferenceListenerPort())` instead of `ref.ListenerAddr(...)` (`:1181`); `DriveReference(ctx, refAddr)` (`:1217`) then receives the UDP addr.
  (The subject side is UNCHANGED — `subj.ListenerAddr(d.SubjectListenerName())` at `:1243` returns the sentinel-reported UDP addr for a QUIC subject listener.)

*(RE-DERIVE the EXACT runner control flow — the cross-side path spans `:1140-1320`; the changes are localized to ref-port exposure + the `refAddr` computation. Keep the non-UDP path byte-identical [the 105 TCP fixtures]. Confirm `ProbeAdmin`/admin stays `/tcp` [admin is HTTP-over-TCP even for a QUIC data listener].)*

- [ ] **Step 3: Verify no regression.** `go build ./...` + `go test ./test/differential/ -run 'TestDifferential' -count=1` restricted to a couple of existing TCP fixtures (e.g. `-run 'TestDifferential/0003-http11-routing'`) — still PASS (the marker is absent → TCP path unchanged). *(The QUIC path is exercised by Task 8; there is no QUIC fixture yet.)*

- [ ] **Step 4: Per-task gates.** `gofmt -l test/differential/ test/differential/fixture/` · `golangci-lint run ./test/differential/...` · `go vet ./...` · `go build ./...`.

- [ ] **Step 5: Commit** — `phase 61.3: runner ReferenceListenerIsUDP marker + UDP-addr dispatch to DriveReference`.

---

## Task 6: writeH3Reply fidelity pickup — synthesize `server: envoy` + `content-length` (ADR-0281 §Consequences deferral)

**Files:**
- Modify: `internal/filter/hcm/h3dispatch.go` (`writeH3Reply` or a helper called from `runH3`)
- Test: `internal/filter/hcm/h3dispatch_test.go` (extend the existing `TestWriteH3Reply_*` tests)

**Interfaces:**
- Produces: the H3 response carries `server: envoy` + `content-length: <len(body)>` (matching the reference Envoy's H3 `direct_response`), added without disturbing the existing pseudo-header-skip / body-write behavior.
- Consumes: nothing new (stdlib `net/http` only — ZERO quic-go; the production import-hygiene gate holds).

- [ ] **Step 1: Write the failing test.** Extend `h3dispatch_test.go`:
```go
// TestWriteH3Reply_SynthesizesServerAndContentLength verifies the H3 response
// carries the fidelity headers the reference Envoy emits for a direct_response:
// server: envoy and content-length. Phase 61.3 (ADR-0282) — the 61.2 arm was
// minimal (ADR-0281 §Consequences deferred this).
func TestWriteH3Reply_SynthesizesServerAndContentLength(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := writeH3Reply(rec, 200, nil, []byte("OK\n")); err != nil {
		t.Fatalf("writeH3Reply: %v", err)
	}
	res := rec.Result()
	if got := res.Header.Get("server"); got != "envoy" {
		t.Errorf("server = %q, want envoy", got)
	}
	if got := res.Header.Get("content-length"); got != "3" {
		t.Errorf("content-length = %q, want 3", got)
	}
	if rec.Body.String() != "OK\n" {
		t.Errorf("body = %q, want OK\\n", rec.Body.String())
	}
}
```
*(RE-DERIVE how the H1 path emits `server`/`content-length` — `internal/filter/hcm/codec.go` / `connection.go` [`writeH1Reply`/`writeStatusReply`] — and MATCH the exact casing/value envoy-go uses on H1 [`server: envoy`; content-length as the decimal body length]. If the router action already supplies a `content-length` header, do NOT double-set it [prefer the action's value; only synthesize when absent] — RE-DERIVE whether `direct_response` supplies one. Confirm whether quic-go's `http.ResponseWriter` auto-adds `content-length` [stdlib `net/http` does when the body fits one Write; if quic-go mirrors that, synthesizing may be redundant or conflict — RE-DERIVE and prefer the minimal correct approach: set `server` always [Envoy-fidelity], set `content-length` only if not already present].)*

- [ ] **Step 2: Run to verify RED.** `go test ./internal/filter/hcm/ -run 'TestWriteH3Reply_SynthesizesServerAndContentLength' -count=1 -v` — FAIL (`server`/`content-length` absent).

- [ ] **Step 3: Implement.** In `writeH3Reply` (or a helper `runH3` calls just before it), after copying the action headers and before `WriteHeader`:
```go
	if h.Get("server") == "" {
		h.Set("server", "envoy")
	}
	if len(body) > 0 && h.Get("content-length") == "" {
		h.Set("content-length", strconv.Itoa(len(body)))
	}
```
*(RE-DERIVE the exact placement so it composes with the existing pseudo-header-skip loop [`h3dispatch.go:35-41`] and does not conflict with quic-go's own framing. `strconv` import if not present. If the H1 path uses a shared constant/helper for `server: envoy`, reuse it rather than a string literal.)*

- [ ] **Step 4: Run to verify GREEN.** `go test ./internal/filter/hcm/ -run 'TestWriteH3Reply' -count=1 -v` — all PASS (the new test + the existing 61.2 `TestWriteH3Reply_StatusHeadersBody`/`_EmptyBody`; confirm the empty-body case does NOT get a `content-length: 0` if the reference omits it — RE-DERIVE the reference's headers-only-response shape).

- [ ] **Step 5: Import-hygiene re-check.** `go list -deps ./internal/filter/hcm | grep -i quic-go || echo HCM-NO-QUICGO` → `HCM-NO-QUICGO` (the fidelity pickup stays stdlib-only).

- [ ] **Step 6: Per-task gates.** `gofmt -l internal/filter/hcm/` · `golangci-lint run ./internal/filter/hcm/...` · `go vet ./...` · `go build ./...`. Run `go test ./internal/filter/hcm/ ./internal/listener/ -count=1` (the 61.2 subject-side `TestQUICListener_ServesH3GET` still passes with the new headers).

- [ ] **Step 7: Commit** — `phase 61.3: writeH3Reply synthesizes server: envoy + content-length (61.2-deferred H3 response fidelity)`.

---

## Task 7: The `0104-http3-downstream-get` cross-side fixture

**Files:**
- Create: `test/fixtures/0104-http3-downstream-get/driver/driver.go`
- Create: `test/fixtures/0104-http3-downstream-get/expectations.yaml`, `README.md`
- Modify: `test/differential/runner_test.go` (add the blank-import after `:130`)

**Interfaces:**
- Consumes: `H3RoundTrip` (Task 2), the UDP harness + `ListenerUDPAddr` (Task 3), the `ReferenceListenerIsUDP` marker + runner dispatch (Task 5), the writeH3Reply fidelity (Task 6, subject side).
- Produces: the cross-side H3-GET differential — the leg's headline proof.

- [ ] **Step 1: Author the driver.** In `driver/driver.go`, following the `0003-http11-routing` template shape:
```go
package driver

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
	"crypto/tls"
)

const fixtureName = "0104-http3-downstream-get"

func init() { fixture.RegisterFixture(fixtureName, &h3Driver{}) }

type h3Driver struct{}

func (h3Driver) BackendCount() int                    { return 1 } // direct_response — throwaway backend (reference_differential_backendcount_min_one)
func (h3Driver) SubjectListenerName() string          { return "l_h3" }
func (h3Driver) ReferenceListenerPort() int           { return 15104 }
func (h3Driver) ReferenceListenerIsUDP() bool         { return true } // phase 61.3 — the QUIC/UDP marker

func (h3Driver) ReferenceBootstrap(backendPorts []int) string {
	return fmt.Sprintf(referenceTmpl /* , any port substitutions */)
}
func (h3Driver) SubjectConfig(refListenerPort, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return fmt.Sprintf(subjectTmpl, subjListenerPort, subjAdminPort)
}

// drive issues one H3 GET /health and returns the body (for CompareBytes). A
// non-200 status returns an error (the status assertion — the runner fails the
// fixture). One shared client (H3RoundTrip) drives BOTH sides.
func (h3Driver) drive(ctx context.Context, addr string) ([]byte, error) {
	tlsConf := &tls.Config{NextProtos: []string{"h3"}, InsecureSkipVerify: true} //nolint:gosec // differential test
	status, _, body, err := helpers.H3RoundTrip(ctx, addr, tlsConf, http.MethodGet, "/health", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("H3 GET %s: %w", addr, err)
	}
	if status != 200 {
		return nil, fmt.Errorf("H3 GET %s: status %d, want 200", addr, status)
	}
	return body, nil
}
func (d h3Driver) DriveReference(ctx context.Context, addr string) ([]byte, error) { return d.drive(ctx, addr) }
func (d h3Driver) DriveSubject(ctx context.Context, addr string) ([]byte, error)   { return d.drive(ctx, addr) }

// ProbeAdmin issues GET /ready against each admin endpoint (RE-DERIVE the
// standard ProbeAdmin shape — helpers.HTTPGetReadyRaw, per the 0067 precedent).
func (h3Driver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	// ... helpers.HTTPGetReadyRaw both sides ...
}

// AssertStats asserts the NAMED downstream-stat subset BOTH sides emit — NOT the
// http3-specific counters envoy-go omits (reference_stats_sink_emits_used_only),
// NOT a whole-map equality. Errorf per property (reference_fatalf_makes_...).
func (h3Driver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	refSt, err := scrapeStats(refAdminAddr) // RE-DERIVE: copy the 0067 local scrapeStats
	if err != nil {
		t.Fatalf("scrape ref /stats: %v", err)
	}
	subjSt, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats: %v", err)
	}
	// RE-DERIVE the EXACT stat_prefix (the HCM StatPrefix in the templates) and
	// the reference's exact stat names. Assert the shared codec-agnostic counters:
	for _, name := range []string{
		"http.ingress_http.downstream_rq_2xx",
		"http.ingress_http.downstream_rq_total",
	} {
		if refSt[name] < 1 {
			t.Errorf("ref %s = %d, want >=1", name, refSt[name])
		}
		if subjSt[name] < 1 {
			t.Errorf("subj %s = %d, want >=1", name, subjSt[name])
		}
	}
}

var _ fixture.Driver = h3Driver{}
var _ fixture.StatsAsserter = h3Driver{}
var _ fixture.ReferenceListenerIsUDP = h3Driver{}
```

*(RE-DERIVE at IMPL: (a) the FULL `fixture.Driver` method set + exact signatures against `fixture/fixture.go:15-52` [confirm `ProbeAdmin`, `SubjectConfig` param order]. (b) `scrapeStats` — copy the `0067` driver's LOCAL unexported copy [driver-side helpers are DUPLICATED, not imported from `test/differential`, per `reference_host_gateway_ip_docker_desktop`]; confirm the /stats parse yields `map[string]int64`. (c) The exact `stat_prefix` — set the HCM `StatPrefix` in BOTH templates to `ingress_http` and assert that prefix; RE-DERIVE the reference's actual stat names by scraping a booted reference [`http.ingress_http.downstream_rq_2xx`]. (d) Whether to ALSO assert a `listener.<addr>.downstream_cx_total` [the addr segment is dynamic — match by suffix or skip; the two `downstream_rq_*` counters are the robust core]. (e) OPTIONALLY assert `server`/`content-length` header parity by re-driving H3 in `AssertStats` and comparing the named header subset — only if it proves non-flaky; else rely on Task 6's subject-side unit test.)*

- [ ] **Step 2: Author the two config templates** (the `const referenceTmpl` / `const subjectTmpl` YAML). BOTH: a `udp_listener_config{quic_options:{}}` listener (reference on container port `15104`, subject on `%d`=`subjListenerPort`), an `envoy.transport_sockets.quic` transport socket wrapping a `DownstreamTlsContext{common_tls_context{tls_certificates:[inline cert/key PEM], alpn_protocols:["h3"]}}`, an HCM `codec_type: HTTP3`, `stat_prefix: ingress_http`, `http3_protocol_options:{}`, a route `/health → direct_response{status:200, body:{inline_string:"OK\n"}}`, the router filter, and admin on `9901` (reference) / `%d`=`subjAdminPort` (subject). RE-DERIVE the exact YAML from: the `0003` templates (HCM/route/admin shape) + the phase-61.1/61.2 QUIC listener proto (`internal/listener/manager_test.go` `mkQUICListener`/`mkQUICDownstreamTS`/`mkHCMFilterWithCodec` — translate the proto to YAML) + the SPEC-61 §11 arm-h3-get reference config (the PROVEN minimal accept config). Inline the cert/key as a Go `const` PEM (reuse `manager_test.go`'s `testAlphaCertPEM`/`testAlphaKeyPEM` values or a fresh self-signed cert) `fmt.Sprintf`'d in — NO container mount.

- [ ] **Step 3: Register the fixture.** Add the blank-import in `runner_test.go` after `:130`:
```go
	_ "github.com/pgdad/envoy-go/test/fixtures/0104-http3-downstream-get/driver"
```

- [ ] **Step 4: `go build ./...`** — the driver + templates compile; the interface asserts (`var _ fixture.Driver = …`) hold.

- [ ] **Step 5: Per-task gates.** `gofmt -l test/fixtures/0104-http3-downstream-get/... test/differential/` · `golangci-lint run ./test/fixtures/0104-http3-downstream-get/...` · `go vet ./...` · `go build ./...`.

- [ ] **Step 6: Commit** — `phase 61.3: 0104-http3-downstream-get cross-side fixture (driver + templates + registration)`.

---

## Task 8: The full cross-side run + per-assertion liveness breaks

**Files:**
- (No new files — runs the `0104` fixture end-to-end + proves each assertion bites)

**Interfaces:**
- Consumes: the whole 61.3 stack (Tasks 2–7).
- Produces: the GREEN cross-side `0104` differential + the break evidence (each load-bearing assertion proven live).

- [ ] **Step 1: Run the fixture.** `go test ./test/differential/ -run 'TestDifferential/0104-http3-downstream-get' -count=1 -v` (the FULL subtest path — a bare `-run '0104'` matches ZERO subtests, `reference_differential_run_selector`). Expect PASS: `CompareBytes` (both `"OK\n"`), `AssertStats` (both sides' `downstream_rq_2xx`/`downstream_rq_total` ≥ 1). VERIFY NON-VACUOUS: confirm the run actually decoded (the admin `downstream_rq_2xx` values are ≥ 1 on BOTH sides — never a green that never ran, `reference_docker_probe_bridge_network`).

- [ ] **Step 2: Prove the body-compare (`CompareBytes`) is live.** Temporarily break the SUBJECT's served body (e.g. change the subject template's `direct_response` body to `"BAD\n"`), re-run with `-count=1`; confirm the fixture FAILS at the byte-compare (`reference_differential_break_protocol_count1` — `-count=1` defeats the cached PASS). Confirm WHICH assertion fires (the `CompareBytes` mismatch, NOT an earlier drive error, `reference_deliberate_break_wrong_assertion`). Restore byte-identical; re-run GREEN.

- [ ] **Step 3: Prove the status assertion is live.** Temporarily change the subject `direct_response` status to `500`; re-run `-count=1`; confirm the fixture FAILS at the drive-hook `status != 200` error. Restore; re-run GREEN.

- [ ] **Step 4: Prove each `AssertStats` counter is live.** Temporarily change one asserted stat name to a non-existent one (or invert the `< 1` predicate); re-run `-count=1`; confirm `AssertStats` `Errorf`s. If a broken assertion is MASKED by an earlier failure, add an isolating break (`reference_deliberate_break_wrong_assertion`). Restore; re-run GREEN. Record WHICH assertion fired for each break in PROGRESS.

- [ ] **Step 5: Run under -race + the touched-package suite.** `go test ./test/helpers/ ./internal/filter/hcm/ ./internal/listener/ -race -count=1` (race-clean) + `go test ./test/differential/ -run 'TestDifferential/0104-http3-downstream-get' -race -count=1` (the H3 client + QUIC serve goroutines race-clean).

- [ ] **Step 6: Record the break evidence** in PROGRESS (each break: which assertion, the observed failure, restored-byte-identical confirmation via `git diff`).

- [ ] **Step 7: Commit** — `phase 61.3: 0104 cross-side run GREEN + per-assertion liveness breaks recorded`.

---

## Task 9: Docs + verify + ADR-0282 + ROADMAP row 61 → done + router roll

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (HTTP/3 section → cross-side-proven)
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0282 §Context/§Decision/§Consequences)
- Modify: `docs/envoy-go/phases/61-http3-downstream-listener/PROGRESS-61.3.md` (final checklist + six-gate + break evidence)
- Modify: `docs/envoy-go/ROADMAP.md` (**row 61 `in-progress` → `done`**; the LIVE HTTP/3 deferred sentence UNCHANGED)
- Modify: `docs/envoy-go/STATE.md` (active-phase header → phase 61.3 IMPL done / row 61 done; NEXT = the roller's self-pick) — controller
- Modify: `next-prompt.txt` (the router roll — self-pick the next subject per the STANDING DIRECTIVE; folded into the squash; `reference_next_prompt_tracked_despite_gitignore`)

- [ ] **Step 1: Extend the BEHAVIOR_CONTRACT HTTP/3 section.** RE-DERIVE the 61.1/61.2 HTTP/3 paragraphs and extend: the downstream QUIC/H3 GET→200 is now CROSS-SIDE PROVEN against `envoyproxy/envoy:contrib-v1.37.2` (fixture `0104`); the H3 response carries `server: envoy` + `content-length` (61.3 fidelity); the harness gained its first non-TCP transport (UDP/QUIC exposure + `H3RoundTrip`); upstream H3 / alt-svc / 0-RTT / h3spec / QUIC robustness / SNI-multi-chain still DEFERRED.

- [ ] **Step 2: Author ADR-0282** (RECOMMENDED — a new per-leg ADR for the harness's first non-TCP transport; the IMPL MAY fold into ADR-0281 and record that instead). §Context (the 61.1/61.2 substrate + codec proven subject-side only; the harness TCP-only barrier; the untested host→container UDP direction de-risked). §Decision (the UDP harness surgery — `udpAddrs`/transport-aware `/udp`/`ListenerUDPAddr`/the `ReferenceListenerIsUDP` marker + runner UDP-addr dispatch; the shared `test/helpers/h3.go` `H3RoundTrip` [test-code quic-go, production gate intact]; the writeH3Reply `server`/`content-length` fidelity pickup; the `0104` observable = status+body+named-stat-subset [NOT HTTPExpectations, NOT full-header byte-parity]; the fixture-number correction 0102→0104). §Consequences (fixtures 105 → 106; ROADMAP row 61 → `done` [all three legs in]; the HTTP/3 family STAYS OPEN; the reusable UDP/H3 harness capability for future QUIC fixtures; the carried deferred-maintenance [M6-1/M-FB1/M-FB2/T5-*/T7-M1] re-deferred to a QUIC-robustness row; the access-log/span Protocol string verified-by-inspection, UNasserted cross-side). DECISIONS tail **ADR-0281 → ADR-0282** (next-free **ADR-0283**).

- [ ] **Step 3: Run the six-gate** (in the worktree `.worktrees/phase-61.3-impl`):
```
gofmt -l .                                                    # GOFMT_CLEAN
golangci-lint run ./...                                       # clean, exit 0
go vet ./...                                                  # clean
go build ./... && echo BUILD_OK                               # BUILD_OK
go mod tidy -diff && echo MODTIDY_CLEAN                       # MODTIDY_CLEAN (no module change in 61.3)
go list -deps ./internal/filter/hcm | grep -i quic-go || echo HCM-NO-QUICGO  # HCM-NO-QUICGO (production gate intact)
go list -deps ./internal/tls | grep -i quic-go || echo TLS-NO-QUICGO         # TLS-NO-QUICGO
grep -rh '^func Fuzz' --include='*.go' . | wc -l             # 55 (+0)
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l            # 106 (+1: 0104)
go test ./test/helpers/ ./internal/filter/hcm/ ./internal/listener/ -count=1   # ok
go test ./test/helpers/ ./internal/filter/hcm/ ./internal/listener/ -race -count=1  # race-clean
```
Confirm quic-go appears in `./internal/listener` + `./test/helpers` deps (the latter is test-only, allowed) but NOT in `./internal/filter/hcm` or `./internal/tls` production deps.

- [ ] **Step 4: FULL non-differential suite + the full 106-dir differential** — DELEGATED to the controller on the frozen squash HEAD. The 105 pre-existing dirs stay byte-stable (61.3 changes NO TCP path; the harness UDP surgery is gated on the `ReferenceListenerIsUDP` marker only `0104` sets; the writeH3Reply fidelity change affects ONLY the H3 path, which only `0104` exercises). `0104` is the +1 new dir. Verify `0104` decoded non-vacuously (both sides' `downstream_rq_2xx ≥ 1`).

- [ ] **Step 5: Sentinel re-check + ROADMAP/STATE.** Run the three sentinel checks MECHANICALLY (`next-prompt.txt` §"AUTONOMOUS LOOP CONTROL"). Expected: (1) row 61 is now `done` → NO row prints `NOT DONE` for row 61; BUT the sentinel does NOT fire because (2) three live "candidates:" sentences (HTTP/3 STAYS OPEN — the family's deferred sentence remains one live match, `reference_sentinel_deferred_sentence_live_vs_historical`; xDS; Observability) AND (3) three never-opened families (gRPC/Runtime/WASM) + Operational-tooling. Confirm the HTTP/3 deferred sentence STAYS exactly one live match. **Flip ROADMAP row 61 `in-progress` → `done`** (ALL three legs landed, ADR-0106 / `reference_roadmap_split_phase_row_done`). Update STATE (controller). Roll `next-prompt.txt` to the roller's next stage — the STANDING DIRECTIVE self-picks the next subject (smallest-defensible candidate; record the pick + rejected alternatives in that stage's BRAINSTORM), OR advances any banked already-SPEC'd work; the sentinel governs termination (it does NOT fire here).

- [ ] **Step 6: Commit** — `phase 61.3: BEHAVIOR_CONTRACT + ADR-0282 + ROADMAP row 61 → done + STATE + router roll`.

---

## Self-review (run against SPEC-61 §8 + ADR-0281 §Consequences with fresh eyes)

**Spec coverage:**
- SPEC-61 §8 / ADR-0281 §Consequences "author the cross-side H3-GET fixture" → Task 7 (`0104`, number-corrected). ✓
- SPEC-61 §8 harness UDP surgery (three starters) → Task 3 (the two MAPPING starters; `tryStartReferenceProxy` maps nothing, so it is NOT surgery-relevant — a re-derivation refinement over SPEC-61 §8's "three starters"). ✓ *(Recorded: only 2 of the 3 starters map ports; the boot-reject starter needs no UDP change.)*
- SPEC-61 §8 `test/helpers/h3.go` `H3RoundTrip` → Task 2. ✓
- ADR-0281 §Consequences named-stat-subset cross-side → Task 7 `AssertStats` (`reference_stats_sink_emits_used_only`). ✓
- ADR-0281 §Consequences writeH3Reply `server`/`date`/`content-length` synthesis → Task 6 (`server`+`content-length`; `date` is a timestamp — NOT synthesized, NOT byte-compared, justified in the design pin). ✓
- ADR-0281 §Consequences exact access-log/span Protocol string cross-side → verified-by-inspection (`"HTTP/3"` matches), UNasserted cross-side (design pin; deferred confirmation, non-blocking). ✓
- ADR-0281 §Consequences "flip row 61 → done" → Task 9. ✓

**Placeholder scan:** every code step shows real code. The RE-DERIVE callouts are DELIBERATE, flagged IMPL-investigation for the genuinely-intricate harness/runner coupling (the exact `runner_test.go` control-flow region, the `http3.Transport.Dial` v0.54.1 signature, the transport-aware starter seam) — the harness is 3444 lines and shifts across phases; shipping hardcoded partial line-edits would be a worse failure than the flagged re-derive (the `feedback_brief_citations_not_evidence` discipline). No `TBD`/`handle edge cases`/`similar to Task N`.

**Type consistency:** `H3RoundTrip(ctx, addr, tlsConf, method, path, headers, body) (int, http.Header, []byte, error)` — consistent Tasks 2/4/7. `ListenerUDPAddr(containerPort int) string` — consistent Tasks 3/5/(harness). `ReferenceListenerIsUDP() bool` — consistent Tasks 5/7 (marker + driver). `udpAddrs map[int]string` — Task 3. `AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string)` — matches `fixture.StatsAsserter` (Task 7). `writeH3Reply(w, status, headers, body) error` — unchanged signature, Task 6 only adds header synthesis in the body.

**Gaps found + fixed:** (1) the SPEC/router fixture number `0102` was STALE (0102/0103 consumed since the SPEC) — corrected to `0104` (design pin + Task 1 correction). (2) SPEC-61 §8's "`HTTPExpectations`" implication was unusable (TCP-only internal client) — the observable moved to Drive-hook status + `CompareBytes` body + `AssertStats` named subset (Global Constraint + design pin). (3) SPEC-61 §8's "three container-starters" refined to TWO mapping starters (the boot-reject starter maps nothing). (4) the reference-container UDP-reachability RISK is de-risked FIRST (Task 4) before the fixture is built on top — an ordering the SPEC did not decompose.
