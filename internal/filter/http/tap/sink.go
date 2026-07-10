package tap

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	datatapv3 "github.com/envoyproxy/go-control-plane/envoy/data/tap/v3"
	"google.golang.org/protobuf/encoding/protojson"
)

// marshalOpts is BYTE-EXACT against the reference's file_per_tap JSON.
// Measured at SPEC time: a reference trace round-tripped through these options
// reproduces the file byte-for-byte (modulo the trailing newline we append).
//
// EmitUnpopulated is WRONG: it emits "body": null / "headers_received_time":
// null / "downstream_connection": null, which the reference never does.
// EmitDefaultValues renders "raw_value": "" and "trailers": [] while OMITTING
// nil message fields — exactly the reference.
//
// NOTE: protojson applies internal/detrand whitespace randomization seeded per
// binary, so the exact bytes vary between build configs (plain vs -race). This
// is cosmetic (JSON whitespace); consumers parse the document. The byte-exact
// golden test compares canonicalized JSON for this reason. See PROGRESS-56.1.
var marshalOpts = protojson.MarshalOptions{
	Multiline:         true,
	Indent:            " ",
	UseProtoNames:     true,
	EmitDefaultValues: true,
}

// traceIDs is the process-local monotonic trace-id source (D-TAP-TRACEID). The
// id appears only in the filename and is NEVER asserted cross-side, so a
// counter suffices; crypto/rand would add an error path for no benefit.
var traceIDs atomic.Uint64

// filePerTapSink writes ONE protojson document to a NEW file per emitted trace,
// named <path_prefix>_<trace_id>.json.
//
// internal/accesslog.AsyncFileSink is a SHAPE precedent only: it opens ONE
// append-only file for the process lifetime, whereas file_per_tap opens a
// DISCRETE FILE PER STREAM.
type filePerTapSink struct{ pathPrefix string }

// newFilePerTapSink creates the path_prefix's parent directory (D-TAP-PATHPREFIX).
// A path whose parent cannot be created is a CONFIG error and fails at parse
// time, rather than silently dropping every trace at stream end. This is a
// documented DEPARTURE: the reference's behavior here was not probed.
func newFilePerTapSink(pathPrefix string) (*filePerTapSink, error) {
	if err := os.MkdirAll(filepath.Dir(pathPrefix), 0o755); err != nil {
		return nil, fmt.Errorf("file_per_tap: create path_prefix parent: %w", err)
	}
	return &filePerTapSink{pathPrefix: pathPrefix}, nil
}

func (s *filePerTapSink) write(tw *datatapv3.TraceWrapper) error {
	b, err := marshalOpts.Marshal(tw)
	if err != nil {
		return fmt.Errorf("file_per_tap: marshal: %w", err)
	}
	b = append(b, '\n')

	name := fmt.Sprintf("%s_%d.json", s.pathPrefix, traceIDs.Add(1))
	// O_EXCL: a trace-id collision must surface, never silently overwrite.
	f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("file_per_tap: open %s: %w", name, err)
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return fmt.Errorf("file_per_tap: write %s: %w", name, err)
	}
	return f.Close()
}
