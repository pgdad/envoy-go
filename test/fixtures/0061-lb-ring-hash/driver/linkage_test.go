package driver

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// collapseFixtureKSourcePath is the file declaring internal/cluster's
// collapseFixtureK. It is relative to THIS package's directory
// (test/fixtures/0061-lb-ring-hash/driver — `go test` runs each test binary with
// its own package dir as the working directory); four levels up is the repo root.
const collapseFixtureKSourcePath = "../../../../internal/cluster/ringhash_test.go"

// TestSourceIPsLinkedToCollapseFixtureK is the phase-76 LINKAGE gate in pure Go.
//
// internal/cluster's TestRingHash_EphemeralPortRing_KeyCollapseRate measures the
// 0061 ring-collapse probability at K = collapseFixtureK; that measurement is only
// about THIS fixture if collapseFixtureK equals sourceIPs. Nothing in the Go build
// links them: internal/cluster cannot import a test fixture (and must not), both
// constants are unexported, and a const in a _test.go file is invisible even to its
// own package's non-test build. So the link is recovered by PARSING the other file's
// SOURCE with go/parser — which requires NO new exported symbol on either side, and
// which (unlike a grep) cannot be spoofed by prose: the parser sees declarations,
// not comments, so the other file's own doc comment "K=collapseFixtureK=16" is
// invisible here.
func TestSourceIPsLinkedToCollapseFixtureK(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, collapseFixtureKSourcePath, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", collapseFixtureKSourcePath, err)
	}

	found := 0
	got := -1
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if name.Name != "collapseFixtureK" || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.INT {
					t.Fatalf("%s: collapseFixtureK is not an integer literal (%T)",
						collapseFixtureKSourcePath, vs.Values[i])
				}
				v, err := strconv.Atoi(lit.Value)
				if err != nil {
					t.Fatalf("%s: collapseFixtureK = %q: %v", collapseFixtureKSourcePath, lit.Value, err)
				}
				found++
				got = v
			}
		}
	}

	if found != 1 {
		t.Fatalf("%s: found %d const declarations of collapseFixtureK, want exactly 1 "+
			"(the gate cannot resolve an ambiguous or missing declaration)",
			collapseFixtureKSourcePath, found)
	}
	if got != sourceIPs {
		t.Errorf("DESYNC: this fixture drives sourceIPs=%d distinct ring_hash keys, but "+
			"internal/cluster's collapse-rate test pins collapseFixtureK=%d (%s). The "+
			"MEASURED leg of TestRingHash_EphemeralPortRing_KeyCollapseRate is therefore "+
			"reporting the collapse probability of a DIFFERENT fixture than this one. "+
			"Change both, or neither.",
			sourceIPs, got, collapseFixtureKSourcePath)
	}
}
