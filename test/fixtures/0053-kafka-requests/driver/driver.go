// Package driver registers the 0053-kafka-requests cross-side differential
// fixture with the runner per phase 31 SPEC §11.2 + IMPL Task 13. It is the
// REQUEST+RESPONSE cross-side StatsAsserter fixture for the kafka_broker network
// filter: a single listener (l_kafka, stat_prefix kafka_r) whose filter chain is
// [kafka_broker, tcp_proxy] fronts a Kafka-aware canned-RESPONSE backend
// (BackendKind=TCPKafkaResponder, Task 12) that echoes the request's
// correlation_id with a minimal per-api_key body.
//
// ============================================================================
// THE CRITICAL INSIGHT (SPEC §11.2 + the prompt)
// ============================================================================
//
// Reference Envoy's kafka_broker filter parses the FULL request and FULL response
// with version-specific Kafka parsers (NOT just the header). So for the reference
// to count request.<root>_request / response.<root>_response the frames the driver
// sends and the bodies the responder echoes must be FULLY-VALID Kafka messages
// (valid header AND valid body) — not merely valid headers. Our envoy-go decoder
// is header-only (codec.go), so it is the more-lenient side; the reference is the
// STRICT side, and cross-side parity requires satisfying the reference.
//
// ApiVersions (api_key 18) is the workhorse: its v0 REQUEST body is EMPTY (a valid
// v0 ApiVersions request == just the header) and its v0 RESPONSE body is tiny
// (error_code INT16 + api_keys ARRAY). The v3 REQUEST exercises flexible/tagged-
// field framing.
//
// ============================================================================
// The 6 arms (cumulative, single listener; mirror the 0051 arm-accounting table)
// ============================================================================
//
//  1. request per-key  : ApiVersions(18) v0 AND v3 (separate conns) →
//     request.api_versions_request +2 (the v3 arm proves
//     flexible/tagged-field framing).
//  2. request unknown-key: api_key 9999 v0 → request.unknown +1.
//  3. request unknown-version: api_key 18 @ 0x7FFF (flexible header) →
//     request.unknown +1 (cumulative request.unknown = 2).
//  4. request failure  : v0 ApiVersions header with a client_id length of -5
//     (invalid NULLABLE_STRING) → request.failure +1 on BOTH
//     sides (same bytes).
//  5. response per-key : send a valid v0 ApiVersions request (registers corr) and
//     READ the echoed response → response.api_versions_response
//     +1. This ALSO increments request.api_versions_request
//     (the request is valid) — accounted in the table.
//  6. response failure : send a valid v0 ApiVersions request whose corr ==
//     kafkaMarkerUncorrelated; the responder echoes a corr that
//     was NEVER registered (corr+50000) → response.failure +1.
//     The request itself is valid → request.api_versions_request
//     +1 (accounted).
//
// # Cross-references
//
//   - phase 31 SPEC §11.2 (the response side was never verified live before this
//     task — D-P4; this fixture proves it).
//   - fixture-0051-mongo-responses (the STRUCTURAL TEMPLATE — cross-side
//     StatsAsserter, single-listener bootstrap, the cumulative arm-accounting
//     discipline, the prometheus scrape).
//   - the Task-12 responder (kafkaRespondLoop / kafkaResponseBody /
//     kafkaMarkerUncorrelated in test/differential/runner_test.go).
//   - project memory reference_network_filter_typeurl_extensions (network-filter
//     @type URLs carry the extensions. segment).
//   - project memory reference_differential_asserter_dispatch (cross-side MUST use
//     StatsAsserter; SubjectAsserter would be a dead vacuous assertion).
//   - project memory reference_wire_format_both_sides_see_same_bytes (the wire is
//     shared; the driver and responder send byte-identical Kafka frames).
package driver

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0053-kafka-requests"

	refAdminPort = 9901

	// In-container reference Envoy listener port for l_kafka. A free port in the
	// 0049-0052 family (19140-19144 are taken; 19145 is fresh).
	refLKafkaPort = 19145

	// stat_prefix root for the l_kafka listener's kafka_broker config. The kafka
	// prom names carry EMPTY labels, so scrape keys are BARE names like
	// envoy_kafka_kafka_r_request_api_versions_request (no {}).
	statPrefixKafka = "kafka_r"
)

func init() {
	fixture.RegisterFixture(fixtureName, &kafkaRequestsDriver{})
}

// kafkaRequestsDriver carries no mutable cross-arm state. Each arm uses a fresh
// connection (D-P5: a hand-rolled multi-frame stream tripped the reference after
// frame 1, so one request per connection).
type kafkaRequestsDriver struct{}

// dMarkerUncorrelated MIRRORS the responder's kafkaMarkerUncorrelated
// correlation_id (runner_test.go, a DIFFERENT package — duplicated here so both
// sides send byte-identical frames). A request with this corr makes the responder
// echo a corr that was NEVER registered (corr+50000) → response.failure.
const dMarkerUncorrelated int32 = 0x6BAD0000

// dMarkerNoReply MIRRORS the responder's kafkaMarkerNoReply (runner_test.go): a
// request with this correlation_id makes the responder read the full request frame
// (so the request-side decoder fires + counts) but write NO response. The
// request-only arms (a1-a4) use it so no echoed response perturbs the response
// counters — the response side is isolated to arms a5/a6 (the divergence-free
// request-arm construction). Both sides send byte-identical frames
// (reference_wire_format_both_sides_see_same_bytes).
const dMarkerNoReply int32 = 0x6BAD0001

// --- big-endian Kafka wire builders (the driver's OWN builders; they MAY differ
// from the unit-test kafkaReqFrame in runner_test.go which is a different package
// and a header-only stub). All frames are: i32(N) ++ N body bytes. ---

func beI16(v int16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(v))
	return b
}

func beI32(v int32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(v))
	return b
}

// uvarint encodes a Kafka UNSIGNED_VARINT (7 bits/byte, MSB continuation).
func uvarint(v uint32) []byte {
	var out []byte
	for v >= 0x80 {
		out = append(out, byte(v)|0x80)
		v >>= 7
	}
	return append(out, byte(v))
}

// nullableString encodes a NULLABLE_STRING: i16(len) ++ bytes (len -1 == null).
func nullableString(s string) []byte {
	return append(beI16(int16(len(s))), []byte(s)...)
}

// compactString encodes a Kafka COMPACT_STRING: uvarint(len+1) ++ bytes.
func compactString(s string) []byte {
	return append(uvarint(uint32(len(s)+1)), []byte(s)...)
}

// frame prepends the 4-byte BE length prefix to a body.
func frame(body []byte) []byte {
	return append(beI32(int32(len(body))), body...)
}

// apiVersionsV0Request builds a FULLY-VALID ApiVersions(18) v0 request. The v0
// header is NOT flexible: i16(18) ++ i16(0) ++ i32(corr) ++ nullableString(clientID).
// The v0 ApiVersions BODY is EMPTY, so a valid v0 request == just the header.
func apiVersionsV0Request(corr int32, clientID string) []byte {
	body := beI16(18)
	body = append(body, beI16(0)...) // api_version 0
	body = append(body, beI32(corr)...)
	body = append(body, nullableString(clientID)...)
	return frame(body)
}

// apiVersionsV3Request builds a FULLY-VALID ApiVersions(18) v3 request (flexible).
// Header: i16(18) ++ i16(3) ++ i32(corr) ++ nullableString(clientID) ++ uvarint(0)
// (header tagged_fields == 0). NOTE: client_id in the header is ALWAYS a regular
// NULLABLE_STRING even in flexible headers — only the BODY uses compact strings.
// Body v3: compactString(client_software_name) ++ compactString(client_software
// _version) ++ uvarint(0) (body tagged_fields).
func apiVersionsV3Request(corr int32, clientID, swName, swVersion string) []byte {
	body := beI16(18)
	body = append(body, beI16(3)...) // api_version 3 (flexible)
	body = append(body, beI32(corr)...)
	body = append(body, nullableString(clientID)...)
	body = append(body, uvarint(0)...) // header tagged_fields
	body = append(body, compactString(swName)...)
	body = append(body, compactString(swVersion)...)
	body = append(body, uvarint(0)...) // body tagged_fields
	return frame(body)
}

// unknownKeyRequest builds a header-valid request for an UNKNOWN api_key (9999),
// api_version 0. The reference routes an unrecognized api_key to request.unknown
// via its sentinel parser (which consumes the remaining bytes). v0/non-flexible
// header shape so both decoders agree.
func unknownKeyRequest(corr int32, clientID string) []byte {
	body := beI16(9999)
	body = append(body, beI16(0)...) // api_version 0
	body = append(body, beI32(corr)...)
	body = append(body, nullableString(clientID)...)
	return frame(body)
}

// unknownVersionRequest builds an api_key 18 request at api_version 0x7FFF. Since
// 0x7FFF >= flexibleSince(18)==3 it is built as a FLEXIBLE header (client_id
// NULLABLE_STRING ++ uvarint(0) header tagged_fields) so both decoders agree on
// the header shape, then a sentinel body (uvarint(0) body tagged_fields). api_key
// 18's maxVersion is 4, so 0x7FFF > 4 → request.unknown on both sides.
func unknownVersionRequest(corr int32, clientID string) []byte {
	body := beI16(18)
	body = append(body, beI16(0x7FFF)...) // api_version 32767 (> maxVersion 4)
	body = append(body, beI32(corr)...)
	body = append(body, nullableString(clientID)...)
	body = append(body, uvarint(0)...) // header tagged_fields (flexible)
	body = append(body, uvarint(0)...) // sentinel body tagged_fields
	return frame(body)
}

// malformedClientIDRequest builds a v0 ApiVersions header whose client_id
// NULLABLE_STRING length is -5 (invalid: < -1). The header decode throws on BOTH
// sides (same bytes) → request.failure.
func malformedClientIDRequest(corr int32) []byte {
	body := beI16(18)
	body = append(body, beI16(0)...) // api_version 0
	body = append(body, beI32(corr)...)
	body = append(body, beI16(-5)...) // invalid NULLABLE_STRING length (< -1)
	return frame(body)
}

// reqWriteSettle is the brief pause after writing a request on a write-only arm,
// before closing the connection, so the request bytes fully traverse the chain and
// the request-side decoder fires on both sides before teardown. The responder
// suppresses the reply (NO-REPLY marker), so there is nothing to read.
const reqWriteSettle = 150 * time.Millisecond

// --- fixture.Driver (required) ---

// BackendCount returns 1: a single TCPKafkaResponder backend (c_kafka cluster).
func (*kafkaRequestsDriver) BackendCount() int { return 1 }

// SubjectListenerName returns the single listener name (l_kafka).
func (*kafkaRequestsDriver) SubjectListenerName() string { return "l_kafka" }

// ReferenceListenerPort returns the reference listener port (l_kafka).
func (*kafkaRequestsDriver) ReferenceListenerPort() int { return refLKafkaPort }

// BackendKind returns TCPKafkaResponder: the correlation-id-echoing canned-response
// backend (Task 12; SPEC §8.3).
func (*kafkaRequestsDriver) BackendKind() fixture.BackendKind { return fixture.TCPKafkaResponder }

// ReferenceBootstrap renders the single-listener reference bootstrap. c_kafka
// points at host.docker.internal:<backend> (STRICT_DNS) so the dockerized
// reference can reach the host-side responder backend.
func (*kafkaRequestsDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:   fmt.Sprintf("0.0.0.0, port_value: %d", refAdminPort),
		listenAddr:  "0.0.0.0",
		kafkaPort:   refLKafkaPort,
		clusterType: "STRICT_DNS",
		dnsLine:     "      dns_lookup_family: V4_ONLY\n",
		backendHost: "host.docker.internal",
		backendPort: backendPorts[0],
		nodeLine:    "",
	})
}

// SubjectConfig renders the single-listener subject bootstrap.
func (*kafkaRequestsDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:   fmt.Sprintf("127.0.0.1, port_value: %d", subjAdminPort),
		listenAddr:  "127.0.0.1",
		kafkaPort:   subjListenerPort,
		clusterType: "STATIC",
		dnsLine:     "",
		backendHost: "127.0.0.1",
		backendPort: backendPorts[0],
		nodeLine:    "node: { id: envoy-go-subject-0053, cluster: envoy-go-differential }\n",
	})
}

// DriveReference / DriveSubject run the identical six-arm workload against each
// side's l_kafka listener and return a side-independent verdict byte stream.
func (d *kafkaRequestsDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr, "ref")
}

func (d *kafkaRequestsDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr, "subj")
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint.
func (*kafkaRequestsDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref ready: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj ready: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- scenario driving (the six arms) ---

const (
	// clientID is a real short client_id for the valid request arms. Its presence
	// (a non-null NULLABLE_STRING) exercises the client_id read path on both sides.
	clientID = "envoy-go"

	// readReplyTimeout bounds the conn.Read for a response so an arm that expects
	// no readable reply does not block the driver. The response arms read their
	// echoed response well within this window.
	readReplyTimeout = 400 * time.Millisecond

	// settleDelay lets the async stat pipeline on both sides catch up before
	// AssertStats scrapes (the 0051 sleep-to-settle precedent).
	settleDelay = 750 * time.Millisecond

	// drainHold keeps a drained connection open briefly before close so the proxy's
	// response decoder fully consumes in-flight bytes (avoiding the reference's racy
	// abandon-at-close extra response failure; see arm 6).
	drainHold = 400 * time.Millisecond
)

// ┌──────────────────────────────────────────────────────────────────────────────┐
// │ CUMULATIVE ARM-ACCOUNTING TABLE (the 0051 discipline) — arms 1-6              │
// │ single listener l_kafka (kafka_r). Per-prefix counters are CUMULATIVE.        │
// │ The two *.failure roots differ cross-side by the reference's deterministic    │
// │ abandon-at-close (+1) — shown as the wantRef|wantSubj split.                  │
// ├──────────────────────────────────────────────────────────────────────────────┤
// │                              a1v0 a1v3 a2  a3  a4  a5  a6  | ref | subj        │
// │  counter                     APIv APIv unk unv mal rsp ufl |     |             │
// │  ──────────────────────────  ──── ──── ──  ──  ──  ──  ──  | ─── | ───         │
// │  request.api_versions_request ✓    ✓    .   .   .   ✓   ✓  |  4  |  4          │
// │  request.unknown              .    .    ✓   ✓   .   .   .  |  2  |  2          │
// │  request.failure              .    .    .   .   ✓   .   .  |  2  |  1          │
// │  response.api_versions_response.   .    .   .   .   ✓   .  |  1  |  1          │
// │  response.failure             .    .    .   .   .   .   ✓  |  2  |  1          │
// └──────────────────────────────────────────────────────────────────────────────┘
//
// request.api_versions_request == 4 (both sides): a1v0 + a1v3 (the two per-key
// request arms) + a5 (the response per-key arm sends a VALID v0 request) + a6 (the
// response-failure arm's marker request is a VALID v0 request — only its ECHOED
// response carries the unregistered corr). The malformed arm (a4) does NOT count
// here (it fails before the per-key inc) and a2/a3 route to request.unknown.
//
// request.unknown == 2 (both sides): a2 (unknown api_key 9999) + a3 (api_key 18 @
// version 0x7FFF > maxVersion 4). a3's header is flexible-shaped so both decoders
// agree on the header shape.
//
// request.failure: ref 2 / subj 1 — a4 (client_id length -5, invalid NULLABLE_STRING
// → the header decode throws). The subject counts the one complete malformed frame
// once; the reference counts the failure (+1) AND a malformed-stream abandon-at-close
// (+1). DETERMINISTIC per side (live-verified stable). See arm 4.
//
// response.api_versions_response == 1 (both sides): a5 — the responder echoes a v0
// ApiVersions response (correlation_id + body). The subject recovers (key=18, ver=0)
// from the correlation map; the reference parses the full v0 response body. Both
// count it. THIS IS THE CORE D-P4 RESPONSE-SIDE PROOF (the response per-key decode +
// correlation works cross-side, never verified live before this task).
//
// response.failure: ref 2 / subj 1 — a6 (the responder echoes a corr that was NEVER
// registered, kafkaMarkerUncorrelated+50000 → both decoders MISS the correlation).
// As on the request side, the subject counts the miss once; the reference counts the
// miss (+1) AND a broken-stream abandon-at-close (+1). DETERMINISTIC per side with a
// SINGLE marker (live-verified). See arm 6.
//
// RESPONSE-SIDE ISOLATION (the divergence-free construction): the request-only arms
// (a1-a4) use the NO-REPLY marker correlation_id (dMarkerNoReply). The responder
// reads each request frame (so the request-side decoder fires + counts) but writes
// NOTHING, so NO echoed response traverses the chain to perturb the response
// counters. This makes the response side DETERMINISTIC and isolated to a5/a6.
// Without it, the responder would echo version-mismatched bodies (a v0 body for the
// a1v3 request) that the reference's version-specific response parser rejects →
// cross-side divergence (observed in iteration 1: ref response_failure=4 / subj=1).
// The LIVE reference is ground truth; the want values below are the LIVE cross-side
// equilibrium (the 0051 op_query==7 precedent for re-deriving from reality).

// driveProxy runs the six arms in declared order against the proxy listener at
// addr. Each arm uses a fresh connection. The "side" label is diagnostic-only and
// is NEVER written to the returned bytes, so equivalent behavior yields
// byte-identical output for the runner's CompareBytes gate.
func (d *kafkaRequestsDriver) driveProxy(ctx context.Context, addr, side string) ([]byte, error) {
	var b bytes.Buffer

	// armsSel gates which arms run (diagnostic: ARMS=14 runs only arms 1 & 4). The
	// default runs all six. The verdict bytes stay side-independent in every subset.
	armsSel := os.Getenv("FIXTURE_0053_ARMS")
	if armsSel == "" {
		armsSel = "123456"
	}
	on := func(c byte) bool { return strings.IndexByte(armsSel, c) >= 0 }

	if on('1') {
		// Arm 1 (request per-key): ApiVersions(18) v0 AND v3 (separate conns) →
		// request.api_versions_request +2. The v3 arm proves flexible/tagged-field
		// framing. Both use the NO-REPLY marker corr so the responder reads the request
		// (request decoder fires) but writes nothing — the response side stays isolated
		// to a5/a6.
		err := driveWriteOnly(ctx, addr, apiVersionsV0Request(dMarkerNoReply, clientID))
		emitArm(&b, side, "req-apiversions-v0", err)
		err = driveWriteOnly(ctx, addr, apiVersionsV3Request(dMarkerNoReply, clientID, "envoy-go", "1.0"))
		emitArm(&b, side, "req-apiversions-v3", err)
	}

	if on('2') {
		// Arm 2 (request unknown-key): api_key 9999 → request.unknown +1 (NO-REPLY).
		err := driveWriteOnly(ctx, addr, unknownKeyRequest(dMarkerNoReply, clientID))
		emitArm(&b, side, "req-unknown-key", err)
	}

	if on('3') {
		// Arm 3 (request unknown-version): api_key 18 @ 0x7FFF → request.unknown +1
		// (cumulative request.unknown = 2; NO-REPLY).
		err := driveWriteOnly(ctx, addr, unknownVersionRequest(dMarkerNoReply, clientID))
		emitArm(&b, side, "req-unknown-version", err)
	}

	if on('4') {
		// Arm 4 (request failure): ONE malformed-client_id frame (corr = NO-REPLY
		// marker, so no response is echoed). The frame is a complete v0 ApiVersions
		// header whose client_id NULLABLE_STRING length is -5 (invalid) → the header
		// decode throws. The two sides count this by DIFFERENT but each-DETERMINISTIC
		// accounting (live-verified stable across many runs — see the README table):
		//
		//   - SUBJECT (header-only sniffer): the complete malformed frame fails its
		//     client_id read → request.failure == 1.
		//   - REFERENCE (full version-specific parser): the malformed frame → +1, then
		//     the request decoder enters a broken state and at connection teardown
		//     abandons the buffered bytes → +1 more → request.failure == 2.
		//
		// The +1 reference excess is the malformed-stream abandon-at-close, a
		// reference-only failure the header-only subject does not model (analogous to
		// reference_close_direction_framework_gap). It is DETERMINISTIC per side, so
		// AssertStats pins the EXACT per-side value (ref 2, subj 1) rather than
		// asserting a fragile equality — non-vacuous and R4-breakable on each side.
		err := driveWriteOnly(ctx, addr, malformedClientIDRequest(dMarkerNoReply))
		emitArm(&b, side, "req-failure", err)
	}

	if on('5') {
		// Arm 5 (response per-key): send a valid v0 ApiVersions request (registers
		// corr 501) and READ the echoed response → response.api_versions_response +1.
		// This ALSO increments request.api_versions_request (the request is valid).
		rep, err := driveAndReadReplyN(ctx, addr, apiVersionsV0Request(501, clientID))
		emitArmRead(&b, side, "resp-apiversions", rep, err)
	}

	if on('6') {
		// Arm 6 (response failure / unregistered): send ONE valid v0 ApiVersions
		// request whose corr == kafkaMarkerUncorrelated. The responder echoes
		// corr+50000 (NEVER registered) → both decoders MISS the correlation. As on
		// the request side (arm 4), the two sides count by DIFFERENT but each-
		// DETERMINISTIC accounting (live-verified stable):
		//
		//   - SUBJECT: the unregistered-correlation response misses the correlation
		//     map → response.failure == 1.
		//   - REFERENCE: the unregistered response → +1, then the RESPONSE decoder
		//     enters a broken state and at connection teardown abandons → +1 more →
		//     response.failure == 2.
		//
		// (Live note: a SINGLE marker is deterministic per side — ref 2 / subj 1
		// across many runs; TWO markers on one conn race the reference's abandon
		// against the 2nd response → ref flaps 2/3. So this arm uses ONE marker and
		// pins the EXACT per-side value, parallel to arm 4.)
		//
		// The marker request is itself a VALID v0 ApiVersions request → it ALSO
		// increments request.api_versions_request (accounted: +1). The reply is
		// drained so the response decode completes before close.
		rep, err := driveAndDrain(ctx, addr, apiVersionsV0Request(dMarkerUncorrelated, clientID))
		emitArmRead(&b, side, "resp-failure", rep, err)
	}

	// Let the async stat pipeline settle before the runner scrapes in AssertStats.
	if err := sleepCtx(ctx, settleDelay); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// emitArm writes a side-independent verdict line for a write-only arm.
func emitArm(b *bytes.Buffer, side, name string, err error) {
	verdict := "ok"
	if err != nil {
		fmt.Fprintf(os.Stderr, "[fixture 0053 %s] arm %s: %v\n", side, name, err)
		verdict = "ERR"
	}
	fmt.Fprintf(b, "arm %s verdict=%s\n", name, verdict)
}

// emitArmRead writes a SIDE-INDEPENDENT verdict line for a response-reading arm.
// The reply BYTE COUNT is NOT folded into the verdict: it is side-DEPENDENT — e.g.
// the unregistered-response arm (a6) drops the bad response on the reference (the
// driver reads 0) but the pure-sniffer subject forwards it (the driver reads it).
// Folding replyLen would break the runner's CompareBytes gate. The reply is still
// READ (to drive the response decode before close); the count is logged to stderr
// only. The verdict is "ok" unless the dial/write itself errored.
func emitArmRead(b *bytes.Buffer, side, name string, replyLen int, err error) {
	verdict := "ok"
	if err != nil {
		fmt.Fprintf(os.Stderr, "[fixture 0053 %s] arm %s: %v\n", side, name, err)
		verdict = "ERR"
	}
	fmt.Fprintf(os.Stderr, "[fixture 0053 %s] arm %s replyLen=%d\n", side, name, replyLen)
	fmt.Fprintf(b, "arm %s verdict=%s\n", name, verdict)
}

// driveWriteOnly opens a fresh TCP connection, writes frame, pauses briefly so the
// request traverses the chain and the request decoder fires on both sides, then
// closes WITHOUT reading (the responder suppresses the reply via the NO-REPLY
// marker, so there is nothing to read). The connection close drives OnDestroy.
func driveWriteOnly(ctx context.Context, addr string, frameBytes []byte) error {
	dl := net.Dialer{}
	conn, err := dl.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write(frameBytes); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}
	return sleepCtx(ctx, reqWriteSettle)
}

// driveAndReadReplyN opens a fresh TCP connection, writes frame, reads the echoed
// reply (bounded), and closes. The read drains the response off the socket so the
// response decoder on both sides sees the COMPLETE frame before the connection
// closes. Returns the reply byte count; a read timeout / EOF folds into (n, nil)
// (side-independent). The connection close drives OnDestroy on both sides.
func driveAndReadReplyN(ctx context.Context, addr string, frameBytes []byte) (int, error) {
	dl := net.Dialer{}
	conn, err := dl.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write(frameBytes); err != nil {
		return 0, fmt.Errorf("write frame: %w", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(readReplyTimeout))
	buf := make([]byte, 8192)
	n, rerr := conn.Read(buf)
	if rerr != nil {
		if ne, ok := rerr.(net.Error); ok && ne.Timeout() {
			return 0, nil // no readable reply — expected for the malformed/unknown arms
		}
		if rerr == io.EOF {
			return n, nil
		}
		return n, nil
	}
	return n, nil
}

// driveAndDrain opens a fresh TCP connection, writes frameBytes (which may hold
// MULTIPLE concatenated request frames), then reads in a loop draining ALL echoed
// response bytes until the read deadline (so each echoed response — e.g. both
// unregistered responses in arm 6 — fully traverses the chain and is decoded before
// close). Returns the total reply byte count. The connection close drives OnDestroy.
func driveAndDrain(ctx context.Context, addr string, frameBytes []byte) (int, error) {
	dl := net.Dialer{}
	conn, err := dl.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write(frameBytes); err != nil {
		return 0, fmt.Errorf("write frame: %w", err)
	}

	total := 0
	buf := make([]byte, 8192)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(readReplyTimeout))
		n, rerr := conn.Read(buf)
		total += n
		if rerr != nil {
			break // timeout / EOF — done draining (side-independent)
		}
	}
	// Hold the connection open briefly after draining so the proxy's response
	// decoder fully consumes all in-flight bytes BEFORE the close, avoiding the
	// reference's racy abandon-at-close extra failure (see arm 6).
	_ = sleepCtx(ctx, drainHold)
	return total, nil
}

// sleepCtx sleeps for d or returns early if ctx is canceled.
func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// --- fixture.StatsAsserter (cross-side MUST use StatsAsserter; SubjectAsserter
// would be a dead vacuous assertion — reference_differential_asserter_dispatch) ---

// AssertStats scrapes /stats/prometheus from BOTH admin endpoints and asserts the
// kafka_broker counters after the six-arm workload. The kafka prom names carry
// EMPTY labels, so keys are BARE names (no {}). An ABSENT key is reported
// DISTINCTLY from a present-but-wrong value (the 0051 discipline).
func (d *kafkaRequestsDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	refStats, err := scrapeKafkaStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref kafka stats: %v", err)
	}
	subjStats, err := scrapeKafkaStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj kafka stats: %v", err)
	}

	if os.Getenv("FIXTURE_0053_DUMP_STATS") != "" {
		dump := func(label string, m map[string]int64) {
			fmt.Fprintf(os.Stderr, "=== %s kafka stats ===\n", label)
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(os.Stderr, "  %s = %d\n", k, m[k])
			}
		}
		dump("ref", refStats)
		dump("subj", subjStats)
	}

	// Diagnostic dump-only mode (bisecting arm subsets): skip the comparisons so the
	// dump above is the sole output. Never set in CI.
	if os.Getenv("FIXTURE_0053_DUMP_ONLY") != "" {
		return
	}

	// DECODE-RAN verification (per side): the tcp_proxy must reach its upstream (the
	// real TCPKafkaResponder) or the proxy closes the downstream before the kafka
	// decoder runs (zero stats).
	//   - REFERENCE: envoy_tcp_downstream_cx_rx_bytes_total > 0 proves request bytes
	//     entered the chain (the reference surfaces tcp_proxy prom stats).
	//   - SUBJECT: the envoy-go subject does NOT surface envoy_tcp_ prom lines, so we
	//     prove decode ran INTRINSICALLY: request.api_versions_request > 0 (a counter
	//     that can only increment if the kafka request decoder consumed valid frames).
	refRx := refStats[canonicalize(`envoy_tcp_downstream_cx_rx_bytes_total{envoy_tcp_prefix="tcp_kafka"}`)]
	if refRx == 0 {
		t.Errorf("ref: envoy_tcp_downstream_cx_rx_bytes_total == 0 (DECODE-RAN check: chain saw no bytes)")
	}
	if subjStats["envoy_kafka_kafka_r_request_api_versions_request"] == 0 {
		t.Errorf("subj: request_api_versions_request == 0 (DECODE-RAN check: kafka decoder never ran on chain bytes)")
	}

	// Expectations keyed by the BARE prom name (kafka labels are empty). Each entry
	// carries a per-side expected value (wantRef / wantSubj) from the CUMULATIVE
	// arm-accounting table above driveProxy (re-verified LIVE cross-side). The
	// per-key roots EQUAL on both sides; the two *.failure roots differ by the
	// reference's deterministic abandon-at-close (+1), a reference-only failure the
	// header-only subject does not model (see arms 4 & 6 + the README table).
	expectations := []struct {
		key      string
		wantRef  int64
		wantSubj int64
	}{
		{"envoy_kafka_kafka_r_request_api_versions_request", 4, 4},   // a1v0 + a1v3 + a5 + a6
		{"envoy_kafka_kafka_r_request_unknown", 2, 2},                // a2 + a3
		{"envoy_kafka_kafka_r_request_failure", 2, 1},                // a4 (ref +1 abandon-at-close)
		{"envoy_kafka_kafka_r_response_api_versions_response", 1, 1}, // a5
		{"envoy_kafka_kafka_r_response_failure", 2, 1},               // a6 (ref +1 abandon-at-close)
	}

	for _, sd := range []struct {
		label string
		stats map[string]int64
		ref   bool
	}{{"ref", refStats, true}, {"subj", subjStats, false}} {
		for _, exp := range expectations {
			want := exp.wantSubj
			if sd.ref {
				want = exp.wantRef
			}
			got, present := sd.stats[exp.key]
			if !present {
				t.Errorf("%s: counter %s ABSENT (creation / name-shape failure)", sd.label, exp.key)
				continue
			}
			if got != want {
				t.Errorf("%s %s = %d, want %d", sd.label, exp.key, got, want)
			}
		}
	}
}

// scrapeKafkaStats issues GET /stats/prometheus and returns a map keyed by the
// BARE prom name (kafka labels are EMPTY, so canonicalize returns names-without-{
// unchanged). Retains envoy_kafka_* AND envoy_tcp_* lines (the latter for the
// decode-ran rx-bytes check).
func scrapeKafkaStats(adminAddr string) (map[string]int64, error) {
	body, err := httpGet("http://" + adminAddr + "/stats/prometheus")
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "envoy_kafka_") && !strings.HasPrefix(line, "envoy_tcp_") {
			continue
		}
		sp := strings.LastIndexByte(line, ' ')
		if sp < 0 {
			continue
		}
		nameLabels, valStr := line[:sp], line[sp+1:]
		v, err := strconv.ParseInt(valStr, 10, 64)
		if err != nil {
			continue
		}
		out[canonicalize(nameLabels)] = v
	}
	return out, nil
}

// canonicalize normalizes "name{...}" to a sorted-label form. The kafka prom names
// have EMPTY labels, so they appear as bare "name" and are returned unchanged; the
// envoy_tcp_ lines may carry labels and are canonicalized for the rx-bytes lookup.
func canonicalize(nameLabels string) string {
	open := strings.IndexByte(nameLabels, '{')
	if open < 0 {
		return nameLabels // bare name (the kafka case)
	}
	name := nameLabels[:open]
	inner := strings.TrimSuffix(nameLabels[open+1:], "}")
	if inner == "" {
		return name // empty-label set "name{}" → "name"
	}
	pairs := strings.Split(inner, ",")
	sort.Strings(pairs)
	return name + "{" + strings.Join(pairs, ",") + "}"
}

// httpGet issues GET url and returns the response body.
func httpGet(url string) ([]byte, error) {
	resp, err := http.Get(url) //nolint:gosec // fixed admin URL, test-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("read %s body: %w", url, err)
	}
	return buf.Bytes(), nil
}

// --- bootstrap rendering (single-listener; the 0051 shape) ---

type bootstrapParams struct {
	adminAddr   string // "<ip>, port_value: <n>" for the admin socket_address
	listenAddr  string // listener bind address (0.0.0.0 for ref; 127.0.0.1 for subj)
	kafkaPort   int    // l_kafka listener port
	clusterType string // STRICT_DNS (ref) | STATIC (subj)
	dnsLine     string // "      dns_lookup_family: V4_ONLY\n" for STRICT_DNS, else ""
	backendHost string
	backendPort int
	nodeLine    string // "node: {...}\n" for subj, "" for ref
}

// kafkaBrokerType / tcpProxyType — the network-filter @type URLs carry the
// extensions. segment (reference_network_filter_typeurl_extensions).
const kafkaBrokerType = "type.googleapis.com/envoy.extensions.filters.network.kafka_broker.v3.KafkaBroker"
const tcpProxyType = "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy"

// renderBootstrap assembles the single-listener bootstrap. The l_kafka filter
// chain is [kafka_broker, tcp_proxy] → c_kafka (the TCPKafkaResponder backend AND
// the boot-satisfying cluster — a zero-cluster boot is rejected by both sides).
func renderBootstrap(p bootstrapParams) string {
	kafkaListener := fmt.Sprintf(`    - name: l_kafka
      address:
        socket_address: { address: %s, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.kafka_broker
              typed_config:
                "@type": %s
                stat_prefix: %s
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": %s
                stat_prefix: tcp_kafka
                cluster: c_kafka
`, p.listenAddr, p.kafkaPort, kafkaBrokerType, statPrefixKafka, tcpProxyType)

	return fmt.Sprintf(`%sadmin:
  address:
    socket_address: { address: %s }
static_resources:
  listeners:
%s  clusters:
    - name: c_kafka
      type: %s
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s      load_assignment:
        cluster_name: c_kafka
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: %s, port_value: %d }
`,
		p.nodeLine,
		p.adminAddr,
		kafkaListener,
		p.clusterType,
		p.dnsLine,
		p.backendHost, p.backendPort,
	)
}

// Compile-time interface assertions.
var (
	_ fixture.Driver           = (*kafkaRequestsDriver)(nil)
	_ fixture.BackendKindAware = (*kafkaRequestsDriver)(nil)
	_ fixture.StatsAsserter    = (*kafkaRequestsDriver)(nil)
)
