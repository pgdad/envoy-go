# 0110-tls-require-client-cert-false

Phase 67. Proves that at **`require_client_certificate: false`** a downstream with
a **`combined_validation_context` (CVC)** does **verify-if-presented**: a presented
client cert is verified against the **SDS-delivered CA (`CA_X`), which REPLACES the
inline `default_validation_context.trusted_ca` (`CA_Y`)** and is rejected if it does
not chain to it, while a **no-cert connection is ACCEPTED**.

A disciplined clone of `0109-xds-sds-combined-validation-context`: same CVC shape,
same in-memory PKI machinery, same per-side SDS receivers. The one semantic delta
is on the `DownstreamTlsContext`: `require_client_certificate` flips **`true` →
`false`**.

## The delta vs 0109

| 0109 | 0110 | effect |
|---|---|---|
| `require_client_certificate: true` | **`false`** (bare scalar) | mandatory mTLS → **verify-if-presented** |
| two arms (trusted / untrusted) | **three arms** (trusted / untrusted / **none**) | the no-cert arm is the flag's discriminator |
| polite dial suffices | **FORCED-SEND** (`GetClientCertificate`) on trusted+untrusted | see below |

`0109` is `require=true` (mandatory mTLS): the served `CA_X` beats the inline
competitor `CA_Y`, and a no-cert connection is **rejected** (alert 116). `0110`
keeps the same CVC REPLACE and adds the **no-cert arm**, which at `require=false`
is now **accepted** — the single arm that separates verify-if-presented from
mandatory mTLS.

## Topology

```
                      SDS (SotW StreamSecrets, h2c)          in-memory,
  driver ──dial──▶ [ proxy: l_rccf ] ◀── sds_cluster ──▶ driver-owned
   (3 arms)          tcp_proxy                                sdsserver.Server
                        │                                     (ONE per side)
                        ▼
                    c_backend (TCP echo)
```

One TLS-terminating `tcp_proxy` listener. Its `DownstreamTlsContext` carries:

- `require_client_certificate: false` — **verify-if-presented** (Go:
  `VerifyClientCertIfGiven`). A **bare scalar** (`{value: false}` errors —
  `reference_protojson_wrapper_scalar_not_object`).
- a **STATIC** server cert (`tls_certificates`, `inline_string`, signed by
  `CA_X`), and
- a **`combined_validation_context`** nesting:
  - `default_validation_context.trusted_ca` = inline `CA_Y` PEM (the competitor),
    and
  - `validation_context_sds_secret_config` naming secret `rccf_validation_ca`,
    fetched over a SotW SDS gRPC stream against `sds_cluster`.

Only the validation context arrives via SDS, so boot's SDS pre-scan sees exactly
one secret — the CVC's **inner** SDS half — and the deferred compose-two edge is
never touched. Each side gets its **own** SDS receiver
(`reference_periodic_sink_differential_two_receivers`).

The served wire is **byte-identical to 0109's**: the management server cannot tell
a CVC client from a plain-SDS client (SPEC §8), so `test/helpers/sdsserver` is
**untouched**.

Reference: Envoy contrib-v1.37.2, `STRICT_DNS` via `host.docker.internal`,
listener port **10446**. Subject: envoy-go, `STATIC`, loopback.

## The PKI — in memory, per run; there is no `pki/` dir

| artifact | issuer | role |
|---|---|---|
| `CA_X` | self | **the anchor that MUST win** — served over SDS as `trusted_ca` |
| server leaf | `CA_X` | `ServerAuth`, SAN `DNS:l_rccf.fixture.test`; injected `inline_string` into both yamls |
| `client_X` | `CA_X` | `ClientAuth` — **must be ACCEPTED** |
| `CA_Y` | self | inline `default_validation_context.trusted_ca` — **the anchor that MUST lose** |
| `client_Y` | `CA_Y` | `ClientAuth` — **must be REJECTED** (proves REPLACE, not union) |

Per-run freshness is safe: the observable is an accept/reject verdict.

## The three arms (the observable)

All three arms fold into ONE byte stream. Both sides must emit, byte-identically:

```
trusted=ok echo=phase67-rccf-probe
untrusted=rejected
none=ok echo=phase67-rccf-probe
```

1. **trusted** — dial presenting `client_X` (chains to the SERVED `CA_X`),
   **FORCED-SEND**: handshake succeeds, write `phase67-rccf-probe\n`, read the echo
   back through `tcp_proxy`. `BackendCount()` returns 1
   (`reference_differential_backendcount_min_one`).
2. **untrusted** — dial presenting `client_Y` (chains to the INLINE `CA_Y`),
   **FORCED-SEND**: the round trip MUST fail → `untrusted=rejected`. If it succeeds
   the driver records `untrusted=ACCEPTED` — the union signal this fixture catches.
3. **none** — dial presenting **NO** client cert: at `require=false` the round trip
   MUST **succeed** → `none=ok echo=…` (the discriminator; at `require=true` this
   arm is rejected with alert 116).

Each arm drives the **full round trip**, not just the handshake: under TLS 1.3 the
server's verdict on the client cert arrives *after* the client's `Handshake()` has
already returned. The dial is inlined in the driver because neither
`helpers.TLSServedLeaf` nor `helpers.TLSRoundTrip` can present a client cert.

## ⚠️ Why FORCED-SEND is mandatory (the row's signature)

At `require=false` the server sets `VerifyClientCertIfGiven` with `ClientCAs=CA_X`
and advertises `CA_X` as the acceptable CA in its `CertificateRequest`. Go's
**polite** client (`Certificates:`) filters by `SupportsCertificate` and would
silently **WITHHOLD** `client_Y` (its issuer `CA_Y` is not advertised). The
handshake would then **succeed** as a no-cert connection, collapsing the untrusted
arm into a second `none` arm — a **vacuous green**
(`reference_go_client_cert_withholding`).

`GetClientCertificate` bypasses `SupportsCertificate` and forces `client_Y` onto
the wire, so the server actually verifies-and-rejects it. This is precisely why
0110 cannot inherit 0109's polite dial: 0109 is `require=true`, where a *withheld*
cert is itself rejected (alert 116), so its polite dial fires either way; at
`require=false` a withheld cert is **accepted**, so the untrusted arm needs the
force.

## Served-this-arm precondition (SPEC §8 stale-server trap)

Before the arms, the driver asserts that **this run's** per-side SDS receiver
recorded at least one `StreamSecrets` request (from the proxy's boot-time initial
fetch). Zero requests means the proxy never fetched the CVC's inner secret from
this server and the verdict would be meaningless.

## ⚠️ Why the driver carries a structural check

`CompareBytes` alone would make the proposition under test *"both sides agree"* —
which is **not** the three propositions above. The breaks that matter are
**symmetric**: changing the served CA, or re-signing `client_Y` with it, changes
**both sides identically**, so the two streams still compare EQUAL and a
pure-`CompareBytes` fixture passes green
(`reference_vacuous_break_receiver_normalizes`). So `driveSide` asserts each side's
OWN bytes against the single correct stream; the runner turns a deviation into
`t.Fatalf("<side> drive: ...")`, naming the side and the violated arm. All three
arms are checked independently and all violations are reported together
(`reference_fatalf_makes_assertions_unreachable`).

## The §3.3 / §3.5 SDS fetch-failure characterization

When the CVC's inner SDS secret cannot be fetched, the reference does **not** "serve
anyway" and does **not** run with an "unpopulated trust store". It **init-holds**
the listener and, once past init, **fails closed per-connection**. envoy-go's §3.5
departure is the boot-side of this: a required secret that never arrives
**boot-fails** rather than admitting traffic. This fixture always delivers the
secret, so neither side exercises that path here — it is **named, not asserted**
(one fixture dir = ONE runner branch,
`reference_differential_fixture_dispatch_constraint`).

## Why the failure text is normalized

The reference (BoringSSL) sends the TLS alert `unknown ca`; envoy-go (Go
`crypto/tls`) sends `bad certificate`. **A driver asserting the error string
cross-side fails 100% of the time** (inherits PLAN-65 C3). The untrusted arm
records only the stable token `rejected`.

## Coverage boundaries (named, unasserted)

- The **alert text** — normalized to `rejected` (above).
- **`ssl.*` stats are now ASSERTED CROSS-SIDE** (phase 75 — this boundary is
  RETIRED; the old *"envoy-go emits none, so a verdict `StatsAsserter` is
  infeasible"* text was true up to phase 74 and is FALSE at this tip). envoy-go
  registers `listener.<addr>.ssl.{handshake,no_certificate,fail_verify_error,
  fail_verify_no_cert}` on TLS-bearing listeners, so the driver's `AssertStats`
  pins all four to exactly `handshake=2`, `no_certificate=1`,
  `fail_verify_error=1`, `fail_verify_no_cert=0` on **both** sides, scraped from
  `/stats/prometheus` (measured live, identical on reference and subject, with
  `downstream_cx_total=3`). `no_certificate=1` against `handshake=2` is the
  DISCRIMINATOR the byte observable cannot supply: arms 1 and 3 **both** accept
  at `require=false`, so the accept/reject verdict cannot tell them apart.
  Still out of scope: `ssl.connection_error` (envoy-go's `other` handshake
  outcome increments nothing — a named departure) and the
  `ssl.ciphers/curves/versions` breakdowns.
- **Listener-liveness proxies** — an INDEPENDENT boundary, unaffected by the
  retirement above and still LIVE. Never assert
  `/listeners` or `total_listeners_active`; never treat a docker-proxy accept as
  listener liveness.
- **SDS stream framing** and the `sds.<secret>.*` counters — impl-specific; only
  the delivered CA's *effect* is asserted.

## Running

Always use the FULL selector (`reference_differential_run_selector` — a bare
`-run '0110'` matches ZERO subtests and reports a vacuous green):

```bash
go test ./test/differential/ -count=1 -run 'TestDifferential/0110-tls-require-client-cert-false' -v
```
