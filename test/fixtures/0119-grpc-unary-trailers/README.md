# 0119-grpc-unary-trailers — H2 response-TRAILER forwarding on the gRPC unary path

Phase 84 (`docs/envoy-go/phases/84-grpc-unary-response-trailers/`) — the
differential proof that envoy-go forwards the trailing HTTP/2 HEADERS block
(`grpc-status` / `grpc-message` / `grpc-status-details-bin`) on the gRPC unary
response path (`ADR-0306`). It validates, cross-side against reference Envoy
`contrib-v1.37.2` over a Docker bridge, that the two sides' raw H2 frame
sequences — HEADERS / DATA / TRAILERS, `end_stream` flags, and verbatim
wire-order trailer fields — are byte-identical.

## Purpose

envoy-go's H2 client/dispatch path (`internal/filter/hcm/h2/client.go`,
`internal/filter/hcm/h2dispatch.go`) captures the upstream's trailing HEADERS
block and re-emits it downstream. A regression that drops it, mis-times
`END_STREAM`, or reorders the response HEADERS/TRAILERS blocks would change
the observed frame sequence — which this fixture drives raw (via
`golang.org/x/net/http2.Framer` + `hpack`, no gRPC or HTTP/2 client library in
the loop) and byte-compares.

## The four arms — and why a fourth, non-gRPC arm exists

The driver speaks four probes through the same TLS+ALPN-h2 listener, each on
its own fresh connection. For the exact per-arm response shapes (frame kinds,
`end_stream` flags, byte lengths and hex payloads) see the doc comment on
`arms()` in `driver/driver.go` rather than this file — the plain arm's body
and its `content-length` are derived from the backend index and request path,
so a second, independently-maintained copy here would drift the moment either
changes.

- **success** — `Check("")` against the health service: `SERVING`, HEADERS +
  DATA + a trailing TRAILERS block with `grpc-status=0`.
- **notfound** — `Check("nope")`: the service is unknown, so the response is
  HEADERS immediately followed by TRAILERS (`grpc-status=5`) with no DATA
  frame at all.
- **unimplemented** — `POST /grpc.health.v1.Health/Nope`: an unknown method on
  a known service, HEADERS + TRAILERS with `grpc-status=12`.
- **plain** — a bare, non-gRPC `GET /plain` through the *same* listener: the
  backend's h2c mux answers `200` with a DATA frame that itself carries
  `END_STREAM` — there is **no trailing HEADERS block at all**.

The three gRPC arms are structurally **blind** to a regression at the
D-84-ENDSTREAM decision site: every gRPC unary response — success or error —
carries a trailing HEADERS block, so a bug that unconditionally emits one
(rather than only when the real response has trailers) is behaviour-identical
on every gRPC path. It was measured, by injecting exactly that regression
against the landed seam, to pass all three gRPC arms **and** all three
pre-existing downstream-ALPN-h2 fixtures (0004/0079/0080) — Go's `x/net`
client tolerates an empty trailing HEADERS block, so nothing upstream of this
fixture would have caught it either. The **plain** arm is the only probe
whose real response has no trailing block, so it is the only arm an
unconditional-`END_STREAM`/trailing-block regression reddens (measured: a
deterministic first divergence partway through the transcript, an extra
spurious TRAILERS line plus a flipped `end_stream` flag on the DATA frame).
This is the fixture's D-84-ENDSTREAM gate.

## The discriminating gate is `CompareBytes`, not stats (shape 31)

The runner byte-compares the two sides' transcripts (`CompareBytes`) — that
comparison, not the stats leg, is what discriminates a broken trailer emit.
`AssertStats` here asserts only that the subject served
`http.ingress_http.downstream_rq_2xx >= 4` (one per arm). That is a
**liveness** guard, not a behavioral one: the reference books
`downstream_rq_2xx` (and `upstream_rq_200`) for a stream even when its
trailers are wrong, so a stats-only leg stays green under exactly the kind of
regression this fixture exists to catch. Its purpose is narrower — with
per-arm failures recorded *into* the transcript as `READ-ERR` lines rather
than returned as an error (so no arm is made unreachable by an earlier one's
failure), a defect that breaks both sides identically the same way would
otherwise produce a byte-equal, all-`READ-ERR` transcript and a vacuous
`CompareBytes` green. `>= 4` real 2xx responses is the signal that four arms
were actually driven and compared, not that both sides failed identically
before ever reaching the backend. See the `AssertStats` doc comment in
`driver/driver.go` for the full reasoning.

## Canonicalization — CLOSED at three rules

1. `x-envoy-upstream-service-time` dropped by exact name from response
   HEADERS (a per-request latency, unavoidably side-specific).
2. `date` dropped by exact name from response HEADERS (a wall-clock
   timestamp).
3. The dial address is scrubbed out of `READ-ERR` lines (the reference dials
   a Docker-mapped container port, the subject a local one).

No sort anywhere. Response HEADERS are recorded in wire order after the two
drops; TRAILERS are recorded verbatim, unfiltered, unsorted — grpc-go emits
`grpc-message` before `grpc-status`, and that order is part of what the
fixture pins. A sort was measured vacuous (wire order already matched
cross-side on every observed block) and was deliberately not added.

## Topology — one cluster, one backend

- `c_grpc`: an H2 upstream cluster (`explicit_http_config.http2_protocol_options{}`)
  pointed at a single backend. Reference is `STRICT_DNS` /
  `host.docker.internal`; subject is `STATIC` / `127.0.0.1` (the shared-bridge
  shape, `reference_docker_probe_bridge_network`).
- Backend: the existing `GRPCHealthResponder` (BackendKind 34) — a real
  `grpc-go` health server behind an h2c mux that also answers plain,
  non-gRPC requests with `backend-<idx>:<path>` (used by the plain arm).
- Downstream listener `l_h2`: TLS+ALPN-h2 (the 0004/0079 PKI shape), routed
  `/` -> `c_grpc`.
- Reference container listener port **10119**, per the `10<fixture index>`
  convention used from fixture 0103 onward.

## Running

```bash
go test ./test/differential/ -run 'TestDifferential/0119-grpc-unary-trailers' -count=1
```

(Requires Docker for the reference container. Use the full
`-run 'TestDifferential/0119-grpc-unary-trailers'` selector — a bare
`-run '0119'` matches zero subtests, `reference_differential_run_selector`.
Always `-count=1`, since Go's test cache will otherwise serve a stale PASS,
`reference_differential_break_protocol_count1`.)

## Config-as-Go-string, no standalone YAML mirrors

Per the differential-fixture convention there are no standalone `envoy.yaml`
/ `envoy-go.yaml` files — both bootstraps are returned as Go string templates
from `driver.ReferenceBootstrap` / `driver.SubjectConfig`. This fixture ships
none, matching 3 of its 4 closest analogues (0068/0079/0080 — the other
Go-built-bootstrap fixtures over an H2 upstream cluster). Fixture
`0004-h2-routing` is the exception in that set and does ship both mirrors
plus a PKI generator; the decision here to ship none stands on the narrower
3-of-4 ground, not a whole-corpus claim. A standalone mirror would only be a
third copy of configuration that exists solely as the two Go template
functions above.

## A note for a future failing run

If this fixture ever reddens with a diff consisting mostly of `READ-ERR
read-frame` lines, the address text inside those lines is *not* the signal —
the scrub rule above only replaces the dial address, not other
address-shaped noise (a wrapped timeout, an OS-level connection-reset
string). The real signal in a `READ-ERR` diff is which `ARM` and which stage
name it fired under; compare those first before reading the raw error text.
