# Upstream Envoy v1.37.2 — /ready observation

> Evidence for ADR-0015. Captured 2026-04-22 from `envoyproxy/envoy:v1.37.2` (ENVOY_TARGET.md pin; SHA per ADR-0008). These bytes are the authoritative contract for phase-01 `internal/admin` and the Task 10 BEHAVIOR_CONTRACT Admin API subsection.

## Environment

- Host: Linux (Ubuntu-family, kernel 6.17), amd64.
- Docker: client `28.4.0`, Docker Desktop engine `28.1.1` (API 1.49).
- Image tag: `envoyproxy/envoy:v1.37.2`.
- Image digest: `envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (matches ADR-0008).
- Minimal bootstrap (`/tmp/envoy-ready-probe.yaml`):

```yaml
admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners: []
  clusters: []
```

- Command:

```
docker run --rm -d --name envoy-ready-probe -p 9901:9901 \
  -v /tmp/envoy-ready-probe.yaml:/etc/envoy/envoy.yaml \
  envoyproxy/envoy:v1.37.2 \
  envoy -c /etc/envoy/envoy.yaml --log-level warn
```

- Container startup log (the one non-fatal warning, no errors):

```
[2026-04-22 21:38:27.982][1][warning][main] [source/server/server.cc:1018] There is no configured limit to the number of allowed active downstream connections. Configure a limit in `envoy.resource_monitors.global_downstream_max_connections` resource monitor.
```

## Ready-state response (raw)

`curl -s -i http://127.0.0.1:9901/ready` produced the following bytes verbatim. Line endings between headers and before the body are CRLF (`\r\n`); the body terminator is a single LF (`\n`).

```
HTTP/1.1 200 OK\r\n
content-type: text/plain; charset=UTF-8\r\n
cache-control: no-cache, max-age=0\r\n
x-content-type-options: nosniff\r\n
date: Wed, 22 Apr 2026 21:38:35 GMT\r\n
server: envoy\r\n
transfer-encoding: chunked\r\n
\r\n
LIVE\n
```

The above is the logical wire form with CRLF/LF sequences rendered visibly. The raw hex dump of the full response (as delivered by curl, after chunked decoding — i.e. the body is the dechunked 5 bytes `LIVE\n`) is:

```
00000000: 4854 5450 2f31 2e31 2032 3030 204f 4b0d  HTTP/1.1 200 OK.
00000010: 0a63 6f6e 7465 6e74 2d74 7970 653a 2074  .content-type: t
00000020: 6578 742f 706c 6169 6e3b 2063 6861 7273  ext/plain; chars
00000030: 6574 3d55 5446 2d38 0d0a 6361 6368 652d  et=UTF-8..cache-
00000040: 636f 6e74 726f 6c3a 206e 6f2d 6361 6368  control: no-cach
00000050: 652c 206d 6178 2d61 6765 3d30 0d0a 782d  e, max-age=0..x-
00000060: 636f 6e74 656e 742d 7479 7065 2d6f 7074  content-type-opt
00000070: 696f 6e73 3a20 6e6f 736e 6966 660d 0a64  ions: nosniff..d
00000080: 6174 653a 2057 6564 2c20 3232 2041 7072  ate: Wed, 22 Apr
00000090: 2032 3032 3620 3231 3a33 383a 3335 2047   2026 21:38:35 G
000000a0: 4d54 0d0a 7365 7276 6572 3a20 656e 766f  MT..server: envo
000000b0: 790d 0a74 7261 6e73 6665 722d 656e 636f  y..transfer-enco
000000c0: 6469 6e67 3a20 6368 756e 6b65 640d 0a0d  ding: chunked...
000000d0: 0a4c 4956 450a                           .LIVE.
```

Body-only hex (`curl -s http://127.0.0.1:9901/ready | xxd`):

```
00000000: 4c49 5645 0a                             LIVE.
```

### Observations

- Status line: `HTTP/1.1 200 OK\r\n` — exact, no reason-phrase deviation.
- Body bytes: `LIVE\n` (5 bytes: `0x4c 0x49 0x56 0x45 0x0a`). Trailing LF is present; it is NOT CRLF.
- `Content-Length` header: **absent**. Envoy uses `transfer-encoding: chunked` for `/ready` (observed with both empty and non-empty static_resources). A byte-exact equivalent admin server must therefore either (a) also emit `transfer-encoding: chunked` with a proper chunked body framing, or (b) be explicit in the BEHAVIOR_CONTRACT that the subject emits `Content-Length: 5` as a permitted deviation and the differential harness normalises both `transfer-encoding: chunked`-framed and `Content-Length`-framed responses to the dechunked body before byte-comparison.
- Header names: lowercase (`content-type`, not `Content-Type`). This is upstream v1.37.2's canonical form — the admin server must emit lowercase header names.
- Header order as delivered on the wire (top-to-bottom):
  1. `content-type: text/plain; charset=UTF-8`
  2. `cache-control: no-cache, max-age=0`
  3. `x-content-type-options: nosniff`
  4. `date: Wed, 22 Apr 2026 21:38:35 GMT`
  5. `server: envoy`
  6. `transfer-encoding: chunked`
- `server` header value: `envoy` (lowercase, no version suffix).
- `content-type`: `text/plain; charset=UTF-8` — the charset token is exactly `UTF-8` (hyphenated, uppercase).
- `cache-control`: `no-cache, max-age=0` — comma + single space separator.
- `x-content-type-options`: `nosniff`.
- `date`: RFC 7231 IMF-fixdate (`Wed, 22 Apr 2026 21:38:35 GMT`). Non-deterministic — regenerated per request.
- `transfer-encoding`: `chunked`. Deterministic across probes in this capture set.

## Pre-init response (raw)

Pre-init response unobservable on this image/platform from this bootstrap.

Two separate 20- and 40-attempt tight probe loops (`curl --connect-timeout 1 --max-time 2`, 50ms spacing in the first, no-spacing in the second) recorded against `docker run -d` container starts. Summaries:

- Loop 1 (20 attempts, 50ms between, from `/tmp/envoy-ready-capture/preinit-loop.log`):
  - Attempt 1 @ `1776893925.359630`: empty (TCP not yet listening — pre-accept, not a pre-init HTTP response).
  - Attempts 2–20: all `HTTP/1.1 200 OK`.
- Loop 2 (40 attempts, no inter-attempt sleep, from `/tmp/envoy-ready-capture/preinit-loop-tight.log`):
  - Attempts 1–40: all `HTTP/1.1 200 OK`. No non-2xx response, no TCP-level refusal.

Across 60 total probes, zero non-200 HTTP responses were captured. The only "non-200" outcome was a single connection-refused attempt before the admin socket reached `LISTEN` state — this is pre-TCP-accept behaviour (kernel RST), not an HTTP pre-init response that a userspace admin server could emit.

Interpretation: with an empty-`listeners`/empty-`clusters` bootstrap, Envoy v1.37.2's initialisation manager fires so quickly after the admin listener binds that the pre-init window (when `/ready` would return `503 Service Unavailable` with a non-`LIVE` body like `PRE_INITIALIZING\n`) closes before any observer can probe it across the network boundary. Per ADR-0015's option (b), this is acceptable: phase-01's subject admin server's `MarkReady` is invoked by `cmd/envoy-go` before the ready sentinel is printed, so the differential harness never observes the subject's pre-init window either. The pre-init response shape is therefore documented-but-test-irrelevant for phase 01.

The phase-01 subject's admin server SHALL still emit a documented pre-init response shape (`503 Service Unavailable` with body `PRE_INITIALIZING\n` — PLAN.md suggestion) before `MarkReady` is called, so that:

- Unit tests in `internal/admin` can exercise the pre-init branch deterministically (Task 9).
- Future phases that introduce slower-initialising subjects inherit a working pre-init contract without ADR churn.

If a later phase succeeds in capturing upstream's actual pre-init bytes (e.g., by seeding the bootstrap with a large cluster list that defers init), that ADR supersedes this evidence file's pre-init section and the admin server updates to match.

## Header allow-list implications

For phase-01's admin `/ready` differential equivalence (Task 10 BEHAVIOR_CONTRACT subsection; Task 14 harness diff):

- **Emit byte-exact** (value MUST match upstream v1.37.2 character-for-character):
  - Status line: `HTTP/1.1 200 OK`.
  - `content-type: text/plain; charset=UTF-8`
  - `cache-control: no-cache, max-age=0`
  - `x-content-type-options: nosniff`
  - `server: envoy`
  - Body: `LIVE\n` (5 bytes, trailing LF, no CRLF).
  - Header names emitted in lowercase.

- **Allow-list — value may differ, header presence MUST match** (the differential harness normalises these before comparison):
  - `date` — RFC 7231 IMF-fixdate, regenerated per request. Compared structurally (header present, value parses as a valid HTTP-date), not byte-exact.

- **Framing — subject MAY deviate, harness normalises** (captured as explicit deviation in BEHAVIOR_CONTRACT):
  - `transfer-encoding: chunked` (upstream) vs. `content-length: 5` (permissible subject variant). The harness MUST dechunk upstream and length-decode subject down to the raw body bytes before byte-comparison. Header presence in the raw wire capture differs; body bytes are identical.

- **Header order**: upstream emits headers in the order listed under Observations above. Phase-01 subject SHOULD match that order; the harness MUST NOT assert header order for differential equivalence (Go `net/http` does not preserve arbitrary header ordering across implementations, and matching by order would be an over-constraint on the subject for no behavioural benefit).

- **No `Content-Length` header** in upstream's capture. If the subject emits `content-length` instead of `transfer-encoding: chunked`, the BEHAVIOR_CONTRACT records this as a documented phase-01 deviation; upgrading the subject to chunked framing is a phase-02+ follow-up, not a phase-01 gate.

## Capture artefacts

- `/tmp/envoy-ready-probe.yaml` — minimal bootstrap used for the probe container.
- `/tmp/envoy-ready-capture/ready-response.txt` — raw `curl -s -i` capture of the ready-state response (the file this evidence section quotes verbatim).
- `/tmp/envoy-ready-capture/preinit-loop.log` — loop 1 (20 attempts, 50ms spacing) status-line log.
- `/tmp/envoy-ready-capture/preinit-loop-tight.log` — loop 2 (40 attempts, no spacing) status-line log.

Artefacts are under `/tmp/` and not committed to the repo; this evidence file is the committed record. A subsequent phase that needs to re-verify upstream byte-exactness re-runs the Step 1–3 procedure from PLAN.md Task 7 against the pinned tag.
