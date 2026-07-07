package accesslog

// otlpformat.go is envoy-go's OTLP access-log operator engine: a small two-phase
// substitution engine for the %…%-command-operator-templated `body`/`attributes`
// (and the substitution-free `resource_attributes`) of an OTLP access logger.
//
// Two phases:
//   - Compile-once (at bootstrap parse time): CompileOTLPTemplate compiles a
//     %OPERATOR%-templated string into ordered literal/operator segments;
//     CompileOTLPValue walks an AnyValue tree (string/kvlist/array) compiling each
//     string leaf; ValidateOTLPValue type-only-walks a resource_attributes tree
//     (no operator substitution — an operator string is a valid literal there).
//   - Substitute-per-record (at access-log emit time): (*OTLPTemplate).evalString
//     and (*OTLPValueTemplate).Eval substitute the compiled operators against a
//     per-request *Record, producing a fresh AnyValue.
//
// Curated strict-reject: only the Record-mapped operator set in otlpOperators is
// supported; any other operator token (unknown, or a valid-Envoy operator we don't
// map, or an arbitrary REQ/RESP header) is rejected at compile time — the
// envoy-go-strict mirror of the reference's own unknown-operator boot-reject. The
// value walk likewise strict-rejects any AnyValue type other than string/kvlist/array.
//
// No escaping: OTLP values are proto strings, so the extractors emit raw field values
// (format.go's escape() is file-sink-only and is NOT called here).

import (
	"fmt"
	"strconv"
	"strings"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

// otlpOperators maps a supported operator TOKEN (inner text between % delimiters, no %)
// to a func(*Record) string extractor. Empty-value dispositions reuse format.go's
// orDash (the same Envoy StreamInfoFormatter behavior).
var otlpOperators = map[string]func(*Record) string{
	"START_TIME":      func(r *Record) string { return r.StartTime.UTC().Format("2006-01-02T15:04:05.000Z") },
	"REQ(:METHOD)":    func(r *Record) string { return r.Method },
	"REQ(:PATH)":      func(r *Record) string { return orDash(r.Path) },
	"REQ(:AUTHORITY)": func(r *Record) string { return orDash(r.Authority) },
	"REQ(USER-AGENT)": func(r *Record) string { return orDash(r.UserAgent) },
	"PROTOCOL":        func(r *Record) string { return r.Protocol },
	"RESPONSE_CODE":   func(r *Record) string { return strconv.Itoa(r.ResponseCode) },
	"BYTES_SENT":      func(r *Record) string { return strconv.FormatInt(r.BytesSent, 10) },
	"DURATION":        func(r *Record) string { return strconv.FormatInt(int64(r.Duration/1e6), 10) },
	"UPSTREAM_HOST":   func(r *Record) string { return orDash(r.UpstreamHost) },
}

type otlpSegment struct {
	lit string
	op  func(*Record) string
}

// OTLPTemplate is a compiled %…%-operator string template. Exported so
// internal/bootstrap can name it in OTLPConfig fields (D-OTLP-2-COMPILE-SITE).
type OTLPTemplate struct{ segs []otlpSegment }

// CompileOTLPTemplate compiles a %OPERATOR%-templated string into ordered
// literal/operator segments, strict-rejecting any operator token outside the
// curated Record-mapped set (and an unterminated operator).
func CompileOTLPTemplate(s string) (*OTLPTemplate, error) {
	var segs []otlpSegment
	var lit strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '%' {
			lit.WriteByte(s[i])
			i++
			continue
		}
		if i+1 < len(s) && s[i+1] == '%' { // %% → literal %
			lit.WriteByte('%')
			i += 2
			continue
		}
		end := strings.IndexByte(s[i+1:], '%')
		if end < 0 {
			return nil, fmt.Errorf("unterminated operator at %q", s[i:])
		}
		token := s[i+1 : i+1+end]
		op, ok := otlpOperators[token]
		if !ok {
			return nil, fmt.Errorf("unsupported access-log operator %%%s%% (not in the curated Record-mapped set)", token)
		}
		if lit.Len() > 0 {
			segs = append(segs, otlpSegment{lit: lit.String()})
			lit.Reset()
		}
		segs = append(segs, otlpSegment{op: op})
		i = i + 1 + end + 1
	}
	if lit.Len() > 0 {
		segs = append(segs, otlpSegment{lit: lit.String()})
	}
	return &OTLPTemplate{segs: segs}, nil
}

func (t *OTLPTemplate) evalString(rec *Record) string {
	if len(t.segs) == 1 && t.segs[0].op == nil {
		return t.segs[0].lit
	}
	var b strings.Builder
	for _, sg := range t.segs {
		if sg.op != nil {
			b.WriteString(sg.op(rec))
		} else {
			b.WriteString(sg.lit)
		}
	}
	return b.String()
}

// otlpValueKind discriminates the compiled AnyValue arm. An EXPLICIT kind (NOT
// slice-non-nil-ness) is load-bearing: an empty kvlist_value/array_value compiles to a
// nil slice, so switching on `kvlist != nil` would misroute an empty collection to the
// string arm — a type-changing bug the cross-side differential would catch.
type otlpValueKind uint8

const (
	otlpValueString otlpValueKind = iota
	otlpValueKvlist
	otlpValueArray
)

// OTLPValueTemplate is a compiled AnyValue tree. Exported (D-OTLP-2-COMPILE-SITE).
type OTLPValueTemplate struct {
	kind   otlpValueKind
	str    *OTLPTemplate
	kvlist []OTLPAttrTemplate
	array  []*OTLPValueTemplate
}

// OTLPAttrTemplate is one ordered key→compiled-value pair. Exported.
type OTLPAttrTemplate struct {
	Key   string
	Value *OTLPValueTemplate
}

// CompileOTLPValue walks an AnyValue tree (string/kvlist/array), compiling each
// string leaf into an OTLPTemplate, and strict-rejects any other AnyValue type at
// any depth (int/bool/double/bytes).
func CompileOTLPValue(v *commonpb.AnyValue) (*OTLPValueTemplate, error) {
	if v == nil {
		return nil, nil
	}
	switch v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		t, err := CompileOTLPTemplate(v.GetStringValue())
		if err != nil {
			return nil, err
		}
		return &OTLPValueTemplate{kind: otlpValueString, str: t}, nil
	case *commonpb.AnyValue_KvlistValue:
		kvs := make([]OTLPAttrTemplate, 0, len(v.GetKvlistValue().GetValues()))
		for _, kv := range v.GetKvlistValue().GetValues() {
			child, err := CompileOTLPValue(kv.GetValue())
			if err != nil {
				return nil, err
			}
			kvs = append(kvs, OTLPAttrTemplate{Key: kv.GetKey(), Value: child})
		}
		return &OTLPValueTemplate{kind: otlpValueKvlist, kvlist: kvs}, nil
	case *commonpb.AnyValue_ArrayValue:
		arr := make([]*OTLPValueTemplate, 0, len(v.GetArrayValue().GetValues()))
		for _, elem := range v.GetArrayValue().GetValues() {
			child, err := CompileOTLPValue(elem)
			if err != nil {
				return nil, err
			}
			arr = append(arr, child)
		}
		return &OTLPValueTemplate{kind: otlpValueArray, array: arr}, nil
	default:
		return nil, fmt.Errorf("unsupported OTLP value type %T (only string, kvlist, array are supported)", v.GetValue())
	}
}

// ValidateOTLPValue type-only-walks a resource_attributes AnyValue tree, accepting
// string/kvlist/array (a string leaf is a valid literal here — NOT scanned for
// operators) and rejecting any other AnyValue type at any depth.
func ValidateOTLPValue(v *commonpb.AnyValue) error {
	if v == nil {
		return nil
	}
	switch v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return nil
	case *commonpb.AnyValue_KvlistValue:
		for _, kv := range v.GetKvlistValue().GetValues() {
			if err := ValidateOTLPValue(kv.GetValue()); err != nil {
				return err
			}
		}
		return nil
	case *commonpb.AnyValue_ArrayValue:
		for _, elem := range v.GetArrayValue().GetValues() {
			if err := ValidateOTLPValue(elem); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported OTLP value type %T (only string, kvlist, array are supported)", v.GetValue())
	}
}

// Eval substitutes the compiled operators against rec, producing a fresh AnyValue
// whose arm matches the compiled kind (string/kvlist/array — preserved even when empty).
func (t *OTLPValueTemplate) Eval(rec *Record) *commonpb.AnyValue {
	if t == nil {
		return nil
	}
	switch t.kind {
	case otlpValueKvlist:
		kvs := make([]*commonpb.KeyValue, len(t.kvlist))
		for i, at := range t.kvlist {
			kvs[i] = &commonpb.KeyValue{Key: at.Key, Value: at.Value.Eval(rec)}
		}
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{Values: kvs}}}
	case otlpValueArray:
		elems := make([]*commonpb.AnyValue, len(t.array))
		for i, vt := range t.array {
			elems[i] = vt.Eval(rec)
		}
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{Values: elems}}}
	default:
		return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: t.str.evalString(rec)}}
	}
}
