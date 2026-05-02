// Package envoygotestpb provides the hand-rolled minimal proto schema for the
// envoy.filters.http.envoy_go_test filter (Phase 07.1 Task 19). The schema is
// envoy-go-only — NOT registered in upstream envoyproxy/go-control-plane, NOT
// emitted from a `.proto` source via protoc. The descriptor is built at
// package-init time from a hand-built `descriptorpb.FileDescriptorProto`,
// projected into a `protoreflect.FileDescriptor` via `protodesc.NewFile`. The
// concrete message types (EnvoyGoTest + EnvoyGoTestPerRoute) are typed
// wrappers over `*dynamicpb.Message`; this gives a real `proto.Message` shape
// (`anypb.UnmarshalTo` / `anypb.UnmarshalNew` round-trip cleanly through
// `BuildPerRouteConfig`) with hand-rolled control over the Go API surface
// (typed Getters / Setters) and zero `protoc` toolchain dependency.
//
// Two messages:
//
//   - EnvoyGoTest (filter-level config):
//     1. mode_default string — default x-envoy-go-test-mode if header absent.
//
//   - EnvoyGoTestPerRoute (per-route config carried via typed_per_filter_config):
//     1. count int32 — echoed into x-envoy-go-test-route-count: <N> response
//        header on encodeHeaders.
//
// TypeURL conventions:
//
//   - "type.googleapis.com/envoy.filters.http.envoy_go_test.v0.EnvoyGoTest"
//   - "type.googleapis.com/envoy.filters.http.envoy_go_test.v0.EnvoyGoTestPerRoute"
//
// The envoy_go_test.v0 namespace is a deliberate envoy-go-only package
// scope; the v0 indicates "schema-evolves-freely-pre-stabilization" per
// internal pre-1.0 convention. Boot wiring (Task 20) registers
// `envoygotest.New` under the EnvoyGoTest type URL in the HTTPRegistry.
package envoygotestpb

import (
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/runtime/protoimpl"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Type URLs for both messages. Used by callers when constructing anypb.Any
// values (tests, fixture bootstraps) and by Boot wiring's HTTPRegistry.
const (
	TypeURLEnvoyGoTest         = "type.googleapis.com/envoy.filters.http.envoy_go_test.v0.EnvoyGoTest"
	TypeURLEnvoyGoTestPerRoute = "type.googleapis.com/envoy.filters.http.envoy_go_test.v0.EnvoyGoTestPerRoute"
)

// fileDesc holds the package-init-built FileDescriptor for the two messages.
// Lazy-built once via fileDescOnce to avoid the descriptor-build cost on
// import in tests that do not exercise the filter.
var (
	fileDesc           protoreflect.FileDescriptor
	envoyGoTestDesc    protoreflect.MessageDescriptor
	perRouteDesc       protoreflect.MessageDescriptor
	descOnce           sync.Once
	descBuildErr       error
)

// init eagerly builds the descriptor + registers the two messages in the
// global proto registry so anypb.UnmarshalNew (used by BuildPerRouteConfig
// and by other call sites that round-trip envoygotest typed_config) resolves
// our type URLs without requiring callers to import the proto subpackage
// explicitly. The blank-import side-effect mirrors protoc-gen-go's emitted
// init() pattern.
func init() {
	initDesc()
}

// initDesc constructs the FileDescriptor + MessageDescriptors from a
// hand-built descriptorpb.FileDescriptorProto. Idempotent via descOnce.
func initDesc() {
	descOnce.Do(func() {
		// Hand-build the FileDescriptorProto for the schema:
		//
		//   syntax = "proto3";
		//   package envoy.filters.http.envoy_go_test.v0;
		//   message EnvoyGoTest        { string mode_default = 1; }
		//   message EnvoyGoTestPerRoute { int32  count        = 1; }
		//
		// Field types from descriptorpb's TYPE_STRING + TYPE_INT32 enums; field
		// labels default to LABEL_OPTIONAL (proto3 = no required/repeated for
		// these fields). JSON names are protoc's lowerCamelCase convention
		// applied manually here.
		var (
			syntax    = "proto3"
			pkg       = "envoy.filters.http.envoy_go_test.v0"
			fname     = "envoy/filters/http/envoy_go_test/v0/envoy_go_test.proto"
			msg1Name  = "EnvoyGoTest"
			msg2Name  = "EnvoyGoTestPerRoute"
			f1Name    = "mode_default"
			f1JSON    = "modeDefault"
			f1Number  = int32(1)
			f1Type    = descriptorpb.FieldDescriptorProto_TYPE_STRING
			f1Label   = descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
			f2Name    = "count"
			f2JSON    = "count"
			f2Number  = int32(1)
			f2Type    = descriptorpb.FieldDescriptorProto_TYPE_INT32
			f2Label   = descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
		)

		fdp := &descriptorpb.FileDescriptorProto{
			Name:    proto.String(fname),
			Package: proto.String(pkg),
			Syntax:  proto.String(syntax),
			MessageType: []*descriptorpb.DescriptorProto{
				{
					Name: proto.String(msg1Name),
					Field: []*descriptorpb.FieldDescriptorProto{
						{
							Name:     proto.String(f1Name),
							JsonName: proto.String(f1JSON),
							Number:   proto.Int32(f1Number),
							Type:     &f1Type,
							Label:    &f1Label,
						},
					},
				},
				{
					Name: proto.String(msg2Name),
					Field: []*descriptorpb.FieldDescriptorProto{
						{
							Name:     proto.String(f2Name),
							JsonName: proto.String(f2JSON),
							Number:   proto.Int32(f2Number),
							Type:     &f2Type,
							Label:    &f2Label,
						},
					},
				},
			},
		}
		fd, err := protodesc.NewFile(fdp, nil)
		if err != nil {
			descBuildErr = fmt.Errorf("envoygotestpb: build file descriptor: %w", err)
			return
		}
		fileDesc = fd
		envoyGoTestDesc = fd.Messages().ByName(protoreflect.Name(msg1Name))
		perRouteDesc = fd.Messages().ByName(protoreflect.Name(msg2Name))
		if envoyGoTestDesc == nil || perRouteDesc == nil {
			descBuildErr = fmt.Errorf("envoygotestpb: message descriptor lookup failed (got %v / %v)", envoyGoTestDesc, perRouteDesc)
			return
		}

		// Register the two MessageTypes in the global proto registry so
		// anypb.Any.UnmarshalNew can resolve our type URLs to a concrete
		// proto.Message via the registry's NewFunc. Without this,
		// BuildPerRouteConfig's a.UnmarshalNew() call returns
		// "proto: not found" for our hand-rolled messages.
		//
		// The NewFunc returns *dynamicpb.Message bound to the descriptor;
		// callers receive a generic dynamic message which works with the
		// protoreflect-based countViaReflect helper in the filter code.
		// (Tests that want the typed wrapper call NewEnvoyGoTest /
		// NewEnvoyGoTestPerRoute directly.)
		mt1 := dynamicpb.NewMessageType(envoyGoTestDesc)
		mt2 := dynamicpb.NewMessageType(perRouteDesc)
		if err := protoregistry.GlobalTypes.RegisterMessage(mt1); err != nil {
			descBuildErr = fmt.Errorf("envoygotestpb: register EnvoyGoTest in global types: %w", err)
			return
		}
		if err := protoregistry.GlobalTypes.RegisterMessage(mt2); err != nil {
			descBuildErr = fmt.Errorf("envoygotestpb: register EnvoyGoTestPerRoute in global types: %w", err)
			return
		}
	})
}

// EnvoyGoTest is the filter-level config message. Single field: mode_default
// — the default x-envoy-go-test-mode value if the per-request header is
// absent. Embeds a *dynamicpb.Message backed by the hand-rolled descriptor;
// satisfies proto.Message via the embedded interface methods.
type EnvoyGoTest struct {
	*dynamicpb.Message
}

// NewEnvoyGoTest allocates a fresh, zero-valued EnvoyGoTest message. Returns
// an error if the descriptor build at package-init time failed (impossible
// in practice — the descriptor is constant across runs).
func NewEnvoyGoTest() (*EnvoyGoTest, error) {
	initDesc()
	if descBuildErr != nil {
		return nil, descBuildErr
	}
	return &EnvoyGoTest{Message: dynamicpb.NewMessage(envoyGoTestDesc)}, nil
}

// SetModeDefault sets the mode_default field. Mirrors protoc-gen-go's
// pointer-receiver field setter convention.
func (m *EnvoyGoTest) SetModeDefault(v string) {
	if m == nil || m.Message == nil {
		return
	}
	fd := m.Descriptor().Fields().ByName("mode_default")
	if fd == nil {
		return
	}
	m.Message.Set(fd, protoreflect.ValueOfString(v))
}

// GetModeDefault returns the mode_default field value, or "" if unset.
func (m *EnvoyGoTest) GetModeDefault() string {
	if m == nil || m.Message == nil {
		return ""
	}
	fd := m.Descriptor().Fields().ByName("mode_default")
	if fd == nil {
		return ""
	}
	return m.Message.Get(fd).String()
}

// EnvoyGoTestPerRoute is the per-route config message carried via
// typed_per_filter_config. Single field: count — echoed into the
// x-envoy-go-test-route-count: <N> response header on encodeHeaders.
type EnvoyGoTestPerRoute struct {
	*dynamicpb.Message
}

// NewEnvoyGoTestPerRoute allocates a fresh, zero-valued EnvoyGoTestPerRoute.
func NewEnvoyGoTestPerRoute() (*EnvoyGoTestPerRoute, error) {
	initDesc()
	if descBuildErr != nil {
		return nil, descBuildErr
	}
	return &EnvoyGoTestPerRoute{Message: dynamicpb.NewMessage(perRouteDesc)}, nil
}

// SetCount sets the count field.
func (m *EnvoyGoTestPerRoute) SetCount(v int32) {
	if m == nil || m.Message == nil {
		return
	}
	fd := m.Descriptor().Fields().ByName("count")
	if fd == nil {
		return
	}
	m.Message.Set(fd, protoreflect.ValueOfInt32(v))
}

// GetCount returns the count field value, or 0 if unset.
func (m *EnvoyGoTestPerRoute) GetCount() int32 {
	if m == nil || m.Message == nil {
		return 0
	}
	fd := m.Descriptor().Fields().ByName("count")
	if fd == nil {
		return 0
	}
	return int32(m.Message.Get(fd).Int())
}

// EnvoyGoTestDescriptor returns the hand-rolled MessageDescriptor for
// EnvoyGoTest. Exported for boot-time registration if needed; tests may use
// it to construct dynamic messages directly bypassing the typed wrapper.
func EnvoyGoTestDescriptor() protoreflect.MessageDescriptor {
	initDesc()
	return envoyGoTestDesc
}

// EnvoyGoTestPerRouteDescriptor returns the per-route MessageDescriptor.
func EnvoyGoTestPerRouteDescriptor() protoreflect.MessageDescriptor {
	initDesc()
	return perRouteDesc
}

// Compile-time assertions: both wrappers satisfy proto.Message via the
// embedded *dynamicpb.Message. protoimpl is referenced to keep the import
// for future use (e.g. version-enforce on toolchain compatibility); the
// assertion below ties it in.
var (
	_ proto.Message = (*EnvoyGoTest)(nil)
	_ proto.Message = (*EnvoyGoTestPerRoute)(nil)
)

// _ keeps protoimpl present in the import set as a future hook (e.g.
// EnforceVersion calls when a stable schema is ready).
var _ = protoimpl.MaxVersion
