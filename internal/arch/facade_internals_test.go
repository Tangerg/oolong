package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// kitCapabilityMethods are the adapters that make an appearance participate in the
// component protocols. They are not a second state API: a Container can only compose
// a dressed controller when the dressed value itself implements these capabilities.
var kitCapabilityMethods = map[string]bool{
	"Do":        true,
	"Draw":      true,
	"Focus":     true,
	"Handle":    true,
	"Measure":   true,
	"Semantics": true,
}

// TestKitDoesNotMirrorControllerState keeps appearance and behavior as two layers,
// not two overlapping facades. A kit value exposes its complete headless Controller
// or Editor for semantic state. The kit value itself implements only the component
// capabilities needed for composition; forwarding a selected state operation would
// give callers two entry points and would inevitably leave the unforwarded operations
// discoverable only through the owner below.
func TestKitDoesNotMirrorControllerState(t *testing.T) {
	root := repoRoot(t)
	directory := filepath.Join(root, "components", "kit")
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		set := token.NewFileSet()
		file, err := parser.ParseFile(set, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || !function.Name.IsExported() ||
				kitCapabilityMethods[function.Name.Name] {
				continue
			}
			owner, operation, ok := delegatedKitOwner(function)
			if !ok {
				continue
			}
			position := set.Position(function.Pos())
			relative, relErr := filepath.Rel(root, position.Filename)
			if relErr != nil {
				return relErr
			}
			t.Errorf("%s:%d: %s delegates state to %s.%s; expose that operation only through Controller or Editor",
				filepath.ToSlash(relative), position.Line, function.Name.Name, owner, operation)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect kit facade: %v", err)
	}
}

// delegatedKitOwner reports a method whose only call is through its controller or
// editor field. Nil guards and zero fallbacks do not change the ownership fact; an
// operation that performs any other call is composition rather than a pure facade.
func delegatedKitOwner(function *ast.FuncDecl) (owner, operation string, delegated bool) {
	if function.Body == nil || function.Recv == nil || len(function.Recv.List) != 1 ||
		len(function.Recv.List[0].Names) != 1 {
		return "", "", false
	}
	receiver := function.Recv.List[0].Names[0].Name
	operations := 0
	foreign := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if kitOwnerAccessor(call, receiver) != "" {
			return true
		}
		operations++
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			foreign = true
			return true
		}
		from := kitOwnerExpression(selector.X, receiver)
		if from == "" {
			foreign = true
			return true
		}
		owner, operation = from, selector.Sel.Name
		return true
	})
	return owner, operation, operations == 1 && owner != "" && !foreign
}

func kitOwnerExpression(expression ast.Expr, receiver string) string {
	if field, ok := expression.(*ast.SelectorExpr); ok {
		from, ok := field.X.(*ast.Ident)
		if ok && from.Name == receiver && (field.Sel.Name == "controller" || field.Sel.Name == "editor") {
			return field.Sel.Name
		}
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return ""
	}
	return kitOwnerAccessor(call, receiver)
}

func kitOwnerAccessor(call *ast.CallExpr, receiver string) string {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	from, ok := selector.X.(*ast.Ident)
	if !ok || from.Name != receiver {
		return ""
	}
	switch selector.Sel.Name {
	case "Controller":
		return "controller"
	case "Editor":
		return "editor"
	default:
		return ""
	}
}

func TestKitFacadeRuleRecognizesStateDelegation(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{"func (w *Widget) Value() int { return w.controller.Value() }", true},
		{"func (w *Widget) Reset() { if w != nil { w.editor.Clear() } }", true},
		{"func (w *Widget) Value() int { return w.Controller().Value() }", true},
		{"func (w *Widget) Reset() { if w != nil { w.Editor().Clear() } }", true},
		{"func (w *Widget) Value() int { return normalize(w.controller.Value()) }", false},
		{"func (w *Widget) Value() int { return normalize(w.Controller().Value()) }", false},
		{"func (w *Widget) Value() int { return w.model.Value() }", false},
		{"func Value() int { return 0 }", false},
	}
	for _, test := range tests {
		file, err := parser.ParseFile(token.NewFileSet(), "rule.go", "package rule\n"+test.source,
			parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %q: %v", test.source, err)
		}
		function := file.Decls[0].(*ast.FuncDecl)
		_, _, got := delegatedKitOwner(function)
		if got != test.want {
			t.Errorf("delegatedKitOwner(%q) = %t, want %t", test.source, got, test.want)
		}
	}
}
