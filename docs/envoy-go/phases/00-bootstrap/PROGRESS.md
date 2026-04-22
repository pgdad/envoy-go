# Phase 00 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim.

## Preamble — execution preconditions

The executor verified PLAN.md's "Execution preconditions" section (lines 84–94) at session start:

- Worktree: `.worktrees/phase-00-bootstrap-impl` on branch `phase/00-bootstrap-impl` off `master`. ✓ created.
- Docker: `docker version` reports client v28.4.0 / Docker Desktop 4.41.2 server. ✓
- Go: `go version go1.26.2 linux/amd64` (above the 1.23 floor set in SPEC §10 #3). ✓
- `golangci-lint`: PLAN's pinned version `v1.55.2` **failed to install** via `go install` on Go 1.26.2 with:
  ```
  ../../go/pkg/mod/golang.org/x/tools@v0.14.0/internal/tokeninternal/tokeninternal.go:78:9: invalid array length -delta * delta (constant -256 of type int64)
  ```
  Root cause: `golangci-lint@v1.55.2` transitively pins `golang.org/x/tools@v0.14.0`, whose `tokeninternal.go:78` uses a const-array-length idiom that Go 1.22+ rejects under stricter constant-arithmetic rules. This affects every Go version ≥ 1.22 — including PLAN's own 1.23 floor — so the PLAN precondition as written is internally inconsistent.

  Resolution: installed `v1.64.8` via `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`. v1.64.8 is the last v1.x stable release; its config format matches what Task 5 writes and it supports every SPEC §5.5 baseline linter. The pin bump is to be codified in **ADR-0009** by Task 5 (superseding PLAN's precondition step 4 and PLAN's Task 15 CI install command). PLAN's reserved "ADR-0009 for Task 16 runner fallback" shifts to ADR-0010 if that contingency materializes.

## Task 1 — Initialize Go module

**Commits:** f31501f
**Notes:** Created `go.mod` with `module github.com/esalaine/envoy-go` and `go 1.23` directives. `go mod tidy && go build ./...` run successfully (no-op build, no source files yet).
**Outputs:**
```
$ go mod tidy && go build ./...
go: warning: "all" matched no packages
go: warning: "./..." matched no packages
```

## Task 2 — ADR-0005, ADR-0006

**Commits:** 31172f1
**Notes:** Appended ADR-0005 (autonomous-planning adaptation) and ADR-0006 (module path `github.com/esalaine/envoy-go`) to DECISIONS.md verbatim from PLAN.md Task 2. No command outputs to quote (pure Markdown append).

## Task 3 — internal/ package placeholders

**Commits:** f2e4576
**Notes:** Created 12 placeholder doc.go files under internal/{bootstrap,listener,cluster,tcp,http,tls,filter,xds,admin,stats,accesslog,runtime}/ per SPEC §4 future-phase mapping. Each file contains only the package-doc comment and the `package <name>` line.
**Outputs:**
```
$ go vet ./internal/...
```

## Task 4 — Envoy pin, ENVOY_TARGET.md, ADR-0008

**Commits:** 0819740
**Notes:** Pinned upstream Envoy at `envoyproxy/envoy:v1.37.2` (SHA256 `c5e8a68e52f4d`). Smoke-tested admin `/ready` on the pulled image. Updated ENVOY_TARGET.md with tag, SHA, release-notes URL, and the refresh procedure. Appended ADR-0008. Note: ADR-0007 slot is intentionally left empty here; Task 6 lands ADR-0007 (minimal YAML schema) and the controller will address physical ordering inside DECISIONS.md at that point.
**Outputs:**
```
$ docker pull envoyproxy/envoy:v1.37.2
v1.37.2: Pulling from envoyproxy/envoy
4f4fb700ef54: Pulling fs layer
e2dbee44c34b: Pulling fs layer
bd23ccda478c: Pulling fs layer
b86abf6ed0de: Pulling fs layer
342cd4258481: Pulling fs layer
54bcd46fe54a: Pulling fs layer
c547fb821bb9: Pulling fs layer
de47083ed7d7: Pulling fs layer
541e4430a844: Pulling fs layer
4f4fb700ef54: Already exists
4f4fb700ef54: Pull complete
541e4430a844: Download complete
e2dbee44c34b: Download complete
c547fb821bb9: Download complete
bd23ccda478c: Download complete
b86abf6ed0de: Download complete
54bcd46fe54a: Download complete
de47083ed7d7: Download complete
de47083ed7d7: Pull complete
e2dbee44c34b: Pull complete
bd23ccda478c: Pull complete
54bcd46fe54a: Pull complete
541e4430a844: Pull complete
c547fb821bb9: Pull complete
b86abf6ed0de: Pull complete
342cd4258481: Download complete
342cd4258481: Pull complete
Digest: sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
Status: Downloaded newer image for envoyproxy/envoy:v1.37.2
docker.io/envoyproxy/envoy:v1.37.2

$ docker inspect --format='{{index .RepoDigests 0}}' envoyproxy/envoy:v1.37.2
envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd

$ curl -fsS http://127.0.0.1:9901/ready
LIVE
```
