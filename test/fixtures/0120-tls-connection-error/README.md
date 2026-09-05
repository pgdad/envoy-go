# 0120-tls-connection-error

Phase 94. Proves that a downstream TLS handshake failing with an **SSL protocol
error** increments `listener.<addr>.ssl.connection_error`, while a downstream
connection failing with a **transport condition** — a clean FIN before any TLS
byte — increments **nothing** under `ssl.*`.

This is a *differential* fixture: the same five client behaviours are driven
against two proxies, upstream Envoy (the **reference**, a pinned container
image) and `envoy-go` (the **subject**, a host process), and the fixture asserts
that both book the same counters.

## Why this fixture exists

`ssl.connection_error` is easy to get *nearly* right. An implementation that
incremented it on **every** failed handshake would look correct on four of the
five arms below. The fixture is built so that exactly one arm separates a
counter that *can move* from a predicate that *discriminates*.

## What is new here, in this tree

1. **The first fixture to drive deliberately FAILING TLS handshakes that are not
   certificate failures.** Two earlier fixtures — `0110-tls-require-client-cert-false`
   and `0111-tls-cvc-empty-dynamic-fallback` — do drive failing handshakes, but
   every one of theirs fails in *certificate verification*. Every other TLS
   client in the tree pins `MinVersion: VersionTLS12` and is configured to
   succeed at the protocol layer. Protocol-level failure had never been driven
   before.
2. **The first fixture to ship a `common_tls_context.tls_params` block**, in both
   `envoy.yaml` and `envoy-go.yaml`, booted on both sides. Verified by grep:
   `tls_params` appears in no other file under `test/`.

A third "first" DOES hold, and the way it was nearly lost is worth recording.
`pki/client.pem` **is** the tree's first *committed* `clientAuth` leaf.

⚠️ **A mid-build measurement appeared to refute this and was itself wrong.** It
decoded the `extendedKeyUsage` of every `*.pem` **present on disk** under `test/`
and found a second holder, `test/fixtures/0018-http-rbac/pki/client.pem`. That
file is **not committed**: `test/fixtures/0018-http-rbac/pki/.gitignore` lists it
along with the other four, and `pki/gen.go`'s `init()` regenerates them on every
test-binary load — deliberately, so the PKI stays fresh and no key is committed.
The only tracked files in that directory are `.gitignore`, `gen.go` and
`gen_test.go`. The probe had measured the WORKING TREE where the claim is about
GIT, and a previous test run had left the generated file lying there.

Re-derived over `git ls-files 'test/**/*.pem'` — i.e. tracked files only —
exactly one committed leaf under `test/` carries `TLS Web Client Authentication`,
and it is this fixture's. **When two measurements disagree, find the variable:
here it was `committed` versus `present on disk`, and both probes reported
truthfully about different things.**

## Topology

```
  driver (host, Go)
      |
      |  5 arms, in a fixed order, inside ONE Drive pair per side
      v
  l_conn_err  ── TLS-terminating tcp_proxy listener
      |            require_client_certificate: true
      |            tls_params.tls_minimum_protocol_version: TLSv1_2
      |            tls_certificates:  inline_string  (pki/server.pem + key)
      |            validation_context.trusted_ca: inline_string (pki/ca.pem)
      v
  c_echo      ── TCP echo backend on the host (BackendCount = 1)
```

Reference listener port **10126** in-container, admin **9901**. The subject's
listener and admin ports are both allocated by the runner at run time.

## The five arms

They all run inside a **single** `DriveReference` / `DriveSubject` pair, because
one fixture directory is one runner branch: there is exactly one Drive pair and
at most one `AssertStats`. The order is fixed and load-bearing.

| # | arm | what the client does | `connection_error` | `handshake` |
|---|---|---|---|---|
| 1 | **(v) valid** | full mTLS handshake with the client cert **force-sent**, then an application echo round-trip | +0 | **+1** |
| 2 | **(i) bad_version** | `ClientHello` whose *maximum* offered version is TLS 1.1 | **+1** | +0 |
| 3 | **(ii) plaintext** | plain TCP; writes `GET / HTTP/1.1\r\nHost: x\r\n\r\n` to the TLS port | **+1** | +0 |
| 4 | **(iii) garbage** | plain TCP; writes six non-TLS bytes (`DE AD BE EF 00 11`) | **+1** | +0 |
| 5 | **(iv) clean FIN** | plain TCP; writes **zero** bytes and closes cleanly | +0 | +0 |

Every row was **measured**, per arm, with a before/after `/stats/prometheus`
snapshot taken around that arm alone — not one scrape at the end of the run —
against a live reference container and a live subject process. The two sides
produced identical deltas on every row.

The positive arm runs **first** so that a broken upstream is caught before any
negative arm can be misread.

### Arm (iv) is the point of the fixture

Arm (iv) opens a TCP connection to the TLS port and closes it without writing a
byte. The server's accept succeeds; its first handshake read returns a bare
`io.EOF`. In `envoy-go` that lands in `classifyHandshakeErr`, which has no EOF
branch and so returns `outcomeOther` — the same classification the three
protocol arms get. The only thing that stops the counter moving is the `io.EOF`
term of `isTransportHandshakeErr`, the predicate that matches the *transport*
complement and lets the caller increment otherwise.

Delete that one term and this arm goes **red on the subject** while all four
other arms stay green. That is what "discriminating negative control" means
here: without arm (iv) the fixture would prove only that the counter *can* move.

The reference books nothing on this arm either — measured, `connection_error`
+0 — so the two sides agree, which is what the differential asserts.

### Over-firing controls

A positive arm cannot catch a counter that fires *too often*. Three stacked
controls were run against both sides and produced identical results:

| control | result |
|---|---|
| 3 × clean FIN | `connection_error` +0 |
| 3 × valid | `handshake` +3, `connection_error` +0 |
| 2 × bad version | `connection_error` +2, `handshake` +0 |

## Design decisions, and the traps behind them

### The bad-version arm is produced CLIENT-side only

The obvious way to write arm (i) would be to lower the listener's minimum
protocol version in YAML. That is **forbidden** here. `envoy-go`'s
`internal/tls/params.go` boot-**rejects** the two pre-1.2 protocol-version enum
values, so a config-side expression of this arm would take the subject down at
boot rather than produce a handshake failure. Neither YAML contains either
value, and the YAML comments deliberately spell them out as prose so that a
literal-text grep over the configs finds nothing.

The client-side form has its own trap, and the driver guards it. Since Go 1.22
the default *client* minimum is TLS 1.2, so setting only `MaxVersion` would make
the dial fail **locally** — nothing would go on the wire, and the server would
see a clean FIN. Arm (i) would silently degrade into a second copy of arm (iv)
and book `connection_error` +0 while the client still reported a failed
handshake. The driver therefore lowers `MinVersion` to TLS 1.0 alongside
`MaxVersion`, and the measurement confirms the hello reaches the wire: the
reference answers `protocol version not supported`, which is a *server* alert.

### Every arm force-sends its client certificate, and logs what it sent

Go's polite client (`Certificates:`) filters the configured chain against the
server's advertised acceptable-CA list and **silently sends an empty chain**
when it does not match. An arm that hits that path is measuring the client
rather than the server, and every byte comparison stays green while it does.
The driver installs `GetClientCertificate`, which bypasses that filtering, and
`log.Printf`s the chain length actually handed to the stack, per arm and per
side. Measured: `client_cert_chain_len=1` on every handshaking arm, and
`ssl.no_certificate` stays 0 on both sides. (`fixture.TB` exposes only `Errorf`,
`Fatalf` and `Helper`, so `log.Printf` is the recording channel.)

### Certificates are delivered `inline_string:`, never `filename:`

`pki/` lives on the **host**, where `envoy-go` runs. It does **not** exist inside
the reference container, and this fixture implements no `ReferenceLogMounter`
bind-mount, so a `filename:` reference would simply not resolve on the reference
side. The PEMs are read by the driver and rendered into both templates as
pre-indented YAML block scalars.

The subject *could* read `filename:` paths, since it runs on the host. It
deliberately does not: a divergent data-source specifier across the two sides
would make any certificate-related divergence ambiguous. Both sides are kept
byte-symmetric.

### The reference port is 10126, not 10120

The tree's convention maps fixture `0NNN` to in-container port `10NNN`, which
would give `10126`'s slot to `10120`. `10120` is **taken**: fixture
`0028-http-lua-multi-script-and-per-route` holds `10120`–`10125` as a
contiguous run of six listeners (`inputs/driver.go:65-70`). `10126` is the
minimal index-preserving repair — the first free port above the occupying run.
It was censused as free by phase 92 (`92/SPEC.md:79-80`, `:393`;
`92/PLAN.md:823`) and re-verified at this tip: zero hits anywhere else under
`test/`.

## What `AssertStats` pins

Absolute per-side values — not deltas — on **both** sides:

```
envoy_listener_ssl_connection_error     == 3    arms (i), (ii), (iii)
envoy_listener_ssl_handshake            == 1    arm (v)
envoy_listener_ssl_fail_verify_error    == 0    NEGATIVE HALF
envoy_listener_ssl_fail_verify_no_cert  == 0    NEGATIVE HALF
```

Absolute values are safe here because nothing pre-moves this listener's `ssl.*`
counters: the stats step runs strictly after both Drives, reference readiness
polls admin `9901` rather than the TLS port, and the subject signals readiness on
stdout. The five arms are the only connections `l_conn_err` ever sees.

**These numbers are arm arithmetic. Adding a sixth arm invalidates them.**

The two zero pins are not decoration. A pin proving `connection_error` moved
says nothing about whether a *certificate* counter also moved; an implementation
that booked every failed handshake under all three names would satisfy the
positive half on its own.

### Arm (v) asserts the echo round-trip, not just the handshake

Measured: a cluster that cannot reach its backend lets the client report
`HANDSHAKE=OK` while the reference's `ssl.handshake` reads **0**, because
`tcp_proxy` tears the downstream connection down before the handshake is booked.
A handshake-only client check would report success while the positive control was
silently zero. The echo round-trip is what closes that gap — and it is why
`BackendCount()` returning 1 is a real backend rather than a formality (the
runner also rejects 0).

## Cross-side divergences that are deliberately NOT asserted

1. **The `envoy_listener_address` label value.** Measured live, the reference
   renders `envoy_listener_ssl_connection_error{envoy_listener_address="0.0.0.0_10126"}`
   and the subject renders the same metric with `envoy_listener_address="___12127"`
   — `envoy-go` binds `[::]` and its listener port is runner-allocated, so the
   label value can never match. `AssertStats` keys on the metric **name** and
   strips the label set entirely. Keying on the name resolves all three
   cross-side scope divergences at once — dotted address form, IPv6 bracket
   normalization, and `stat_prefix` — because the Prometheus name carries none
   of them.

2. **Arm (iii)'s wire-level reply.** Measured: the reference answers the garbage
   bytes with a fatal TLS alert record (the bytes read back begin
   `15 03 01 00 02 02`), while `envoy-go` answers with nothing and the client's
   read returns `n=0 err=EOF`. Both are valid rejections. This is why the driver
   returns a **non-nil empty** `[]byte` from both sides instead of the bytes it
   read: returning read bytes would fail the runner's byte comparison for a
   reason that has nothing to do with this fixture's subject. The arm inspects
   its read result for exactly one forbidden outcome — the payload coming back
   verbatim, which would mean the port never terminated TLS at all.

3. **The client-visible alert text on every failing arm.** BoringSSL and Go's
   `crypto/tls` emit different alerts for the same rejection, so no cross-side
   equality can be built on error text. The driver collapses every failure to a
   single token for the log. The drive bytes can therefore only distinguish
   FAILED from SUCCEEDED — *which* failure occurred is a stats question, and
   that is where all of this fixture's discrimination lives.

## Running it

```sh
go test ./test/differential/ -run 'TestDifferential/0120-tls-connection-error' -count=1 -v
```

`-count=1` is **not optional**. The harness builds `envoy-go` as a subprocess, so
a production-code edit is not a compile-time input to this test binary and the
Go test cache will serve a stale **pass**.

A `-run` selector that matches nothing prints `ok ... [no tests to run]` and
exits **0**, so check that the log actually contains `=== RUN` lines before
believing a green result.

## Files

| file | what it is |
|---|---|
| `driver/driver.go` | the fixture driver: config rendering, the five arms, `AssertStats` |
| `envoy.yaml` | reference bootstrap template (`STRICT_DNS` + `host.docker.internal`) |
| `envoy-go.yaml` | subject bootstrap template (`STATIC` + `127.0.0.1`) |
| `pki/` | committed CA, server leaf + key, client leaf + key |
| `expectations.yaml` | the prose record of the arm table, the pins, and the unasserted divergences |
