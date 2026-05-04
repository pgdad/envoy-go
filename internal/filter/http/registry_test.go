package http

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func dummyFactory(*anypb.Any, FactoryCtx) (FilterInstanceFactory, error) {
	return func() HTTPFilter { return HTTPFilter{} }, nil
}

func TestRegistry_RegisterLookup(t *testing.T) {
	reg := NewHTTPRegistry()
	reg.Register("type.googleapis.com/foo.Filter", dummyFactory)
	if _, ok := reg.Lookup("type.googleapis.com/foo.Filter"); !ok {
		t.Fatalf("expected Lookup to find registered type_url")
	}
	if _, ok := reg.Lookup("type.googleapis.com/missing"); ok {
		t.Fatalf("expected Lookup to miss unknown type_url")
	}
}

func TestRegistry_DuplicateRegisterPanics(t *testing.T) {
	reg := NewHTTPRegistry()
	reg.Register("type.googleapis.com/dup", dummyFactory)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic on duplicate Register")
		}
		if !strings.Contains(r.(string), "duplicate") {
			t.Fatalf("expected panic message to mention 'duplicate'; got %q", r)
		}
	}()
	reg.Register("type.googleapis.com/dup", dummyFactory)
}

func TestRegistry_PostFreezeRegisterPanics(t *testing.T) {
	reg := NewHTTPRegistry()
	reg.Register("type.googleapis.com/foo", dummyFactory)
	reg.Freeze()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic on post-Freeze Register")
		}
		if !strings.Contains(r.(string), "frozen") || !strings.Contains(r.(string), "post-boot") {
			t.Fatalf("expected panic message mentioning 'frozen' + 'post-boot'; got %q", r)
		}
	}()
	reg.Register("type.googleapis.com/late", dummyFactory)
}

func TestRegistry_FreezeIdempotent(t *testing.T) {
	reg := NewHTTPRegistry()
	reg.Freeze()
	reg.Freeze() // should not panic
}

func TestRegistry_LookupAfterFreezeOK(t *testing.T) {
	reg := NewHTTPRegistry()
	reg.Register("type.googleapis.com/foo", dummyFactory)
	reg.Freeze()
	if _, ok := reg.Lookup("type.googleapis.com/foo"); !ok {
		t.Fatalf("Lookup must be allowed post-Freeze")
	}
}

func TestRegistry_RegisterPerRouteValidator_BeforeFreezeSucceeds(t *testing.T) {
	r := NewHTTPRegistry()
	called := 0
	r.RegisterPerRouteValidator("envoy.filters.http.header_mutation", func(m proto.Message) error {
		called++
		return nil
	})
	r.Freeze()
	v := r.PerRouteValidator("envoy.filters.http.header_mutation")
	if v == nil {
		t.Fatal("validator not retrievable")
	}
	if err := v(nil); err != nil {
		t.Errorf("validator should succeed; got %v", err)
	}
	if called != 1 {
		t.Errorf("validator should have been called once; got %d", called)
	}
}

func TestRegistry_RegisterPerRouteValidator_AfterFreezePanics(t *testing.T) {
	r := NewHTTPRegistry()
	r.Freeze()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on register-after-freeze")
		}
	}()
	r.RegisterPerRouteValidator("envoy.filters.http.header_mutation", func(m proto.Message) error { return nil })
}

func TestRegistry_PerRouteValidator_LookupNotRegisteredReturnsNil(t *testing.T) {
	r := NewHTTPRegistry()
	r.Freeze()
	if v := r.PerRouteValidator("envoy.filters.http.header_mutation"); v != nil {
		t.Errorf("unregistered validator should return nil; got non-nil func")
	}
}

func TestRegistry_PerRouteValidator_DoesNotConflictWithRegister(t *testing.T) {
	r := NewHTTPRegistry()
	r.Register("type.googleapis.com/envoy.filters.http.header_mutation", func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error) {
		return nil, errors.New("test factory")
	})
	r.RegisterPerRouteValidator("envoy.filters.http.header_mutation", func(m proto.Message) error { return nil })
	r.Freeze()
	if _, ok := r.Lookup("type.googleapis.com/envoy.filters.http.header_mutation"); !ok {
		t.Error("Register and RegisterPerRouteValidator should be independent")
	}
	if r.PerRouteValidator("envoy.filters.http.header_mutation") == nil {
		t.Error("PerRouteValidator should be retrievable")
	}
}

func TestRegistry_ConcurrentLookup_RaceClean(t *testing.T) {
	reg := NewHTTPRegistry()
	for i := 0; i < 16; i++ {
		reg.Register("type.googleapis.com/f"+string(rune('a'+i)), dummyFactory)
	}
	reg.Freeze()
	var wg sync.WaitGroup
	for g := 0; g < 64; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_, _ = reg.Lookup("type.googleapis.com/fa")
			}
		}()
	}
	wg.Wait()
}
