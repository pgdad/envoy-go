# Phase 102 — `stats-flush-interval-bounds` — BRAINSTORM

**Stage:** BRAINSTORM (lifecycle **DONE -> 1**). **Self-picked** under the 2026-07-12 standing
directive, with no human consulted and no banked mid-lifecycle work to advance first (check (1) was
SILENT at `want=133` before this stage's ADD — §8.1).

**Subject in one sentence.** envoy-go accepts, boots and runs every bootstrap `stats_flush_interval`
the reference refuses — `0s` and `-1s` silently fold to a 5 s flush, `300s` and `1000s` are honoured as
written, a sub-millisecond value flushes about every millisecond, and the protobuf maximum saturates —
while the reference refuses all of them at `--mode validate` AND at boot (rc=1), with or without a stats
sink configured. It refuses the range class with `value must be inside range [1ms, 5m0s)` and a negative
or protobuf-maximum value with a **different** message, `Invalid duration: …`.

**Both sides were MEASURED at this stage's own tip (`33f24624`)** through the real binaries. A
Docker-only reference agent ran the image by digest `7edd5b0fd763…`, verified by `docker image inspect`
(Id and RepoDigest both match `ENVOY_TARGET.md:4`). A Docker-free subject + prototype agent ran
alongside it, on a disjoint port band and in throwaway worktrees: the phase-98 split. **The candidate
banked by `101/BRAINSTORM.md` §0.8 / §4.2 and ADR-0323 §Consequences (g) was re-measured rather than
inherited** (method note 69). Its core held: the reference's `[1ms, 5m)` range and the envoy-go
fail-OPEN accept. **The banked record was INCOMPLETE on four axes** (§0.1-§0.4).

⚠️ **Master moved mid-stage.** `698a001b` (*"claude: enable the superpowers plugin at project scope"*,
`.claude/settings.json` `17 0`, local-only) landed on `master` while the agents ran. The stage branch
was fast-forwarded onto it. `git diff 33f24624 698a001b -- docs/` reads **0** lines, so every
`ROADMAP.md` measurement below holds on both commits. **Every binary and prototype in this document was
built from `33f24624`**; the moved commit touches no `.go` file.

---

## 0. What this stage refuted or sharpened — ELEVEN claims, by execution

Each item was produced by running something. Reference arms are numbered R*, subject arms S*, and
prototype arms P* (tables in §2 and §6).

### 0.1 🔴 THE REFERENCE HAS *TWO* REJECT CLASSES, NOT ONE

The banked record (`101/BRAINSTORM.md` §0.8) measured `0s`, `0.0005s`, `300s` and `1000s`, which all
read `Proto constraint validation failed (BootstrapValidationError.StatsFlushInterval: value must be
inside range [1ms, 5m0s))`. It never ran a negative value or the protobuf maximum. Measured here:
**`-1s` → `Invalid duration: Expected positive duration`** and **`315576000000s` → `Invalid duration:
Duration out-of-range`**. Both are rc=1, but they come from a different check (the reference's own
Duration validation, which runs before PGV). **A single envoy-go message cannot be a byte-substring of
both classes.** That matters to any boot-reject fixture, which asserts one shared substring on both
sides (§7). The prototype's single message (§6) matches the range class only.

### 0.2 🔴 THE REFERENCE REFUSES AT *BOOT*, NOT ONLY AT VALIDATE — AND envoy-go RUNS EVERY ONE

The banked record was a validate-mode record. Measured here, on a real boot of B1 (a statsd sink): the
reference exits rc=1 (`server.cc:453`, admin and listener never answer, curl code `000`) on `0s`, `-1s`,
`0.0005s`, `300s` and `1000s`, and serves on `absent`, `0.001s` and `0.5s` (`/ready` = `LIVE`, listener
`222`). envoy-go boots **all eight** and serves `222` on each. **The divergence is a runtime one, not a
validate-mode artefact.**

### 0.3 THE SILENT FOLD IS MEASURED AT RUNTIME, NOT ONLY READ FROM THE PARSE

The banked record said `0s` *"silently becomes 5 s"*. That was a reading of `parseStatsSinks`. Measured
on the tip binary with a UDP statsd receiver, one marker line per flush (`p102.server.live`):
**`0s` → 4 flushes, mean 5001.71 ms; `-1s` → 4 flushes, mean 5000.38 ms**. That matches `absent`
(5001.58 ms). **`1000s` → 0 flushes in ~21 s; `300s` → 0 in ~4 s.** An operator who wrote `0s` gets a
5 s cadence and no warning.

### 0.4 🔴 THE REJECT DOES NOT DEPEND ON A STATS SINK — ON EITHER SIDE

B0 (no `stats_sinks`) and B1 (a statsd sink) gave **identical rc and message on all 13 values, on both
sides**. The reference refuses a **bare** out-of-range `stats_flush_interval` even when nothing would
ever flush. This falsifies, for out-of-range values, the phrase *"a bare `stats_flush_interval` is
parsed-and-inert"* at `BEHAVIOR_CONTRACT.md:845` (D-MS-FLUSH-INERT). That phrase remains true for
in-range values. ⇒ The repair belongs in `parseStatsSinks`, unconditionally. It must not be gated on
`len(StatsSinkConfigs) > 0`.

### 0.5 🔴 THE EXISTING SUITE CANNOT SEE THIS ROW AT ALL

The prototype reject (§6, `7 2`) reads **391 `=== RUN`, 0 FAIL** over `./internal/bootstrap/...
./cmd/envoy-go/... ./internal/statssink/...`. That is identical to the tip's 391 / 0. `FuzzStatsSinkConfigParse` ran for
20 s with ~1.16 M execs and no crash. **A behaviour-inverting patch is green (method note 50). Every RED
arm the row needs must be NEW**, and the SPEC must name them before the PLAN builds them.

### 0.6 THE BLAST RADIUS IS EMPTY — CENSUSED, NOT ASSUMED

The banked record called the blast radius *"UNCENSUSED"*. Censused here, case-insensitively and
directory-scoped, over `cmd/`, `internal/`, `test/` and every fixture's `envoy-go.yaml` AND `envoy.yaml`:

- `cmd/envoy-go/main_test.go:1216` sets `0.5s`.
- `internal/bootstrap/bootstrap_test.go:1894` and `:1936` set `2s` and `3s`.
- The `statssink_fuzz_test.go` seeds at `:43` and `:141` set `2s` and `3s`.
- Fixtures `0089`, `0090`, `0091`, `0092`, `0093`, `0094`, `0098` and `0101` set `0.5s` on both sides.
- Fixtures `0112` and `0113` set `0.1s` on both sides.

**All of these are inside `[1ms, 5m)`.** No Go-built `durationpb` reaches `StatsFlushInterval`, and no
test asserts the `0s` / negative fold (the only 5 s assertion, `bootstrap_test.go:1887`, is the
**absent** case). No fuzz target requires a seed to parse OK. `FuzzBootstrapLoad` requires a
`bootstrap: ` error prefix, which the prototype message carries.

### 0.7 GO-BUILT DURATIONS: THE TIP ACCEPTS SHAPES NO CONFIG CAN CARRY

Fed through `parseStatsSinks` (subject agent, unit probe in a throwaway worktree):

| `{seconds, nanos}` | tip | prototype |
|---|---|---|
| `{0, 999999}` | ok, 999.999 µs | reject |
| `{0, 1000000}` | ok, 1 ms | ok, 1 ms |
| `{299, 999999999}` | ok | ok |
| `{300, 0}` | ok, 5 m | reject |
| `{1, -1}` | ok, 999.999999 ms | reject |
| `{-1, 0}` | ok, **folds to 5 s** | reject |
| `{0, 1000000000}` | ok, 1 s | reject |
| `{1, 2147483647}` | ok, 3.147483647 s | reject |
| `{MaxInt64, 0}` | ok, **saturates** to `2562047h47m16.854775807s`, never flushes | reject |

Method note 106 was applied before the fact: the rule bounds every field its arithmetic reads. **The
reference's handling of out-of-range nanos is NEITHER measured NOR source-read by this stage.** YAML and
protojson normalise or refuse such values before either side sees them, so no arm can reach it. The only
basis is row 101's source reading of the reference's Duration validation (`next-prompt.txt`, "ROW 101's
NANOS BOUND HAS NO REFERENCE ARM"). Do not cite it as measured.

### 0.8 SUB-MILLISECOND CADENCE IS UNMEASURABLE ON THE REFERENCE IN THIS SETUP

envoy-go flushes `0.0015s` at **1.50 ms** (2664 flushes, σ 0.51 ms) and `0.001s` at **1.05 ms**. The
reference flushes **both** at roughly **2-3 ms**, slower than either configured value. This held in four
setups: host receiver and in-namespace receiver, each at concurrency 32 and at concurrency 1. The
ordering even flips between runs (in-namespace at concurrency 32, `0.0015s` median 1.999 ms vs `0.001s`
3.446 ms). **Whether the reference truncates `0.0015s` to 1 ms (as its milliseconds conversion would
suggest) cannot be observed here.** Banked, not chartered (§4.2).

### 0.9 envoy-go RUNS `0.0005s` AT ~1 ms, NOT 0.5 ms — UNEXPLAINED

`0.0005s` read 4015 flushes over ~4 s, mean **1.00 ms**, σ 0.32. The ticker is built with 500 µs. The
cause was not found (runtime timer granularity under the flush workload is one hypothesis, not tested).
**Moot under the row's reject**, and recorded so nobody re-derives it.

### 0.10 THE ONE AGREEING ARM AGREES THROUGH THE PARSER, NOT THROUGH A RULE

`315576000001s` is refused on **both** sides at the tip. The reference refuses it in JSON parsing
(`Unable to parse JSON as proto … duration out of range`), and envoy-go refuses it in protojson
(`google.protobuf.Duration value out of range`). That agreement belongs to the decoder and survives any
repair (method note 101 applied to a reject). The prototype leaves it unchanged.

### 0.11 THIS STAGE'S OWN BRIEF CARRIED A WRONG ADDRESS

The reference brief said the host-gateway IP was *"typically 172.17.0.1"*. On this Docker Desktop host
that address is the VM's bridge, and the first four cadence runs read **0 datagrams**. Inside a
container `host.docker.internal` resolves to `192.168.65.2`, and a test datagram there reached the host
receiver. **The memory index already says this** (`topic_docker_reference_probes`: *"host-gateway IP …
literal, NOT bridge IPAM"*). The brief paraphrased instead of copying, which is method note 67's class
in a probe brief. The agent caught it and re-ran.

---

## 1. The pick, and why it is defensible as "smallest first"

### 1.1 Charter, in one sentence

`stats_flush_interval` accepts exactly what the reference accepts and refuses at boot (and in `-mode
validate`) everything it refuses. Absent means 5 s. Any value inside `[1ms, 5m)` is honoured as
written. Every value outside that range — zero, negative, sub-millisecond, five minutes or more, the
protobuf maximum, and Go-built out-of-range nanos — is a boot reject computed from `GetSeconds()` /
`GetNanos()`, never from the saturating `AsDuration`. The reject does not depend on whether a stats sink
is configured.

### 1.2 Why "smallest defensible" selects it — a trade-off, stated, not a ranking

| candidate | prod floor (built + run at this tip) | reference measured at this tip? | cross-side surface | severity |
|---|---|---|---|---|
| **`stats_flush_interval` bounds (THIS ROW)** | **`bootstrap.go` `7 2`** (§6) | **YES: 13 values × 2 bootstraps at validate, 8 boots, timed cadence** | boot vs reject; flush cadence | fail-OPEN: a refused config runs, `0s` silently becomes 5 s |
| the sub-ms cadence (§0.8) | not prototyped | **NO: unobservable in this setup** | 1 ms vs 1.5 ms | cosmetic |
| `downstream_cx_total` post-filter | moves an `Inc` | phase-100 only | counter value | off by one per dropped cx; moves fixture pins |
| validate-mode admin-socket / gRPC-sink cluster gaps (`REVIEW_FINDINGS.md`) | not prototyped | **NO** | validate rc only | validate-only (boot already fails closed) |
| order-dependent pairwise fold | not prototyped | **NO** | serve vs close | wrong chain / close |
| SNI case-insensitivity | `4 1` pattern side + unmeasured input side | phase-99 | serve vs DEFAULT | wrong chain |

It wins on four counts:

- It is the only candidate whose reference side is measured end to end at this tip, including both
  reject classes and the boot path.
- Its production floor is one function, `7 2`.
- Its blast radius is censused EMPTY (§0.6).
- It was named next in line by two closed documents (`101/BRAINSTORM.md` §4.2, ADR-0323 (g)) and by
  `REVIEW_FINDINGS.md`, and it survived re-measurement.

**It is adopted after re-measurement, not inherited, and the re-measurement widened it** (§0.1, §0.4).

### 1.3 The verdict, SPLIT before pricing (method note 52)

1. **Fail-OPEN accept of out-of-range values.** `0.0005s`, `0.000999999s`, `300s`, `1000s` and
   `315576000000s` boot and are honoured: sub-ms runs at ~1 ms, and the rest run as written or saturate.
2. **The silent `0s` / negative fold to 5 s.** This is the non-positive subset of (1), with its own
   observable: a 5 s cadence the operator did not write.

**Both are closed by ONE rule in ONE function.** Splitting them would land a range check that still
folds `0s`, which re-mints claim 2 inside the repair for claim 1. They are folded into one row, as row
100's `0s` fold and row 101's width fix were (a repair that would leave its own sibling divergence
standing carries the antidote with it).

### 1.4 What this row does NOT buy — stated plainly

- It does **not** decide byte-parity of reject messages. Parity is in rc and in which values are refused;
  the reference has two message classes (§0.1). **The SPEC decides envoy-go's wording**, per class or as
  one message, and which substring a fixture can pin.
- It does **not** touch the sub-ms cadence (§0.8), the `0.0005s` ~1 ms oddity (§0.9), or
  `stats_flush_on_admin` (still strict-rejected, ADR-0080 / ADR-0262).
- It does **not** add a stat name. `+0` is predicted.
- It does **not** touch any listener-side candidate row 100 or row 101 banked.

---

## 2. The divergence, MEASURED — both sides, matched negatives, timed observables

**Bootstraps (identical shape on both sides):**

- **B0:** admin, one static listener with an HCM `direct_response` `222`, a placeholder STATIC cluster,
  no `stats_sinks`, plus `stats_flush_interval: <V>` (omitted for `absent`).
- **B1:** B0 plus a UDP `envoy.stat_sinks.statsd` sink, prefix `p102`.

**Matched negatives:** `0.001s`, `0.0015s`, `0.5s` and `299.999999999s` sit on the accept side of each
edge, on byte-identical bootstraps (method note 55).

### 2.1 Reference (`envoyproxy/envoy@sha256:7edd5b0fd763…`, digest verified)

**R-A — `--mode validate`, every value × {B0, B1}; B0 and B1 identical on all 13:**

| V | rc | message (the `goo.gle/debug…` token varies run to run) |
|---|---|---|
| absent, `0.001s`, `0.0015s`, `0.5s`, `299.999999999s` | 0 | `configuration … OK` |
| `0s`, `0.0005s`, `0.000999999s`, `300s`, `1000s` | 1 | `… StatsFlushInterval: value must be inside range [1ms, 5m0s))` |
| `-1s` | 1 | `Invalid duration: Expected positive duration` |
| `315576000000s` | 1 | `Invalid duration: Duration out-of-range` |
| `315576000001s` | 1 | `Unable to parse JSON as proto (… duration out of range)` |

**R-B — real boot on B1** (40 polls × 0.5 s; admin on `17001`, listener on `17002`):

| V | result |
|---|---|
| absent, `0.001s`, `0.5s` | **boots**: `/ready` `LIVE`, listener `222` |
| `0s`, `0.0005s`, `300s`, `1000s` | **rc=1** at `server.cc:453`, range message; admin and listener never answer |
| `-1s` | **rc=1**, `Invalid duration: Expected positive duration` |

Boot was decided by the admin reply and the container state, never by a `timeout` exit code.
`/config_dump` echoes `"stats_flush_interval": "0.500s"` / `"0.001500s"` / `"0.001s"`, and omits the
field when it is absent. There is no flush-count stat (`/stats` matching `flush` reads only
`filesystem.flushed_by_timer: 0` and `server.dropped_stat_flushes: 0`).

**R-C — cadence**, with one marker per flush (`p102.server.uptime`; 116 lines per flush at the default
concurrency of 32, 85 at `--concurrency 1`):

| V | host receiver (via the Docker Desktop UDP proxy) | in-namespace receiver |
|---|---|---|
| `0.5s` | 8 flushes / 3507.761 ms, mean **501.109 ms**, σ 0.909 | 6 / 2505.568 ms, mean **501.114 ms**, σ 0.214 |
| absent | 5 / 20005.658 ms, mean **5001.414 ms**, σ 0.497 | 4 / 15003.959 ms, mean **5001.320 ms** |
| `0.0015s` | 658 / 1997.728 ms, mean 3.041 ms | 883 / 1996.980 ms, median 1.999 ms |
| `0.001s` | 629 / 1996.878 ms, mean 3.180 ms | 515 / 1993.883 ms, median 3.446 ms |

### 2.2 Subject (envoy-go at `33f24624`, the tip binary)

**S-A — `-mode validate`, every value × {B0, B1}; B0 and B1 identical:** every value except
`315576000001s` reads **rc=0 `configuration OK`**. `315576000001s` reads rc=1 `bootstrap: protojson: …
google.protobuf.Duration value out of range` (§0.10).

**S-B — real boot on B1:** all eight values (absent, `0s`, `-1s`, `0.0005s`, `0.001s`, `0.5s`, `300s`,
`1000s`) **boot**. Each answered a GET with `222` at ~1 s, logged `listener l_a ready`, and exited 0 on
SIGTERM.

**S-C — cadence** (marker `p102.server.live`; exactly 20 lines per flush, ratio 20.00 in every fast run;
window = spawn to SIGTERM; an in-memory receiver, because the first fsync-per-packet receiver capped at
~70 ms):

| V | window | flushes | mean | σ |
|---|---|---|---|---|
| `0.5s` | ~7.0 s | 13 | **500.07 ms** | 0.47 |
| absent | ~21 s | 4 | **5001.58 ms** | 3.65 |
| **`0s`** | ~21 s | 4 | **5001.71 ms** | 1.09 |
| **`-1s`** | ~21 s | 4 | **5000.38 ms** | 0.36 |
| `0.0015s` | ~4.0 s | 2664 | 1.50 ms | 0.51 |
| `0.001s` | ~4.0 s | 3816 | 1.05 ms | 0.24 |
| **`0.0005s`** | ~4.0 s | 4015 | **1.00 ms** | 0.32 |
| **`1000s`** | ~21 s | **0** | — | — |
| **`300s`** | ~4 s | **0** | — | — |

No final flush was sent at shutdown on any run (D-MS-FINAL-FLUSH).

### 2.3 The mechanism, at the tip, by symbol

`internal/bootstrap/bootstrap.go` `parseStatsSinks`: `result.FlushInterval = statsFlushIntervalDefault`
(5 s), then `if d := bs.GetStatsFlushInterval(); d != nil { if v := d.AsDuration(); v > 0 {
result.FlushInterval = v } }`. There is no upper bound, no lower bound above zero, and `AsDuration`
saturates. `cmd/envoy-go/main.go` passes `bs.FlushInterval` to `statssink.NewFlusher`, whose `Start`
calls `time.NewTicker(f.interval)`. **The `> 0` guard is the only thing standing between a `0s` config
and a `NewTicker` panic.** A repair that removes the fold must keep `FlushInterval > 0` true for every
accepted value. Under `[1ms, 5m)` it is.

---

## 3. Hazards for the SPEC

### 3.1 The suite is blind (§0.5)

Every RED arm is new. At minimum there must be one per reject class and per edge:

- `0s`, `-1s`
- `0.000999999s` (just under), `0.001s` (the accept edge)
- `299.999999999s` (the accept edge), `300s` (just over)
- the protobuf maximum
- Go-built `{0, 1000000000}`, `{1, -1}`, `{MaxInt64, 0}`

There must also be a **no-sink** arm (§0.4) and an **absent → 5 s** pin that stays green. Each reject
arm must assert its PHRASE, or it is green for any reject (method note 105).

### 3.2 Two reject classes vs one message (§0.1)

Whatever wording the SPEC picks, a boot-reject fixture's shared substring must appear in BOTH sides'
stderr **for the arm it runs**. A `0s` arm can share `value must be inside range [1ms, 5m0s)`. A `-1s`
arm cannot, unless envoy-go mirrors `Expected positive duration`. **Decide the wording per class before
choosing the fixture arm.**

### 3.3 The `NewTicker` panic guard (§2.3)

Do not delete the `> 0` check without the range check that subsumes it. An NC that deletes only the
lower bound must panic or reject, not boot silently. **Name the mechanism before running the NC**
(method note 7d).

### 3.4 Occurrence set of the claims the repair falsifies

Measured with `git grep -n -i` over `docs/ internal/ cmd/` (excluding `STATE_HISTORY.md`):

- `internal/bootstrap/bootstrap.go` around `:227` (`statsFlushIntervalDefault` comment, *"absent or
  non-positive"*) and around `:534-535` (`FlushInterval` field comment, *"default 5s when
  absent/non-positive"*). **Both are falsified.** Anchor on the symbols, not the lines (method note 3).
- `BEHAVIOR_CONTRACT.md:845` — *"a bare `stats_flush_interval` is parsed-and-inert"*. **Half-falsified**:
  true in range, false out of range (§0.4). Lead with what survives (method note 63).
- `BEHAVIOR_CONTRACT.md:837` and ADR-0262's parse-arm bullet — *"default 5s"*. **Survive** (the absent
  case).
- `SPEC-47.1.md` D-MS-FLUSH (*"default 5s when absent"*). **Survives.** It is a closed document, so it
  is evidence, not text to edit.

### 3.5 The `0089`-`0113` fixtures sit inside the range

`0.5s` and `0.1s` are both inside `[1ms, 5m)` and unchanged by the repair. **The SPEC must not "tidy"
them.** Their cadence pins are the accept-side regression gate the row gets for free.

---

## 4. Rejected alternatives — every cost RE-DERIVED or explicitly NOT re-derived

### 4.1 Gating the reject on `len(StatsSinkConfigs) > 0` — **REJECTED: wrong on B0** (§0.4)

The reference refuses a bare out-of-range value with no sink. A sink-gated check would leave every B0
arm fail-OPEN.

### 4.2 Sub-millisecond cadence parity (1 ms truncation vs 1.5 ms) — **REJECTED: the reference side is unobservable here** (§0.8)

The reference's flush floor in this setup is 2-3 ms. Banked; a future stage needs a lower-overhead
reference probe (a smaller stat set, or a non-proxied receiver) before chartering it.

### 4.3 Splitting the fold from the range check — **REJECTED: a half-repair re-mints the fold** (§1.3)

### 4.4 Validate-mode admin-socket and gRPC-sink cluster checks (`REVIEW_FINDINGS.md`, ADR-0268) — **REJECTED: unmeasured at this tip, validate-only** (boot fails closed on both sides per the finding's own wording)

### 4.5 `downstream_cx_total` post-filter accounting · the close KIND under `continue…: false` · the order-dependent pairwise fold · SNI case-insensitivity · nested-descent precedence · duplicate-matcher reject parity · the `rank > 2` guard · `server_names` partial-wildcard acceptance · the HCM `stat_prefix` panic · the receiver port race · the dead-port false-green · a TCP listener filter on a QUIC listener — **REJECTED; unchanged from `101/BRAINSTORM.md` §4.3-4.8 and `next-prompt.txt`'s banked list, NOT re-derived at this tip.**

### 4.6 The exported `Pipeline.Run` overflow above `9223372036854` ms (ADR-0323 (h)) — **REJECTED as a row**

A fold-in for the next row that touches `Pipeline.Run`. This row does not touch it.

---

## 5. Family attribution

**An Observability / bootstrap MAINTENANCE row claiming NO family ordinal**, on the row-85-through-91
and 95-101 precedent. It repairs a landed deliverable (phase 47.1's ADR-0262 parse arm, D-MS-FLUSH) and
extends no family charter. ⚠️ **The row cell is worded so that it does NOT spell the `-family row`
phrase** that check (3) and NC-D count (§8.4: NC-D unmoved at 96 / 68).

---

## 6. The cost FLOOR — prototyped, run, reverted

**P1** (subject agent, throwaway detached worktree off `33f24624`, since removed; `git worktree list`
shows only the canonical root and this stage's worktree):

```go
	if d := bs.GetStatsFlushInterval(); d != nil {
		// Reference PGV: gte 1ms, lt 5m. Read the raw fields (AsDuration
		// saturates); nanos outside [0, 999999999] is rejected first so the
		// arithmetic below cannot overflow.
		secs, nanos := d.GetSeconds(), d.GetNanos()
		if nanos < 0 || nanos > 999999999 || secs < 0 || secs >= 300 || (secs == 0 && nanos < 1000000) {
			return fmt.Errorf("bootstrap: stats_flush_interval: value must be inside range [1ms, 5m0s)")
		}
		result.FlushInterval = time.Duration(secs)*time.Second + time.Duration(nanos)
	}
```

**Measured results:**

- `git diff --numstat`: **`internal/bootstrap/bootstrap.go 7 2`**.
- `go build ./...` and `go vet ./internal/bootstrap/ ./cmd/envoy-go/` are clean.
- `GOTOOLCHAIN=go1.26.2 golangci-lint run ./internal/bootstrap/` exits rc 0 with empty output. ⚠️
  **No planted control was run, so that zero is UNPROVEN** (method note 7i). The IMPL's lint gate is the
  proof.
- **391 / 0 on both the tip and P1** (§0.5).
- `FuzzStatsSinkConfigParse` ran 20 s with no crash.

**P1 driven** (rebuilt binary, B0 and B1 identical):

| V | P1 validate | P1 boot (B1) | reference |
|---|---|---|---|
| absent | rc0 | boots, `222` | rc0, boots ✅ |
| `0.001s` | rc0 | boots, 2826 flushes, mean 1.06 ms | rc0, boots ✅ (cadence §0.8) |
| `0.0015s` | rc0 | boots, mean 1.50 ms | rc0 ✅ (cadence §0.8) |
| `0.5s` | rc0 | boots, mean 500.00 ms | rc0, 501.1 ms ✅ |
| `299.999999999s` | rc0 | boots, 0 flushes in ~3 s | rc0 ✅ |
| `0s`, `0.0005s`, `0.000999999s`, `300s`, `1000s` | rc1, range message | **dead at 1 s, curl `000`, exit 1** | rc1, range message ✅ |
| `-1s` | rc1, **range** message | dead, exit 1 | rc1, **`Invalid duration: Expected positive duration`** ⚠️ class differs |
| `315576000000s` | rc1, **range** message | dead, exit 1 | rc1, **`Invalid duration: Duration out-of-range`** ⚠️ class differs |
| `315576000001s` | rc1, protojson | dead, exit 1 | rc1, JSON parse ✅ |

**rc parity on every value. Message-class parity on the range class only** (§0.1, §3.2).

⚠️ **THIS IS A FLOOR, NOT AN ESTIMATE** (`reference_measured_prototype_is_a_lower_bound`). It omits:

- the per-class message decision (§3.2)
- the two `bootstrap.go` comment sites (§3.4)
- every new unit arm (§3.1)
- an NC roster
- the fixture (§7)
- ADR-0324
- the `BEHAVIOR_CONTRACT.md` `:845` edit and a delta-only `+0` ledger entry

**Fixture floor by SHAPE:** a both-sides boot-reject fixture is exactly `0042`'s shape: **2 files, 289
lines** (`README.md` + `driver/driver.go`, `git ls-files … | xargs cat | wc -l`). Siblings are `0044`
290, `0056` 352 and `0058` 363. Those are the files' sizes, not a measured `numstat` of a new fixture;
re-measure when built. It is **not** `0126`'s +794 listener shape.

**Stat surface: `+0` predicted.** The repair adds no name.

---

## 7. The differential measurement

### 7.1 There is no existing gate

Every stats-sink fixture (`0089`-`0113`) sets an in-range value on both sides, so no fixture can see
the divergence.

### 7.2 What the gate can be

The harness already carries a both-sides boot-reject branch: `BootRejectFixture` in
`test/differential/harness.go`, run by `runBootRejectFixture` in `runner_test.go`. It asserts both sides
fail to boot AND both stderrs contain `ExpectedBootErrorSubstring()`. It runs **one arm per fixture
directory** ("one dir = ONE runner branch"). A `0s` (or `300s`) arm with the substring `value must be
inside range [1ms, 5m0s)` is **RED at the tip** (envoy-go boots) and green under any repair whose
message carries that substring. ⇒ The natural fixture is **`0127`**, reference port `15127` on the
`15000 + index` convention. It reads **ZERO** hits (`git grep -c '\b15127\b' -- test/ internal/ cmd/`,
rc 1) at this tip; re-census before use.

**The SPEC must decide:**

- one fixture (range class) or two (plus a negative class, which needs the message mirrored)
- whether the remaining reject arms are unit + `TestEnvoyGoBinary_ModeValidate`-style binary arms

A two-side-BOOTING fixture cannot carry any reject arm.

---

## 8. Sentinel — RUN MECHANICALLY, ACTUAL OUTPUT, BOTH SIDES OF THIS STAGE'S OWN ADD

All commands were copied verbatim from `next-prompt.txt`, with `/usr/bin/grep` named.

### 8.1 PRE-ADD, at `33f24624` (`ROADMAP.md` 251 lines, tail row 101 `done` at `:163`)

- (1) **SILENT**.
- (2) **SIX**, at `:211 :217 :223 :233 :239 :247`.
- (3) **SILENT**.

`stop` was verified absent at the git root.

### 8.2 The four NCs and the check-(2) positive control, PRE-ADD — ALL FIRED

- **NC-A:** the substitution was inspected first (`NC LANDED? [ in-progress ]`), then it read **ONE**
  line, `NOT DONE: row 62`.
- **NC-B** (`want=132`): **ONE** line, `GATE FAIL: examined 133 data rows, expected 132`.
- **NC-C:** residual **0**, and `NEVER OPENED: gRPC   <- NC FIRED`.
- **NC-D:** **96 / 68** under `--`.
- **Check-(2) positive control:** residual **0**, with **6** substitutions asserted.

### 8.3 Escape-aware malformed set and per-line digests, PRE-ADD

`sed 's/\\|//g' ROADMAP.md | awk -F'|' '/^\| *[0-9]/ && NF!=8'` → exactly **{57, 69}** at file lines
**119** (NF 9) and **131** (NF 10).

Per-line md5, **trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`, first 12 hex):

- `211 10d7807bf02d`
- `217 4a92f7e62fc6`
- `223 2a7eb298b9fd`
- `233 242e53c6f7a3`
- `239 b2680e6f4fbf`
- `247 6caa1c3ce0e7`

**All six are byte-identical to the phase-101 close.**

### 8.4 POST-ADD — measured on the other side of this stage's own ADD

Row 102 was installed after `:163` as `in-progress`, gated FIRST in a scratch file. Results:

- **8 fields naive, 8 escape-aware.**
- **Zero** hits for either sentinel match phrase, for the bare word `deferred` (case-insensitive), for
  `-family row`, and for `\|`.
- `ROADMAP.md` went **251 -> 252** (`git diff --numstat`: `1 0`).

Re-run at `want=134`:

| check | pre-ADD (§8.1-8.2) | **post-ADD** |
|---|---|---|
| (1) | SILENT | **ONE** — `NOT DONE: row 102` |
| (2) | SIX at `:211 :217 :223 :233 :239 :247` | **SIX at `:212 :218 :224 :234 :240 :248`** — every window shifted +1, as an insertion ABOVE them must |
| (3) | SILENT | SILENT |
| NC-A (row 62 doctored) | ONE | **TWO** — `NOT DONE: row 62`, `NOT DONE: row 102` (substitution inspected: `NC LANDED? [ in-progress ]`) |
| NC-B | ONE at `want=132` | **TWO at `want=133`** — `NOT DONE: row 102`, `GATE FAIL: examined 134 data rows, expected 133` |
| NC-C | FIRED, residual 0 | FIRED, residual 0 |
| NC-D (`--`) | 96 / 68 | **96 / 68** — unmoved; the new cell spells no `-family row` |
| check-(2) positive control | residual 0, 6 substitutions | residual 0, 6 substitutions |
| escape-aware malformed set | {57, 69} at `:119`, `:131` | **{57, 69} at `:119`, `:131`** — unmoved (both above the insert) |
| row 102 NF | — | **8 naive, 8 escape-aware** (at `:164`) |

Per-line md5 at the SHIFTED lines, trailing newline INCLUDED:

- `212 10d7807bf02d`
- `218 4a92f7e62fc6`
- `224 2a7eb298b9fd`
- `234 242e53c6f7a3`
- `240 b2680e6f4fbf`
- `248 6caa1c3ce0e7`

**All six are byte-identical to §8.3: the windows MOVED and did not CHANGE.** The stale swallowed-panic
claim now sits at `:234`, **recorded, not tidied.**

Fixture registration, re-run verbatim (the blank-import extractor over
`test/differential/runner_test.go`): **dirs 128 = imports 128**, both `comm` directions EMPTY. That is
unmoved, as a BRAINSTORM adds no fixture.

Memory-slug audit over `next-prompt.txt` + `STATE.md`: **19** distinct slugs by the one
`(reference|feedback|topic)_[a-z0-9_]+` pattern, each tested with `[ -f … ]`. **Exactly the six banked
`ROADMAP.md` slugs read MISSING, no seventh.**

⇒ **THE SENTINEL DOES NOT FIRE on either side of this ADD. `stop` was evaluated and NOT created**
(absent at the git root and in the stage worktree).

---

## 9. Findings the next stage must not re-learn

1. **The reference has TWO reject classes**: range, and `Invalid duration` for negatives and the
   protobuf maximum (§0.1).
2. **It rejects at BOOT, with or without a sink** (§0.2, §0.4). Do not gate the check on sinks.
3. **The existing suite is blind**: a behaviour-inverting `7 2` is 391 / 0 (§0.5).
4. **The blast radius is empty**, censused (§0.6). Leave `0089`-`0113` alone (§3.5).
5. **Compute from `GetSeconds()` / `GetNanos()`, bound both**; `AsDuration` saturates (§0.7).
6. **Sub-ms cadence is unobservable on the reference here**; never pin it (§0.8).
7. **The `> 0` guard is also a `NewTicker` panic guard** (§2.3, §3.3).
8. **On this host the reference reaches the host at `192.168.65.2`, not `172.17.0.1`.** Copy the probe
   recipe from memory; do not paraphrase it (§0.11).

---

## 10. What the SPEC owes

1. **Specify the validity rule exactly**:
   - Accept `{seconds, nanos}` with `0 ≤ nanos ≤ 999999999`, `seconds ≥ 0`, and a total in
     `[1 ms, 300 s)`.
   - Absent means 5 s.
   - Everything else is a boot reject, computed from the fields.
   - **Decide the message per class** (§0.1, §3.2), keeping the `bootstrap: ` prefix `FuzzBootstrapLoad`
     requires (§0.6).
2. **Name the new RED arms** (§3.1): unit arms with asserted phrases, a no-sink arm, a binary
   `-mode validate` arm, and the Go-built field arms.
3. **Decide the fixture** (§7.2): `0127` boot-reject on the range class (port `15127`, re-censused), and
   whether a second class earns a second directory.
4. **Specify the NC roster** with a named mechanism per row (§3.3), including the lower-bound-only
   deletion and the nanos clause.
5. **Draft ADR-0324's §Context** (next-free **ADR-0324**, the house `> **STATUS: PROPOSED` block form)
   amending ADR-0262's D-MS-FLUSH parse arm. **Predict the ledger delta (`+0` names)**.
6. **Re-derive the occurrence set** (§3.4) and state what the row does NOT buy (§1.4).
7. **Re-measure the reference's `-1s` and `315576000000s` messages at the SPEC's tip** before pinning
   any substring. Two values on one run are one sample (method note 34).

---

## 11. Probe hygiene

- **Worktrees:**
  - The stage worktree is `/home/esa/git/envoy-go-wt-102b` (branch `wt-phase-102-brainstorm`, off
    `33f24624`, fast-forwarded to `698a001b`).
  - The subject agent's throwaway prototype and tip worktrees were **created and removed**, verified by
    `git worktree list`.
- **Docker:**
  - One agent only (the reference agent), with containers `p102ref-*` torn down by name
    (`docker ps -a --filter name=p102ref-` → header only).
  - One helper container, `p102ref-udptest`, used the local `python:3.13-alpine` image; every other
    container used the pinned Envoy image.
  - No network was created, and no other container was touched (foreign `pv4-116-up-881018` and
    `golink-ai` were present and left alone).
- **Ports:**
  - The band is `17000-17099`, censused first: `git grep -Inw -E '170[0-9][0-9]' -- test/ internal/
    cmd/` read **0**, and nothing was bound under `ss -tan` / `ss -uan`.
  - The reference agent used `17000-17049` (receiver `17000`, admin `17001`, listener `17002`).
  - The subject agent used `17050-17090`.
  - Only client TIME-WAIT was left.
  - ⚠️ **This paragraph spells band numbers; it is a hit in the next census.**
- **Scratch:** everything is under the session scratchpad (`ref/`, `subj/`), with nothing in any
  worktree.
- **Scope:** no production `.go` was written and none of the six gates was run. **That is a
  BRAINSTORM's SCOPE, not an omission.** The one standing departure (no `REVIEW.md`, none of 93-101) is
  not this stage's to fix.
