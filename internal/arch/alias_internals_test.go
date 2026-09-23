package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// TestNoMethodRenamesAnother keeps one operation under one name.
//
// A method whose body is nothing but a call to another method of the same type,
// passing its own parameters through unchanged, is a second name for a fact that
// already has one. Which name a caller reaches then depends on which they happened
// to find, the two drift the moment either grows a guard the other lacks, and the
// duplicate has to be kept accurate in documentation forever. Parameterising a call
// is a different thing and stays allowed: an argument the caller did not supply is
// knowledge this method owns.
func TestNoMethodRenamesAnother(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()

	type method struct {
		receiver string
		target   string
		position token.Position
	}
	declared := map[string]bool{}
	var candidates []method

	walk(t, root, func(dir, path string) {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			receiver := receiverType(function)
			if receiver == "" {
				continue
			}
			// A type is identified by its directory as well as its name: two packages
			// may name a type the same and share nothing else.
			owner := dir + "." + receiver
			declared[owner+"."+function.Name.Name] = true
			if target, isAlias := aliasedMethod(function); isAlias {
				candidates = append(candidates, method{
					receiver: owner,
					target:   target,
					position: fset.Position(function.Pos()),
				})
			}
		}
	})

	for _, candidate := range candidates {
		if !declared[candidate.receiver+"."+candidate.target] {
			continue
		}
		relative, err := filepath.Rel(root, candidate.position.Filename)
		if err != nil {
			t.Fatalf("locate %s: %v", candidate.position.Filename, err)
		}
		t.Errorf("%s:%d: this method is another name for %s; keep one of the two",
			filepath.ToSlash(relative), candidate.position.Line, candidate.target)
	}
}

// receiverType is the name of the type a method is declared on, without its pointer
// or type arguments, and empty for anything that is not a method.
func receiverType(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) != 1 {
		return ""
	}
	expression := function.Recv.List[0].Type
	if star, ok := expression.(*ast.StarExpr); ok {
		expression = star.X
	}
	switch typed := expression.(type) {
	case *ast.IndexExpr:
		expression = typed.X
	case *ast.IndexListExpr:
		expression = typed.X
	}
	name, ok := expression.(*ast.Ident)
	if !ok {
		return ""
	}
	return name.Name
}

// aliasedMethod reports the method this one is a second name for: a body that is one
// call on the receiver and nothing else, handed exactly this method's own parameters
// in their own order. Anything the call adds — a literal, a conversion, a second
// operation, a result the method then uses — makes it this method's work rather than
// a rename.
func aliasedMethod(function *ast.FuncDecl) (target string, aliased bool) {
	if function.Recv == nil || len(function.Recv.List) != 1 ||
		len(function.Recv.List[0].Names) != 1 || function.Body == nil ||
		len(function.Body.List) != 1 {
		return "", false
	}
	receiver := function.Recv.List[0].Names[0].Name
	if receiver == "_" {
		return "", false
	}

	var call *ast.CallExpr
	switch statement := function.Body.List[0].(type) {
	case *ast.ReturnStmt:
		if len(statement.Results) != 1 {
			return "", false
		}
		call, _ = statement.Results[0].(*ast.CallExpr)
	case *ast.ExprStmt:
		call, _ = statement.X.(*ast.CallExpr)
	}
	if call == nil {
		return "", false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	if from, ok := selector.X.(*ast.Ident); !ok || from.Name != receiver {
		return "", false
	}
	if !forwardsItsOwnParameters(function.Type.Params, call.Args) {
		return "", false
	}
	return selector.Sel.Name, true
}

// forwardsItsOwnParameters reports whether args are the declared parameters, in
// order. An unnamed parameter cannot be forwarded at all, so a signature holding one
// is never a rename.
func forwardsItsOwnParameters(params *ast.FieldList, args []ast.Expr) bool {
	var names []string
	if params != nil {
		for _, field := range params.List {
			if len(field.Names) == 0 {
				return false
			}
			for _, name := range field.Names {
				if name.Name == "_" {
					return false
				}
				names = append(names, name.Name)
			}
		}
	}
	if len(names) != len(args) {
		return false
	}
	for i, arg := range args {
		ident, ok := arg.(*ast.Ident)
		if !ok || ident.Name != names[i] {
			return false
		}
	}
	return true
}

func TestMethodAliasRuleRecognizesRenames(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{"func (m Modes) Enter() string { return m.enter() }", true},
		{"func (d *Decoder) Open() Line { return d.materialise() }", true},
		{"func (w *Widget) Set(v int) { w.set(v) }", true},
		{"func (w *Widget) Bind(a string, k ...Chord) { w.bind(a, k...) }", true},
		{"func (l *List[T]) Focus(has bool) { l.focus(has) }", true},
		// Parameterising a call is knowledge this method owns, not a rename.
		{"func (p *Parser) Flush() []Event { return p.drain(true) }", false},
		{"func (i *Ingress) Close() error { return i.CloseWithError(nil) }", false},
		{"func (c *Container) FocusNext() bool { return c.step(1) }", false},
		{"func (c *Container) HeightForWidth(w int) int { return c.Measure(Down, w) }", false},
		// Not one bare call on the receiver.
		{"func (k Key) String() string { return k.Chord().String() }", false},
		{"func (s Session) Report(p string) error { return s.host().report(p) }", false},
		{"func (w *Widget) Value() int { return normalize(w.value()) }", false},
		{"func (w *Widget) Reset() { w.clear(); w.redraw() }", false},
		{"func (w *Widget) Set(v int) { w.set(other) }", false},
		{"func (w *Widget) Set(int) { w.set(0) }", false},
		{"func Enter() string { return \"\" }", false},
	}
	for _, test := range tests {
		file, err := parser.ParseFile(token.NewFileSet(), "rule.go", "package rule\n"+test.source,
			parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %q: %v", test.source, err)
		}
		function, ok := file.Decls[0].(*ast.FuncDecl)
		if !ok {
			t.Fatalf("%q is not a function declaration", test.source)
		}
		if _, got := aliasedMethod(function); got != test.want {
			t.Errorf("aliasedMethod(%q) = %t, want %t", test.source, got, test.want)
		}
	}
}

func TestMethodAliasRuleIdentifiesTheReceiverType(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{"func (m Modes) Enter() string { return \"\" }", "Modes"},
		{"func (d *Decoder) Open() int { return 0 }", "Decoder"},
		{"func (l *List[T]) Len() int { return 0 }", "List"},
		{"func (p Pair[K, V]) Key() int { return 0 }", "Pair"},
		{"func Enter() string { return \"\" }", ""},
	}
	for _, test := range tests {
		file, err := parser.ParseFile(token.NewFileSet(), "rule.go", "package rule\n"+test.source,
			parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %q: %v", test.source, err)
		}
		function, ok := file.Decls[0].(*ast.FuncDecl)
		if !ok {
			t.Fatalf("%q is not a function declaration", test.source)
		}
		if got := receiverType(function); got != test.want {
			t.Errorf("receiverType(%q) = %q, want %q", test.source, got, test.want)
		}
	}
}
