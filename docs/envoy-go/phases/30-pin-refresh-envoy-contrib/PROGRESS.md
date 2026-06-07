# Phase 30 IMPL Progress — reference-image pin-refresh to `envoyproxy/envoy:contrib-v1.37.2`

Phase 30 is the project's FIRST reference-image pin change since ADR-0008 — a pure
infrastructure pin-refresh that ships **ZERO production LoC, ZERO new go.mod deps, and
ZERO new package/stat/fixture/fuzzer/BackendKind**. The only runtime-load-bearing edit is
`docs/envoy-go/ENVOY_TARGET.md` (`parseEnvoyTarget` reads `**Tag:**`/`**SHA256:**`;
`StartReferenceProxy` boots `pin.SHA256` — editing it repoints every reference-booting
fixture). Because there is no code change, the PLAN has NO TDD red/green — each task is
*act → verify-with-exact-command → commit*; the "test" is the gate run (ADR-0052 atomic
six-gate). IMPL date: **2026-06-07**.

The pin swap: `envoyproxy/envoy:v1.37.2` @ `sha256:c5e8a68e…` (ADR-0008, the standard image)
→ **`envoyproxy/envoy:contrib-v1.37.2`** @ `sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`
(ADR-0227, superseding ADR-0008 — the contrib **variant** of the SAME upstream version, a
behavioral SUPERSET: same Envoy v1.37.2 source + extra compiled-in extensions). Motivation:
unblock the phase-31 `kafka_broker` §9 network filter (a contrib-only Envoy extension absent
from the standard reference image). The `/contrib v1.32.4` go.mod dep + all kafka_broker work
are phase 31, NOT phase 30.

---

## Task 1: Baselines/anchors gate (standard pin)

The regression baseline — counts + the contrib image present locally + a clean six-gate on
the UNCHANGED standard pin (no file change; a verification gate).

- Counts confirmed at master tip: **54** fixtures (tail `0052-mongo-fault-delay`) / **39**
  fuzzers / **360** stats / BackendKind tail **30** (`TCPMongoResponder`) / DECISIONS tail
  **ADR-0227** (next-free ADR-0228).
- Contrib image present locally: `envoyproxy/envoy contrib-v1.37.2 7edd5b0fd763` (299 MB),
  RepoDigest `sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`.
- Six-gate on the standard pin: GREEN (build/vet/lint clean; `-race -short` all-ok; the full
  54-fixture differential byte-identical PASS; conformance h2spec 53/53 + proxy-wasm 10/10).
  KNOWN FLAKE recorded: a transient first-run HTTP-fixture subject-startup `EOF` (roams across
  fixtures e.g. 0014/0020/0022/0023/0028/0036) is a pre-existing artifact unrelated to phase 30
  — it appeared in this standard-image baseline too; clears on a per-fixture or full re-run.

**Task 1 DONE** (no commit — verification gate).

---

## Task 2: Flip `ENVOY_TARGET.md` to the contrib pin

Flipped the four pin lines (the only runtime-load-bearing edit):

- `**Tag:**` → `envoyproxy/envoy:contrib-v1.37.2`
- `**SHA256:**` → `envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`
- `**Pinned in:**` → ADR-0227 (supersedes ADR-0008)
- `**Last verified:**` → the IMPL date (2026-06-07)

Release-notes URL + proto-major-version UNCHANGED — same upstream version. `harness_test.go`'s
`v1.34.0` strings are a PARSER unit-test sample (not the live pin) and stay as-is (D30-5);
historical SHA references in prior phases' PROGRESS/SPEC/PLAN stay frozen.

**Task 2 DONE — commit `f773079`** (`phase 30 T2: flip ENVOY_TARGET.md pin to envoyproxy/envoy:contrib-v1.37.2 (ADR-0227)`).

---

## Task 3: Re-baseline the 54-fixture differential suite on the contrib pin

The authoritative re-run against the flipped pin:

```
$ go test ./test/differential/... -count=1
ok  	github.com/esalaine/envoy-go/test/differential	177.233s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	0.001s
DIFF_EXIT=0
```

**54/54 byte-identical PASS, ZERO divergence** — contrib is a behavioral superset, so no
fixture diverges. No divergence-classification needed (SPEC §5.4 / D-3.7 step 3) → **no
ADR-0228 consumed**. Clean EXIT=0 on the FIRST run this session — no HTTP-startup-flake
re-run was needed.

**Task 3 DONE** (no commit — gate).

---

## Task 4: Conformance sanity (image-independent)

Conformance exercises the envoy-go SUBJECT, not the reference container (`test/conformance/`
references no pin) — re-ran as sanity:

```
$ go test ./test/conformance/... -count=1
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.545s
ok  	github.com/esalaine/envoy-go/test/conformance/proxy-wasm	0.264s
CONF_EXIT=0
```

h2spec: **53 total tests, 0 failures**. proxy-wasm: all **10** families PASS.

**Task 4 DONE** (no commit — gate).

---

## Task 5: ADR-0227 body + `BEHAVIOR_CONTRACT.md` pin lines

- Landed the ADR-0227 §Decision/§Consequences body in place (replacing the SPEC-time
  drafted-placeholder), superseding ADR-0008.
- Updated the two `BEHAVIOR_CONTRACT.md` current-pin lines to the contrib digest + ADR-0227,
  and added the phase-30 contract note.
- Left the frozen phase-06.2 access-log empirical-evidence provenance block intact — exactly
  ONE `c5e8a68e…` match remains in `BEHAVIOR_CONTRACT.md` (line 550).

**Task 5 DONE — commit `b02147d`** (`phase 30 T5: ADR-0227 Decision/Consequences body + BEHAVIOR_CONTRACT contrib pin lines`).

---

## Task 6: Completion bundle — six-gate green on the contrib pin + lifecycle

### Step 1: the full six-gate LIVE on the contrib pin

| # | Gate | Command | Result |
|---|------|---------|--------|
| 1 | BUILD | `go build ./...` | `BUILD ok` (EXIT=0) |
| 2 | VET   | `go vet ./...` | `VET ok` (EXIT=0) |
| 3 | LINT  | `golangci-lint run` | `LINT ok` (EXIT=0) |
| 4 | RACE  | `go test -race -short ./...` | all `ok`, 0 FAIL lines (EXIT=0) |
| 5 | DIFF  | `go test ./test/differential/... -count=1` | `ok …/test/differential 177.233s` — **54/54 byte-identical PASS, ZERO divergence** (EXIT=0) |
| 6 | CONF  | `go test ./test/conformance/... -count=1` | h2spec **53/53** + proxy-wasm **10/10** (EXIT=0) |

All six gates GREEN LIVE on the contrib pin (clean EXIT=0 observed for each). No
HTTP-startup-flake re-run was needed this session — the differential suite passed clean on the
first run.

### Step 6: final verification

```
$ echo "fixtures: $(ls -d test/fixtures/[0-9]* | wc -l) (expect 54)"
fixtures: 54 (expect 54)
$ echo "fuzzers:  $(grep -rh '^func Fuzz' $(find ./internal -name fuzz_test.go) | wc -l) (expect 39)"
fuzzers:  39 (expect 39)
$ grep -E '^\*\*(Tag|SHA256):\*\*' docs/envoy-go/ENVOY_TARGET.md
**Tag:** `envoyproxy/envoy:contrib-v1.37.2`
**SHA256:** `envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`
$ grep -n 'c5e8a68e' docs/envoy-go/BEHAVIOR_CONTRACT.md
550:ENVOY_TARGET.md SHA c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd;
```

54 fixtures / 39 fuzzers; Tag `contrib-v1.37.2` + SHA256 `7edd5b0f…`; exactly ONE `c5e8a68e`
match (the frozen phase-06.2 provenance block at line 550).

### Lifecycle

- `docs/envoy-go/ROADMAP.md`: row 30 `in-progress → done` (flat infra row — NO parent rollup;
  no other row touched).
- `docs/envoy-go/STATE.md`: active-phase advanced to `phase 30 IMPL done`; lifecycle-state
  phase 30 CLOSED; next-skill `superpowers:brainstorming` for the phase-31 `kafka_broker`
  §9 row; reference-pin line updated to the contrib digest; counts UNCHANGED; the prior
  PLAN-done block folded into the Earlier chain.
- `next-prompt.txt`: rewritten for the phase-31 `kafka_broker` BRAINSTORM cold-start.
- This `PROGRESS.md`: the durable phase-30 IMPL record.

**Task 6 DONE** — the completion-bundle commit (this commit).

---

## Counts at phase-30 IMPL done (UNCHANGED by phase 30)

| Metric | Value |
|--------|-------|
| Active differential fixtures | **54** (tail `0052-mongo-fault-delay`) |
| Fuzzers | **39** |
| Stat surface | **360** |
| BackendKind tail | **30** (`TCPMongoResponder`) |
| DECISIONS.md tail | **ADR-0227** (next-free **ADR-0228**; ADR-0227 body landed in-place, no new number) |
| Conformance | h2spec 53/53; proxy-wasm 10/10 |
| Reference pin | `envoyproxy/envoy:contrib-v1.37.2` @ `sha256:7edd5b0f…` (ADR-0227, superseding ADR-0008) |

ADR-0045 split-gate: 0 production LoC / 6 tasks → **NO split**.
