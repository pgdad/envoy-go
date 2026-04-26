# envoy-go Conformance Tool Image Pins

This file records the exact container image digests used in automated conformance
test gates.  Pinning by digest (not by tag) guarantees reproducibility: the same
image runs in every CI invocation, local developer run, and review environment.

All pins are append-only.  A new pin supersedes the old one for the same tool;
the old pin is kept for audit purposes.

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

Section 6.6 (PUSH_PROMISE) is explicitly excluded per ADR-0051 (server has
`ENABLE_PUSH=0`).

### First run result (2026-04-25)

```
53 tests, 53 passed, 0 skipped, 0 failed
```
