package accesslog

import (
	"testing"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

func FuzzCompileOTLPValue(f *testing.F) {
	f.Add("")                               // empty
	f.Add("%REQ(:METHOD)% %RESPONSE_CODE%") // valid operators
	f.Add("100%% literal")                  // %% literal
	f.Add("%FOOBAR%")                       // unknown operator
	f.Add("%REQ(:METHOD)")                  // unterminated
	f.Fuzz(func(t *testing.T, s string) {
		// the scanner directly
		_, _ = CompileOTLPTemplate(s) // must not panic
		// the tree walker over a string_value leaf carrying the fuzz string
		v := &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: s}}
		_, _ = CompileOTLPValue(v) // must not panic
		_ = ValidateOTLPValue(v)   // must not panic
	})
}
