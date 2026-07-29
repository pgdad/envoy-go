package stats

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------------
// Why this file exists, and why the byte-stable guard was not enough.
//
// TestExtractTagsTerminalError_ByteStable pins the terminal rejection message
// against silent DRIFT. It cannot detect that the message is WRONG, because its
// golden is hand-written: if the constant and the golden carry the same wrong
// number, both legs pass. That is exactly what happened while this row was being
// built. The three byte-mirror root arms landed first; the terminal message was
// rewritten second and enumerated NINE top-level segments when twelve were live;
// the byte-stable guard was written third against the nine-item text and went
// green. A prose sweep then copied "nine" into four fixture files.
//
// The same shape appears in TestHelpText_KeySetExact: a key typo copied into the
// golden passes a golden-comparison guard. A guard whose expectation is authored
// by hand shares the author's mistake.
//
// So this guard derives BOTH sides from the code:
//
//	Leg 1 extracts the top-level detectors from ExtractTags' own AST and asserts
//	      the count AND the set the message claims match it. Catches a root added
//	      without updating the message -- the defect that produced this file.
//	Leg 2 drives every root the message NAMES through ExtractTags and asserts it
//	      is accepted. Catches a phantom entry, which set comparison against the
//	      same AST cannot: a root named in both places but unreachable at runtime
//	      passes leg 1 and fails leg 2.
//	Leg 3 does the AST set comparison for the four mid-name segments. It is NOT
//	      an acceptance probe -- see the comment on that test for why one cannot
//	      work here.
//	Leg 4 asserts the two species stay distinct: a mid-name segment must NOT
//	      parse root-anchored. That is the invariant the message's two-clause
//	      shape exists to state, and summing the species is the standing
//	      documentation error.
//
// Legs 1 and 2 fail on opposite defects and neither subsumes the other.
// -----------------------------------------------------------------------------

// claimedRe extracts the "(want one of the N: a.|b.|c.)" clauses in order.
var claimedRe = regexp.MustCompile(`\(want one of the (\d+): ([^)]*)\)`)

// parseClaims pulls the (count, members) pairs out of the terminal message.
func parseClaims(t *testing.T, format string) [][2]any {
	t.Helper()
	ms := claimedRe.FindAllStringSubmatch(format, -1)
	if len(ms) != 2 {
		t.Fatalf("terminal message shape changed: want exactly 2 %q clauses, got %d.\n  msg: %s",
			"(want one of the N: ...)", len(ms), format)
	}
	out := make([][2]any, 0, 2)
	for _, m := range ms {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("unparsable count %q in terminal message", m[1])
		}
		members := strings.Split(m[2], "|")
		out = append(out, [2]any{n, members})
	}
	return out
}

// topLevelDetectorsFromSource counts strings.HasPrefix(internal, ...) and
// strings.CutPrefix(internal, ...) calls inside ExtractTags, and returns the
// literal prefixes. Deriving from the AST rather than a grep means a detector
// written across two lines, or one inside a comment, cannot skew the count.
func topLevelDetectorsFromSource(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "name.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing name.go: %v", err)
	}
	var fn *ast.FuncDecl
	for _, d := range f.Decls {
		if g, ok := d.(*ast.FuncDecl); ok && g.Name.Name == "ExtractTags" {
			fn = g
			break
		}
	}
	if fn == nil {
		t.Fatalf("ExtractTags not found in name.go -- this guard is looking at the wrong symbol")
	}
	var got []string
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "strings" {
			return true
		}
		if sel.Sel.Name != "HasPrefix" && sel.Sel.Name != "CutPrefix" {
			return true
		}
		arg0, ok := call.Args[0].(*ast.Ident)
		if !ok || arg0.Name != "internal" {
			return true
		}
		lit, ok := call.Args[1].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if s, err := strconv.Unquote(lit.Value); err == nil {
			got = append(got, s)
		}
		return true
	})
	return got
}

// midNameDetectorsFromSource returns the segment literals passed to
// strings.Index(internal, ...) inside ExtractTags. They are declared as local
// consts, so the literal is resolved from the enclosing function's const decls
// rather than read off the call site.
func midNameDetectorsFromSource(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "name.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing name.go: %v", err)
	}
	var fn *ast.FuncDecl
	for _, d := range f.Decls {
		if g, ok := d.(*ast.FuncDecl); ok && g.Name.Name == "ExtractTags" {
			fn = g
			break
		}
	}
	if fn == nil {
		t.Fatalf("ExtractTags not found in name.go -- this guard is looking at the wrong symbol")
	}

	// Collect const ident -> string literal for consts declared inside the body.
	consts := map[string]string{}
	ast.Inspect(fn, func(n ast.Node) bool {
		gd, ok := n.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			return true
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if s, err := strconv.Unquote(lit.Value); err == nil {
						consts[name.Name] = s
					}
				}
			}
		}
		return true
	})

	var got []string
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "strings" || sel.Sel.Name != "Index" {
			return true
		}
		arg0, ok := call.Args[0].(*ast.Ident)
		if !ok || arg0.Name != "internal" {
			return true
		}
		switch a := call.Args[1].(type) {
		case *ast.Ident:
			if s, ok := consts[a.Name]; ok {
				got = append(got, s)
			} else {
				t.Errorf("strings.Index(internal, %s): segment literal not resolvable; this guard "+
					"cannot see it and would silently under-count", a.Name)
			}
		case *ast.BasicLit:
			if s, err := strconv.Unquote(a.Value); err == nil {
				got = append(got, s)
			}
		}
		return true
	})
	return got
}

func TestTerminalError_TopLevelCountMatchesCode(t *testing.T) {
	claims := parseClaims(t, noRecognizedSegmentErrFmt)
	claimedN := claims[0][0].(int)
	claimedRoots := claims[0][1].([]string)

	derived := topLevelDetectorsFromSource(t)

	// Leg 1 -- the claimed COUNT must equal the number of detectors in the code.
	if claimedN != len(derived) {
		t.Errorf("terminal message claims %d top-level segments; ExtractTags has %d.\n"+
			"  claimed: %v\n  in code: %v\n"+
			"  a root was added without updating the message (or vice versa)",
			claimedN, len(derived), claimedRoots, derived)
	}

	// The claimed COUNT must also match the list it introduces -- otherwise the
	// number and the enumeration disagree with each other.
	if claimedN != len(claimedRoots) {
		t.Errorf("terminal message says %d but lists %d: %v", claimedN, len(claimedRoots), claimedRoots)
	}

	// Set equality, reported as missing/extra separately -- never as a count,
	// which would pass a message with two roots simultaneously wrong.
	inCode := make(map[string]bool, len(derived))
	for _, r := range derived {
		inCode[r] = true
	}
	claimed := make(map[string]bool, len(claimedRoots))
	for _, r := range claimedRoots {
		claimed[r] = true
	}
	var missing, extra []string
	for r := range inCode {
		if !claimed[r] {
			missing = append(missing, r)
		}
	}
	for r := range claimed {
		if !inCode[r] {
			extra = append(extra, r)
		}
	}
	if len(missing) > 0 {
		t.Errorf("roots ExtractTags accepts but the message does not name: %v", missing)
	}
	if len(extra) > 0 {
		t.Errorf("roots the message names but ExtractTags does not accept: %v", extra)
	}
}

func TestTerminalError_NamedRootsAreAccepted(t *testing.T) {
	claims := parseClaims(t, noRecognizedSegmentErrFmt)

	// Leg 2 -- every root the message NAMES must actually be accepted. Counting
	// alone cannot catch a phantom entry.
	//
	// The probe uses TWO trailing segments, not one. The label-extracting arms
	// (SN1 cluster., SN2 http., SN3 listener.) consume the first segment as a tag
	// VALUE and require a non-empty <rest> after it, so a single-segment probe is
	// rejected for a reason that has nothing to do with the claim under test --
	// it would report every one of those roots as a phantom. A probe's input is
	// itself a claim; this one is chosen to satisfy every arm shape at once.
	for _, root := range claims[0][1].([]string) {
		name := root + "probe_scope.probe_leaf"
		if _, _, err := ExtractTags(name); err != nil {
			t.Errorf("message names %q as a recognized top-level segment, but ExtractTags(%q) rejected it: %v",
				root, name, err)
		}
	}

}

func TestTerminalError_MidNameSegmentsMatchCode(t *testing.T) {
	claims := parseClaims(t, noRecognizedSegmentErrFmt)
	claimedN := claims[1][0].(int)
	claimedSegs := claims[1][1].([]string)

	derived := midNameDetectorsFromSource(t)

	// Leg 3 -- set equality against the code, NOT an acceptance probe.
	//
	// An acceptance probe cannot work here and the reason is worth recording.
	// Each infix arm gates on a per-segment ALLOW-LIST of leaf counter names
	// (".rbac." requires a .allowed/.denied/.shadow_allowed/.shadow_denied
	// suffix; ".http_local_rate_limit." requires enabled/ok/rate_limited/
	// enforced; and so on). So a generic probe leaf is rejected by every arm,
	// and a passing probe would only prove the leaf names were guessed right --
	// it would couple this guard to four separate allow-lists that have nothing
	// to do with the claim under test. The claim is about which SEGMENTS are
	// detected, so that is what is compared.
	if claimedN != len(derived) {
		t.Errorf("terminal message claims %d mid-name segments; ExtractTags detects %d.\n"+
			"  claimed: %v\n  in code: %v", claimedN, len(derived), claimedSegs, derived)
	}
	if claimedN != len(claimedSegs) {
		t.Errorf("terminal message says %d but lists %d: %v", claimedN, len(claimedSegs), claimedSegs)
	}

	inCode := make(map[string]bool, len(derived))
	for _, s := range derived {
		inCode[s] = true
	}
	claimed := make(map[string]bool, len(claimedSegs))
	for _, s := range claimedSegs {
		claimed[s] = true
	}
	var missing, extra []string
	for s := range inCode {
		if !claimed[s] {
			missing = append(missing, s)
		}
	}
	for s := range claimed {
		if !inCode[s] {
			extra = append(extra, s)
		}
	}
	if len(missing) > 0 {
		t.Errorf("mid-name segments ExtractTags detects but the message does not name: %v", missing)
	}
	if len(extra) > 0 {
		t.Errorf("mid-name segments the message names but ExtractTags does not detect: %v", extra)
	}
}

func TestTerminalError_MidNameSegmentsAreNotRoots(t *testing.T) {
	claims := parseClaims(t, noRecognizedSegmentErrFmt)

	// The distinction the message exists to draw: a mid-name segment widens the
	// set of accepted NAMES but not the set of accepted ROOTS. Counting the four
	// as roots is the standing documentation error -- it reads as sixteen.
	for _, seg := range claims[1][1].([]string) {
		rooted := strings.TrimPrefix(seg, ".") + "probe_leaf"
		if _, _, err := ExtractTags(rooted); err == nil {
			t.Errorf("mid-name segment %q parses as a ROOT in %q; the message's two-species split is wrong",
				seg, rooted)
		}
	}
}
