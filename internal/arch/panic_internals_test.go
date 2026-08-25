package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"sort"
	"strings"
	"testing"
)

// internalAssertions are the panics no caller argument can reach, with the guard that
// makes each one unreachable. They are invariant assertions rather than preconditions,
// so they are not part of any exported contract and belong here instead of in a doc
// comment that would tell callers about a state they cannot produce.
//
// The table is keyed by the function the panic is in, which is where a reviewer looks.
// Every entry must match a real site and every unexported site must have an entry, so
// a panic that moves into caller-reachable code fails this test until it is either
// documented on the exported API or explained here.
var internalAssertions = map[string]string{
	// The exported entry reserves through the protocol's own maximum and reports
	// ErrImageIDsExhausted, so the narrowing conversion can never see an overflow.
	"core/term:imageID": "Terminal.Transmit returns ErrImageIDsExhausted before reserving past math.MaxUint32",

	// Dimensions are validated in one place for both of a renderer's buffers. The
	// exported callers, NewSurface and Surface.Resize, document the panic themselves.
	"core/grid:surfaceSize": "NewSurface and Surface.Resize document the overflow panic",

	// Collection checks run inside the exported operations that accept the collection,
	// and those operations document the rule.
	"components/headless:checkItemKeys": "NewContainer, Container.Set and Container.Add document the duplicate-key panic",
	"components/headless:checkFields":   "NewForm and Form.Set document the nil-field panic",

	// Frame discipline. The transaction is reachable only through a Frame the running
	// Root handed out, and both of those types document what violating it costs.
	"components/headless:Frame.enlist":       "the Frame type documents staging outside, twice in, or across root frames",
	"components/headless:transaction.begin":  "Root.Draw documents that it is not reentrant",
	"components/headless:Scroll.updateState": "the Frame type documents staging outside the owning frame",

	// Identity counters. These are uint64 and are consumed once per operation, so
	// exhausting one is not a state a caller can arrange within a session; the
	// exported operations that can plausibly approach one say so anyway.
	"components/headless:Editor.requireContentRevision": "revisions are uint64 and consumed one per content change",
	"components/headless:blockOffset":                   "Transcript.Append documents the identity-exhaustion panic",
	"components/headless:treeCopy.identityAt":           "Tree.SetNodes documents identity exhaustion for node collections",

	// Cycle detection during the ownership copy, which Tree.SetNodes documents.
	"components/headless:treeCopy.copyNext": "Tree.SetNodes documents the cyclic-graph panic",
}

// TestEveryPanickingPreconditionIsDocumented makes the panic principle executable.
//
// A panic in this repository says the caller wrote their program wrong. The caller can
// only act on that if the rule is visible before they run it, so a precondition on an
// exported function has to be in that function's doc comment — where `go doc` shows it
// — and not only in the message the crash prints. Panics a caller cannot reach are
// declared in internalAssertions with the guard that makes them unreachable.
//
// The test fails in both directions: an undocumented exported precondition, and a
// table entry whose site has gone. The second half is what stops the table from
// becoming a list of exemptions nobody rechecks.
func TestEveryPanickingPreconditionIsDocumented(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	seen := map[string]bool{}

	walk(t, root, func(dir, path string) {
		if !publicPackageDir(dir) {
			return
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", relative(root, path), err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || !panics(function) {
				continue
			}
			name := functionName(function)
			key := dir + ":" + name
			if exportedAPI(function) {
				if !strings.Contains(strings.ToLower(documentation(function)), "panic") {
					t.Errorf("%s: %s panics but its doc comment does not say so; a caller "+
						"who cannot see the rule in go doc learns it at runtime, which is "+
						"what the panic was meant to prevent", relative(root, path), name)
				}
				continue
			}
			seen[key] = true
			if _, declared := internalAssertions[key]; !declared {
				t.Errorf("%s: %s panics from unexported code with no entry in "+
					"internalAssertions; either document the rule on the exported API "+
					"that has it, or record the guard that makes it unreachable",
					relative(root, path), name)
			}
		}
	})

	stale := make([]string, 0, len(internalAssertions))
	for key := range internalAssertions {
		if !seen[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	for _, key := range stale {
		t.Errorf("internalAssertions names %s, which no longer panics: remove the entry "+
			"rather than leaving an exemption for code that is gone", key)
	}
}

// publicPackageDir reports whether a directory holds a package this repository
// publishes. Internal packages, the workspace tooling and the examples are governed by
// other rules and are not part of anyone's contract.
func publicPackageDir(dir string) bool {
	if dir == "." || strings.HasPrefix(dir, "examples") {
		return false
	}
	return !slices.Contains(strings.Split(dir, "/"), "internal")
}

// panics reports whether the function body calls panic directly. A helper that panics
// is judged where it is written, not at every call site, which is the same place a
// reviewer reads the guard.
func panics(function *ast.FuncDecl) bool {
	found := false
	ast.Inspect(function, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		identifier, ok := call.Fun.(*ast.Ident)
		if ok && identifier.Name == "panic" {
			found = true
			return false
		}
		return true
	})
	return found
}

// exportedAPI reports whether a caller outside the package can name this function. An
// exported method on an unexported type is not reachable and is judged as internal.
func exportedAPI(function *ast.FuncDecl) bool {
	if !function.Name.IsExported() {
		return false
	}
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return true
	}
	return ast.IsExported(receiverName(function))
}

func functionName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return function.Name.Name
	}
	return receiverName(function) + "." + function.Name.Name
}

func receiverName(function *ast.FuncDecl) string {
	expression := function.Recv.List[0].Type
	if star, ok := expression.(*ast.StarExpr); ok {
		expression = star.X
	}
	if index, ok := expression.(*ast.IndexExpr); ok {
		expression = index.X
	}
	if list, ok := expression.(*ast.IndexListExpr); ok {
		expression = list.X
	}
	if identifier, ok := expression.(*ast.Ident); ok {
		return identifier.Name
	}
	return ""
}

func documentation(function *ast.FuncDecl) string {
	if function.Doc == nil {
		return ""
	}
	return function.Doc.Text()
}
