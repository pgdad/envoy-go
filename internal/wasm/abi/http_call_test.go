// Tests for the abi.HttpCallShim wire-decode + dispatch shim per 25.2 SPEC
// §5.1 #37 + §11.3 D-25.2-3 + AMEND-B3.
//
// Coverage:
//
//   - TestHttpCallShim_NonHostHandle_InternalFailure: a `host` value that
//     does not satisfy httpCallHost returns InternalFailure (programmer
//     error — wrong host wired).
//   - TestHttpCallShim_DispatchOk_WritesCallID: well-formed dispatch with
//     known cluster writes the allocated call_id to ret_call_id_ptr +
//     returns Ok.
//   - TestHttpCallShim_BadArgument_WritesZeroCallID: host returns
//     BadArgument (unknown cluster); shim writes 0 to ret_call_id_ptr +
//     propagates BadArgument.
//   - TestHttpCallShim_DecodePairs_RoundTrip: a fakeHost capturing the
//     decoded headers slice; verify the wire-format pairs decode produces
//     the same []HeaderPair25_2 that the shim receives.

package abi

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// fakeHttpCallHost is a minimal in-memory httpCallHost for shim tests.
// Records every DispatchHttpCall25_2 invocation; serves a canned response.
type fakeHttpCallHost struct {
	ctxID uint32

	dispatchReturnCallID uint32
	dispatchReturnStatus WasmResult

	lastCluster  string
	lastHeaders  []HeaderPair25_2
	lastBody     []byte
	lastTrailers []HeaderPair25_2
	lastTimeout  uint32
}

func (f *fakeHttpCallHost) CurrentCtxID() uint32 { return f.ctxID }

func (f *fakeHttpCallHost) DispatchHttpCall25_2(_ context.Context, _ uint32, cluster string, headers []HeaderPair25_2, body []byte, trailers []HeaderPair25_2, timeoutMs uint32) (uint32, WasmResult) {
	f.lastCluster = cluster
	f.lastHeaders = headers
	f.lastBody = append([]byte(nil), body...)
	f.lastTrailers = trailers
	f.lastTimeout = timeoutMs
	return f.dispatchReturnCallID, f.dispatchReturnStatus
}

// mustInstantiateMinimalModule builds a tiny wazero module with a 1-page
// memory + returns the api.Module so tests can read+write guest memory
// directly without going through the full hostcall envelope. Reuses the
// existing minimalModuleWasm helper if present; otherwise constructs an
// inline minimal binary.
func mustInstantiateMinimalModule(t *testing.T, ctx context.Context) (api.Module, func()) {
	t.Helper()
	rt := wazero.NewRuntime(ctx)
	// Hand-rolled minimal wasm: magic + version + memory section (1 page) +
	// export memory.
	src := []byte{
		// magic + version
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
		// memory section: 1 memory with min=1 no max
		0x05, 0x03, 0x01, 0x00, 0x01,
		// export section: 1 export "memory" kind=2 idx=0
		0x07, 0x0a, 0x01, 0x06, 'm', 'e', 'm', 'o', 'r', 'y', 0x02, 0x00,
	}
	mod, err := rt.Instantiate(ctx, src)
	if err != nil {
		_ = rt.Close(ctx)
		t.Fatalf("Instantiate: %v", err)
	}
	return mod, func() {
		_ = mod.Close(ctx)
		_ = rt.Close(ctx)
	}
}

func TestHttpCallShim_NonHostHandle_InternalFailure(t *testing.T) {
	ctx := context.Background()
	mod, cleanup := mustInstantiateMinimalModule(t, ctx)
	defer cleanup()

	status := HttpCallShim(ctx, mod, "not-an-httpCallHost",
		0, 0, // cluster
		0, 0, // headers
		0, 0, // body
		0, 0, // trailers
		1000, // timeout
		0,    // ret_call_id_ptr
	)
	if status != WasmResultInternalFailure {
		t.Errorf("status=%v; want InternalFailure on non-host handle", status)
	}
}

func TestHttpCallShim_DispatchOk_WritesCallID(t *testing.T) {
	ctx := context.Background()
	mod, cleanup := mustInstantiateMinimalModule(t, ctx)
	defer cleanup()

	host := &fakeHttpCallHost{
		ctxID:                100,
		dispatchReturnCallID: 42,
		dispatchReturnStatus: WasmResultOk,
	}

	// Write the cluster name to guest memory at offset 64.
	cluster := []byte("cluster_a")
	if !mod.Memory().Write(64, cluster) {
		t.Fatalf("Write cluster")
	}

	// Reserve offset 256 for ret_call_id_ptr (4 bytes).
	status := HttpCallShim(ctx, mod, host,
		64, uint32(len(cluster)), // cluster (data, size)
		0, 0, // headers (empty)
		0, 0, // body (empty)
		0, 0, // trailers (empty)
		5000, // timeout
		256,  // ret_call_id_ptr
	)
	if status != WasmResultOk {
		t.Fatalf("status=%v; want Ok", status)
	}
	if host.lastCluster != "cluster_a" {
		t.Errorf("host.lastCluster=%q; want cluster_a", host.lastCluster)
	}
	if host.lastTimeout != 5000 {
		t.Errorf("host.lastTimeout=%d; want 5000", host.lastTimeout)
	}
	got, ok := mod.Memory().ReadUint32Le(256)
	if !ok {
		t.Fatalf("ReadUint32Le ret_call_id")
	}
	if got != 42 {
		t.Errorf("ret_call_id=%d; want 42", got)
	}
}

func TestHttpCallShim_BadArgument_WritesZeroCallID(t *testing.T) {
	ctx := context.Background()
	mod, cleanup := mustInstantiateMinimalModule(t, ctx)
	defer cleanup()

	host := &fakeHttpCallHost{
		ctxID:                100,
		dispatchReturnCallID: 0,
		dispatchReturnStatus: WasmResultBadArgument,
	}

	cluster := []byte("unknown")
	if !mod.Memory().Write(64, cluster) {
		t.Fatalf("Write cluster")
	}
	// Pre-seed ret_call_id_ptr with a non-zero value so we can verify it's
	// cleared on BadArgument.
	if !mod.Memory().WriteUint32Le(256, 0xdeadbeef) {
		t.Fatalf("WriteUint32Le pre-seed")
	}

	status := HttpCallShim(ctx, mod, host,
		64, uint32(len(cluster)),
		0, 0,
		0, 0,
		0, 0,
		1000,
		256,
	)
	if status != WasmResultBadArgument {
		t.Errorf("status=%v; want BadArgument", status)
	}
	got, ok := mod.Memory().ReadUint32Le(256)
	if !ok {
		t.Fatalf("ReadUint32Le ret_call_id")
	}
	if got != 0 {
		t.Errorf("ret_call_id=%d; want 0 (zeroed on BadArgument)", got)
	}
}

func TestHttpCallShim_DecodePairs_RoundTrip(t *testing.T) {
	ctx := context.Background()
	mod, cleanup := mustInstantiateMinimalModule(t, ctx)
	defer cleanup()

	host := &fakeHttpCallHost{
		ctxID:                100,
		dispatchReturnCallID: 7,
		dispatchReturnStatus: WasmResultOk,
	}

	// Encode pairs in the proxy-wasm wire format:
	//   u32 num_pairs
	//   (u32 key_len, u32 value_len) * num_pairs
	//   (key_bytes, NUL, value_bytes, NUL) * num_pairs
	pairs := []HeaderPair25_2{
		{Key: ":method", Value: "POST"},
		{Key: ":path", Value: "/echo"},
		{Key: "x-custom", Value: "hello"},
	}
	encoded := encodePairs25_2(pairs)

	// Write everything to guest memory at separate offsets.
	cluster := []byte("cluster_a")
	body := []byte("the body bytes")
	if !mod.Memory().Write(64, cluster) {
		t.Fatalf("Write cluster")
	}
	headersOffset := uint32(128)
	if !mod.Memory().Write(headersOffset, encoded) {
		t.Fatalf("Write headers")
	}
	bodyOffset := uint32(512)
	if !mod.Memory().Write(bodyOffset, body) {
		t.Fatalf("Write body")
	}

	status := HttpCallShim(ctx, mod, host,
		64, uint32(len(cluster)),
		headersOffset, uint32(len(encoded)),
		bodyOffset, uint32(len(body)),
		0, 0, // trailers empty
		3000,
		2048,
	)
	if status != WasmResultOk {
		t.Fatalf("status=%v; want Ok", status)
	}

	// Verify the host saw the decoded pairs verbatim.
	if len(host.lastHeaders) != len(pairs) {
		t.Fatalf("lastHeaders len=%d; want %d", len(host.lastHeaders), len(pairs))
	}
	for i, want := range pairs {
		if host.lastHeaders[i] != want {
			t.Errorf("header[%d] = %+v; want %+v", i, host.lastHeaders[i], want)
		}
	}
	if string(host.lastBody) != string(body) {
		t.Errorf("lastBody=%q; want %q", host.lastBody, body)
	}
}

// encodePairs25_2 is a test-only encoder mirroring the wasm-package
// EncodePairs implementation. Used to construct valid pairs frames for
// the decode round-trip test above; defining it here keeps the test
// self-contained + avoids an import cycle.
func encodePairs25_2(pairs []HeaderPair25_2) []byte {
	size := 4
	for _, p := range pairs {
		size += 8 + len(p.Key) + 1 + len(p.Value) + 1
	}
	out := make([]byte, size)
	n := uint32(len(pairs))
	out[0] = byte(n)
	out[1] = byte(n >> 8)
	out[2] = byte(n >> 16)
	out[3] = byte(n >> 24)
	pos := 4
	for _, p := range pairs {
		k := uint32(len(p.Key))
		out[pos] = byte(k)
		out[pos+1] = byte(k >> 8)
		out[pos+2] = byte(k >> 16)
		out[pos+3] = byte(k >> 24)
		pos += 4
		v := uint32(len(p.Value))
		out[pos] = byte(v)
		out[pos+1] = byte(v >> 8)
		out[pos+2] = byte(v >> 16)
		out[pos+3] = byte(v >> 24)
		pos += 4
	}
	for _, p := range pairs {
		pos += copy(out[pos:], p.Key)
		out[pos] = 0
		pos++
		pos += copy(out[pos:], p.Value)
		out[pos] = 0
		pos++
	}
	return out
}
