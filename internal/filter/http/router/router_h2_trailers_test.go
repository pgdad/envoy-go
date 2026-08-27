package router

import (
	"bytes"
	"context"
	stdtls "crypto/tls"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"net"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"

	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
)

// ---------------------------------------------------------------------------
// Phase 84.1 Task 5 — trailing-HEADERS backend
//
// runH2TrailerBackend is a SEPARATE backend from runH2Backend, deliberately.
// runH2Backend is depended on by four other test files in this package
// (router_h2_test.go, retry_test.go, hedge_h2_test.go, router_weighted_test.go
// reach it via startH2Backend); adding a trailing-block arm to its behavior
// switch would put a new frame sequence one enum value away from every
// existing H2 test. This one only ever serves the trailer shapes.
// ---------------------------------------------------------------------------

// h2TrailerBehavior selects the frame sequence runH2TrailerBackend emits after
// it reads each request HEADERS frame.
type h2TrailerBehavior int

const (
	// h2TrailerNone: HEADERS(:status 200) + DATA(body, END_STREAM). No trailing
	// block at all — the stacked no-trailers control (PLAN Task 2 reasoning:
	// a populate-site test that only ever sees trailers cannot tell "populated
	// from the response" from "populated unconditionally").
	h2TrailerNone h2TrailerBehavior = iota
	// h2TrailerEmit: HEADERS(:status 200) + DATA(body) + trailing
	// HEADERS(trailers, END_STREAM). The fields come from the caller, so the
	// SAME behavior serves both the well-formed and the malformed arm.
	h2TrailerEmit
	// h2TrailerPeerReset: read HEADERS, then immediately RST_STREAM
	// (INTERNAL_ERROR). This is the discriminating control for the Task 5
	// part-B arm: the client surfaces a stream-scoped *h2.Error whose Code is
	// ErrInternalError — IDENTICAL to the malformed-trailers rejection's code —
	// but which does NOT carry the h2.ErrMalformedTrailers sentinel. It must
	// keep the pre-existing 502 + evict disposition.
	h2TrailerPeerReset
)

// runH2TrailerBackend handles one connection: client preface + SETTINGS
// exchange, then serves an unbounded sequence of streams on that ONE
// connection (the pooled-conn reuse path — the conn-not-evicted assertion
// depends on a second request landing on this same goroutine).
//
// leading is the LEADING response header block. Phase 92 Task 9 added it: the
// response-header-validation arms have to drive blocks that no map can
// express — DUPLICATE field names (two content-length fields), an UPPERCASE
// field name, and connection-specific fields such as keep-alive / upgrade /
// proxy-connection — so the carrier is an ORDERED SLICE and every field is
// written to the encoder exactly as supplied, in the given wire order, with no
// filtering, no normalization and no de-duplication.
//
// A nil leading selects h2DefaultLeadingBlock(body), which is the block this
// backend hard-coded before the seam existed, so every pre-seam caller keeps
// its exact wire output. A NON-nil but EMPTY slice is honored as an
// intentionally empty leading block, not as "use the default".
func runH2TrailerBackend(conn net.Conn, behavior h2TrailerBehavior, body []byte, trailers []hpack.HeaderField, leading []hpack.HeaderField) {
	defer func() { _ = conn.Close() }()
	prefaceBuf := make([]byte, 24)
	if _, err := io.ReadFull(conn, prefaceBuf); err != nil {
		return
	}
	if string(prefaceBuf) != "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n" {
		return
	}
	fr := http2.NewFramer(conn, conn)
	frame, err := fr.ReadFrame()
	if err != nil {
		return
	}
	if _, ok := frame.(*http2.SettingsFrame); !ok {
		return
	}
	if err := fr.WriteSettings(http2.Setting{ID: http2.SettingMaxFrameSize, Val: 16384}); err != nil {
		return
	}
	if _, err := fr.ReadFrame(); err != nil { // client SETTINGS_ACK
		return
	}
	if err := fr.WriteSettingsAck(); err != nil {
		return
	}
	// The HPACK encoder is connection-scoped (dynamic table persists across
	// streams); allocate once outside the per-stream loop.
	var hbuf bytes.Buffer
	henc := hpack.NewEncoder(&hbuf)
	// The leading block is loop-invariant — resolve the caller's override (or
	// the default) once, before the per-stream loop, so every stream on this
	// connection emits the same field sequence.
	lead := leading
	if lead == nil {
		lead = h2DefaultLeadingBlock(body)
	}
	for {
		streamID, ok := nextH2HeadersStreamID(fr)
		if !ok {
			return // conn closed / read error
		}
		if behavior == h2TrailerPeerReset {
			if err := fr.WriteRSTStream(streamID, http2.ErrCodeInternal); err != nil {
				return
			}
			continue
		}
		hbuf.Reset()
		for _, hf := range lead {
			_ = henc.WriteField(hf)
		}
		if err := fr.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      streamID,
			BlockFragment: hbuf.Bytes(),
			EndStream:     false,
			EndHeaders:    true,
		}); err != nil {
			return
		}
		emit := behavior == h2TrailerEmit
		if err := fr.WriteData(streamID, !emit /* endStream */, body); err != nil {
			return
		}
		if !emit {
			continue
		}
		hbuf.Reset()
		for _, tf := range trailers {
			_ = henc.WriteField(tf)
		}
		if err := fr.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      streamID,
			BlockFragment: hbuf.Bytes(),
			EndStream:     true,
			EndHeaders:    true,
		}); err != nil {
			return
		}
	}
}

// h2DefaultLeadingBlock is the leading response header block
// runH2TrailerBackend emits when the caller supplies none. It reproduces,
// field for field and in order, the block the backend hard-coded before phase
// 92 Task 9 introduced the caller-supplied seam — keeping it in ONE place is
// what makes "supply nothing" byte-identical to the pre-seam behavior.
func h2DefaultLeadingBlock(body []byte) []hpack.HeaderField {
	return []hpack.HeaderField{
		{Name: ":status", Value: "200"},
		{Name: "content-type", Value: "text/plain"},
		{Name: "content-length", Value: strconv.Itoa(len(body))},
	}
}

// nextH2HeadersStreamID reads frames until a HEADERS frame arrives, returning
// its stream id. Non-HEADERS frames (WINDOW_UPDATE / PING / SETTINGS / the
// client's own RST_STREAM after a rejected trailer block) are skipped.
func nextH2HeadersStreamID(fr *http2.Framer) (uint32, bool) {
	for {
		frame, err := fr.ReadFrame()
		if err != nil {
			return 0, false
		}
		if hf, ok := frame.(*http2.HeadersFrame); ok {
			return hf.StreamID, true
		}
	}
}

// startH2TrailerBackend listens on a fresh TLS port with NextProtos=["h2"] and
// runs runH2TrailerBackend for each accepted conn. leading is threaded through
// unchanged — nil selects the default leading block (see runH2TrailerBackend).
func startH2TrailerBackend(t *testing.T, pki *h2BackendPKI, behavior h2TrailerBehavior, body []byte, trailers []hpack.HeaderField, leading []hpack.HeaderField) net.Listener {
	t.Helper()
	pair, err := stdtls.X509KeyPair(pki.leafCertPEM, pki.leafKeyPEM)
	if err != nil {
		t.Fatalf("server keypair: %v", err)
	}
	cfg := &stdtls.Config{
		Certificates: []stdtls.Certificate{pair},
		NextProtos:   []string{"h2"},
		MinVersion:   stdtls.VersionTLS12,
		MaxVersion:   stdtls.VersionTLS13,
	}
	ln, err := stdtls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("listen tls: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go runH2TrailerBackend(c, behavior, body, trailers, leading)
		}
	}()
	return ln
}

// ---------------------------------------------------------------------------
// Part A — populate at the ONE success site
// ---------------------------------------------------------------------------

// TestRouterActionH2_PopulatesTrailersAtSuccessSite is the phase-84.1 Task 5
// RED anchor for part A: the success ActionResponse return in
// doH2ClusterAction must carry h2.H2Response.Trailers verbatim (order and
// values preserved — the trailing HPACK block's wire order is the same
// RFC 9113 §8.1.2 order guarantee the leading block gets).
func TestRouterActionH2_PopulatesTrailersAtSuccessSite(t *testing.T) {
	pki := mkH2BackendPKI(t)
	body := []byte("upstream-ok\n")
	want := []hpack.HeaderField{
		{Name: "grpc-status", Value: "0"},
		{Name: "grpc-message", Value: "ok"},
	}
	ln := startH2TrailerBackend(t, pki, h2TrailerEmit, body, want, nil)
	defer func() { _ = ln.Close() }()

	c := h2EndpointCluster(t, ln.Addr().String(), pki)
	a := &routerActionH2{cluster: c}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())
	if err != nil {
		t.Fatalf("doH2ClusterAction: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("status = %d, want 200", resp.Status)
	}
	if !bytes.Equal(resp.Body, body) {
		t.Errorf("body = %q, want %q", resp.Body, body)
	}
	if len(resp.Trailers) != len(want) {
		t.Fatalf("ActionResponse.Trailers = %#v (len %d), want len %d", resp.Trailers, len(resp.Trailers), len(want))
	}
	for i := range want {
		if resp.Trailers[i] != want[i] {
			t.Errorf("ActionResponse.Trailers[%d] = %#v, want %#v", i, resp.Trailers[i], want[i])
		}
	}
}

// TestRouterActionH2_NoTrailersStaysEmpty is the STACKED CONTROL for the test
// above: an upstream response with NO trailing HEADERS block must leave
// ActionResponse.Trailers empty. Without it, an implementation that populated
// Trailers unconditionally (from any non-nil slice, or with a fabricated
// value) would pass the populate test.
func TestRouterActionH2_NoTrailersStaysEmpty(t *testing.T) {
	pki := mkH2BackendPKI(t)
	body := []byte("upstream-ok\n")
	ln := startH2TrailerBackend(t, pki, h2TrailerNone, body, nil, nil)
	defer func() { _ = ln.Close() }()

	c := h2EndpointCluster(t, ln.Addr().String(), pki)
	a := &routerActionH2{cluster: c}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())
	if err != nil {
		t.Fatalf("doH2ClusterAction: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("status = %d, want 200", resp.Status)
	}
	if !bytes.Equal(resp.Body, body) {
		t.Errorf("body = %q, want %q", resp.Body, body)
	}
	if len(resp.Trailers) != 0 {
		t.Errorf("ActionResponse.Trailers = %#v, want empty (backend emitted no trailing block)", resp.Trailers)
	}
}

// TestRouterActionH2_LocalReplySitesCarryNoTrailers behaviorally pins three
// of the six non-success ActionResponse returns in router_h2.go. The
// remaining three (circuit-breaker 503, grant-race-exhausted 503, pool-overflow
// 503) need cluster-internal state to reach and are covered by the structural
// audit below, which states its denominator.
func TestRouterActionH2_LocalReplySitesCarryNoTrailers(t *testing.T) {
	pki := mkH2BackendPKI(t)

	t.Run("dial-failure-502", func(t *testing.T) {
		c := h2EndpointCluster(t, "127.0.0.1:1", pki)
		a := &routerActionH2{cluster: c}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())
		if err != nil {
			t.Fatalf("doH2ClusterAction: %v", err)
		}
		if resp.Status != 502 {
			t.Fatalf("status = %d, want 502", resp.Status)
		}
		if resp.Trailers != nil {
			t.Errorf("dial-failure 502 ActionResponse.Trailers = %#v, want nil", resp.Trailers)
		}
	})

	t.Run("roundtrip-protocol-error-502", func(t *testing.T) {
		ln := startH2Backend(t, pki, h2BackendMalformed, nil)
		defer func() { _ = ln.Close() }()
		c := h2EndpointCluster(t, ln.Addr().String(), pki)
		a := &routerActionH2{cluster: c}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())
		if err != nil {
			t.Fatalf("doH2ClusterAction: %v", err)
		}
		if resp.Status != 502 {
			t.Fatalf("status = %d, want 502", resp.Status)
		}
		if resp.Trailers != nil {
			t.Errorf("protocol-error 502 ActionResponse.Trailers = %#v, want nil", resp.Trailers)
		}
	})

	t.Run("ctx-cancel-status0", func(t *testing.T) {
		ln := startH2Backend(t, pki, h2BackendHang, nil)
		defer func() { _ = ln.Close() }()
		c := h2EndpointCluster(t, ln.Addr().String(), pki)
		a := &routerActionH2{cluster: c}
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(200 * time.Millisecond)
			cancel()
		}()
		resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())
		if err == nil {
			t.Fatal("doH2ClusterAction returned nil err; want stream-scoped CANCEL error")
		}
		if resp.Status != 0 {
			t.Fatalf("status = %d, want 0 (ctx-cancel sentinel)", resp.Status)
		}
		if resp.Trailers != nil {
			t.Errorf("ctx-cancel ActionResponse.Trailers = %#v, want nil", resp.Trailers)
		}
	})
}

// ---------------------------------------------------------------------------
// Structural audit — WHICH ActionResponse literals set Trailers, package-wide
//
// This is an AUDIT, not a sample: it parses every non-test .go file in this
// package, enumerates EVERY ActionResponse composite literal, and states its
// denominator. A behavioral test can only reach the sites a test can drive;
// three of doH2ClusterAction's six non-success returns need cluster-internal
// state (circuit-breaker admission, grant-race exhaustion, pool overflow).
//
// It also carries the retryExecutorH2 / hedgeExecutorH2 / router_weighted.go
// ZERO-EDIT claim: those three propagate ActionResponse BY VALUE, so if none
// of their literals sets Trailers and none of them rebuilds the struct on the
// success path, the field rides through untouched. The one place they DO
// rebuild it — retry.go's synthesized 504 — must NOT carry trailers (it
// REPLACES the upstream response wholesale), and the audit pins that.
// ---------------------------------------------------------------------------

// actionResponseLit is one ActionResponse composite literal found in the
// package source.
type actionResponseLit struct {
	fn          string // enclosing top-level func (or method) name
	file        string
	line        int
	hasTrailers bool
	statusLit   int  // the Status: value when it is an integer literal
	statusIsLit bool // false when Status is an expression (e.g. resp.Status)
}

// collectActionResponseLits parses the package's non-test sources and returns
// every ActionResponse composite literal with its enclosing func.
func collectActionResponseLits(t *testing.T) []actionResponseLit {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parser.ParseDir: %v", err)
	}
	pkg, ok := pkgs["router"]
	if !ok {
		t.Fatalf("package %q not found in parsed dir (got %v)", "router", pkgs)
	}
	var out []actionResponseLit
	for name, f := range pkg.Files {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				cl, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				id, ok := cl.Type.(*ast.Ident)
				if !ok || id.Name != "ActionResponse" {
					return true
				}
				lit := actionResponseLit{fn: fd.Name.Name, file: name, line: fset.Position(cl.Pos()).Line}
				for _, el := range cl.Elts {
					kv, ok := el.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, ok := kv.Key.(*ast.Ident)
					if !ok {
						continue
					}
					switch key.Name {
					case "Trailers":
						lit.hasTrailers = true
					case "Status":
						if bl, ok := kv.Value.(*ast.BasicLit); ok && bl.Kind == token.INT {
							if v, err := strconv.Atoi(bl.Value); err == nil {
								lit.statusLit, lit.statusIsLit = v, true
							}
						}
					}
				}
				out = append(out, lit)
				return true
			})
		}
	}
	return out
}

// trailersAssignSites returns every `<expr>.Trailers = ...` assignment in the
// package's non-test sources, as "func (file:line)". The composite-literal walk
// above is BLIND to variable assignment: `ar := ActionResponse{...};
// ar.Trailers = x` carries no Trailers key on any literal and would read as "no
// site populates Trailers". Tasks 6 and 8 both touch this struct, so the audit
// needs the second form covered before they land, not after.
//
// The sweep is PACKAGE-WIDE rather than a named-func roster: a roster has to be
// kept in step with the code it audits, and the one function someone forgets to
// add is exactly the one that would carry the defect.
func trailersAssignSites(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parser.ParseDir: %v", err)
	}
	var out []string
	for name, f := range pkgs["router"].Files {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				as, ok := n.(*ast.AssignStmt)
				if !ok {
					return true
				}
				for _, lhs := range as.Lhs {
					if se, ok := lhs.(*ast.SelectorExpr); ok && se.Sel.Name == "Trailers" {
						out = append(out, fd.Name.Name+" ("+name+":"+strconv.Itoa(fset.Position(as.Pos()).Line)+")")
					}
				}
				return true
			})
		}
	}
	return out
}

// TestActionResponseLiterals_NoTrailersAssignmentOutsideTheLiteral closes the
// literal-only walk's blind spot: the router package must populate Trailers via
// the ONE composite literal in doH2ClusterAction and by no other route.
func TestActionResponseLiterals_NoTrailersAssignmentOutsideTheLiteral(t *testing.T) {
	if sites := trailersAssignSites(t); len(sites) != 0 {
		t.Errorf("the router package assigns .Trailers outside a composite literal at %v; the literal audit is blind to that form", sites)
	}
}

// TestActionResponseLiterals_OnlySuccessSitePopulatesTrailers is the
// structural half of "one populate site, and the others do not".
func TestActionResponseLiterals_OnlySuccessSitePopulatesTrailers(t *testing.T) {
	lits := collectActionResponseLits(t)
	if len(lits) == 0 {
		t.Fatal("collectActionResponseLits found 0 ActionResponse literals — the AST walk is broken, not the code")
	}
	t.Logf("audited %d ActionResponse composite literals across the router package", len(lits))

	var withTrailers []actionResponseLit
	for _, l := range lits {
		if l.hasTrailers {
			withTrailers = append(withTrailers, l)
		}
	}
	if len(withTrailers) != 1 {
		t.Fatalf("ActionResponse literals setting Trailers = %d (%+v), want exactly 1", len(withTrailers), withTrailers)
	}
	if withTrailers[0].fn != "doH2ClusterAction" {
		t.Errorf("the Trailers-setting literal is in %s (%s:%d), want doH2ClusterAction",
			withTrailers[0].fn, withTrailers[0].file, withTrailers[0].line)
	}
	if withTrailers[0].statusIsLit {
		t.Errorf("the Trailers-setting literal has an integer Status literal (%d) — it must be the success site forwarding resp.Status",
			withTrailers[0].statusLit)
	}
}

// TestActionResponseLiterals_DoH2ClusterActionNonSuccessSites pins the SET of
// non-success ActionResponse returns inside doH2ClusterAction by their Status
// values, not by a bare count (a count guard is blind to one site being
// swapped for another). Every one of them must leave Trailers unset.
//
// The expected multiset after phase 92 (was 7 sites after phase 84.1 Task 5;
// phase 92 adds the malformed-response-HEADERS 502, making it 8):
//
//	503 circuit-breaker admission reject
//	503 grant-race retries exhausted
//	503 connection-pool overflow
//	502 dial / acquire failure
//	  0 RoundTrip ctx-cancel        (CANCEL sentinel)
//	  0 RoundTrip malformed trailers (INTERNAL_ERROR sentinel, Task 5 part B)
//	502 RoundTrip malformed response headers (phase 92, ADR-0313 — 502, NOT the
//	    trailer sentinel's Status: 0; the reference answers 502)
//	502 RoundTrip transport error
func TestActionResponseLiterals_DoH2ClusterActionNonSuccessSites(t *testing.T) {
	var got []int
	var nonLiteral []actionResponseLit
	for _, l := range collectActionResponseLits(t) {
		if l.fn != "doH2ClusterAction" || l.hasTrailers {
			continue
		}
		if !l.statusIsLit {
			nonLiteral = append(nonLiteral, l)
			continue
		}
		got = append(got, l.statusLit)
	}
	if len(nonLiteral) != 0 {
		t.Errorf("doH2ClusterAction has non-success ActionResponse literals with a non-literal Status: %+v", nonLiteral)
	}
	sort.Ints(got)
	want := []int{0, 0, 502, 502, 502, 503, 503, 503}
	if len(got) != len(want) {
		t.Fatalf("doH2ClusterAction non-Trailers ActionResponse Status set = %v (n=%d), want %v (n=%d)", got, len(got), want, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("doH2ClusterAction non-Trailers ActionResponse Status set = %v, want %v", got, want)
		}
	}
}

// TestActionResponseLiterals_RetrySynthesized504DropsTrailers pins the ONE
// by-value-propagation exception named in the Task 5 brief: retry.go's
// per-try-timeout 504 REPLACES the driver's ActionResponse wholesale, so the
// upstream trailers are dropped with it. Driving that behaviorally needs a
// per-try-timeout race; the literal is the load-bearing fact and it is
// derived from the source, not asserted from memory.
func TestActionResponseLiterals_RetrySynthesized504DropsTrailers(t *testing.T) {
	var found int
	for _, l := range collectActionResponseLits(t) {
		if l.statusIsLit && l.statusLit == 504 {
			found++
			if l.hasTrailers {
				t.Errorf("synthesized 504 at %s:%d (%s) sets Trailers; it must REPLACE the upstream response, trailers included",
					l.file, l.line, l.fn)
			}
		}
	}
	if found != 2 {
		t.Errorf("synthesized-504 ActionResponse literals = %d, want 2 (retryExecutorH1 + retryExecutorH2)", found)
	}
}

// TestRouterActionH2_TrailersSurviveRetryExecutorByValue proves the
// zero-edit-by-value claim for retryExecutorH2 with a live drive rather than
// only structurally: a retry_policy-carrying route whose first attempt
// succeeds must still surface the upstream trailers.
func TestRouterActionH2_TrailersSurviveRetryExecutorByValue(t *testing.T) {
	pki := mkH2BackendPKI(t)
	body := []byte("upstream-ok\n")
	want := []hpack.HeaderField{{Name: "grpc-status", Value: "0"}}
	ln := startH2TrailerBackend(t, pki, h2TrailerEmit, body, want, nil)
	defer func() { _ = ln.Close() }()

	c := h2EndpointCluster(t, ln.Addr().String(), pki)
	rp, err := NewRetryPolicy("5xx", 1, nil, 0, 0, 0)
	if err != nil {
		t.Fatalf("NewRetryPolicy: %v", err)
	}
	act := H2ClusterAction(c, nil, cluster.SubsetMatch{}, rp, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, _, aerr := act(ctx, h2RequestForTest())
	if aerr != nil {
		t.Fatalf("H2ClusterAction(retry): %v", aerr)
	}
	if resp.Status != 200 {
		t.Fatalf("status = %d, want 200", resp.Status)
	}
	if len(resp.Trailers) != 1 || resp.Trailers[0] != want[0] {
		t.Errorf("retryExecutorH2 ActionResponse.Trailers = %#v, want %#v", resp.Trailers, want)
	}
}

// ---------------------------------------------------------------------------
// Part B — the malformed-trailers arm (routed here from Task 3)
// ---------------------------------------------------------------------------

// malformedTrailerBlock is a trailing HEADERS block that validateResponseTrailers
// rejects: content-length carries framing, which a trailer section has none of.
func malformedTrailerBlock() []hpack.HeaderField {
	return []hpack.HeaderField{{Name: "content-length", Value: "5"}}
}

// TestRouterActionH2_MalformedTrailersRSTsStreamAndKeepsConn is the phase-84.1
// Task 5 RED anchor for part B. A locally-detected malformed response-trailer
// block must NOT become a 502 local reply and must NOT evict the pooled conn:
// the reference resets the STREAM. The router therefore has to
//
//	(1) discriminate on errors.Is(err, h2.ErrMalformedTrailers) — NOT on
//	    err.Code == ErrInternalError, which a peer RST_STREAM shares (see the
//	    control below),
//	(2) return the ORIGINAL *h2.Error value, unwrapped: serverStream.dispatch
//	    reads the RST code via a bare type assertion writeErr.(*Error)
//	    (h2/stream.go), so any fmt.Errorf("%w", err) wrap silently degrades to
//	    the default INTERNAL_ERROR path without carrying the code, and
//	(3) skip EvictH2ConnOnError.
func TestRouterActionH2_MalformedTrailersRSTsStreamAndKeepsConn(t *testing.T) {
	pki := mkH2BackendPKI(t)
	body := []byte("upstream-ok\n")
	ln := startH2TrailerBackend(t, pki, h2TrailerEmit, body, malformedTrailerBlock(), nil)
	defer func() { _ = ln.Close() }()

	c, reg := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
	a := &routerActionH2{cluster: c}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())

	if err == nil {
		t.Fatalf("doH2ClusterAction returned nil err with resp.Status=%d; want the malformed-trailers *h2.Error", resp.Status)
	}
	if resp.Status == 502 {
		t.Errorf("status = 502; a malformed trailer block must RST the stream, not synthesize a 502 local reply")
	}
	if resp.Status != 0 {
		t.Errorf("status = %d, want 0 (no response is finalized; the reset is signaled via the returned *h2.Error)", resp.Status)
	}
	if resp.Trailers != nil {
		t.Errorf("rejected-trailers ActionResponse.Trailers = %#v, want nil", resp.Trailers)
	}
	if !errors.Is(err, h2.ErrMalformedTrailers) {
		t.Fatalf("errors.Is(err, h2.ErrMalformedTrailers) = false for %#v (%v), want true", err, err)
	}
	// The bare type assertion serverStream.dispatch performs. A %w wrap would
	// still satisfy errors.Is above but fail HERE, and downstream would get a
	// default INTERNAL_ERROR rather than the carried code.
	hErr, ok := err.(*h2.Error) //nolint:errorlint // this IS the dispatch-side assertion under test
	if !ok {
		t.Fatalf("err is %T, want *h2.Error unwrapped (serverStream.dispatch uses a bare type assertion)", err)
	}
	if hErr.Code != h2.ErrInternalError {
		t.Errorf("err code = %v, want INTERNAL_ERROR (RST_STREAM(INTERNAL_ERROR) downstream)", hErr.Code)
	}
	if hErr.Stream == 0 {
		t.Errorf("err stream id = 0; a connection-scoped error would tear the pooled conn down")
	}

	// Conn NOT evicted: the pool has no exported size accessor, so observe it
	// via REUSE. upstream_cx_http2_total counts dials. It is 1 after the
	// rejected request; a second request that HITS the pooled conn leaves it
	// at 1, while an evicted (and Closed) conn forces a fresh dial to 2.
	// The 502 suppression, MEASURED rather than reasoned: the pre-Task-5 path
	// ran IncStatusClass(502) on its way to the local reply, so a regression
	// that re-routed this error through the 502 arm would show up here even if
	// the ActionResponse assertions above were somehow satisfied.
	if got := counterValue(t, reg, "cluster.c_h2_backend.upstream_rq_5xx"); got != 0 {
		t.Errorf("upstream_rq_5xx = %d, want 0 (a stream reset books no upstream HTTP outcome)", got)
	}
	if got := counterValue(t, reg, "cluster.c_h2_backend.upstream_cx_http2_total"); got != 1 {
		t.Fatalf("upstream_cx_http2_total after the rejected request = %d, want 1", got)
	}
	if _, _, err2 := doH2ClusterAction(ctx, a, h2RequestForTest()); !errors.Is(err2, h2.ErrMalformedTrailers) {
		t.Fatalf("second request: err = %v, want the same malformed-trailers rejection (the backend serves the same shape)", err2)
	}
	if got := counterValue(t, reg, "cluster.c_h2_backend.upstream_cx_http2_total"); got != 1 {
		t.Errorf("upstream_cx_http2_total after a SECOND request = %d, want 1 (the conn must be REUSED — a malformed trailer block resets the stream, not the conn)", got)
	}
}

// ---------------------------------------------------------------------------
// Part B follow-up — the retry.go interaction (Task 5 fix)
//
// The arm above added a SECOND Status:0 producer to doH2ClusterAction.
// retryExecutorH2's per-try-timeout predicate was written when Status:0 meant
// only "ctx-cancel sentinel", so it would launder a malformed-trailers
// rejection that lands at/after the attempt deadline into a synthesized 504
// with err=nil — a gateway error instead of a stream reset, with the *h2.Error
// discarded.
//
// ⚠️ WHY THIS IS GATED AS A PURE PREDICATE AND NOT END-TO-END. The bug needs
// (malformed error returned) AND (attempt deadline elapsed) to coincide, and
// those two race inside h2.RoundTrip's select: once both cs.doneCh and
// ctx.Done() are ready, Go picks uniformly at random.
//
// The end-to-end frequency can only be measured with a DISCRIMINATING probe —
// one that evaluates the OLD and the NEW predicate over the SAME sample —
// because a laundered rejection and a legitimate per-try timeout are
// observationally identical downstream (both are a 504 with err=nil), so a bare
// 504 count cannot separate them. So measured: the unnarrowed predicate
// mis-booked 551/2000 requests (27.6%) at a 300µs per_try_timeout, 33-45/2000 at
// 400µs, and 0 at >=1.2ms. The window is short-timeout-heavy.
//
// Two constructions that do NOT work, both refuted by measurement rather than
// argument. A 1µs per_try_timeout: the attempt then fails at dial/CANCEL and
// becomes a LEGITIMATE 504 — malformed 0/2000, and 20/20 identical 504s before
// and after the fix — so a test built on it would be a broken gate, red in both
// arms. And any longer timeout: at >=1.2ms the laundering rate is zero, so
// there is nothing to catch. There is no per_try_timeout value that makes the
// end-to-end path a reliable gate.
//
// The predicate is therefore split out of retryExecutorH2 and takes `now` as a
// parameter, so the exact bug state can be constructed deterministically.
// ---------------------------------------------------------------------------

// malformedTrailersErrForTest builds the shape h2.RoundTrip surfaces for a
// rejected trailer block, without reaching into the h2 package's unexported
// constructor. Kept structurally identical to h2's malformedTrailersError.
func malformedTrailersErrForTest() error {
	return &h2.Error{Code: h2.ErrInternalError, Stream: 1, Msg: "content-length not permitted in a trailer section", Underlying: h2.ErrMalformedTrailers}
}

// TestRetryPolicy_PerTryTimedOutH2_ExcludesMalformedTrailers is the
// DISCRIMINATING gate for the Task 5 fix. Removing the sentinel exclusion from
// perTryTimedOutH2 flips the two malformed-trailers rows and nothing else,
// which is exactly what a deliberate break confirmed.
func TestRetryPolicy_PerTryTimedOutH2_ExcludesMalformedTrailers(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	past := base.Add(-time.Millisecond) // deadline already elapsed at `now`
	future := base.Add(time.Second)     // deadline not reached at `now`
	cancelErr := h2.NewStreamError(h2.ErrCancel, 0, "upstream roundtrip: ctx canceled")

	cases := []struct {
		name         string
		perTry       time.Duration
		parentCtxErr error
		resp         ActionResponse
		attemptErr   error
		deadline     time.Time
		want         bool
	}{
		{"ctx-cancel sentinel at an elapsed deadline is a per-try timeout",
			10 * time.Millisecond, nil, ActionResponse{Status: 0}, cancelErr, past, true},
		{"localOrigin 502 at an elapsed deadline is a per-try timeout",
			10 * time.Millisecond, nil, ActionResponse{Status: 502, localOrigin: true}, nil, past, true},
		{"malformed trailers at an elapsed deadline is NOT a per-try timeout",
			10 * time.Millisecond, nil, ActionResponse{Status: 0}, malformedTrailersErrForTest(), past, false},
		// Defense in depth: doH2ClusterAction's malformed arm returns Status:0
		// WITHOUT localOrigin today, so this shape is not currently producible.
		// It pins that the exclusion is evaluated whichever of the two legs of
		// the (localOrigin || Status==0) admission test let the attempt in, so a
		// future producer that sets localOrigin cannot slip past.
		{"malformed trailers admitted via the localOrigin leg is NOT a per-try timeout",
			10 * time.Millisecond, nil, ActionResponse{Status: 502, localOrigin: true}, malformedTrailersErrForTest(), past, false},
		{"a clean upstream 200 is never misclassified",
			10 * time.Millisecond, nil, ActionResponse{Status: 200}, nil, past, false},
		{"a PARENT client-cancel is not a per-try timeout",
			10 * time.Millisecond, context.Canceled, ActionResponse{Status: 0}, cancelErr, past, false},
		{"no per_try_timeout configured",
			0, nil, ActionResponse{Status: 0}, cancelErr, past, false},
		{"deadline not reached",
			10 * time.Millisecond, nil, ActionResponse{Status: 0}, cancelErr, future, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rp := &RetryPolicy{perTryTimeout: tc.perTry}
			got := rp.perTryTimedOutH2(tc.parentCtxErr, tc.resp, tc.attemptErr, tc.deadline, base)
			if got != tc.want {
				t.Errorf("perTryTimedOutH2 = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRouterActionH2_MalformedTrailersSurvivesRetryExecutor drives the live
// stack. It is a WIRING gate, not the discriminating one (see the block
// comment above): it pins that a malformed-trailers rejection reaching
// retryExecutorH2 comes back UNWRAPPED — matching no retry_on flag, never
// retried, and never converted to a synthesized 504 — on both the plain retry
// path and the per_try_timeout path at normal timings.
func TestRouterActionH2_MalformedTrailersSurvivesRetryExecutor(t *testing.T) {
	for _, tc := range []struct {
		name   string
		perTry time.Duration
	}{
		{"no-per-try-timeout", 0},
		{"with-per-try-timeout", 500 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pki := mkH2BackendPKI(t)
			ln := startH2TrailerBackend(t, pki, h2TrailerEmit, []byte("x"), malformedTrailerBlock(), nil)
			defer func() { _ = ln.Close() }()

			c, reg := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
			// WITHOUT this the retry counters are never registered, every Inc is a
			// silent no-op and the ==0 assertions below would be VACUOUS (a counter
			// that cannot move proves nothing). The sibling control below proves
			// they can in fact move.
			c.EnsureRetryStats()
			// retry_on covers 5xx + gateway-error + reset + connect-failure so that
			// a mis-shaped outcome would be RETRIED and the assertion below would
			// see it; the correct outcome (Status 0, not localOrigin) matches none.
			rp, err := NewRetryPolicy("5xx,gateway-error,reset,connect-failure", 2, nil, 0, 0, tc.perTry)
			if err != nil {
				t.Fatalf("NewRetryPolicy: %v", err)
			}
			act := H2ClusterAction(c, nil, cluster.SubsetMatch{}, rp, nil)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			resp, _, aerr := act(ctx, h2RequestForTest())

			if aerr == nil {
				t.Fatalf("retryExecutorH2 returned nil err with resp.Status=%d; the *h2.Error was discarded", resp.Status)
			}
			if resp.Status == 504 {
				t.Errorf("status = 504: the rejection was laundered into a synthesized per-try-timeout gateway error")
			}
			if resp.Status != 0 {
				t.Errorf("status = %d, want 0", resp.Status)
			}
			if !errors.Is(aerr, h2.ErrMalformedTrailers) {
				t.Fatalf("errors.Is(err, h2.ErrMalformedTrailers) = false for %v", aerr)
			}
			if _, ok := aerr.(*h2.Error); !ok { //nolint:errorlint // the dispatch-side bare assertion is what is under test
				t.Errorf("err is %T, want *h2.Error unwrapped after passing through retryExecutorH2", aerr)
			}
			if got := counterValue(t, reg, "cluster.c_h2_backend.upstream_rq_per_try_timeout"); got != 0 {
				t.Errorf("upstream_rq_per_try_timeout = %d, want 0", got)
			}
			if got := counterValue(t, reg, "cluster.c_h2_backend.upstream_rq_retry"); got != 0 {
				t.Errorf("upstream_rq_retry = %d, want 0 (a stream reset matches no retry_on flag)", got)
			}
			if got := counterValue(t, reg, "cluster.c_h2_backend.upstream_rq_total"); got != 1 {
				t.Errorf("upstream_rq_total = %d, want 1 (exactly one attempt)", got)
			}
		})
	}
}

// TestRouterActionH2_GenuinePerTryTimeoutStillBooks504 is the STACKED CONTROL
// for the two ==0 assertions above: it proves upstream_rq_per_try_timeout is
// registered and CAN move on this cluster, so reading 0 there is a measurement
// and not a no-op. It doubles as the regression gate on the perTryTimedOutH2
// extraction — a genuine per-try timeout must still Inc the counter, synthesize
// the 504 and clear the error.
func TestRouterActionH2_GenuinePerTryTimeoutStillBooks504(t *testing.T) {
	pki := mkH2BackendPKI(t)
	ln := startH2Backend(t, pki, h2BackendHang, nil)
	defer func() { _ = ln.Close() }()

	c, reg := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
	c.EnsureRetryStats()
	rp, err := NewRetryPolicy("5xx", 0, nil, 0, 0, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("NewRetryPolicy: %v", err)
	}
	act := H2ClusterAction(c, nil, cluster.SubsetMatch{}, rp, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, _, aerr := act(ctx, h2RequestForTest())
	if aerr != nil {
		t.Fatalf("err = %v, want nil (a per-try timeout is a synthesized 504)", aerr)
	}
	if resp.Status != 504 {
		t.Fatalf("status = %d, want 504", resp.Status)
	}
	if got := counterValue(t, reg, "cluster.c_h2_backend.upstream_rq_per_try_timeout"); got != 1 {
		t.Errorf("upstream_rq_per_try_timeout = %d, want 1", got)
	}
}

// TestRouterActionH2_PeerResetInternalErrorStillEvictsAnd502s is the
// DISCRIMINATING CONTROL for the test above. A peer RST_STREAM(INTERNAL_ERROR)
// finishes the client stream with an *h2.Error carrying the SAME code as the
// malformed-trailers rejection. It must keep the pre-existing disposition:
// 502 local reply, err==nil, pooled conn evicted. Without this arm, an
// implementation discriminating on err.Code == ErrInternalError would pass the
// malformed-trailers test while silently breaking every peer-reset request.
func TestRouterActionH2_PeerResetInternalErrorStillEvictsAnd502s(t *testing.T) {
	pki := mkH2BackendPKI(t)
	ln := startH2TrailerBackend(t, pki, h2TrailerPeerReset, nil, nil, nil)
	defer func() { _ = ln.Close() }()

	c, reg := h2EndpointClusterWithRegistry(t, ln.Addr().String(), pki)
	a := &routerActionH2{cluster: c}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())
	if err != nil {
		t.Fatalf("doH2ClusterAction: err = %v, want nil (a peer reset becomes a 502 local reply)", err)
	}
	if resp.Status != 502 {
		t.Fatalf("status = %d, want 502 (peer RST_STREAM(INTERNAL_ERROR) keeps the local-reply path)", resp.Status)
	}
	if resp.Trailers != nil {
		t.Errorf("peer-reset 502 ActionResponse.Trailers = %#v, want nil", resp.Trailers)
	}
	if got := counterValue(t, reg, "cluster.c_h2_backend.upstream_cx_http2_total"); got != 1 {
		t.Fatalf("upstream_cx_http2_total after the first request = %d, want 1", got)
	}
	// Evicted: the second request cannot reuse the (closed) conn, so it dials.
	if _, _, err2 := doH2ClusterAction(ctx, a, h2RequestForTest()); err2 != nil {
		t.Fatalf("second request: err = %v, want nil", err2)
	}
	if got := counterValue(t, reg, "cluster.c_h2_backend.upstream_cx_http2_total"); got != 2 {
		t.Errorf("upstream_cx_http2_total after a SECOND request = %d, want 2 (the poisoned conn must have been EVICTED)", got)
	}
}

// TestRouterActionH2_Upstream5xxStillForwardedWithoutTrailers is the second
// discriminating control: a plain upstream 5xx is not a RoundTrip error at
// all, so it stays on the forward-verbatim path and carries no trailers.
func TestRouterActionH2_Upstream5xxStillForwardedWithoutTrailers(t *testing.T) {
	pki := mkH2BackendPKI(t)
	body := []byte("upstream-503\n")
	ln := startH2Backend(t, pki, h2Backend503, body)
	defer func() { _ = ln.Close() }()

	c := h2EndpointCluster(t, ln.Addr().String(), pki)
	a := &routerActionH2{cluster: c}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, _, err := doH2ClusterAction(ctx, a, h2RequestForTest())
	// Capture the sentinel verdict BEFORE the nil-check Fatalf: asserting
	// !errors.Is(err, ...) after establishing err == nil is dead code that reads
	// as coverage.
	if matched := errors.Is(err, h2.ErrMalformedTrailers); matched {
		t.Errorf("a plain upstream 5xx matched the malformed-trailers sentinel: %v", err)
	}
	if err != nil {
		t.Fatalf("doH2ClusterAction: %v", err)
	}
	if resp.Status != 503 {
		t.Errorf("status = %d, want 503 forwarded verbatim", resp.Status)
	}
	if len(resp.Trailers) != 0 {
		t.Errorf("upstream-503 ActionResponse.Trailers = %#v, want empty", resp.Trailers)
	}
}
