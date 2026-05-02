package listenerfilter

import (
	"sync"
	"testing"

	"google.golang.org/protobuf/types/known/anypb"
)

func dummyFactory(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error) {
	return func() ListenerFilter { return nil }, nil
}

func TestRegistryRegisterAndLookup(t *testing.T) {
	r := NewListenerFilterRegistry()
	r.Register("type.googleapis.com/foo", dummyFactory)
	got, ok := r.Lookup("type.googleapis.com/foo")
	if !ok {
		t.Errorf("Lookup(\"foo\") returned ok=false; want true")
	}
	if got == nil {
		t.Errorf("Lookup(\"foo\") returned nil factory")
	}
	_, missing := r.Lookup("type.googleapis.com/bar")
	if missing {
		t.Errorf("Lookup(\"bar\") returned ok=true on absent registration")
	}
}

func TestRegistryDuplicateRegisterPanics(t *testing.T) {
	r := NewListenerFilterRegistry()
	r.Register("type.googleapis.com/foo", dummyFactory)
	defer func() {
		recv := recover()
		if recv == nil {
			t.Errorf("expected panic on duplicate register; got none")
		}
	}()
	r.Register("type.googleapis.com/foo", dummyFactory)
}

func TestRegistryFreezeBlocksRegister(t *testing.T) {
	r := NewListenerFilterRegistry()
	r.Register("type.googleapis.com/foo", dummyFactory)
	r.Freeze()
	defer func() {
		recv := recover()
		if recv == nil {
			t.Errorf("expected panic on post-freeze register; got none")
		}
	}()
	r.Register("type.googleapis.com/bar", dummyFactory)
}

func TestRegistryFreezeIsIdempotent(t *testing.T) {
	r := NewListenerFilterRegistry()
	r.Freeze()
	r.Freeze() // must not panic
	r.Freeze()
}

func TestRegistryConcurrentLookup(t *testing.T) {
	r := NewListenerFilterRegistry()
	r.Register("type.googleapis.com/foo", dummyFactory)
	r.Freeze()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = r.Lookup("type.googleapis.com/foo")
		}()
	}
	wg.Wait()
}
