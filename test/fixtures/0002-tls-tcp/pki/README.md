# Phase-03 TLS test PKI

This directory holds the deterministic test PKI for differential fixture `0002-tls-tcp`. The committed `.pem` files are the authoritative source consumed by `envoy.yaml`, `envoy-go.yaml`, and `test/helpers/tls.go` via inline-bytes DataSources.

## Layout

- `ca.pem` — self-signed ECDSA P-256 root CA. `Subject: CN=envoy-go test CA`.
- `server-alpha.pem` / `.key.pem` — leaf for the `alpha.envoy-go.test` downstream SNI chain.
- `server-beta.pem` / `.key.pem` — leaf for the `beta.envoy-go.test` downstream SNI chain.
- `upstream-alpha.pem` / `.key.pem` — leaf for the 3 upstream-alpha TLS backends. Carries IP SAN `127.0.0.1` + DNS SANs `alpha.envoy-go.test` and `localhost` so both the reference proxy (Docker, reaches backends via `host.docker.internal`) and the subject proxy (host subprocess, dials `127.0.0.1`) can validate against the same cert.
- `upstream-beta.pem` / `.key.pem` — same for the 3 upstream-beta backends.
- `gen/main.go` — the deterministic generator.

## Validity window

- `NotBefore: 2026-01-01T00:00:00Z`
- `NotAfter:  2046-01-01T00:00:00Z` (20-year window — overshoots realistic project lifespan)

If the PKI ever needs re-issue (validity window widened, SAN added, etc.), update the `notBefore` / `notAfter` constants and/or the generator logic in `gen/main.go`, then run `go run ./pki/gen` from this directory. Every PEM byte regenerates deterministically from the fixed seed so `git diff --exit-code pki/` is clean on re-runs.

## Regeneration command

```bash
cd test/fixtures/0002-tls-tcp
go run ./pki/gen
```

Verifies byte-determinism when re-run:

```bash
go run ./pki/gen
git diff --exit-code pki/ && echo ok  # expect: ok
```

## Why deterministic

- `git diff` is clean on regeneration — no noisy commits on PKI rotation.
- CI never runs `go run ./pki/gen`. The committed PEMs are the authoritative source.
- Fixtures can embed PEM bytes as inline DataSources and expect them to stay byte-identical across developer workstations.
- Task 15's gate sweep verifies determinism via `git diff --exit-code` after invoking the generator.

## Why IP SAN on upstream leaves

Subject-side backends are reached via literal `127.0.0.1` endpoints (STATIC cluster type per ADR-0027 pattern). Go's `crypto/tls` validates IP-addressed dials against IP SANs, not DNS SANs, so the upstream leaves carry `IPAddresses: [127.0.0.1]` in addition to the DNS SANs used for `ServerName`-driven reference-side validation. The Docker-side reference reaches backends via `host.docker.internal` which resolves to the host's address; its cert validation is against `ServerName = alpha.envoy-go.test` (matching DNS SAN), independent of the resolved IP.

## Why two SNIs

The fixture exercises downstream TLS termination, upstream TLS origination, AND SNI-based filter-chain dispatch in one run. Two SNIs keep cluster-a and cluster-b cleanly separable for distribution assertions (`[3,3,3]` per SNI per side).
