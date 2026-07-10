package matchpredicate

import (
	"errors"
	"testing"

	cmatcherv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/matcher/v3"
	"google.golang.org/protobuf/proto"
)

// mustMarshal renders a MatchPredicate to wire bytes for the seed corpus.
func mustMarshal(t *testing.T, mp *cmatcherv3.MatchPredicate) []byte {
	t.Helper()
	b, err := proto.Marshal(mp)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	return b
}

// TestFuzzSeed_DepthCapFires is a non-fuzz guard that the depth-33 seed really
// trips the cap. A cap that is never exercised is not a cap (D-TAP-DEPTHCAP).
func TestFuzzSeed_DepthCapFires(t *testing.T) {
	deep := nest(MaxDepth + 1)
	if _, err := Compile(deep); !errors.Is(err, ErrDepthExceeded) {
		t.Fatalf("depth %d: err = %v, want ErrDepthExceeded", MaxDepth+1, err)
	}
	// And it survives a round-trip through the wire form the fuzzer feeds.
	var back cmatcherv3.MatchPredicate
	if err := proto.Unmarshal(mustMarshal(t, deep), &back); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if _, err := Compile(&back); !errors.Is(err, ErrDepthExceeded) {
		t.Errorf("after round-trip: err = %v, want ErrDepthExceeded", err)
	}
}

func FuzzMatchPredicateCompile(f *testing.F) {
	// Structured seeds.
	for _, mp := range []*cmatcherv3.MatchPredicate{
		anyMatch(),
		reqHdr("x-tap", "yes"),
		{Rule: &cmatcherv3.MatchPredicate_NotMatch{NotMatch: anyMatch()}},
		{Rule: &cmatcherv3.MatchPredicate_AndMatch{AndMatch: &cmatcherv3.MatchPredicate_MatchSet{
			Rules: []*cmatcherv3.MatchPredicate{anyMatch(), anyMatch()}}}},
		nest(MaxDepth),     // at the cap: must compile
		nest(MaxDepth + 1), // over the cap: must reject, must not overflow
		nest(512),          // far over: the cap must bound recursion long before the stack does
	} {
		b, err := proto.Marshal(mp)
		if err != nil {
			f.Fatalf("seed marshal: %v", err)
		}
		f.Add(b)
	}
	// Unstructured seeds.
	for _, b := range [][]byte{nil, {}, {0x00}, {0xff, 0xff, 0xff, 0xff}, []byte("not-a-proto")} {
		f.Add(b)
	}

	f.Fuzz(func(t *testing.T, b []byte) {
		var mp cmatcherv3.MatchPredicate
		if err := proto.Unmarshal(b, &mp); err != nil {
			return // not our concern: the filter parse layer rejects bad wire bytes
		}
		prog, err := Compile(&mp)
		if prog == nil && err == nil {
			t.Fatalf("Compile returned (nil, nil) for %x — must return exactly one", b)
		}
		if prog != nil && err != nil {
			t.Fatalf("Compile returned both Program and error for %x — exclusive", b)
		}
	})
}
