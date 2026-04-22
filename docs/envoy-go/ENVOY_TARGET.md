# envoy-go Reference Envoy Pin

**Tag:** `envoyproxy/envoy:v1.37.2`
**SHA256:** `envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`
**Upstream release notes:** https://www.envoyproxy.io/docs/envoy/v1.37.2/version_history/v1.37/v1.37.2
**Envoy proto major version:** `v3`
**Pinned in:** ADR-0008
**Last verified:** 2026-04-21

## Refresh procedure

Per doctrine D-3.7, the pin is changed only via a dedicated phase that re-baselines the differential suite. To execute that phase:

1. Pick a new candidate tag per SPEC §5.6 selection criteria (stable, current within 6 months, no API transition in flight).
2. `docker pull envoyproxy/envoy:<new-tag>`; capture the SHA256 with `docker inspect --format='{{index .RepoDigests 0}}'`.
3. Run all differential fixtures against the new image: `go test ./test/differential/...`. Investigate any divergence — fix envoy-go to match, or extend `BEHAVIOR_CONTRACT.md` (with an ADR), or revert.
4. Update this file with the new tag, SHA, release-notes URL, and `Last verified` date.
5. Append a new ADR superseding ADR-0008 (and any contract-extension ADRs from step 3).
6. Land as a single commit on the pin-refresh phase branch.

The pin is never changed ad-hoc — every change is a phase with a green differential surface.
