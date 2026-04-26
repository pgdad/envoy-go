# envoy-go Conformance Tool Image Pins

This file records the exact container image digests used in automated conformance
test gates.  Pinning by digest (not by tag) guarantees reproducibility: the same
image runs in every CI invocation, local developer run, and review environment.

## Refresh procedure

Per doctrine D-3.7, pins are changed only via a dedicated phase. All pins are
append-only: a new pin supersedes the old one for the same tool, and the old pin
is kept for audit purposes. To execute a pin refresh:

1. `docker pull <image>:<new-tag>`; capture the SHA256 with `docker inspect --format='{{index .RepoDigests 0}}'`.
2. Append a new row to the pin table with the new tag and digest, preserving the prior row for audit.
3. Run the conformance gate: `go test -run TestH2Spec ./test/conformance/h2spec/` (or equivalent for the tool).
4. Confirm all tests PASS at the new digest.
5. Mark the prior row with a "superseded by <digest>" annotation.
6. Land the change as a single commit on a dedicated pin-refresh phase branch.

---

## h2spec — HTTP/2 RFC 9113 conformance

| Field              | Value                                                                |
|--------------------|----------------------------------------------------------------------|
| **Tool**           | [summerwind/h2spec](https://github.com/summerwind/h2spec)            |
| **Image ref**      | `summerwind/h2spec`                                                  |
| **Tag at pin time**| `latest` (resolves to v2.6.0; v2.6.0 tag not published separately)  |
| **Digest**         | `sha256:5f4a65c30cae8569558ced048b4bfe0dcf01a221e36767ae504ccd8348a7aeb0` |
| **Full ref**       | `summerwind/h2spec@sha256:5f4a65c30cae8569558ced048b4bfe0dcf01a221e36767ae504ccd8348a7aeb0` |
| **Pulled**         | 2026-04-25                                                           |
| **Gate file**      | `test/conformance/h2spec/h2spec.go`                                  |
| **ADR**            | ADR-0051                                                             |

### Sections tested (threshold)

| Section | Description                             | Cases |
|---------|-----------------------------------------|-------|
| 3.5     | HTTP/2 Connection Preface               | 2     |
| 4.1     | Frame Format                            | 3     |
| 4.2     | Frame Size                              | 3     |
| 4.3     | Header Compression and Decompression    | 3     |
| 5.1     | Stream States                           | 13    |
| 5.1.1   | Stream Identifiers                      | 2     |
| 5.1.2   | Stream Concurrency                      | 1     |
| 5.3.1   | Stream Dependencies                     | 2     |
| 5.4.1   | Connection Error Handling               | 2     |
| 5.5     | Extending HTTP/2                        | 2     |
| 7       | Error Codes                             | 2     |
| 8.1     | HTTP Request/Response Exchange          | 1     |
| 8.1.2   | HTTP Header Fields                      | 1     |
| 8.1.2.1 | Pseudo-Header Fields                   | 4     |
| 8.1.2.2 | Connection-Specific Header Fields      | 2     |
| 8.1.2.3 | Request Pseudo-Header Fields           | 7     |
| 8.1.2.6 | Malformed Requests and Responses       | 2     |
| 8.2     | Server Push                             | 1     |
| **Total** |                                       | **53** |

Section 6.6 (PUSH_PROMISE) is excluded because phase 05.1 disables server push per ADR-0047 / 05.1 SPEC §2.1; the section's tests are conformance-irrelevant for this surface. Per ADR-0055 (phase 05.2), the exclusion stays — the flow-control discipline tightening does not change the server-push posture.

### First run result (2026-04-25)

```
53 tests, 53 passed, 0 skipped, 0 failed
```
