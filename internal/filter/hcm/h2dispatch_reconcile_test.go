package hcm

// h2dispatch_reconcile_test.go — Phase 89 (h2-decode-filter-mutations), ADR-0311.
//
// The unit roster for the H/2 decode-filter delta reconciler. WriteH2 hands the
// decode chain an *http.Request whose Header map is a SEPARATE container from
// the ordered []hpack.HeaderField carrier the upstream HEADERS block is built
// from. Without a reconcile step, every decode-side filter header mutation is
// silently dropped on the H/2 path (H1 and H3 and the H/2 ENCODE side all
// apply theirs). These tests drive the real seam:
//
//	chainDispatchAction.WriteH2 -> rf.SetH2Request -> RunDecodeHeaders
//	  -> [reconcile] -> rf.SetH2Request (re-issue) -> rf.RunAction
//
// captureH2Action (connection_test.go) is the ONLY mechanism that observes
// rf.h2Req at RunAction time — there is no exported accessor — and it is
// sufficient. The action closure copies the request the router filter holds,
// so `captured` IS what the upstream leg would have sent.
//
// chainDispatchAction is built as a STRUCT LITERAL on purpose: h2Dispatcher.Match
// builds its own action internally and cannot be handed a capture closure.

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/net/http2/hpack"

	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/filter/http/router"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// headerMutatingFilter is a decode+encode test filter that applies a
// caller-supplied mutation to the decode header map. Every existing in-package
// DecodeHeaders stub is READ-ONLY (orderRecordingFilter, and the three stubs in
// chain_integration_test.go), so none of them can drive a reconcile assertion;
// this is the smallest complete HTTPFilter shape that can.
//
// The shape is cloned from orderRecordingFilter (chain_dispatch_test.go): both
// halves of the HTTPFilter envelope on one value, so it can be installed as
// HTTPFilter{Decoder: f, Encoder: f}.
//
// Optional behaviors, all off by default:
//   - onData: a second mutation applied from DecodeData (needs a non-empty body).
//   - headersStatus/dataStatus: non-Continue statuses for the early-exit arms.
//   - localReply: triggers dcb.SendLocalReply from DecodeHeaders.
type headerMutatingFilter struct {
	onHeaders func(http.Header)
	onData    func(http.Header, []byte)

	headersStatus filter_http.FilterHeadersStatus
	dataStatus    filter_http.FilterDataStatus

	// localReply, when non-nil, makes DecodeHeaders call SendLocalReply.
	localReply *localReplySpec

	// hdrs retains the decode header map so onData can mutate the SAME map
	// DecodeHeaders saw (DecodeData is handed bytes, not headers).
	hdrs http.Header
	// headersFired / dataFired prove the stub actually ran — an arm whose
	// callback never fires is vacuous.
	headersFired *bool
	dataFired    *bool

	dcb filter_http.DecoderFilterCallbacks
}

// localReplySpec is the SendLocalReply payload for the LocalReplyDone arm.
type localReplySpec struct {
	status  int
	body    string
	headers filter_http.OrderedHeaders
}

func (f *headerMutatingFilter) DecodeHeaders(h http.Header, _ bool) filter_http.FilterHeadersStatus {
	f.hdrs = h
	if f.headersFired != nil {
		*f.headersFired = true
	}
	if f.onHeaders != nil {
		f.onHeaders(h)
	}
	if f.localReply != nil {
		f.dcb.SendLocalReply(f.localReply.status, f.localReply.body, f.localReply.headers)
		return filter_http.StopIteration
	}
	return f.headersStatus
}

func (f *headerMutatingFilter) DecodeData(b []byte, _ bool) filter_http.FilterDataStatus {
	if f.dataFired != nil {
		*f.dataFired = true
	}
	if f.onData != nil {
		f.onData(f.hdrs, b)
	}
	return f.dataStatus
}

func (f *headerMutatingFilter) DecodeTrailers(http.Header) filter_http.FilterTrailersStatus {
	return filter_http.TrailersContinue
}

func (f *headerMutatingFilter) SetDecoderCallbacks(cb filter_http.DecoderFilterCallbacks) {
	f.dcb = cb
}
func (f *headerMutatingFilter) OnDestroy() {}

func (f *headerMutatingFilter) EncodeHeaders(http.Header, bool) filter_http.FilterHeadersStatus {
	return filter_http.Continue
}
func (f *headerMutatingFilter) EncodeData([]byte, bool) filter_http.FilterDataStatus {
	return filter_http.DataContinue
}
func (f *headerMutatingFilter) EncodeTrailers(http.Header) filter_http.FilterTrailersStatus {
	return filter_http.TrailersContinue
}
func (f *headerMutatingFilter) SetEncoderCallbacks(filter_http.EncoderFilterCallbacks) {}

// mutatingChain returns a TWO-entry chainConfig: the supplied mutating filter
// ahead of the terminal router. mk is invoked once per request (ADR-0071's
// two-step factory pattern), so the caller can hand back a fresh instance or a
// shared one it retains a pointer to.
func mutatingChain(t *testing.T, mk func() *headerMutatingFilter) []chainEntry {
	t.Helper()
	rfFactory, err := router.New(nil, filter_http.FactoryCtx{})
	if err != nil {
		t.Fatalf("router.New: %v", err)
	}
	return []chainEntry{
		{name: "test.header_mutating", factory: func() filter_http.HTTPFilter {
			hf := mk()
			return filter_http.HTTPFilter{Name: "test.header_mutating", Decoder: hf, Encoder: hf}
		}},
		{name: "envoy.filters.http.router", factory: rfFactory},
	}
}

// ---------------------------------------------------------------------------
// Count-and-set helpers
// ---------------------------------------------------------------------------

// h2HeaderValues returns EVERY case-insensitive match of name among the
// H2Request's regular headers, in wire order.
//
// h2HeaderValue (connection_test.go) returns only the FIRST match. It therefore
// cannot see the [a, c, c] duplication an in-place `fields[:0]` rebuild can
// produce, and it cannot distinguish "removed" from "still present later in the
// slice" — a removal arm written with h2HeaderValue ALONE is vacuous. Every
// removal/duplicate row below asserts through this helper instead.
func h2HeaderValues(req h2.H2Request, name string) []string {
	var out []string
	for _, f := range req.Headers {
		if strings.EqualFold(f.Name, name) {
			out = append(out, f.Value)
		}
	}
	return out
}

// h2HeaderNames returns the carrier's field names in wire order, so a row can
// assert POSITION and not merely membership.
func h2HeaderNames(req h2.H2Request) []string {
	out := make([]string, 0, len(req.Headers))
	for _, f := range req.Headers {
		out = append(out, f.Name)
	}
	return out
}

// ---------------------------------------------------------------------------
// Driver
// ---------------------------------------------------------------------------

// reconcileOpts tunes the WriteH2 drive for the arms that need a body or a
// non-Continue status.
type reconcileOpts struct {
	body          []byte
	onData        func(http.Header, []byte)
	headersStatus filter_http.FilterHeadersStatus
	dataStatus    filter_http.FilterDataStatus
	localReply    *localReplySpec
	headersFired  *bool
	dataFired     *bool
}

// driveWriteH2 seeds BOTH containers from `seed` — the ordered carrier
// (h2req.Headers) and the decode map (c.req.Header) — exactly as the production
// path does (h2/stream.go builds both from the same decoded HEADERS block via
// buildH2Request and buildRequest, and buildRequest uses http.Header.Add, so
// the map keys are canonical-MIME). It then drives the real WriteH2 seam and
// returns what the terminal router filter held at RunAction time, plus the
// wire-writer and the WriteH2 error.
//
// mutate == nil installs a ROUTER-ONLY chain (the no-filter baseline).
func driveWriteH2(t *testing.T, seed []hpack.HeaderField, mutate func(http.Header), opts reconcileOpts) (h2.H2Request, *captureH2Writer, error) {
	t.Helper()
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/health"), action: &directResponseAction{status: 200, bodyText: "OK\n"}},
	}}

	var chainCfg []chainEntry
	if mutate == nil && opts.onData == nil && opts.localReply == nil &&
		opts.headersStatus == filter_http.Continue && opts.dataStatus == filter_http.DataContinue {
		chainCfg = routerOnlyChain(t)
	} else {
		chainCfg = mutatingChain(t, func() *headerMutatingFilter {
			return &headerMutatingFilter{
				onHeaders:     mutate,
				onData:        opts.onData,
				headersStatus: opts.headersStatus,
				dataStatus:    opts.dataStatus,
				localReply:    opts.localReply,
				headersFired:  opts.headersFired,
				dataFired:     opts.dataFired,
			}
		})
	}

	f := newH2DispatchFilter(t, tt, chainCfg, nil)

	hreq, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	hreq.Proto = "HTTP/2.0"
	for _, hf := range seed {
		hreq.Header.Add(hf.Name, hf.Value)
	}

	var captured h2.H2Request
	c := &chainDispatchAction{f: f, action: captureH2Action(&captured), req: hreq, routeIdx: 0}

	h2req := h2.H2Request{
		Method:    "GET",
		Path:      "/health",
		Scheme:    "https",
		Authority: "localhost",
		Body:      opts.body,
		Headers:   append([]hpack.HeaderField(nil), seed...),
	}
	w := &captureH2Writer{}
	werr := c.WriteH2(context.Background(), h2req, w)
	return captured, w, werr
}

// reconcileCase is one row of the 13-row reconciler table.
type reconcileCase struct {
	name   string
	seed   []hpack.HeaderField
	mutate func(http.Header)
	// check runs the row's per-property assertions. t.Errorf per property —
	// NEVER t.Fatalf mid-table, which would make later rows' assertions dead
	// code inside a shared subtest body.
	check func(t *testing.T, captured h2.H2Request)
}

// ---------------------------------------------------------------------------
// The reconciler table
// ---------------------------------------------------------------------------

func TestWriteH2_DecodeFilterHeaderReconcile(t *testing.T) {
	// Row 1's expectation is the no-filter baseline carrier, measured against
	// the SAME seed with a router-only chain.
	baselineSeed := []hpack.HeaderField{
		{Name: "x-alpha", Value: "1"},
		{Name: "x-beta", Value: "2"},
	}
	baseline, _, err := driveWriteH2(t, baselineSeed, nil, reconcileOpts{})
	if err != nil {
		t.Fatalf("baseline WriteH2: %v", err)
	}

	cases := []reconcileCase{
		{
			// Row 1 — no-op passthrough: a filter is present but mutates
			// nothing. The delta is empty, so the reconciler must early-return
			// and leave the carrier BYTE-STABLE relative to the no-filter path.
			name: "1_noop_passthrough_byte_stable",
			seed: baselineSeed,
			mutate: func(http.Header) {
			},
			check: func(t *testing.T, captured h2.H2Request) {
				if !reflect.DeepEqual(captured.Headers, baseline.Headers) {
					t.Errorf("carrier = %v, want byte-identical to the no-filter baseline %v",
						captured.Headers, baseline.Headers)
				}
			},
		},
		{
			// Row 2 — add: a net-new name lands AT THE TAIL, lowercase, and
			// every pre-existing field keeps its position.
			name: "2_add_lands_at_tail_lowercase",
			seed: []hpack.HeaderField{{Name: "x-alpha", Value: "1"}, {Name: "x-beta", Value: "2"}},
			mutate: func(h http.Header) {
				h.Set("X-New", "added")
			},
			check: func(t *testing.T, captured h2.H2Request) {
				wantNames := []string{"x-alpha", "x-beta", "x-new"}
				if got := h2HeaderNames(captured); !reflect.DeepEqual(got, wantNames) {
					t.Errorf("names = %v, want %v", got, wantNames)
				}
				if got := h2HeaderValues(captured, "x-new"); !reflect.DeepEqual(got, []string{"added"}) {
					t.Errorf("x-new = %v, want [added]", got)
				}
			},
		},
		{
			// Row 3 — remove: ZERO occurrences on the carrier. Asserted via
			// h2HeaderValues; h2HeaderValue alone cannot see a later duplicate.
			name: "3_remove_leaves_zero_occurrences",
			seed: []hpack.HeaderField{{Name: "x-alpha", Value: "1"}, {Name: "x-drop", Value: "gone"}, {Name: "x-beta", Value: "2"}},
			mutate: func(h http.Header) {
				h.Del("X-Drop")
			},
			check: func(t *testing.T, captured h2.H2Request) {
				if got := h2HeaderValues(captured, "x-drop"); len(got) != 0 {
					t.Errorf("x-drop occurrences = %v, want none", got)
				}
				wantNames := []string{"x-alpha", "x-beta"}
				if got := h2HeaderNames(captured); !reflect.DeepEqual(got, wantNames) {
					t.Errorf("names = %v, want %v", got, wantNames)
				}
			},
		},
		{
			// Row 4 — value change: the old value is gone, the new value is
			// present exactly ONCE, at the tail.
			name: "4_value_change_single_occurrence_at_tail",
			seed: []hpack.HeaderField{{Name: "x-alpha", Value: "1"}, {Name: "x-change", Value: "old"}, {Name: "x-beta", Value: "2"}},
			mutate: func(h http.Header) {
				h.Set("X-Change", "new")
			},
			check: func(t *testing.T, captured h2.H2Request) {
				if got := h2HeaderValues(captured, "x-change"); !reflect.DeepEqual(got, []string{"new"}) {
					t.Errorf("x-change = %v, want exactly [new]", got)
				}
				wantNames := []string{"x-alpha", "x-beta", "x-change"}
				if got := h2HeaderNames(captured); !reflect.DeepEqual(got, wantNames) {
					t.Errorf("names = %v, want %v (changed name re-appended at the tail)", got, wantNames)
				}
			},
		},
		{
			// Row 5 — multi-value: ONE FIELD PER VALUE (never a comma join),
			// all at the tail, in the map's value order.
			name: "5_multi_value_one_field_per_value",
			seed: []hpack.HeaderField{{Name: "x-alpha", Value: "1"}},
			mutate: func(h http.Header) {
				h.Add("X-Multi", "a")
				h.Add("X-Multi", "b")
			},
			check: func(t *testing.T, captured h2.H2Request) {
				if got := h2HeaderValues(captured, "x-multi"); !reflect.DeepEqual(got, []string{"a", "b"}) {
					t.Errorf("x-multi = %v, want [a b] (one field per value, map value order)", got)
				}
				wantNames := []string{"x-alpha", "x-multi", "x-multi"}
				if got := h2HeaderNames(captured); !reflect.DeepEqual(got, wantNames) {
					t.Errorf("names = %v, want %v", got, wantNames)
				}
			},
		},
		{
			// Row 6 — duplicate-name CHANGED. The carrier is seeded with two
			// NON-ADJACENT x-dup fields. The reference (contrib-v1.37.2) is an
			// ordered multimap: it removes EVERY occurrence and appends the new
			// value list AT THE TAIL. This row is what a first-occurrence
			// (collapse-in-place) implementation FAILS.
			name: "6_duplicate_name_changed_collapses_to_tail",
			seed: []hpack.HeaderField{
				{Name: "x-dup", Value: "one"},
				{Name: "x-mid", Value: "mid"},
				{Name: "x-dup", Value: "three"},
			},
			mutate: func(h http.Header) {
				h.Set("X-Dup", "replaced")
			},
			check: func(t *testing.T, captured h2.H2Request) {
				if got := h2HeaderValues(captured, "x-dup"); !reflect.DeepEqual(got, []string{"replaced"}) {
					t.Errorf("x-dup = %v, want exactly [replaced] (both originals dropped)", got)
				}
				wantNames := []string{"x-mid", "x-dup"}
				if got := h2HeaderNames(captured); !reflect.DeepEqual(got, wantNames) {
					t.Errorf("names = %v, want %v (TAIL-append, NOT first-position collapse)", got, wantNames)
				}
			},
		},
		{
			// Row 7 — duplicate-name UNTOUCHED. Same seeding, but the filter
			// mutates something else. Both x-dup occurrences must stay at their
			// ORIGINAL NON-ADJACENT positions: untouched names are never rebuilt.
			name: "7_duplicate_name_untouched_keeps_positions",
			seed: []hpack.HeaderField{
				{Name: "x-dup", Value: "one"},
				{Name: "x-mid", Value: "mid"},
				{Name: "x-dup", Value: "three"},
			},
			mutate: func(h http.Header) {
				h.Set("X-Other", "v")
			},
			check: func(t *testing.T, captured h2.H2Request) {
				wantNames := []string{"x-dup", "x-mid", "x-dup", "x-other"}
				if got := h2HeaderNames(captured); !reflect.DeepEqual(got, wantNames) {
					t.Errorf("names = %v, want %v (untouched duplicates keep original positions)", got, wantNames)
				}
				if got := h2HeaderValues(captured, "x-dup"); !reflect.DeepEqual(got, []string{"one", "three"}) {
					t.Errorf("x-dup = %v, want [one three] unchanged", got)
				}
			},
		},
		{
			// Row 7b — APPEND-TO-EXISTING keeps the original IN PLACE. This is
			// the SECOND reference rule, and it is the one a single
			// remove-everywhere-then-tail-append implementation gets wrong.
			//
			// Measured on envoyproxy/envoy:contrib-v1.37.2, downstream H2 ->
			// upstream H2, raw-framer recorder: with a client-sent
			// `x-client-dup: c0` at index [9], a route appending `v1` under
			// APPEND_IF_EXISTS_OR_ADD leaves `c0` AT [9] and puts `v1` at the
			// tail. Contrast row 6, where OVERWRITE_IF_EXISTS_OR_ADD removes
			// every occurrence and re-appends one copy at the tail.
			//
			// Under the unrefined rule this row reads [x-mid x-keep x-keep] —
			// the untouched original relocated to the tail. That is the
			// divergence this row exists to catch.
			name: "7b_append_to_existing_keeps_original_in_place",
			seed: []hpack.HeaderField{
				{Name: "x-keep", Value: "k0"},
				{Name: "x-mid", Value: "mid"},
			},
			mutate: func(h http.Header) {
				h.Add("X-Keep", "k1")
			},
			check: func(t *testing.T, captured h2.H2Request) {
				wantNames := []string{"x-keep", "x-mid", "x-keep"}
				if got := h2HeaderNames(captured); !reflect.DeepEqual(got, wantNames) {
					t.Errorf("names = %v, want %v (APPEND keeps the original at its position; only the added value goes to the tail)", got, wantNames)
				}
				if got := h2HeaderValues(captured, "x-keep"); !reflect.DeepEqual(got, []string{"k0", "k1"}) {
					t.Errorf("x-keep = %v, want [k0 k1]", got)
				}
			},
		},
		{
			// Row 8 — pseudo skip. WriteH2 injects :method / :authority / :path
			// onto c.req.Header (h2dispatch.go :471-499) so chain filters can
			// read them; they must NEVER reach the regular-header carrier, and
			// a filter writing a pseudo key must not either. The guard is a
			// BYTE TEST on the map key's leading ':', not a three-name list.
			name: "8_pseudo_headers_never_reach_the_carrier",
			seed: []hpack.HeaderField{{Name: "x-alpha", Value: "1"}},
			mutate: func(h http.Header) {
				// Raw-map access: http.Header.Set canonicalizes and would not
				// preserve the leading colon.
				h[":method"] = []string{"POST"}
				h[":custom"] = []string{"nope"}
			},
			check: func(t *testing.T, captured h2.H2Request) {
				for _, n := range h2HeaderNames(captured) {
					if strings.HasPrefix(n, ":") {
						t.Errorf("carrier carries pseudo-header %q; want none", n)
					}
				}
				if captured.Method != "GET" {
					t.Errorf("Method = %q, want GET (scalar untouched)", captured.Method)
				}
				if captured.Path != "/health" {
					t.Errorf("Path = %q, want /health (scalar untouched)", captured.Path)
				}
				if captured.Authority != "localhost" {
					t.Errorf("Authority = %q, want localhost (scalar untouched)", captured.Authority)
				}
			},
		},
		{
			// Row 9 — host skip (D-89-HOST: SKIP). Reference Envoy normalizes
			// `host` into `:authority`; a regular `host` field alongside
			// `:authority` is a conformance hazard. A filter mutating Host in
			// the decode map produces NO regular host field on the carrier and
			// does not move the :authority scalar.
			name: "9_host_is_never_emitted_as_a_regular_field",
			seed: []hpack.HeaderField{{Name: "x-alpha", Value: "1"}},
			mutate: func(h http.Header) {
				h.Set("Host", "evil.example.com")
				h["host"] = []string{"evil-lower.example.com"}
			},
			check: func(t *testing.T, captured h2.H2Request) {
				if got := h2HeaderValues(captured, "host"); len(got) != 0 {
					t.Errorf("host occurrences = %v, want none (host is skipped, not forwarded)", got)
				}
				if captured.Authority != "localhost" {
					t.Errorf("Authority = %q, want localhost (unchanged by a host mutation)", captured.Authority)
				}
			},
		},
		{
			// Row 10 — case. Go canonicalizes map keys via
			// http.CanonicalHeaderKey (header_mutation does exactly this to its
			// config keys), and NOTHING below the reconciler lowercases:
			// h2/client.go appends req.Headers verbatim. Uppercase on the H/2
			// wire is a protocol error, so the reconciler must lowercase.
			name: "10_canonical_map_key_emits_lowercase",
			seed: []hpack.HeaderField{{Name: "x-alpha", Value: "1"}},
			mutate: func(h http.Header) {
				h.Set("X-Mixed-Case", "v")
			},
			check: func(t *testing.T, captured h2.H2Request) {
				names := h2HeaderNames(captured)
				found := false
				for _, n := range names {
					if n == "X-Mixed-Case" {
						t.Errorf("carrier carries canonical-cased name %q; H/2 wire names must be lowercase", n)
					}
					if n == "x-mixed-case" {
						found = true
					}
				}
				if !found {
					t.Errorf("names = %v, want a lowercase x-mixed-case field", names)
				}
			},
		},
		{
			// Row 11 — RFC 9113 §8.2.2 drop (D-89-VALIDATE: IN, VALUE-AWARE).
			// The fix INTRODUCES this hazard: at the tip these mutations never
			// reach upstream at all. Measured against a conformant peer, a
			// leaked `connection: close` yields 400 + body
			// `request header "Connection" is not valid in HTTP/2` with ZERO
			// backend delivery. So the three illegal pairs are DROPPED (not
			// rejected — rejecting would turn a config-legal mutation into a
			// client-facing 5xx).
			//
			// `te: trailers` is the OVER-FIRING CONTROL: a name-only guard
			// would wrongly drop it, and `te` is CONDITIONALLY legal.
			name: "11_rfc9113_illegal_pairs_dropped_te_trailers_kept",
			seed: []hpack.HeaderField{{Name: "x-alpha", Value: "1"}},
			mutate: func(h http.Header) {
				h.Set("Connection", "close")
				h.Set("Transfer-Encoding", "chunked")
				h.Add("Te", "gzip")
				h.Add("Te", "trailers")
				h.Set("X-Benign", "kept")
			},
			check: func(t *testing.T, captured h2.H2Request) {
				for _, n := range []string{"connection", "transfer-encoding"} {
					if got := h2HeaderValues(captured, n); len(got) != 0 {
						t.Errorf("%s occurrences = %v, want none (RFC 9113 §8.2.2 connection-specific)", n, got)
					}
				}
				if got := h2HeaderValues(captured, "te"); !reflect.DeepEqual(got, []string{"trailers"}) {
					t.Errorf("te = %v, want exactly [trailers] (gzip dropped, trailers KEPT)", got)
				}
				if got := h2HeaderValues(captured, "x-benign"); !reflect.DeepEqual(got, []string{"kept"}) {
					t.Errorf("x-benign = %v, want [kept]", got)
				}
			},
		},
		{
			// Row 13 — order preservation across a mixed delta. Untouched names
			// hold their original positions; the ADDED/CHANGED set is appended
			// at the tail SORTED BY NAME (Go map iteration is nondeterministic,
			// so the sort is what makes the wire bytes reproducible).
			// x-beta (changed) sorts before x-delta (added).
			name: "13_order_untouched_prefix_then_sorted_tail",
			seed: []hpack.HeaderField{
				{Name: "x-alpha", Value: "1"},
				{Name: "x-beta", Value: "2"},
				{Name: "x-gamma", Value: "3"},
			},
			mutate: func(h http.Header) {
				h.Set("X-Beta", "changed")
				h.Set("X-Delta", "added")
			},
			check: func(t *testing.T, captured h2.H2Request) {
				wantNames := []string{"x-alpha", "x-gamma", "x-beta", "x-delta"}
				if got := h2HeaderNames(captured); !reflect.DeepEqual(got, wantNames) {
					t.Errorf("names = %v, want exactly %v", got, wantNames)
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			captured, _, err := driveWriteH2(t, tc.seed, tc.mutate, reconcileOpts{})
			if err != nil {
				t.Fatalf("WriteH2: %v", err)
			}
			tc.check(t, captured)
		})
	}
}

// ---------------------------------------------------------------------------
// T1c — early-exit arms: no reconcile is owed when the action never runs
// ---------------------------------------------------------------------------

// TestWriteH2_Reconcile_TerminateStreamSkipsAction covers RunDecodeHeaders'
// TerminateStream non-nil-error path (chain.go :385-386). WriteH2 returns the
// error from :519 BEFORE rf.RunAction, so the capture stays zero-valued and no
// reconcile is owed.
//
// The ctx-cancel path (chain.go :381-383) is deliberately NOT used: it requires
// a goroutine plus a deadline and cannot distinguish "parked" from "returned
// late". TerminateStream reaches the same return statement without parking.
func TestWriteH2_Reconcile_TerminateStreamSkipsAction(t *testing.T) {
	fired := false
	captured, _, err := driveWriteH2(t,
		[]hpack.HeaderField{{Name: "x-alpha", Value: "1"}},
		func(h http.Header) { h.Set("X-New", "added") },
		reconcileOpts{headersStatus: filter_http.TerminateStream, headersFired: &fired},
	)
	if !fired {
		t.Fatal("DecodeHeaders never fired; the arm is vacuous")
	}
	if !errors.Is(err, filter_http.ErrStreamTerminatedByFilter) {
		t.Errorf("WriteH2 err = %v, want ErrStreamTerminatedByFilter", err)
	}
	if !reflect.DeepEqual(captured, h2.H2Request{}) {
		t.Errorf("captured = %+v, want the zero H2Request (action never ran)", captured)
	}
}

// TestWriteH2_Reconcile_LocalReplyDoneSkipsAction covers h2dispatch.go's
// LocalReplyDone branch (:530), which had ZERO unit coverage before phase 89.
// A non-terminal filter calls SendLocalReply from DecodeHeaders; the chain
// transitions to encode mode, WriteH2 emits the synthesized reply and returns
// BEFORE rf.RunAction. No reconcile is owed and the local-reply wire shape is
// unaffected by the reconciler.
func TestWriteH2_Reconcile_LocalReplyDoneSkipsAction(t *testing.T) {
	fired := false
	captured, w, err := driveWriteH2(t,
		[]hpack.HeaderField{{Name: "x-alpha", Value: "1"}},
		func(h http.Header) { h.Set("X-New", "added") },
		reconcileOpts{
			localReply: &localReplySpec{
				status:  418,
				body:    "teapot",
				headers: filter_http.OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}},
			},
			headersFired: &fired,
		},
	)
	if !fired {
		t.Fatal("DecodeHeaders never fired; the arm is vacuous")
	}
	if err != nil {
		t.Errorf("WriteH2 err = %v, want nil on the SendLocalReply path", err)
	}
	if got := w.statusOf(); got != "418" {
		t.Errorf("wire :status = %q, want 418 (local reply emitted)", got)
	}
	if !reflect.DeepEqual(captured, h2.H2Request{}) {
		t.Errorf("captured = %+v, want the zero H2Request (action never ran)", captured)
	}
}

// TestWriteH2_Reconcile_DecodeDataErrorSkipsAction covers the RunDecodeData
// non-nil-error return at h2dispatch.go :552.
//
// ⚠️ That call site is guarded by `if hasBody` (:515, :551), so the arm is
// SILENTLY VACUOUS unless h2req.Body is non-empty. The arm sets a body AND
// asserts the DecodeData stub actually fired, which is what proves the guard
// was crossed.
func TestWriteH2_Reconcile_DecodeDataErrorSkipsAction(t *testing.T) {
	headersFired, dataFired := false, false
	captured, _, err := driveWriteH2(t,
		[]hpack.HeaderField{{Name: "x-alpha", Value: "1"}},
		func(h http.Header) { h.Set("X-New", "added") },
		reconcileOpts{
			body:         []byte("hello"),
			dataStatus:   filter_http.DataTerminateStream,
			headersFired: &headersFired,
			dataFired:    &dataFired,
		},
	)
	if !headersFired {
		t.Fatal("DecodeHeaders never fired; the arm is vacuous")
	}
	if !dataFired {
		t.Fatal("DecodeData never fired — hasBody was false, so the RunDecodeData arm is VACUOUS")
	}
	if !errors.Is(err, filter_http.ErrStreamTerminatedByFilter) {
		t.Errorf("WriteH2 err = %v, want ErrStreamTerminatedByFilter from RunDecodeData", err)
	}
	if !reflect.DeepEqual(captured, h2.H2Request{}) {
		t.Errorf("captured = %+v, want the zero H2Request (action never ran)", captured)
	}
}

// TestWriteH2_Reconcile_DecodeDataMutationIsApplied is the positive control for
// the arm above: with a body present and DecodeData returning DataContinue, a
// header mutation made from the DATA callback must ALSO reach the carrier,
// because the reconcile is placed AFTER the hasBody block, not between
// RunDecodeHeaders and RunDecodeData.
func TestWriteH2_Reconcile_DecodeDataMutationIsApplied(t *testing.T) {
	dataFired := false
	captured, _, err := driveWriteH2(t,
		[]hpack.HeaderField{{Name: "x-alpha", Value: "1"}},
		func(h http.Header) { h.Set("X-From-Headers", "h") },
		reconcileOpts{
			body:      []byte("hello"),
			dataFired: &dataFired,
			onData: func(h http.Header, b []byte) {
				h.Set("X-From-Data", string(b))
			},
		},
	)
	if err != nil {
		t.Fatalf("WriteH2: %v", err)
	}
	if !dataFired {
		t.Fatal("DecodeData never fired — hasBody was false; the arm is VACUOUS")
	}
	if got := h2HeaderValues(captured, "x-from-headers"); !reflect.DeepEqual(got, []string{"h"}) {
		t.Errorf("x-from-headers = %v, want [h]", got)
	}
	if got := h2HeaderValues(captured, "x-from-data"); !reflect.DeepEqual(got, []string{"hello"}) {
		t.Errorf("x-from-data = %v, want [hello] (DecodeData mutation must survive)", got)
	}
}

// ---------------------------------------------------------------------------
// T1d — the SetH2Request re-issue arm
// ---------------------------------------------------------------------------

// TestWriteH2_SetH2Request_ReissuedAfterReconcile pins the value-copy seam.
//
// h2dispatch.go calls rf.SetH2Request(h2req) at :457, BEFORE RunDecodeHeaders.
// router.Filter holds the H2Request BY VALUE, so a post-:457 mutation of the
// local h2req is invisible to RunAction unless SetH2Request is RE-ISSUED. This
// arm asserts the re-issue happened, and that it carried the WHOLE value (the
// pseudo-header scalars and the body), not a zero-value or a headers-only
// fragment.
//
// It lives in package hcm, not package router: repo-wide there are ZERO
// non-comment test call sites for SetAction/SetRequest/SetH2Action/
// SetH2Request/RunAction, and no test in package router ever constructs a
// router.Filter. captureH2Action here observes the real seam.
func TestWriteH2_SetH2Request_ReissuedAfterReconcile(t *testing.T) {
	captured, _, err := driveWriteH2(t,
		[]hpack.HeaderField{{Name: "x-alpha", Value: "1"}},
		func(h http.Header) { h.Set("X-Post-Set", "reissued") },
		reconcileOpts{body: []byte("body-bytes")},
	)
	if err != nil {
		t.Fatalf("WriteH2: %v", err)
	}
	if got := h2HeaderValues(captured, "x-post-set"); !reflect.DeepEqual(got, []string{"reissued"}) {
		t.Errorf("x-post-set = %v, want [reissued]; a mutation made AFTER the :457 SetH2Request "+
			"did not reach RunAction, so SetH2Request was not re-issued", got)
	}
	if captured.Method != "GET" || captured.Path != "/health" || captured.Authority != "localhost" || captured.Scheme != "https" {
		t.Errorf("scalars = {%q %q %q %q}, want {GET /health localhost https}; the re-issue must carry the whole value",
			captured.Method, captured.Path, captured.Authority, captured.Scheme)
	}
	if string(captured.Body) != "body-bytes" {
		t.Errorf("Body = %q, want body-bytes; the re-issue must carry the whole value", captured.Body)
	}
}
