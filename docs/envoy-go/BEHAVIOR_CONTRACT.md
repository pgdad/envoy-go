# envoy-go Behavior Contract

This file is the canonical reference for differential equivalence between envoy-go and the upstream Envoy image pinned in `ENVOY_TARGET.md` (see doctrine `D-3.3` and `D-3.7`). When a phase introduces new features, it extends the per-layer subsections below with concrete rules. When a phase's observed behavior diverges from this contract, either the contract is updated *via ADR* or the implementation is fixed — never both silently.

The contract is the contract. Do **not** consult Envoy C++ source to resolve ambiguity; settle ambiguities with an ADR in `DECISIONS.md` that extends the relevant subsection here.

---

## Equivalence Matrix (§7.2 of BOOTSTRAP_PROMPT.md)

| Dimension | Required equivalence |
|---|---|
| Response status | Exact |
| Response body | Byte-exact for deterministic handlers; semantically equal for filter-modified bodies |
| Response headers | Set-equal modulo documented allow-list (`server`, `date`, timing/identity headers explicitly listed) |
| Response trailers | Set-equal under the same allow-list discipline |
| HTTP/2 & HTTP/3 framing | Structurally equivalent (same frame types/order on equivalent events); not byte-equal |
| Access log records | Semantically equal after field-mapping |
| Stats | Names match Envoy's documented stat tree; presence required; values exact on deterministic flows |
| xDS wire behavior | ADS message sequences match the protocol state machine; effective-config diff on identical snapshots |
| Timing | Not compared by default; a phase may opt in to latency bounds |

"Semantically equal" is defined per dimension in the subsections below. Where a dimension has no subsection yet, the matrix row is its complete definition and phases may only tighten (not relax) it.

---

## Header allow-list

_to be filled per-phase as needed._

The allow-list enumerates response headers whose values are permitted to differ between envoy-go and upstream Envoy without constituting a differential failure. Each entry names the header, the permitted divergence (presence-only / format-only / value-range), the phase that introduced the entry, and the ADR that justifies it.

---

## Stat-name mapping

_to be filled per-phase as needed._

The mapping describes, for each emitted stat, the canonical Envoy stat name, the envoy-go internal name (if different), the tag set, and the flows under which values are required to be exact. When a phase introduces a new stat subsystem, it extends this table.

---

## Access log field mapping

_to be filled per-phase as needed._

The access-log field mapping enumerates every field that must appear (and the field it maps to on upstream Envoy), the ignore-list for values that are inherently non-deterministic (timestamps, connection ids, durations), and the format normalization rules used by the differential harness before comparison.

---

## xDS wire state machine

_to be filled per-phase as needed._

This subsection captures the ADS/delta state machine as the contract expects: allowed message orderings, ACK/NACK semantics, version/nonce rules, resource-subscription handling, reconnection behavior, and initial-fetch-timeout semantics. It is extended as xDS-family phases land.

---

## Timing tolerances

_to be filled per-phase as needed._

Timing is not compared by default. A phase may opt in to latency bounds (p50 / p95 / p99 vs upstream, wall-clock or CPU-normalized) by adding a subsection here naming the fixture, the bound, and the measurement methodology. Opt-ins are additive; removing one requires a superseding ADR.

---

## Test harness host networking

Rules governing how fixtures reach a host-side backend from inside the reference Envoy container. These rules are harness-runtime constraints (not equivalence-matrix dimensions), but they are part of the contract because they determine whether a fixture is valid on the pinned reference image.

### DNS — `host.docker.internal` requires `dns_lookup_family: V4_ONLY`

Every fixture whose reference bootstrap uses `host.docker.internal` as a cluster endpoint address MUST set `dns_lookup_family: V4_ONLY` on that cluster.

- **Introduced by:** phase 00 (fixture `0000-tcp-echo`).
- **Justification ADR:** `ADR-0010` in `DECISIONS.md`.
- **Rule:** a `STRICT_DNS` (or other DNS-resolving) cluster whose `socket_address.address` is the literal string `host.docker.internal` must carry `dns_lookup_family: V4_ONLY` at the cluster level.
- **Reason:** Docker Desktop (Linux) resolves `host.docker.internal` to both IPv4 and IPv6 records. Envoy's DNS resolver, left to default (`AUTO`), may select the IPv6 record, which Docker Desktop does not bridge from container to host — the connection fails with "Network is unreachable" and the differential run stalls until the context deadline. `V4_ONLY` eliminates the selection and keeps the fixture deterministic across Docker Desktop, CI runners, and docker-ce hosts.
- **Applies to:** TCP, HTTP/1.1, and HTTP/2 fixtures on this pin.
- **Does not yet apply to:** HTTP/3 / QUIC fixtures — QUIC is UDP and the dual-stack routing behavior differs; the rule must be re-evaluated (via a superseding ADR) when the first QUIC fixture lands.
- **Host-side backend bind address:** because the Docker-provided host gateway on Docker Desktop is a non-loopback IP, host-side test backends MUST bind to `0.0.0.0` (not `127.0.0.1`). A loopback-only bind is unreachable from the container.
- **`extra_hosts` wiring:** CI and developer environments typically need `--add-host=host.docker.internal:host-gateway` on the reference container (set via `testcontainers-go`'s `HostConfigModifier` in `test/differential/harness.go`). Fixtures inherit this from the harness; they do not re-declare it.

Future fixtures that need a different reachability pattern (e.g. container-to-container, or IPv6-on-purpose) add a new rule under this heading via ADR. The V4_ONLY rule above is never silently relaxed — any deviation is an ADR that explicitly supersedes ADR-0010 on the relevant scope.
