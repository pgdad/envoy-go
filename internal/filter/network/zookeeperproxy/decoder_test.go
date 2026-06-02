package zookeeperproxy

import (
	"bytes"
	"encoding/binary"
	"testing"

	zookeeper_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/zookeeper_proxy/v3"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/stats"
)

// --- test frame builders (big-endian; 4-byte length prefix EXCLUDES itself) ---

func be32(v int32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(v))
	return b
}

func be64(v int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

func zkFrame(parts ...[]byte) []byte {
	payload := bytes.Join(parts, nil)
	return append(be32(int32(len(payload))), payload...)
}

// connectFrame: protocol_version(4=0) + last_zxid(8=0) + timeout(4) + session(8=0)
// + password(4-byte len + 16 bytes) [+ optional readonly bool(1)].
// The leading protocol_version=0 doubles as the sniffed ConnectXid (AMEND-A5).
func connectFrame(readonly *bool) []byte {
	parts := [][]byte{be32(0), be64(0), be32(30000), be64(0), be32(16), make([]byte, 16)}
	if readonly != nil {
		b := byte(0)
		if *readonly {
			b = 1
		}
		parts = append(parts, []byte{b})
	}
	return zkFrame(parts...)
}

// dataFrame: xid(4) + opcode(4) + payload.
func dataFrame(xid, opcode int32, payload []byte) []byte {
	return zkFrame(be32(xid), be32(opcode), payload)
}

func newTestDecoder(t *testing.T) (*requestDecoder, *rosterStats, *compiledConfig) {
	t.Helper()
	reg := stats.NewRegistry()
	cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk"})
	if err != nil {
		t.Fatal(err)
	}
	rs := newRosterStats(reg, "zk")
	return newRequestDecoder(cfg, rs), rs, cfg
}

func counterValue(t *testing.T, rs *rosterStats, suffix string) uint64 {
	t.Helper()
	return rs.counters[suffix].Load()
}

// --- special-xid constants (response-side xid, present for completeness) ---

// TestSpecialXidConstants verifies the special xid constants and ensures the
// watchXid constant is exercised (it is declared for completeness as per AMEND-A5,
// but is response-side only; used at 28.2). This prevents a lint "unused constant"
// on watchXid.
func TestSpecialXidConstants(t *testing.T) {
	if connectXid != 0 {
		t.Fatalf("connectXid = %d, want 0", connectXid)
	}
	if watchXid != -1 {
		t.Fatalf("watchXid = %d, want -1", watchXid)
	}
	if pingXid != -2 {
		t.Fatalf("pingXid = %d, want -2", pingXid)
	}
	if authXid != -4 {
		t.Fatalf("authXid = %d, want -4", authXid)
	}
	if setWatchesXid != -8 {
		t.Fatalf("setWatchesXid = %d, want -8", setWatchesXid)
	}
}

// --- special-xid dispatch (AMEND-A5) ---

func TestDecodeConnect(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnData(connectFrame(nil))
	if got := counterValue(t, rs, "connect_rq"); got != 1 {
		t.Fatalf("connect_rq = %d, want 1", got)
	}
	if got := counterValue(t, rs, "connect_readonly_rq"); got != 0 {
		t.Fatalf("connect_readonly_rq = %d, want 0", got)
	}
}

func TestDecodeConnectReadonly(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	ro := true
	d.decodeOnData(connectFrame(&ro))
	if got := counterValue(t, rs, "connect_readonly_rq"); got != 1 {
		t.Fatalf("connect_readonly_rq = %d, want 1", got)
	}
	if got := counterValue(t, rs, "connect_rq"); got != 0 {
		t.Fatalf("connect_rq = %d, want 0 (readonly connect counts ONLY connect_readonly_rq)", got)
	}
}

func TestDecodePing(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnData(zkFrame(be32(pingXid), be32(opPing)))
	if got := counterValue(t, rs, "ping_rq"); got != 1 {
		t.Fatalf("ping_rq = %d, want 1", got)
	}
}

// authFrame builds a real ZooKeeper auth request frame (length-prefix stripped
// inside zkFrame): xid(-4) | opcode(100) | type(0) | schemeLen | scheme | credLen.
// This mirrors upstream parseAuthRequest's wire layout (decoder.cc:396-413): the
// "Skip opcode + type" step means schemeLen sits at frame offset 12 — a frame
// WITHOUT the type field fails upstream's parse (peekString read-beyond-buffer).
func authFrame(scheme string) []byte {
	s := []byte(scheme)
	return zkFrame(be32(authXid), be32(opSetAuth) /* opcode 100 */, be32(0) /* type */, be32(int32(len(s))), s, be32(0) /* cred len */)
}

// auth: xid −4 → skip opcode(100) + type int(4) → scheme string (4-byte len +
// bytes) → dynamic auth.<scheme>_rq counter (AMEND-A3). A builtin scheme
// ("digest") gets its own counter.
func TestDecodeAuthScheme(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnData(authFrame("digest"))
	got := rs.reg.NewCounterIfAbsent("zk.zookeeper.auth.digest_rq").Load()
	if got != 1 {
		t.Fatalf("auth.digest_rq = %d, want 1", got)
	}
	// NO static auth_rq counter exists (AMEND-A3) — nothing else incremented.
}

// A non-builtin scheme ("foobar", valid charset but NOT in the upstream builtin
// set) takes the unknown_scheme fallback: auth.unknown_scheme_rq increments and
// NO auth.foobar_rq counter is ever created (upstream getBuiltin parity;
// live-verified scheme "foobar" → zkauth.zookeeper.auth.unknown_scheme_rq).
func TestDecodeAuthSchemeNonBuiltin(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnData(authFrame("foobar"))
	// The unknown_scheme fallback counter incremented (this is the live-checkable
	// assertion: we deliberately do NOT probe auth.foobar_rq via NewCounterIfAbsent
	// because that would CREATE it at 0 and mask a divergence — instead we scan the
	// registry to prove no foobar counter exists at all).
	if got := rs.reg.NewCounterIfAbsent("zk.zookeeper.auth.unknown_scheme_rq").Load(); got != 1 {
		t.Fatalf("auth.unknown_scheme_rq = %d, want 1 (non-builtin scheme fallback)", got)
	}
	rs.reg.Walk(func(m stats.Metric) {
		if m.Name() == "zk.zookeeper.auth.foobar_rq" {
			t.Fatalf("auth.foobar_rq must NEVER be created (non-builtin → unknown_scheme)")
		}
	})
}

func TestDecodeSetWatches(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnData(zkFrame(be32(setWatchesXid), be32(opSetWatches)))
	if got := counterValue(t, rs, "setwatches_rq"); got != 1 {
		t.Fatalf("setwatches_rq = %d, want 1", got)
	}
}

// --- reassembly + the high-water mark (D-S28.1-3; AMEND-A8) ---

// Partial frames: a frame split across two decodeOnData calls (cumulative chain
// buffer — the chain Buffer is never drained, so call 2 sees call 1's bytes too).
func TestDecodePartialFrameReassembly(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	frame := dataFrame(1, opGetData, []byte("/path"))
	cut := len(frame) / 2
	// First read: chain buffer holds the first half.
	d.decodeOnData(frame[:cut])
	if got := counterValue(t, rs, "getdata_rq"); got != 0 {
		t.Fatalf("getdata_rq = %d after partial frame, want 0", got)
	}
	// Second read: chain buffer now holds the WHOLE accumulating buffer
	// (the chain Buffer accumulates: zookeeperproxy never drains it).
	d.decodeOnData(frame)
	if got := counterValue(t, rs, "getdata_rq"); got != 1 {
		t.Fatalf("getdata_rq = %d after reassembly, want 1", got)
	}
}

// The high-water mark: re-delivered (undrained) chain-buffer bytes are NOT
// double-counted (D-S28.1-3 — the multi-read no-double-count proof).
// padTo is defined in the Task 10 test block below; these opcodes have min-lengths
// in the table so we need valid (padded) frames to pass min-length validation.
func TestDecodeHighWaterMarkNoDoubleCount(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	f1 := dataFrame(1, opGetData, make([]byte, 5)) // 8+5=13 bytes >= getdata min(13)
	f2 := dataFrame(2, opCreate, make([]byte, 16)) // 8+16=24 bytes >= create min(24)
	// Read 1: chain buffer = f1.
	d.decodeOnData(f1)
	// Read 2: chain buffer = f1 + f2 (accumulated — f1 is RE-DELIVERED).
	d.decodeOnData(append(append([]byte{}, f1...), f2...))
	if got := counterValue(t, rs, "getdata_rq"); got != 1 {
		t.Fatalf("getdata_rq = %d, want 1 (re-delivered bytes must not double-count)", got)
	}
	if got := counterValue(t, rs, "create_rq"); got != 1 {
		t.Fatalf("create_rq = %d, want 1", got)
	}
}

// Two complete frames in one read decode in sequence.
func TestDecodeTwoFramesOneRead(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	// 8+5=13 bytes >= getdata min(13); 8+5=13 bytes >= exists min(13)
	buf := append(dataFrame(1, opGetData, make([]byte, 5)), dataFrame(2, opExists, make([]byte, 5))...)
	d.decodeOnData(buf)
	if counterValue(t, rs, "getdata_rq") != 1 || counterValue(t, rs, "exists_rq") != 1 {
		t.Fatal("both frames in a single read must decode")
	}
}

// The decoder NEVER mutates the chain bytes it is fed (R3 precondition; the
// fuzzer re-asserts this). Using opClose (universal 8-byte min) avoids padding.
func TestDecodeDoesNotMutateInput(t *testing.T) {
	d, _, _ := newTestDecoder(t)
	frame := dataFrame(1, opClose, nil) // opClose has no entry in dataRequestMinLength → universal 8
	orig := append([]byte(nil), frame...)
	d.decodeOnData(frame)
	if !bytes.Equal(frame, orig) {
		t.Fatal("decodeOnData mutated its input slice")
	}
}

// --- data-request dispatch (Task 10) ---

// padTo returns filler payload bytes meeting dataRequestMinLength for the given
// opcode so that min-length validation passes. The dataFrame builder adds xid(4)
// + opcode(4) = 8 bytes; padTo supplies the additional bytes to reach the opcode's
// minimum. For opcodes whose minimum is the universal 8 (not in the table), padTo
// returns nil.
func padTo(opcode int32) []byte {
	minLen, ok := dataRequestMinLength[opcode]
	if !ok {
		return nil // universal 8-byte minimum; no extra bytes needed
	}
	extra := minLen - 8 // xid(4)+opcode(4) already in the frame header
	if extra <= 0 {
		return nil
	}
	return make([]byte, extra)
}

// Every wire opcode dispatches to its <opname>_rq counter — including the
// digit-suffixed ones (reference_proto_roster_extraction_digits guard).
func TestDecodeDataRequestAllOpcodes(t *testing.T) {
	cases := []struct {
		opcode int32
		suffix string
	}{
		{opGetData, "getdata_rq"},
		{opCreate, "create_rq"},
		{opCreate2, "create2_rq"},
		{opGetChildren2, "getchildren2_rq"},
		{opSetWatches2, "setwatches2_rq"},
		{opGetAllChildrenNumber, "getallchildrennumber_rq"},
		{opClose, "close_rq"},
		{opMulti, "multi_rq"},
		{opDelete, "delete_rq"},
		{opCheckWatches, "checkwatches_rq"},
	}
	for _, tc := range cases {
		t.Run(tc.suffix, func(t *testing.T) {
			d, rs, _ := newTestDecoder(t)
			d.decodeOnData(dataFrame(1, tc.opcode, padTo(tc.opcode)))
			if got := counterValue(t, rs, tc.suffix); got != 1 {
				t.Fatalf("%s = %d, want 1", tc.suffix, got)
			}
		})
	}
}

// SetAuth as a DATA request (xid > 0, opcode 100): upstream's data-request
// switch has NO SetAuth case (decoder.cc:134-244 v1.37.2) — it decode-errors
// (onDecodeError(nullopt): plain decoder_error, no per-opcode counter, no
// correlation). Mirror exactly (cross-side stat parity is load-bearing).
func TestDecodeSetAuthDataRequest(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	scheme := []byte("digest")
	payload := append(append(append(be32(0), be32(int32(len(scheme)))...), scheme...), be32(0)...)
	d.decodeOnData(dataFrame(5, opSetAuth, payload))
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1 (upstream: SetAuth data request is a decode error)", got)
	}
	// The dynamic auth counter must NOT fire (that's the AuthXid -4 path only):
	if got := rs.reg.NewCounterIfAbsent("zk.zookeeper.auth.digest_rq").Load(); got != 0 {
		t.Fatalf("auth.digest_rq = %d, want 0 (data-xid SetAuth is not an auth request)", got)
	}
	// setauth_rq stays dead:
	if got := counterValue(t, rs, "setauth_rq"); got != 0 {
		t.Fatalf("setauth_rq = %d, want 0", got)
	}
	// NO correlation write (upstream returns before requests_by_xid_ write):
	if _, ok := d.requestsByXid[5]; ok {
		t.Fatal("requestsByXid[5] must NOT be written for a decode-errored SetAuth data request")
	}
}

// Unknown opcode → decoder_error (no per-opcode counter — opcode unknown).
func TestDecodeUnknownOpcode(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnData(dataFrame(1, 9999, nil))
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
}

// Oversized frame (len > max_packet_bytes) → decoder_error + buffer abandoned;
// later reads decode normally (the 0046 arm-4 unit mirror).
func TestDecodeOversizedThenRecovers(t *testing.T) {
	reg := stats.NewRegistry()
	cfg, _ := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
		MaxPacketBytes: wrapperspb.UInt32(64)})
	rs := newRosterStats(reg, "zk")
	d := newRequestDecoder(cfg, rs)
	// Oversized: length prefix says 1000 > 64.
	d.decodeOnData(append(be32(1000), make([]byte, 10)...))
	if got := rs.counters["decoder_error"].Load(); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
	// Later read (fresh bytes appended after the abandoned buffer): decodes fine.
	prior := d.chainConsumed
	good := dataFrame(1, opGetData, padTo(opGetData))
	d.decodeOnData(append(make([]byte, prior), good...)) // chain buffer grew by `good`
	if got := rs.counters["getdata_rq"].Load(); got != 1 {
		t.Fatalf("getdata_rq = %d, want 1 (decoder must recover after abandon)", got)
	}
}

// Min-length validation (D-S28.1-1): a known opcode with a too-short frame →
// decoder_error + (flag-gated) <opname>_decoder_error.
// opGetData minimum is 13 (XID+OPCODE+INT+BOOL); sending only xid+opcode (8 bytes)
// triggers the error.
func TestDecodeMinLengthViolation(t *testing.T) {
	reg := stats.NewRegistry()
	cfg, _ := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
		EnablePerOpcodeDecoderErrorMetrics: true})
	rs := newRosterStats(reg, "zk")
	d := newRequestDecoder(cfg, rs)
	// Only xid+opcode (8 bytes) — getdata minimum is 13.
	d.decodeOnData(dataFrame(1, opGetData, nil))
	if got := rs.counters["decoder_error"].Load(); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
	if got := rs.counters["getdata_decoder_error"].Load(); got != 1 {
		t.Fatalf("getdata_decoder_error = %d, want 1 (flag enabled + opcode known)", got)
	}
}

// Flag gating (AMEND-A2): _rq_bytes increments ONLY when the flag is true;
// request_bytes increments ALWAYS; the wire footprint includes the 4-byte prefix.
func TestDecodeFlagGatedRequestBytes(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		reg := stats.NewRegistry()
		msg := &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
			EnablePerOpcodeRequestBytesMetrics: enabled}
		cfg, _ := parseConfig(msg)
		rs := newRosterStats(reg, "zk")
		d := newRequestDecoder(cfg, rs)
		frame := dataFrame(1, opGetData, padTo(opGetData))
		d.decodeOnData(frame)
		wantWire := uint64(len(frame)) // 4-byte prefix + payload
		if got := rs.counters["request_bytes"].Load(); got != wantWire {
			t.Fatalf("request_bytes = %d, want %d (always counted)", got, wantWire)
		}
		gotGated := rs.counters["getdata_rq_bytes"].Load()
		if enabled && gotGated != wantWire {
			t.Fatalf("getdata_rq_bytes = %d, want %d (flag on)", gotGated, wantWire)
		}
		if !enabled && gotGated != 0 {
			t.Fatalf("getdata_rq_bytes = %d, want 0 (flag off — gates increments not creation)", gotGated)
		}
	}
}

// --- control-frame decoder_error paths (closing the Task-10 review gap) ---

// TestDecodeControlFrameErrors verifies the flag-gated connect_decoder_error
// and auth_decoder_error counters are live, and that the universal sub-8-byte
// path fires plain decoder_error only.
func TestDecodeControlFrameErrors(t *testing.T) {
	mkDecoder := func(flagOn bool) (*requestDecoder, *rosterStats) {
		reg := stats.NewRegistry()
		cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{
			StatPrefix:                         "zk",
			EnablePerOpcodeDecoderErrorMetrics: flagOn,
		})
		if err != nil {
			t.Fatal(err)
		}
		rs := newRosterStats(reg, "zk")
		return newRequestDecoder(cfg, rs), rs
	}

	t.Run("short connect frame", func(t *testing.T) {
		d, rs := mkDecoder(true)
		// xid=0 (connectXid) but frame only 12 bytes < 28-byte fixed header:
		// decodeFrame universal 8-byte check passes; onConnect fires decoderError("connect").
		d.decodeOnData(zkFrame(be32(connectXid), be32(0), be32(0)))
		if rs.counters["decoder_error"].Load() != 1 || rs.counters["connect_decoder_error"].Load() != 1 {
			t.Fatalf("connect short frame: decoder_error=%d connect_decoder_error=%d, want 1/1",
				rs.counters["decoder_error"].Load(), rs.counters["connect_decoder_error"].Load())
		}
	})

	t.Run("negative connect pwLen", func(t *testing.T) {
		d, rs := mkDecoder(true)
		// Exactly 28-byte frame (fixedLen): passes len<fixedLen check; pwLen=-1 fires decoderError.
		// be32(0)=protocol_version, be64(0)=last_zxid, be32(30000)=timeout,
		// be64(0)=session_id, be32(-1)=password_length (negative).
		d.decodeOnData(zkFrame(be32(0), be64(0), be32(30000), be64(0), be32(-1)))
		if rs.counters["connect_decoder_error"].Load() != 1 {
			t.Fatal("negative pwLen must take the connect decoder_error path")
		}
	})

	t.Run("short auth frame", func(t *testing.T) {
		d, rs := mkDecoder(true)
		// xid=-4 (authXid): 16-byte frame passes decodeFrame universal 8-byte check
		// but is < the 20-byte auth floor (XID+OPCODE+INT+INT+INT, decoder.cc:397-398)
		// → onAuth fires decoderError("auth").
		// Layout: xid | opcode | type | schemeLen (4×4 = 16 bytes < 20-byte floor).
		d.decodeOnData(zkFrame(be32(authXid), be32(opSetAuth), be32(0), be32(0)))
		if rs.counters["auth_decoder_error"].Load() != 1 {
			t.Fatal("short auth frame must take the auth decoder_error path")
		}
	})

	t.Run("negative auth schemeLen", func(t *testing.T) {
		d, rs := mkDecoder(true)
		// 20-byte frame passes the len<20 floor; schemeLen at offset 12 = -1
		// fires decoderError("auth"). Layout: xid | opcode | type | schemeLen(-1) | pad(4).
		// The 4-byte pad is needed so the frame meets the 20-byte floor and the
		// negative-schemeLen branch is independently exercised (not shadowed by floor).
		d.decodeOnData(zkFrame(be32(authXid), be32(opSetAuth), be32(0), be32(-1), be32(0)))
		if rs.counters["auth_decoder_error"].Load() != 1 {
			t.Fatal("negative schemeLen must take the auth decoder_error path")
		}
	})

	t.Run("sub-8-byte frame", func(t *testing.T) {
		d, rs := mkDecoder(true)
		// 4-byte frame: passes nextFrame (frameLen=0 valid), but len(frame)=0 < 8
		// → decodeFrame fires decoderError("") — plain counter only, no per-opcode.
		d.decodeOnData(zkFrame(be32(1))) // 4-byte payload = 4-byte frame < universal 8 min
		if rs.counters["decoder_error"].Load() != 1 {
			t.Fatal("sub-8-byte frame must take the plain decoder_error path")
		}
	})

	t.Run("flag off gates per-opcode counters", func(t *testing.T) {
		d, rs := mkDecoder(false)
		d.decodeOnData(zkFrame(be32(connectXid), be32(0), be32(0))) // 12-byte short connect
		if rs.counters["decoder_error"].Load() != 1 {
			t.Fatal("plain decoder_error must fire regardless of flag")
		}
		if rs.counters["connect_decoder_error"].Load() != 0 {
			t.Fatal("per-opcode counter must NOT fire when flag is off")
		}
	})
}

// --- correlation structures ---

// Correlation structures (R5): data requests land in requestsByXid (insert
// overwrites); control requests append to the per-xid FIFO queue.
func TestDecodeCorrelationStructuresPopulated(t *testing.T) {
	d, _, _ := newTestDecoder(t)
	d.decodeOnData(dataFrame(7, opGetData, padTo(opGetData)))
	pr, ok := d.requestsByXid[7]
	if !ok || pr.opname != "getdata" || pr.wireOpcode != opGetData {
		t.Fatalf("requestsByXid[7] = (%+v, %v), want getdata entry", pr, ok)
	}
	// Insert overwrites (AMEND-A7): cumulative chain buffer = first-frame + second-frame.
	// chainConsumed after first call = len(dataFrame(7, opGetData, padTo(opGetData))).
	first := dataFrame(7, opGetData, padTo(opGetData))
	second := dataFrame(7, opExists, padTo(opExists))
	d.decodeOnData(append(first, second...))
	if d.requestsByXid[7].opname != "exists" {
		t.Fatalf("requestsByXid[7].opname = %q, want exists (insert overwrites)", d.requestsByXid[7].opname)
	}
	// Control FIFO: two pings queue in order.
	d2, _, _ := newTestDecoder(t)
	ping := zkFrame(be32(pingXid), be32(opPing))
	d2.decodeOnData(append(append([]byte{}, ping...), ping...))
	if got := len(d2.controlRequestsByXid[pingXid]); got != 2 {
		t.Fatalf("control queue len = %d, want 2 (FIFO per control xid)", got)
	}
}
