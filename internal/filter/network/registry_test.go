package network

import (
	"sync"
	"testing"

	"google.golang.org/protobuf/types/known/anypb"
)

func TestRegistryRegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	var f NetworkFilterFactory = func(*anypb.Any, FactoryCtx) (FilterInstanceFactory, error) { return nil, nil }
	r.Register("type.url/A", f)
	if _, ok := r.Lookup("type.url/A"); !ok {
		t.Errorf("registered factory not found")
	}
	if _, ok := r.Lookup("missing"); ok {
		t.Errorf("missing key returned ok=true")
	}
}

func TestRegistryDuplicateRegisterPanics(t *testing.T) {
	r := NewRegistry()
	var f NetworkFilterFactory = func(*anypb.Any, FactoryCtx) (FilterInstanceFactory, error) { return nil, nil }
	r.Register("dup", f)
	defer func() {
		if recover() == nil {
			t.Errorf("expected panic on duplicate register")
		}
	}()
	r.Register("dup", f)
}

func TestRegistryFreezeBlocksRegister(t *testing.T) {
	r := NewRegistry()
	r.Freeze()
	defer func() {
		if recover() == nil {
			t.Errorf("expected panic on post-freeze register")
		}
	}()
	r.Register("late", func(*anypb.Any, FactoryCtx) (FilterInstanceFactory, error) { return nil, nil })
}

func TestRegistryKnownTypeURLsSorted(t *testing.T) {
	r := NewRegistry()
	noop := func(*anypb.Any, FactoryCtx) (FilterInstanceFactory, error) { return nil, nil }
	r.Register("b", noop)
	r.Register("a", noop)
	r.Register("c", noop)
	got := r.KnownTypeURLs()
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("KnownTypeURLs not sorted: %v", got)
	}
}

func TestRegistryConcurrentLookup(t *testing.T) {
	r := NewRegistry()
	r.Register("x", func(*anypb.Any, FactoryCtx) (FilterInstanceFactory, error) { return nil, nil })
	r.Freeze()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r.Lookup("x") }()
	}
	wg.Wait()
}
