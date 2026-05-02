# Fixture 0008 — Listener-chain match

**Purpose:** Differential equivalence of per-connection `filter_chains[]`
selection between envoy-go and reference Envoy v1.37.2 across the
8-dimension `FilterChainMatch` surface and the `default_filter_chain`
fall-through path. This is the project's first fixture asserting per-chain
dispatch correctness, and the empirical demonstration of the §11.1, §11.2
and §11.3 pins from the 07.2 SPEC.

**Differential surface:** per-connection response body — each backend echoes
its own `ln.Addr().String()`, so the response body uniquely identifies which
chain handled the connection. The 5-connection workload covers the priority-
ordering corners of the chain-match algorithm.

## Workload (5 connections)

| # | Variant  | Listener | Source                | Winning chain              | Backend     | Pin                   |
|---|----------|----------|-----------------------|----------------------------|-------------|-----------------------|
| 1 | primary  | l_test_a | non-loopback          | `chain_dstport_alpha`      | P_dstport   | populated > catch-all |
| 2 | primary  | l_test_b | 127.0.0.1:driver_port | `chain_srcprefix_loopback` | P_srcprefix | populated > catch-all |
| 3 | primary  | l_test_b | non-loopback          | `chain_other` (empty)      | P_other     | empty-match catch-all |
| 4 | c4       | l_test_b | non-loopback          | `chain_default`            | P_default   | SPEC §11.1            |
| 5 | primary  | l_test_a | 127.0.0.1:driver_port | `chain_dstport_alpha`      | P_dstport   | SPEC §11.3            |

The full per-connection rationale (which chains are eliminated and why) is in
`expectations.yaml`.

## STATIC vs. STRICT_DNS divergence

Subject (envoy-go) uses `type: STATIC` with endpoints at `127.0.0.1:<port>`.
Reference (Envoy container) uses `type: STRICT_DNS` with `dns_lookup_family:
V4_ONLY` and endpoints at `host.docker.internal:<port>` (per ADR-0010). The
cluster *behaviour* is equivalent — same backend process targeted from both
proxies — but the *config shape* diverges by ADR. Same convention as fixtures
0000-0007a.

## Dual-listener rationale

The fixture configures **two** listeners (`l_test_a`, `l_test_b`) that carry
the **same** chain set. The dual-listener shape is required for
`destination_port` to be a genuine discriminator: a single-port listener
cannot exercise the `destination_port` priority dimension because every
connection on that listener would carry the same destination port. Two
listeners give us a two-valued destination-port domain (port_a, port_b) so a
chain pinned to `destination_port: port_a` is selectively eligible only on
`l_test_a` — the precise condition needed to exercise SPEC §11.3's priority-
ordering pin.

## c4-variant rationale

Connection 4 must demonstrate the `default_filter_chain` no-match fallback
(SPEC §11.1). With the primary variant's `chain_other` (empty-match catch-
all) present, no connection can fail to match a `filter_chains[]` entry —
the empty-match chain is universally eligible. To force a no-match scenario,
the c4 variant (`envoy-go-c4.yaml` / `envoy-c4.yaml`) **removes**
`chain_other`. In c4, a non-loopback connection to `l_test_b` matches no
`filter_chains[]` entry (chain_dstport_alpha eliminated by port_b ≠ port_a;
chain_srcprefix_loopback eliminated by non-loopback source) and Envoy
dispatches to `default_filter_chain` instead. The runner loads the c4
variant for connection 4 only; the primary variant covers connections 1, 2,
3, 5. The variant-driven approach lands via the harness's
`AlternateConfigDriver` optional interface (introduced at Task 13).

## Chain-match precedence demonstration (connection 5)

Connection 5 is the precedence pin: a connection from `127.0.0.1:driver_port`
to `l_test_a` satisfies BOTH `chain_dstport_alpha`'s `destination_port`
(== port_a) AND `chain_srcprefix_loopback`'s `source_prefix_ranges` +
`source_ports`. Per SPEC §11.3, `destination_port` (priority slot 0) BEATS
`source_prefix_ranges` (priority slot 6) — Envoy dispatches to
`chain_dstport_alpha → c_dstport → P_dstport`. The differential gate
asserts the subject emits the same response body, confirming that envoy-go's
`chainmatch.SelectChain` (Task 5) implements the same priority-ordered
specificity vector.

## Listener filters

This fixture exercises chain-match **without** listener filters. The
`tls_inspector → application_protocols` chain-match path is empirically
pinned at SPEC §11.4 (carry-forward; resolved at Task 16) and integrated
into `BEHAVIOR_CONTRACT.md ## Listener filters` at the 07.2 phase-done
commit (Task 17). Cross-reference: `docs/envoy-go/BEHAVIOR_CONTRACT.md`,
section `## Listener filters` (introduced 07.2).

## Run locally

```bash
go test ./test/differential/ -run TestDifferential/0008-listener-chain-match -v
```

## Re-baseline

If upstream Envoy's pin (ADR-0008) bumps and the differential gate fails,
follow ADR-0008 §"refresh procedure" to re-record evidence. If the chain-
match precedence ordering itself shifts (SPEC §7.2 priority-ordered table),
the §11 empirical pins must be re-scraped and the relevant ADR superseded —
this is a structural change (not a byte refresh) and requires an ADR per
Decision K.
