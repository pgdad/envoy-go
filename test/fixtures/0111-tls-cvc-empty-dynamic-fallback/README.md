# 0111-tls-cvc-empty-dynamic-fallback

Phase 68. Proves that when a **`combined_validation_context` (CVC)**'s
**SDS-delivered dynamic `validation_context` ACKs EMPTY** (`trusted_ca` absent entirely — `S1`),
the trust anchor **FALLS BACK to the inline `default_validation_context.trusted_ca`
(`CA_A`)** and the listener **SERVES** — matching the reference's `MergeFrom` (an
empty half contributes nothing). At **`require_client_certificate: true`** this is
mandatory mTLS against the **fallback** anchor.

A disciplined clone of `0110-tls-require-client-cert-false`: same CVC shape, same
in-memory PKI machinery, same per-side SDS receivers. Two semantic deltas: the SDS
half is served **EMPTY** (so the inline default WINS via fallback), and
`require_client_certificate` flips **`false` → `true`**.

## The delta vs 0110

| 0110 | 0111 | effect |
|---|---|---|
| SDS serves a real CA (`WithValidationContext`) | **SDS serves EMPTY** (`WithEmptyValidationContext`, S1) | trust anchor falls back to the inline default |
| SDS-served CA **wins**, inline default **loses** | **inline default `CA_A` wins** (via fallback) | the CA roles INVERT |
| `require_client_certificate: false` | **`true`** (bare scalar) | verify-if-presented → **mandatory mTLS** |
| `none` arm **accepted** | `none` arm **rejected** (alert 116) | require=true enforced against the fallback anchor |

`0110` is `require=false` with the SDS-served CA replacing the inline default.
`0111` INVERTS both: the SDS half is empty so the inline `CA_A` is the live anchor
via the phase-68 fallback, under mandatory mTLS.

## Topology

```
                      SDS (SotW StreamSecrets, h2c)          in-memory,
  driver ──dial──▶ [ proxy: l_edf ] ◀── sds_cluster ──▶ driver-owned
   (3 arms)          tcp_proxy                                sdsserver.Server
                        │                          (ONE per side; serves EMPTY S1)
                        ▼
                    c_backend (TCP echo)
```

One TLS-terminating `tcp_proxy` listener. Its `DownstreamTlsContext` carries:

- `require_client_certificate: true` — **mandatory mTLS** (Go:
  `RequireAndVerifyClientCert` against the fallback pool). A **bare scalar**
  (`{value: true}` errors — `reference_protojson_wrapper_scalar_not_object`).
- a **STATIC** server cert (`tls_certificates`, `inline_string`, signed by
  `CA_A`), and
- a **`combined_validation_context`** nesting:
  - `default_validation_context.trusted_ca` = inline `CA_A` PEM (the **fallback
    anchor**), and
  - `validation_context_sds_secret_config` naming secret `edf_validation_ca`,
    fetched over a SotW SDS gRPC stream against `sds_cluster` — **served EMPTY**
    (`validation_context:{}`, `trusted_ca` UNSET; the `S1` fully-absent-`trusted_ca` shape).

Only the validation context arrives via SDS, so boot's SDS pre-scan sees exactly
one secret — the CVC's **inner** SDS half — and the deferred compose-two edge is
never touched. Each side gets its **own** SDS receiver
(`reference_periodic_sink_differential_two_receivers`).

Reference: Envoy contrib-v1.37.2, `STRICT_DNS` via `host.docker.internal`,
listener port **10447**. Subject: envoy-go, `STATIC`, loopback.

## The PKI — in memory, per run; there is no `pki/` dir

| artifact | issuer | role |
|---|---|---|
| `CA_A` | self | inline `default_validation_context.trusted_ca` — **the fallback anchor that MUST win** |
| server leaf | `CA_A` | `ServerAuth`, SAN `DNS:l_edf.fixture.test`; injected `inline_string` into both yamls |
| `client_A` | `CA_A` | `ClientAuth` — **must be ACCEPTED** (via the fallback) |
| `CA_B` | self | a **foreign** CA, templated into no yaml — used only to sign `client_B` |
| `client_B` | `CA_B` | `ClientAuth` — **must be REJECTED** (upper-bounds the fallback pool to `CA_A`) |

Per-run freshness is safe: the observable is an accept/reject verdict.

## The three arms (the observable)

All three arms fold into ONE byte stream. Both sides must emit, byte-identically:

```
trusted=ok echo=phase68-edf-probe
untrusted=rejected
none=rejected
```

1. **trusted** — dial presenting `client_A` (chains to the inline default `CA_A` /
   fallback anchor), **FORCED-SEND**: handshake succeeds, write
   `phase68-edf-probe\n`, read the echo back through `tcp_proxy`. `BackendCount()`
   returns 1 (`reference_differential_backendcount_min_one`).
2. **untrusted** — dial presenting `client_B` (chains to the FOREIGN `CA_B`),
   **FORCED-SEND**: the round trip MUST fail → `untrusted=rejected`. If it succeeds
   the driver records `untrusted=ACCEPTED` — the union / accept-all signal this
   fixture catches (the fallback pool is `CA_A` ONLY).
3. **none** — dial presenting **NO** client cert: at `require=true` the round trip
   MUST fail → `none=rejected` (alert 116, `certificate_required`) — require=true
   is enforced against the fallback anchor.

Each arm drives the **full round trip**, not just the handshake: under TLS 1.3 the
server's verdict on the client cert arrives *after* the client's `Handshake()` has
already returned. The dial is inlined in the driver because neither
`helpers.TLSServedLeaf` nor `helpers.TLSRoundTrip` can present a client cert.

## Why FORCED-SEND on the untrusted arm (RD3 — NOT the require=true discriminator)

At `require=true` the forced-send is **not** the observable's discriminator: a
*polite* dial yields the same `untrusted=rejected` (a withheld `client_B` is
rejected for no-cert), and a permissive `CA_A∪CA_B` union pool **advertises** `CA_B`
so a polite client would send `client_B` too — the union hazard is caught in **both**
send-modes. Forced-send is **retained** (`reference_go_client_cert_withholding`) so
the untrusted arm actually **exercises verify-and-reject** against the fallback pool
rather than collapsing into a no-cert duplicate of the `none` arm, and to keep both
sides symmetric cross-side. It is **not** claimed to flip the require=true observable
— it does not.

## Served-this-arm precondition (SPEC §8 stale-server trap)

Before the arms, the driver asserts that **this run's** per-side SDS receiver
recorded at least one `StreamSecrets` request (from the proxy's boot-time initial
fetch). The served resource is **empty**, but the fetch still happens (and is
ACKed), so `Requests() > 0` holds — the assert is about the fetch *occurring*, not
the resource being non-empty. Zero requests means the proxy never fetched the CVC's
inner secret from this server and the verdict would be meaningless.

## Why the driver carries a structural check

`CompareBytes` alone would make the proposition under test *"both sides agree"* —
which is **not** the three propositions above. The breaks that matter are
**symmetric**: changing the fallback CA, or re-signing `client_B` with it, changes
**both sides identically**, so the two streams still compare EQUAL and a
pure-`CompareBytes` fixture passes green
(`reference_vacuous_break_receiver_normalizes`). So `driveSide` asserts each side's
OWN bytes against the single correct stream; the runner turns a deviation into
`t.Fatalf("<side> drive: ...")`, naming the side and the violated arm. All three
arms are checked independently and all violations are reported together
(`reference_fatalf_makes_assertions_unreachable`).

## The S1 / S2 / S3 siblings (SPEC §8)

- **S1** (`validation_context:{}`, `trusted_ca` UNSET — **this fixture**): the
  reference ACKs the empty half, **merges it away**, and falls back to the inline
  default. envoy-go now **MATCHES** via `xds.ErrEmptyValidationContext`. A
  **residual** S1-only stream-posture divergence remains (envoy-go
  NACKs/`update_rejected` on the empty resource while still serving via the
  fallback) — **named, unasserted**.
- **S1b** (`trusted_ca:{}`, a present `DataSource` with its specifier oneof unset),
  **S2** (`trusted_ca:{inline_bytes:""}`, set-but-empty `DataSource`), and **S3**
  (corrupt PEM): these are **NOT** fallback triggers — they stay a **reject /
  boot-FAIL** on both sides (the reference NACKs; for S1b, PGV `specifier … is
  required`, SPEC-66 §3.9(b)/ADR-0287). The fallback fires ONLY on the FULLY-ABSENT
  `trusted_ca` (S1); the sentinel gate is `vc.GetTrustedCa() == nil` alone. Named as
  boot-FAIL siblings, not asserted here (one fixture dir = ONE runner branch,
  `reference_differential_fixture_dispatch_constraint`).

## Why the failure text is normalized

The reference (BoringSSL) sends `unknown ca` / `certificate required`; envoy-go (Go
`crypto/tls`) sends `bad certificate` / `certificate required`. **A driver asserting
the error string cross-side fails 100% of the time** (inherits PLAN-65 C3). The
negative arms record only the stable token `rejected`; failure pins are **per-side**,
never cross-side string equality (`reference_differential_reference_parses_full_message`).

## Coverage boundaries (named, unasserted)

- The **alert text** — normalized to `rejected` (above).
- **No `ssl.*` stats** — envoy-go emits none, so a verdict `StatsAsserter` is
  infeasible (inherits PLAN-65 C3); a pre-existing framework gap. Never assert
  `/listeners` or `total_listeners_active`; never treat a docker-proxy accept as
  listener liveness.
- **SDS stream framing** and the `sds.<secret>.*` counters — impl-specific; only
  the delivered-empty half's *effect* (the fallback) is asserted.

## Running

Always use the FULL selector (`reference_differential_run_selector` — a bare
`-run '0111'` matches ZERO subtests and reports a vacuous green):

```bash
go test ./test/differential/ -count=1 -run 'TestDifferential/0111-tls-cvc-empty-dynamic-fallback' -v
```
