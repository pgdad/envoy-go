package tap

import (
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	datatapv3 "github.com/envoyproxy/go-control-plane/envoy/data/tap/v3"
)

// toHeaderValues renders a lowercase-keyed header map as sorted HeaderValues.
//
// Sorting by (key, value) is a DOCUMENTED departure from the reference's codec
// order; it is invisible to the differential, which compares SETS. A sort is
// required regardless: http.Header is a Go map with no stable iteration order.
//
// RawValue is left nil: protojson's EmitDefaultValues renders it as "" — which
// is exactly what the reference emits.
func toHeaderValues(h http.Header) []*corev3.HeaderValue {
	out := make([]*corev3.HeaderValue, 0, len(h))
	for k, vs := range h {
		for _, v := range vs {
			out = append(out, &corev3.HeaderValue{Key: k, Value: v})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GetKey() != out[j].GetKey() {
			return out[i].GetKey() < out[j].GetKey()
		}
		return out[i].GetValue() < out[j].GetValue()
	})
	return out
}

// OnDestroy is the ONE stream-end hook. It resolves the predicate tree and, on
// a match, assembles and emits the buffered trace.
//
// Emission is UNCONDITIONALLY at stream end. Never emit from DecodeHeaders or
// EncodeHeaders: the reference assembles the WHOLE stream and emits once, even
// when a request arm is already true at decode (AMEND-TAP-NOEARLYEMIT).
//
// This fires EXACTLY ONCE per stream: FilterChain.Destroy() is destroyOnce-
// guarded and its `if Decoder != nil {…} else if Encoder != nil {…}` loop
// reaches this value only through the Decoder branch. No guard is needed.
func (f *tapFilter) OnDestroy() {
	ev := f.cfg.prog.NewEvaluator()
	if f.sawReq {
		ev.FeedRequestHeaders(f.reqHdrs)
	}
	if f.sawResp {
		ev.FeedResponseHeaders(f.respHdrs)
	}
	if !ev.Resolve() {
		return // no match: no counter, no file
	}
	// rq_tapped counts the MATCH DECISION, not a successful write.
	if f.cfg.rqTapped != nil {
		f.cfg.rqTapped.Inc()
	}
	// A sink write failure is deliberately non-fatal and does not affect the
	// counter: the reference increments on the tap decision independently of
	// sink-write success.
	_ = f.cfg.sink.write(f.buildTrace())
}

func (f *tapFilter) buildTrace() *datatapv3.TraceWrapper {
	bt := &datatapv3.HttpBufferedTrace{}
	if f.sawReq {
		// Trailers stay nil: trailers are structurally invisible to envoy-go's
		// filters. EmitDefaultValues renders nil repeated Trailers as [].
		m := &datatapv3.HttpBufferedTrace_Message{Headers: toHeaderValues(f.reqHdrs)}
		if f.sawReqBody {
			m.Body = bodyProto(f.reqBody, f.reqTrunc, f.cfg.bodyAsString)
		}
		bt.Request = m
	}
	if f.sawResp {
		m := &datatapv3.HttpBufferedTrace_Message{Headers: toHeaderValues(f.respHdrs)}
		if f.sawRespBody {
			m.Body = bodyProto(f.respBody, f.respTrunc, f.cfg.bodyAsString)
		}
		bt.Response = m
	}
	if f.cfg.recordConn && f.decCB != nil {
		bt.DownstreamConnection = &datatapv3.Connection{
			LocalAddress:  addrProto(f.decCB.DownstreamLocalAddr()),
			RemoteAddress: addrProto(f.decCB.DownstreamRemoteAddr()),
		}
	}
	return &datatapv3.TraceWrapper{
		Trace: &datatapv3.TraceWrapper_HttpBufferedTrace{HttpBufferedTrace: bt},
	}
}

// bodyProto renders a captured body as a data/tap/v3.Body. Under AS_STRING the
// bytes are sanitized to valid UTF-8 (D-TAP-BODY-UTF8): Go's protojson.Marshal
// ERRORS on invalid UTF-8 in a proto3 string field and the sink swallows that
// error (trace.go OnDestroy), so a raw non-UTF8 body would silently drop the
// whole trace. strings.ToValidUTF8 is a documented DEPARTURE from the
// reference's lossy C++ mangling -- lossless-of-the-trace and deterministic.
// AS_BYTES base64-encodes arbitrary bytes faithfully (the proto default).
// EmitDefaultValues renders truncated unconditionally (AMEND-TAP-BODY2-TRUNCFLAG).
func bodyProto(buf []byte, trunc bool, asString bool) *datatapv3.Body {
	if asString {
		return &datatapv3.Body{
			BodyType:  &datatapv3.Body_AsString{AsString: strings.ToValidUTF8(string(buf), "�")},
			Truncated: trunc,
		}
	}
	return &datatapv3.Body{
		BodyType:  &datatapv3.Body_AsBytes{AsBytes: buf},
		Truncated: trunc,
	}
}

// addrProto renders a net.Addr as a core.Address{socket_address}. A nil or
// unsplittable address yields nil (the field is then omitted by protojson).
func addrProto(a net.Addr) *corev3.Address {
	if a == nil {
		return nil
	}
	host, portStr, err := net.SplitHostPort(a.String())
	if err != nil {
		return nil
	}
	p, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		return nil
	}
	return &corev3.Address{Address: &corev3.Address_SocketAddress{
		SocketAddress: &corev3.SocketAddress{
			Address:       host,
			PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: uint32(p)},
		},
	}}
}
