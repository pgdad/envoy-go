# 0123-listener-transport-protocol

Cross-side differential for phase 98 (`chain-match-transport-protocol-reject`):
on a **plaintext TCP listener carrying no listener filter at all**, reference
Envoy (`contrib-v1.37.2`, Docker) stamps the connection's
`transport_protocol` as **`raw_buffer`** before filter-chain selection runs, so
a `filter_chain_match: {transport_protocol: raw_buffer}` chain is **eligible and
serves**. envoy-go left that input **empty**, the exact-value branch in
`listenerfilter.SelectChain` rejected the chain, and the listener fell through
to its default chain.

The row carries a **second** divergence on the same dimension: envoy-go
**boot-rejected** any `transport_protocol` string outside
`{tls, raw_buffer, quic, ""}` that the reference **accepts, boots and serves**.

This is Task 8 of the phase-98 IMPL spine. It builds on:

- Tasks 2-6 — the unit roster in `internal/listener/manager_test.go` and
  `quic_test.go`, recorded RED/GREEN at the un-fixed tip.
- Task 7 — this fixture's driver and its two rendered bootstraps.
- Tasks 11-12 — the two production edits: **lift the reject**, then **stamp
  `raw_buffer`** on an unclassified TCP connection.

## Shape: no YAML, no PKI, and ONE renderer for BOTH sides

Like `0104`, `0121` and `0122`, this fixture ships **no `.yaml` bootstrap files
and no `pki/` directory**. Unlike them it does not carry two hand-maintained
template literals either: `renderBootstrap` in `driver/driver.go` builds **both**
sides, and the only values that differ cross-side are the bind address, the admin
port and the three listener ports.

That is deliberate. The proposition under test is that two proxies given **the
same listener shape** select **different chains**, so shape-identity is
load-bearing. Two hand-maintained YAML blobs would make it a review exercise that
silently rots; one renderer makes it structural. At three listeners x two chains
the duplicated form would also have been ~250 lines of YAML per side.

Every listener is **plaintext**. There is no `transport_socket` anywhere, so the
fixture ships no certificates at all.

## Topology — THREE listeners, one string apart

| listener | `fc_indexed` `transport_protocol` | ref port | expected, **both sides** |
|---|---|---|---|
| `l_bogus` | `totally_bogus_value` | 15123 | `DEFAULT` |
| `l_raw` | `raw_buffer` | 15223 | **`INDEXED`** |
| `l_tls` | `tls` | 15224 | `DEFAULT` |

Each listener is rendered from one template:

```yaml
- name: l_<arm>
  address:
    socket_address: { address: <bind>, port_value: <port> }
  # ⚠️ NO listener_filters KEY. NO transport_socket KEY. Both absences are
  # experimental controls — see the two hazards below.
  filter_chains:
    - name: fc_indexed
      filter_chain_match:
        transport_protocol: <the one string that differs>
      filters:
        - HCM: stat_prefix <arm>_indexed,
               route "/" -> direct_response 200 "<arm>-indexed\n"
  default_filter_chain:
    name: fc_default
    filters:
      - HCM: stat_prefix <arm>_default,
             route "/" -> direct_response 200 "<arm>-default\n"
```

Both chains of a listener answer the **same** path (`GET /tp`) with the **same**
status (200) and **different** bodies, so the body alone names the chain that
served. All six bodies are distinct across the whole fixture, so a single
observed body names **both** the listener and the chain — a body reused across
listeners could not tell "`l_raw` served its default" from "the request reached
`l_tls`".

`l_tls` is named for the **string in its match**, not for any TLS it terminates.
Giving it a TLS transport socket would make its indexed chain legitimately
eligible on the reference and destroy its role as `l_raw`'s matched negative.

## 🔴 TWO WAYS THIS FIXTURE SILENTLY DISARMS ITSELF

### (a) Adding a listener filter to **any** listener disarms `l_raw`

`tls_inspector` — or any other listener filter — stamps
`inputs.TransportProtocol` from envoy-go's **own** pipeline, and for a
non-TLS preamble it stamps exactly `"raw_buffer"`. With one present, **both
sides answer `INDEXED` on `l_raw` even at the un-fixed tip**, because the
subject reaches the right answer through a mechanism this row is not about.

The fixture would then run **GREEN over a live divergence in the exact dimension
it claims to cover** — the worst outcome available to it, strictly worse than
failing. The absence of `listener_filters` from all three listeners is therefore
**a control, not an omission**. Do not add one "for realism".

### (b) `l_bogus` **alone** is a false-agreement arm

`l_bogus` catches the boot-reject divergence and nothing else. A subject that
parses `transport_protocol` and then **ignores the dimension entirely** answers
`DEFAULT` on `l_bogus` too — the right answer for the wrong reason. On its own it
cannot distinguish "the matcher was enforced and correctly did not match" from
"the matcher was never consulted".

**Only the `l_raw` / `l_tls` pair excludes a constant answer**, and it excludes
every constant:

| a subject that stamps... | `l_raw` | `l_tls` |
|---|---|---|
| nothing (`""` — the un-fixed tip) | **FAIL** | pass |
| a constant `"tls"` | **FAIL** | pass |
| a constant `"raw_buffer"` | pass | **FAIL** |
| the correct per-connection value | pass | pass |

Deleting either half of that pair reduces the fixture to a shape that cannot tell
*matched* from *unenforced*. Both halves are load-bearing.

### (c) A smaller one: the six `stat_prefix` values must stay distinct

Two HCMs sharing one `stat_prefix` in a single process **panic envoy-go at
boot**:

```
panic: stats: duplicate metric registration: "http.<prefix>.downstream_rq_total"
```

This fixture instantiates **six** HCMs and therefore sits **six identifiers**
away from that banked defect. Collapsing two prefixes does not weaken an
assertion — it stops the subject booting at all.

## The un-fixed-tip verdict — which arm is actually RED

With the boot reject lifted by hand and **nothing else changed**, the un-fixed
subject answers `DEFAULT` on **all three** listeners. So, at the un-fixed tip:

| arm | verdict | why it is in the roster |
|---|---|---|
| `l_raw` | 🔴 **RED** | the **only** arm that fails; the headline proof |
| `l_bogus` | 🟢 GREEN — **structurally** | the boot-reject probe; agrees for the wrong reason |
| `l_tls` | 🟢 GREEN — **structurally** | `l_raw`'s matched negative; agrees for the wrong reason |

⚠️ **Neither structurally-green arm is evidence on its own.** Reading either
one's green as a pass for this row is exactly the false-agreement error the
fixture is shaped to prevent. They earn their places only as, respectively, the
boot-reject probe and the matched negative.

Before the reject is lifted, `l_bogus`'s string makes the **subject fail to
boot** — the whole fixture fails at the subject-start step, not at an assertion:

```
listener manager: listener: "l_bogus": filter_chains[0]: transport_protocol
  "totally_bogus_value" must be "tls", "raw_buffer", "quic", or empty
```

## Asserted

`AssertStats` runs every assertion **absolutely, per side, per listener**, with
one `Errorf` per property (never `Fatalf` — a `Fatalf` on the first failing arm
would make the other two arms' checks unreachable dead code, and with one known-
RED arm that masking would hide precisely the pair that distinguishes *matched*
from *unenforced*). `Fatalf` is reserved for a broken precondition: the `/stats`
scrape failing, or a side having no recorded drive.

- **Status**, per arm per side: `200`. Asserted **before** the body — a body
  compared against a response that never reached a chain is not evidence.
- **Body**, per arm per side, **absolutely**: `"bogus-default\n"`,
  `"raw-indexed\n"`, `"tls-default\n"`. A wrong-chain body is reported **by
  name** (served by the INDEXED / DEFAULT chain), not as a generic mismatch.
  **The body is the primary discriminator**: it is controlled by this fixture,
  whereas a counter is controlled by each proxy's stats implementation.
- **Body, cross-side exact**, via the runner's `CompareBytes` over the
  three-line per-listener stream `driveAll` emits.
- **The six per-chain counters**, as **corroboration**, each as a pinned
  **value**:

  | name | pinned value | name | pinned value |
  |---|---|---|---|
  | `http.raw_indexed.downstream_rq_total` | **1** | `http.raw_default.downstream_rq_total` | **0** |
  | `http.tls_indexed.downstream_rq_total` | **0** | `http.tls_default.downstream_rq_total` | **1** |
  | `http.bogus_indexed.downstream_rq_total` | **0** | `http.bogus_default.downstream_rq_total` | **1** |

- **Admin** `/ready` (`ProbeAdmin`) on both sides.

The counter values are arithmetic over `armCount == 1` (one `GET /tp` per
listener per side). ⚠️ Changing `armCount` invalidates all six pins.

### Why the **value pair**, and why a name-presence pin would be vacuous

A scrape map returns the **zero value for a missing key**, so a `== 0` pin on a
name a side never emits would be silently **green, not red**. Each pin is
therefore preceded by an explicit **presence** check.

⚠️ **But the presence check is only a guard — it is never the assertion.** A pin
that asserts "the name is present" is **vacuous on both sides**: measured this
stage on the phase-97 two-chain shape, the reference publishes the per-chain
counters **at `0` before any traffic at all** — a pre-traffic scrape read **168**
lines matching `chain_(indexed|default)`, including the whole `downstream_cx_*`
family, with **zero connections made**. The per-chain HCM stat scope is created
at **config** time, not at first request, so such a pin is **satisfied from boot**
and says nothing about which chain served.

⇒ The only non-vacuous assertion is the **value pair** — served chain `1`,
non-served chain `0` — **plus its mirror arm**, which is what `l_raw` and `l_tls`
are: one string apart, same shape, opposite expected chains.

The `== 0` halves are known **satisfiable** rather than vacuous, by measurement
on the subject at this tip (one GET per listener against a hand-repaired subject,
flat `/stats`):

```
http.bogus_default.downstream_rq_total: 1     http.bogus_indexed.downstream_rq_total: 0
http.raw_default.downstream_rq_total: 1       http.raw_indexed.downstream_rq_total: 0
http.tls_default.downstream_rq_total: 1       http.tls_indexed.downstream_rq_total: 0
```

All six names **present**, the three non-serving ones **present at 0**. The
reference half of SPEC §7.3's obligation needs Docker and lands with the fixture
run.

### Why no `no_filter_chain_match` arm

envoy-go has **no production site for that string at all**, so a `== 0` pin on it
would read the zero value for a missing key and pass for the wrong reason
forever. That divergence is **name-level**, not value-level, and is out of scope
here. It is also why **every** listener carries a default chain: "which chain
served" is read from the body, never from the absence of a match.

## The stats endpoint: the **flat** `/stats`, measured not inherited

`scrapeStats` reads `http://<admin>/stats` and parses `name: value` lines into a
**map** — line order is never pinned (flat `/stats` is alphabetical, prometheus
is registration order). The split is on the **last** `": "` so a name containing a
colon is not truncated, and a value that does not `ParseUint` (every histogram) is
**skipped**, not coerced.

⚠️ The claim that *"every fixture driver that cross-asserts stats scrapes
`/stats/prometheus`"* is **false**, and was re-derived at this commit rather than
cited. Of the **88** non-test fixture drivers declaring `AssertStats`, with
whole-line `//` comments stripped:

| endpoint | drivers |
|---|---|
| `/stats/prometheus` | **31** |
| the flat `"/stats"` | **47** |
| neither — a sink **receiver**, not an admin scrape (`0089`-`0094`, `0098`, `0101`, `0112`, `0113`) | **10** |

⚠️ A **"31 / 57"** split folds those ten stats-sink drivers into the flat column;
the flat figure is **47**. The shape precedent `0122` is one of the 47 — and its
**only** `/stats/prometheus` occurrence is a **comment denying that it uses it**,
which is how the miscount arises. A name-presence grep cannot tell a use from a
disclaimer.

The names this fixture pins are plain dotted HCM counters carrying no address
token, so they are cross-side comparable verbatim on the flat surface, and the
flat form sidesteps the prometheus spelling
(`envoy_http_downstream_rq_total{envoy_http_conn_manager_prefix="<prefix>"}`)
entirely.

## UNasserted — a CLOSED enumeration

Everything asserted is listed above. These are the **only** exclusions; a name in
neither list reads as asserted.

1. `no_filter_chain_match`, on either side — name-level divergence, see above.
2. The `listener.<addr>.*` scope. The address token differs cross-side
   (`listener.0.0.0.0_15123.*` vs `listener.127_0_0_1_<port>.*` — different
   address **and** different dot-mangling), so any assertion keyed on the full
   listener-scoped name is cross-side **infeasible**
   (`reference_listener_stat_scope_cross_side_divergence`).
3. All `ssl.*` — no chain here terminates TLS.
4. All **histograms** — `scrapeStats` skips every line whose value does not
   `ParseUint`.
5. All `tracing.*` — no tracing provider on either side.
6. Every `http.<prefix>.*` name outside the six pinned `downstream_rq_total`
   ones. envoy-go emits only stats it increments
   (`reference_stats_sink_emits_used_only`).
7. Response header set equality, and connection-level framing.
8. `cluster.c_unused.*` — no route dials it.

## The subject-side cluster, and `BackendCount() == 1`

Every route on every chain is a pure `direct_response`, so **no cluster is ever
dialed**. envoy-go nevertheless boot-rejects a bootstrap whose
`static_resources.clusters` key is absent, and the runner rejects
`BackendCount() == 0` (`reference_differential_backendcount_min_one`). Both
bootstraps therefore carry a throwaway `c_unused` STATIC cluster pointed at the
runner-allocated, never-dialed backend port — rendered on **both** sides so the
shapes stay identical.

## Ports — censused, not inherited

`15123` follows the `15000 + <fixture index>` convention. `15223` and `15224` are
deliberately **off** that convention so fixtures `0124`/`0125` keep `15124` and
`15125`. Each of the three was re-censused at this tip: zero files under
`git grep -l '\b<port>\b' -- test/ internal/ cmd/` and zero live sockets under
`ss -tanH`.

The three **subject** ports are `subjListenerPort`, `+1` and `+2`. That is safe
because the harness allocates via a **16-port block** probed bindable on the
wildcard address before the base is returned; three of sixteen is well inside the
reservation. The three **reference** host ports are Docker-assigned and are
looked up per listener via `ref.ListenerAddr(refPort)` — never derived by offset
from a sibling's host address, which would be quietly wrong.

## Why the QUIC arm does not ride here

The differential harness cannot mix TCP and UDP listeners in one fixture, and one
fixture directory dispatches to exactly **one** runner branch
(`reference_differential_fixture_dispatch_constraint`). The
`transport_protocol: quic` arm — which parses and can never match on a TCP
connection — lives in `internal/listener/quic_test.go` instead.

## ⚠️ This fixture is INERT until its blank import lands

`TestDifferential` looks the fixture up in `fixture.DriverRegistry` by directory
name and **`t.Skipf`s** — a silent, green skip — when no driver is registered.
Registration happens through the driver package's `init()`, which runs only if
`test/differential/runner_test.go` blank-imports
`.../test/fixtures/0123-listener-transport-protocol/driver`. That import is a
**separate, later task**. Until it lands, this fixture contributes nothing to the
suite and passes vacuously by skipping.

All **four** registration gates must hold: the `RegisterFixture` call in
`init()`, the blank import, byte-identity of the registered name with the
directory name, and the `NNNN-` directory shape. Three of the four fail as a
**skip**; the fourth leaves no trace at all.

## A note on `expectations.yaml`

`expectations.yaml` in this directory is **prose** (ADR-0019) and is **not read by
any code** — measured at this tip: `git grep -n expectations` over
`test/differential/*.go` reads zero lines, and no package under `test/` imports a
YAML library. The enforcers are `AssertStats`, the runner's `CompareBytes` and
`ProbeAdmin`. Keep the two documents in agreement by hand; nothing checks it.
