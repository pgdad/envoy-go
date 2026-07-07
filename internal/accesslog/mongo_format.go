package accesslog

import (
	"bytes"
	"strconv"
	"time"
)

// MongoRecord is the mongo_proxy access-log record (parent §11.8; 29.3). One JSON
// line per decoded message in BOTH directions: {"time","message","upstream_host"}.
// The time field is per-message wall clock → timing-bearing → the access log is
// differential-INVISIBLE (AMEND-B10): the proof is unit goldens (time by shape) +
// a BEHAVIOR_CONTRACT coverage boundary, NO fixture dir.
type MongoRecord struct {
	Time         time.Time
	Message      string // message.toString() per opcode (full=true requests / full=false replies)
	UpstreamHost string // "addr" or "-" (Envoy missing-value convention)
}

// MongoFormat renders a MongoRecord as one JSON line (newline-terminated),
// mirroring upstream AccessLog::logMessage (proxy.cc:37-57). rec MUST be a
// *MongoRecord (the sink is constructed with this formatter only).
func MongoFormat(rec any) []byte {
	r := rec.(*MongoRecord)
	var b bytes.Buffer
	b.Grow(128 + len(r.Message))
	b.WriteString(`{"time":`)
	b.WriteString(strconv.Quote(r.Time.UTC().Format("2006-01-02T15:04:05.000Z")))
	b.WriteString(`,"message":`)
	b.WriteString(strconv.Quote(r.Message))
	b.WriteString(`,"upstream_host":`)
	b.WriteString(strconv.Quote(orDash(r.UpstreamHost)))
	b.WriteString("}\n")
	return b.Bytes()
}
