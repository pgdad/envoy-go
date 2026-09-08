# 0121-listener-default-chain-tls

Phase 96. Proves that a listener carrying **no `filter_chains[]` key at all** —
one whose only chain is its `default_filter_chain`, and whose only TLS context
lives there — registers the five listener-scope `ssl.*` counters and books
`ssl.handshake` **and** `ssl.no_certificate` once per completed handshake, on
both sides.

This is a *differential* fixture: the same three client behaviours are driven
against two proxies, upstream Envoy (the **reference**, a pinned container
image) and `envoy-go` (the **subject**, a host process), and the fixture asserts
that both book the same counters.

## Why this fixture exists

`envoy-go`'s listener manager derives its "this listener terminates TLS"
boolean by walking `filter_chains[]`. A listener whose TLS lives **only** in
`default_filter_chain` leaves that boolean **false**, so
`registerListenerMetrics` skips the whole `ssl.*` family — while the
success-path `Inc` on the serve path is guarded by a *different* predicate that
is **true**. The first completing TLS handshake therefore dereferences a nil
counter and takes the **process** down (`internal/listener/manager.go:1393`,
`SIGSEGV` in `sync/atomic.(*Uint64).Add`). There is no `recover()` on that path.

The stat hole is the visible half; the crash is the half that matters. This
fixture pins the *reference's* behaviour on exactly that shape, so the repair
has a cross-side target rather than a guess.

## What is new here, in this tree

- The **first** fixture whose listener carries **no `filter_chains[]` key at
  all**. Every other TLS fixture expresses its chain(s) under `filter_chains[]`.
  ⚠️ Adding `filter_chains[]` to either bootstrap makes the fixture **vacuous** —
  the shape *is* the subject of the row.
- The **first** cross-side `ssl.*` assertion on a `default_filter_chain`.
  `0120-tls-connection-error` is the only earlier cross-side `ssl.*` assertion at
  all, and its TLS lives in a normal `filter_chains[0]` slot.
- The **first** fixture to pin `ssl.no_certificate` as a **mover** rather than as
  a zero.
- The **first** TLS fixture with **three** PEMs and **no committed client leaf**.
  The absence is load-bearing — see below.

## Topology

```
                    3 x (TLS handshake + HTTP/1.1 GET /)
  driver  ────────────────────────────────────────────────►  l_dfc
                                                               │
                                                    default_filter_chain
                                                    (DownstreamTlsContext)
                                                               │
                                                              HCM
                                                               │
                                                  direct_response 200
                                                  "phase96-default-chain-ok\n"

  c_placeholder (STRICT_DNS ref / STATIC subj) — allocated, NEVER dialled
```

One listener, one chain, one route. The backend the runner allocates is never
connected to.

## The three arms

All three arms are identical, and that is the design. Each is:

1. a fresh TCP connection (no pooling, `Connection: close`),
2. a TLS handshake against the committed fixture CA with `ServerName
   l_dfc.fixture.test` and `InsecureSkipVerify: false`,
3. an HTTP/1.1 `GET /`,
4. an assertion of **HTTP 200 and the exact body**, *before* any counter is read.

**The drive count is N = 3, by decision.** N = 1 is viable and strictly weaker:
the value `1` is consistent both with a per-connection counter and with a
fire-once one, so N = 1 cannot discriminate them. N = 3 can. ⚠️ The `want*`
constants in `driver/driver.go` are **arm arithmetic** — changing `armCount`
invalidates the four nonzero pins.

⚠️ One connection **per arm** is what makes the arithmetic legible. A pooled or
reused connection would book **one** handshake for three requests and the pins
would read 1 against a want of 3 with nothing wrong on either side.

## Design decisions, and the traps behind them

### The route is `direct_response`, not a route to the cluster

A route to a fast-failing upstream **suppresses** the listener-scope `ssl.*`
counters on the reference side
(`reference_ssl_stats_suppressed_by_fast_failing_upstream`): the connection is
torn down before the handshake is booked, the pin reads 0, and *the
configuration looks fine while the client reports a successful handshake*.
`direct_response` makes that failure mode structurally impossible — no upstream
is ever dialled.

The placeholder cluster is retained anyway and cannot be dropped: an omitted
`clusters:` key **boot-rejects** `envoy-go`, and the runner `t.Fatalf`s on
`BackendCount() < 1`. `BackendCount()` returns **1**; nothing ever connects to it.

### There is no `require_client_certificate`, and no `validation_context`

That absence is the reason `ssl.no_certificate` is the **second mover**. The
listener sends no `CertificateRequest`, so a handshake that completes without a
client certificate books **both** `ssl.handshake` and `ssl.no_certificate`.

⚠️ A forecast map of `{handshake: N, everything else: 0}` fails against
**correct** code, on **both** sides, at every drive count. The map in
`AssertStats` was measured on both sides before it was written.

### Three PEMs, and no client leaf

`pki/` holds exactly `ca.pem`, `server.pem`, `server.key.pem`. There is
deliberately **no client leaf**: one in this directory would invite an arm that
presents it, and `ssl.no_certificate` would stop being the second mover.
`pki/gen` regenerates all three byte-identically (verified with `sha256sum -c`
across a second run).

⚠️ The leaf **must** carry a DNS SAN matching the `serverName` the driver dials
(`l_dfc.fixture.test`). Without it every arm fails verification **client**-side,
never reaches the server, and the five counters read zero **with no server-side
fault at all** — a vacuous red that looks exactly like a real one.

### Certificates are delivered `inline_string:`, never `filename:`

`pki/` exists on the **host**, where `envoy-go` runs. It does **not** exist
inside the reference **container**, and this fixture implements no
`ReferenceLogMounter` bind-mount. The PEMs are rendered into both templates as
pre-indented block scalars (22 spaces).

⚠️ The PEM substitution keys are written in the YAML header comments **without**
their double-brace delimiters. `text/template` expands actions inside `#` lines
too, and a multi-line PEM expanded into a comment splatters its continuation
lines outside the comment and produces invalid YAML.

### The reference port is 10127 — not 10121, and not 10126

Recorded here so the next fixture does not re-derive it:

- The `10<fixture index>` convention would give **10121**. It is **taken**:
  `0028` holds `10120`–`10125` as one contiguous six-listener run
  (`0028/inputs/driver.go:65-70`).
- **10126** is **taken** by `0120-tls-connection-error`.
- **10127** is the first free port above both. Censused at this tip:
  `git grep -n 10127 -- test/ internal/ cmd/` reads **zero** hits, and the
  negative control `git grep -n 10126` reads four occupied files — so the census
  command is not silently matching nothing.

⚠️ Count fixture directories with `ls -d test/fixtures/*/ | wc -l`, not with
`grep -cE '^[0-9]{4}-'`: the latter reads 120 where the former reads 122,
because it drops `0007a-cors` and `0007b-iteration-probe`.

### Every `ssl` match is anchored

⚠️ `^listener\.[^:]*\.ssl\.` on the dotted `/stats` side, `^envoy_listener_ssl`
on the prometheus side. An **unanchored** `grep ssl` over the reference's full
`/stats` on this very shape reads **24** lines against the anchored form's
**17** — the extras are the `http.<prefix>.downstream_cx_ssl_active`/`_total`
pairs. The subject has its own false witness for the unanchored form,
`server.acce`**`ssl`**`og_dropped`. Both witnesses appear simultaneously, one per
side, on the same config pair.

## What `AssertStats` pins

Per side, over `/stats/prometheus`, as a **named subset** with **exact**
equality:

| metric | want |
|---|---|
| `envoy_listener_ssl_handshake` | **3** |
| `envoy_listener_ssl_no_certificate` | **3** — ⚠️ 3, not 0 |
| `envoy_listener_ssl_fail_verify_error` | 0 |
| `envoy_listener_ssl_fail_verify_no_cert` | 0 |
| `envoy_listener_ssl_connection_error` | 0 |
| `envoy_listener_downstream_cx_total` | **3** (liveness) |

⚠️ **Exact equality, never a floor.** A floor cannot tell a per-connection
counter from an over-firing one, which is the whole reason N is 3.

⚠️ **`t.Errorf` per violation, never `t.Fatalf`.** A `Fatalf` on the reference
side would make every subject-side assertion dead code
(`reference_fatalf_makes_assertions_unreachable`). The only `Fatalf` is the
scrape itself, where there is nothing left to assert.

⚠️ The keys are metric **names** with the **label set stripped entirely**.
Stripping is required, not a convenience: the reference renders
`envoy_listener_address="0.0.0.0_10127"` and the subject an IPv6-wildcard form
on a runner-allocated port, so any label-preserving key is cross-side
incomparable by construction.

⚠️ The `_ fixture.StatsAsserter` compile-time assertion in `driver.go` is
**mandatory**. The runner dispatches the stats step through a silent type
assertion with no `else` branch, so a signature typo makes `ok == false` and the
entire stats leg — where all of this fixture's discrimination lives — never runs
while every tool stays quiet and green.

## Cross-side divergences deliberately NOT asserted

A closed enumeration; the full reasoning is in `expectations.yaml`.

1. **`no_filter_chain_match` — not asserted at all, not even `== 0`.** The
   divergence is **name**-level, not value-level: the reference emits
   `listener.0.0.0.0_10127.no_filter_chain_match: 0`, the subject does not emit
   the name (the string has no production site in this repository). Because
   `scrapeProm` returns the zero value for a missing key, a `== 0` pin would
   pass on the subject **for the wrong reason** — silently vacuous rather than
   red. `downstream_cx_total` stands in its place.
2. **The twelve reference-only listener-scope `ssl.*` names**:
   `certificate.<name>.expiration_unix_time_seconds`, `ciphers.<suite>`,
   `curves.<curve>`, `fail_verify_cert_hash`, `fail_verify_san`,
   `ocsp_staple_failed`, `ocsp_staple_omitted`, `ocsp_staple_requests`,
   `ocsp_staple_responses`, `session_reused`, `versions.<version>`,
   `was_key_usage_invalid`. ⚠️ Never assert a name-**set** equality: the
   subject's five are a strict subset of the reference's seventeen. ⚠️ And never
   take a presence set **before** the drive — the reference registers
   **fourteen** at boot and seventeen only after the first handshake; the three
   latecomers are the dynamic `ciphers.`/`curves.`/`versions.` families.
3. **The `envoy_listener_address` label value**, which differs by construction.

## Running it

```sh
go test ./test/differential/ -count=1 -v \
  -run 'TestDifferential/0121-listener-default-chain-tls'
```

⚠️ `-count=1` is not optional — the suite's failure mode is a silent pass. ⚠️ A
`-run` selector matching nothing prints `[no tests to run]` and **exits 0**;
assert a nonzero `=== RUN` count beside `RC=0`.

**Expected state before the production repair lands:** RED on the subject side.
The reference completes all three arms (TLSv1.3, ALPN `http/1.1`, HTTP 200); the
subject completes arm 1's handshake and the process then `SIGSEGV`s at
`internal/listener/manager.go:1393`, so arms 2 and 3 get connection refused and
the failure surfaces at the runner's `subj drive` step rather than at
`AssertStats` — the subject process is dead before the stats leg can run. Do not
"repair" that by weakening the assertions.

Regenerate the PKI (manual only; CI never invokes it):

```sh
cd test/fixtures/0121-listener-default-chain-tls && go run ./pki/gen
```

## Files

| file | what it is |
|---|---|
| `envoy.yaml` | reference bootstrap; `STRICT_DNS` + `host.docker.internal`, in-container ports 9901 / 10127 |
| `envoy-go.yaml` | subject bootstrap; `STATIC` + `127.0.0.1`, runner-allocated ports |
| `driver/driver.go` | the three arms, `scrapeProm`, and `AssertStats` |
| `pki/ca.pem` | fixture CA |
| `pki/server.pem` | server leaf; SANs `l_dfc.fixture.test`, `localhost`, `host.docker.internal`, `127.0.0.1` |
| `pki/server.key.pem` | server key |
| `pki/gen/main.go` | deterministic regenerator for the three PEMs above |
| `expectations.yaml` | the proposition, the measured map, and the closed not-asserted enumeration |
| `README.md` | this file |
