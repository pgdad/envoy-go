# Phase 18.2 — `ext-authz-grpc` (sibling SPEC stub)

> **This is a placeholder stub**, not the authoritative 18.2 SPEC. It is drafted at the phase-18 parent SPEC commit (the same commit that authored `docs/envoy-go/phases/18.1-ext-authz-http/SPEC.md`) so the phases directory carries a forward-pointer for 18.2. **It is superseded by `docs/envoy-go/phases/18.2-ext-authz-grpc/SPEC.md`** — the full sub-phase SPEC drafted at 18.2's lifecycle-state 1 → 2, after 18.1 is `done`. Mirrors the `docs/envoy-go/phases/08.2-graceful-drain/` stub-then-full-SPEC pattern.

**Phase id:** `18.2`
**Slug:** `18.2-ext-authz-grpc`
**Status:** `planned` (ROADMAP row `18.2` added `planned` at the phase-18 parent SPEC commit; depends-on `18.1`)
**Parent:** `docs/envoy-go/phases/18-http-filter-ext-authz/SPEC.md` (parent master SPEC — the cross-cutting design §4, the full §5 13-pin empirical-pin block, the §6 amendment block, the §7 9-ADR anchor map).

---

## Scope (per parent SPEC §2)

Phase 18.2 lands `envoy.filters.http.ext_authz` in **gRPC service mode** — the secondary-transport landing — against the `internal/filter/http/extauthz/` package that 18.1 establishes:

- **The NEW `internal/grpcclient/` gRPC-client framework primitive (ADR-0158)** — envoy-go's FIRST gRPC infrastructure of any kind. Dial + connection-management over `google.golang.org/grpc` + the `envoy.service.auth.v3.Authorization/Check` client stub (ships in `go-control-plane v1.32.4` per parent §5.P1 — no codegen) + gRPC-status → `{allow, deny, error}` error-classification. Couples to envoy-go's existing cluster manager for `GrpcService.EnvoyGrpc` cluster-name → endpoint + transport-socket resolution (parent §5.P13 RATIFIED-PENDING-IMPL-TIME — the most-likely ADR-0044 escape-valve surface for phase 18). Lives OUTSIDE `internal/filter/` (mirroring `internal/matcher/` + `internal/jwks/`) to anchor cross-phase reusability — strategic for the entire RPC-delegating filter family (`ext_proc`, `global_ratelimit`).
- **The `grpc_service` arm activated in the `compiledConfig` dispatch** — ADR-0157's §Decision is **amended at 18.2 IMPL** to replace the 18.1 PARSE-REJECT stub with the real gRPC `checkFn`.
- **The gRPC-mode `AttributeContext` / `CheckRequest` builder (ADR-0160 gRPC-mode portion)** — source/destination Peers (`socket_address`; `principal` via the phase-16 ADR-0144 `DownstreamPrincipal()` reuse; `certificate` gated by `include_peer_certificate` per parent §5.P3); `request.http` per the parent §5.P4 populated set (`id`/`method`/`headers` map lowercased incl. pseudo-headers/`path`/`host`/`scheme`/`size`/`protocol`/`body`-or-`raw_body`); `request.time` as a `Timestamp{seconds,nanos}`; `tls_session.sni` gated by `include_tls_session`; `context_extensions` merged from listener-level + per-route `CheckSettings.context_extensions`; the `encode_raw_headers` `headers`-vs-`header_map` discipline.
- **The `CheckResponse` → disposition mapping (ADR-0161 gRPC-mode portion)** — `OkHttpResponse.{headers (HeaderValueOption append/overwrite), headers_to_remove, response_headers_to_add}` allow-path upstream mutation; `DeniedHttpResponse.{status, body, headers}` deny-path downstream emission (verbatim headers incl. `content-type` per parent §5.P11); the error-classification boundary (gRPC transport failure → error; any well-formed `CheckResponse` with non-OK `status.code` → deny; empty `CheckResponse{}` → allow — per parent §5.P10).
- **The gRPC auth-server test-helper** — an in-process `envoy.service.auth.v3.Authorization/Check` server returning scriptable `CheckResponse` values (the FIRST in-process gRPC server in the envoy-go test tree).
- **Differential fixture `0021-http-ext-authz-grpc`** (~6–8 gRPC-mode scenarios) + the 23rd fuzzer.

**18.2-landing ADRs:** ADR-0158 (the `internal/grpcclient/` primitive — §Context anchored at the parent SPEC commit); ADR-0157 §Decision amendment (gRPC arm activation); ADR-0160 gRPC-mode portion; ADR-0161 gRPC-mode portion. ADR-0044 escape-valve: the §5.P13 gRPC dial / TLS-to-auth-cluster plumbing is the most-likely surface.

## Phase-done rollup

Per parent SPEC §8: the 18.2 phase-done commit flips ROADMAP row `18.2` `in-progress → done` AND the parent row `18` `in-progress → done` IN ONE OPERATION (the commit-message body must name both transitions for grep-verifiability). 18.2's phase-done closes the parent row 18; the §9 HTTP filters family then has 11 rows landed.

## When 18.2 is drafted

After 18.1 is `done`. The 18.2 lifecycle-state 1 → 2 session authors `docs/envoy-go/phases/18.2-ext-authz-grpc/SPEC.md` (the full sub-phase SPEC, mirroring the 18.1 SPEC's 15-section structure scoped to the gRPC surface). Per the 08.2 precedent, 18.2 MAY run its own `superpowers:brainstorming`-scoped-to-SPEC session — the gRPC-infrastructure-from-scratch lift may warrant a fresh brainstorm pass; the 18.2 SPEC session makes that call. This stub is superseded at that point.

## References

- Parent master SPEC: `docs/envoy-go/phases/18-http-filter-ext-authz/SPEC.md` (§2 scope table, §3 split rationale, §4 cross-cutting, §5 empirical pins, §6 amendments, §7 ADR map, §8 phase-done gate).
- 18.1 SPEC: `docs/envoy-go/phases/18.1-ext-authz-http/SPEC.md` (the foundational filter scaffold 18.2 extends).
- BRAINSTORM: `docs/envoy-go/phases/18-http-filter-ext-authz/BRAINSTORM.md` (§1.4 split analysis, §3.1 gRPC-client primitive, §7 ADR roster).
- Sibling-stub precedent: `docs/envoy-go/phases/08.2-graceful-drain/` (stub-then-full-SPEC pattern).
