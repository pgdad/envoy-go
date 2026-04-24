# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `03-tls`
- **phase-directory:** `docs/envoy-go/phases/03-tls/` — exists; contains `SPEC.md`, `PLAN.md`, and `PROGRESS.md` (all 15 PLAN tasks landed on branch `phase/03-tls-impl` under `.worktrees/phase-03-tls-impl`, atomic main commit + SHA-fill follow-up per phase-01/02 precedent, final Task-15 all-gates evidence quoted verbatim). REVIEW.md does not yet exist.
- **lifecycle-state:** `4` — implementation complete, not yet reviewed. PLAN.md's 15 tasks all landed per PLAN mapping. Eight phase-03 ADRs landed sequentially as ADR-0029..ADR-0036 (PLAN anticipated ADR-0029..ADR-0035; ADR-0035 was consumed by a Task-13 deviation ADR for fixture-0002 differential scope — downstream TLS + SNI only, upstream TLS unit-tested only — so the originally-PLAN-labelled final ADR shifted to ADR-0036; this is documented in ADR-0035 itself). Five of six SPEC §3 gates green on a fresh `go test -count=1 -timeout=5m ./...` re-run at state-4 transition: (a) fixture `0002-tls-tcp` byte-exact equality + downstream-TLS SNI routing per Task-15 PROGRESS output (upstream-TLS per-cluster distribution is unit-tested only, not differentially asserted, per ADR-0035); (b) fixtures `0000-tcp-echo` + `0001-tcp-proxy-rr` still green (regression-clean after ADR-0034 fixture.Driver split); (c) N/A (no conformance suites in phase 03); (d) `FuzzTLSContextParse` + `FuzzBootstrapLoad` + `FuzzTcpProxyFilter` all clean at ADR-0018's 30s CI budget; (e) `go vet`, `golangci-lint run`, `go test ./...` all clean (Task-15 outputs verbatim in PROGRESS.md). (f) deferred to state 5. Per SKILL_ROUTING.md / BOOTSTRAP_PROMPT §5 state 4, the next session invokes `superpowers:verification-before-completion` to re-run every gate from a fresh session and quote the outputs into a dedicated verification section of PROGRESS.md. Integration-topology note: the planner session's separate `STATE.md → lifecycle-state 3` commit on master (`7d994b3`, created after the impl worktree was cut) broke ADR-0003's fast-forward invariant (master was no longer an ancestor of `phase/03-tls-impl`). Restored at state-4 transition by cherry-picking that commit onto impl (new SHA `ee27cf3`, content-identical to `7d994b3`) so master could be fast-forwarded. One-off operational artefact, not a pattern change — no ADR issued.
- **next-skill:** `superpowers:verification-before-completion`
- **next-skill-scope:** Re-run every SPEC §3 phase-done gate from a fresh session, quote each command's verbatim output into PROGRESS.md (BOOTSTRAP §5 state 4 names PROGRESS as the capture target). Commands: `go build ./...`; `go vet ./...`; `golangci-lint run ./...`; `go test ./...`; `go test ./internal/bootstrap/ -run=FuzzBootstrapLoad -fuzz=FuzzBootstrapLoad -fuzztime=30s`; `go test ./internal/filter/tcpproxy/ -run=FuzzTcpProxyFilter -fuzz=FuzzTcpProxyFilter -fuzztime=30s`; `go test ./internal/tls/ -run=FuzzTLSContextParse -fuzz=FuzzTLSContextParse -fuzztime=30s`; `go test ./test/differential/ -v -timeout=10m` (all three fixtures — 0000/0001/0002 — must PASS). Docker + the Envoy v1.32.4 image (per ADR-0013) required for the differential suite. PKI determinism re-check: `cd test/fixtures/0002-tls-tcp && go run ./pki/gen && git diff --exit-code pki/`. Work from the impl worktree (master was fast-forwarded to the impl tip at state-4 transition so both branches now share the same HEAD; no branch split). After verification passes, advance STATE to lifecycle-state 5 with `next-skill: superpowers:requesting-code-review` per §5 state 5. Fuzz-seed corpus note: Task 15's fuzz run found new interesting inputs across all three targets (41/23/70) — none crashed — and those entries were NOT persisted under `testdata/fuzz/` (ADR-0018 budget discipline); the verifier re-run will produce a similar delta and should likewise NOT commit new seed corpora unless a crash occurs.
- **last-commit:** ee27cf3
- **last-updated:** 2026-04-24

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
