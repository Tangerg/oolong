package arch

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// renderingPackages are the retained and passive projection packages whose Draw and
// Measure contracts section 4.2 governs. Lower grid and text primitives mutate only
// the new frame they are handed; these packages are where application-owned state,
// callbacks and composition first meet that frame.
var renderingPackages = []string{
	"components/headless",
	"components/kit",
	"latex",
	"markdown",
}

type renderFunction struct {
	name         string
	receiver     string
	receiverName string
	declaration  *ast.FuncDecl
	imports      map[string]string
	set          *token.FileSet
}

func (f *renderFunction) identity() string {
	if f.receiver == "" {
		return f.name
	}
	return f.receiver + "." + f.name
}

type renderPackageModel struct {
	functions        []*renderFunction
	functionByObject map[token.Pos]*renderFunction
	callbackByObject map[token.Pos]callbackRule
	localNames       map[string]bool
	types            *types.Info
}

// TestRenderingCannotReachIntrinsicEffects makes the mechanically decidable half of
// observational purity executable. It follows package-local calls rather than only
// inspecting an entry body: moving a goroutine or an I/O call into a helper must not
// make it legal. Calls through application-supplied interfaces remain a contract of
// that interface and are covered separately by the callback and behavioral gates.
func TestRenderingCannotReachIntrinsicEffects(t *testing.T) {
	root := repoRoot(t)
	for _, directory := range renderingPackages {
		t.Run(strings.ReplaceAll(directory, "/", "_"), func(t *testing.T) {
			model := loadRenderPackage(t, filepath.Join(root, directory), directory)
			for _, entry := range model.functions {
				if entry.receiver == "" || (entry.name != "Measure" && !strings.HasPrefix(entry.name, "Draw")) {
					continue
				}
				for _, finding := range model.effectsFrom(entry) {
					t.Errorf("%s: %s reaches %s through %s", finding.position,
						entry.identity(), finding.effect, strings.Join(finding.path, " -> "))
				}
			}
		})
	}
}

type renderEffect struct {
	position token.Position
	effect   string
	path     []string
}

func (m renderPackageModel) effectsFrom(entry *renderFunction) []renderEffect {
	var findings []renderEffect
	seen := make(map[*renderFunction]bool)
	var visit func(*renderFunction, []string)
	visit = func(function *renderFunction, path []string) {
		if seen[function] {
			return
		}
		seen[function] = true
		path = append(slices.Clone(path), function.identity())
		callees := make(map[*renderFunction]bool)
		ast.Inspect(function.declaration.Body, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.GoStmt:
				findings = append(findings, renderEffect{
					position: function.set.Position(node.Go), effect: "a goroutine", path: slices.Clone(path),
				})
			case *ast.CallExpr:
				if effect := m.renderCallEffect(function, node); effect != "" {
					findings = append(findings, renderEffect{
						position: function.set.Position(node.Pos()), effect: effect, path: slices.Clone(path),
					})
				}
				local, unresolved := m.localCallees(function, node)
				if unresolved != "" {
					findings = append(findings, renderEffect{
						position: function.set.Position(node.Pos()),
						effect:   "an unresolved package-local call " + unresolved,
						path:     slices.Clone(path),
					})
				}
				for _, callee := range local {
					callees[callee] = true
				}
			}
			return true
		})
		for callee := range callees {
			visit(callee, path)
		}
	}
	visit(entry, nil)
	slices.SortFunc(findings, func(a, b renderEffect) int {
		if byPosition := strings.Compare(a.position.String(), b.position.String()); byPosition != 0 {
			return byPosition
		}
		return strings.Compare(a.effect, b.effect)
	})
	return findings
}

func (m renderPackageModel) localCallees(from *renderFunction, call *ast.CallExpr) ([]*renderFunction, string) {
	var object types.Object
	var name string
	directLocal := false
	switch called := call.Fun.(type) {
	case *ast.Ident:
		name = called.Name
		directLocal = true
		object = m.types.Uses[called]
	case *ast.SelectorExpr:
		name = called.Sel.Name
		if identifier, ok := called.X.(*ast.Ident); ok {
			if _, imported := from.imports[identifier.Name]; imported {
				return nil, ""
			}
			directLocal = identifier.Name == from.receiverName
		}
		if selection := m.types.Selections[called]; selection != nil {
			object = selection.Obj()
		} else {
			object = m.types.Uses[called.Sel]
		}
	default:
		return nil, ""
	}
	function, ok := object.(*types.Func)
	if !ok {
		if object == nil && directLocal && m.localNames[name] {
			return nil, name
		}
		return nil, ""
	}
	if local := m.functionByObject[function.Pos()]; local != nil {
		return []*renderFunction{local}, ""
	}
	return nil, ""
}

var publicationCalls = map[string]bool{
	"Append": true,
	"Commit": true,
	"Flush":  true,
	"Notify": true,
	"Post":   true,
	"Print":  true,
}

func (m renderPackageModel) renderCallEffect(function *renderFunction, call *ast.CallExpr) string {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if publicationCalls[selector.Sel.Name] {
		return "the publication operation " + selector.Sel.Name
	}
	if phase, classified := m.nonProjectionCallback(selector); classified {
		return "the " + string(phase) + " callback " + selector.Sel.Name
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return ""
	}
	path, imported := function.imports[identifier.Name]
	if !imported {
		return ""
	}
	if reason := effectPackageCall(path, selector.Sel.Name); reason != "" {
		return reason
	}
	return ""
}

func (m renderPackageModel) nonProjectionCallback(selector *ast.SelectorExpr) (callbackPhase, bool) {
	selection := m.types.Selections[selector]
	if selection == nil {
		return "", false
	}
	rule, ok := m.callbackByObject[selection.Obj().Pos()]
	if ok && rule.phase != callbackProjection {
		return rule.phase, true
	}
	return "", false
}

func effectPackageCall(path, name string) string {
	switch path {
	case "bufio", "io", "io/fs", "net", "net/http", "os", "os/exec", "syscall":
		return "I/O through " + path + "." + name
	case "log", "log/slog":
		return "logging through " + path + "." + name
	case "crypto/rand", "math/rand", "math/rand/v2":
		return "nondeterminism through " + path + "." + name
	case "fmt":
		if strings.HasPrefix(name, "Print") || strings.HasPrefix(name, "Fprint") {
			return "output through fmt." + name
		}
	case "time":
		switch name {
		case "After", "AfterFunc", "NewTicker", "NewTimer", "Now", "Since", "Sleep", "Tick", "Until":
			return "clock or scheduling work through time." + name
		}
	}
	return ""
}

func loadRenderPackage(t *testing.T, directory, packageKey string) renderPackageModel {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	model := renderPackageModel{
		functionByObject: make(map[token.Pos]*renderFunction),
		callbackByObject: make(map[token.Pos]callbackRule),
		localNames:       make(map[string]bool),
		types: &types.Info{
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		},
	}
	set := token.NewFileSet()
	var files []*ast.File
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		file, err := parser.ParseFile(set, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		files = append(files, file)
		aliases := importAliases(t, file)
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			renderer := &renderFunction{
				name: function.Name.Name, declaration: function, imports: aliases, set: set,
			}
			if function.Recv != nil && len(function.Recv.List) == 1 {
				renderer.receiver = renderReceiverIdentity(function.Recv.List[0].Type)
				if len(function.Recv.List[0].Names) == 1 {
					renderer.receiverName = function.Recv.List[0].Names[0].Name
				}
			}
			model.functions = append(model.functions, renderer)
			model.localNames[renderer.name] = true
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec := specification.(*ast.TypeSpec)
				structure, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, field := range structure.Fields.List {
					if _, ok := field.Type.(*ast.FuncType); !ok {
						continue
					}
					for _, name := range field.Names {
						key := packageKey + ":" + typeSpec.Name.Name + "." + name.Name
						if rule, exists := renderingCallbacks[key]; exists {
							model.callbackByObject[name.Pos()] = rule
						}
					}
				}
			}
		}
	}
	config := types.Config{Importer: importer.Default(), Error: func(error) {}}
	_, _ = config.Check(packageKey, set, files, model.types)
	for _, function := range model.functions {
		if object, ok := model.types.Defs[function.declaration.Name].(*types.Func); ok {
			model.functionByObject[object.Pos()] = function
		}
	}
	return model
}

func importAliases(t *testing.T, file *ast.File) map[string]string {
	t.Helper()
	aliases := make(map[string]string, len(file.Imports))
	for _, specification := range file.Imports {
		path, err := strconv.Unquote(specification.Path.Value)
		if err != nil {
			t.Fatalf("unquote import %s: %v", specification.Path.Value, err)
		}
		name := filepath.Base(path)
		if specification.Name != nil {
			name = specification.Name.Name
		}
		if name != "_" && name != "." {
			aliases[name] = path
		}
	}
	return aliases
}

func TestRenderEffectRuleFollowsHelpersAndDistinguishesPureCalls(t *testing.T) {
	tests := []struct {
		name          string
		source        string
		callbackPhase callbackPhase
		eraseMethod   string
		want          []string
	}{
		{
			name: "direct goroutine",
			source: `package sample
				type Widget struct{}
				func (Widget) Draw() { go work() }
				func work() {}`,
			want: []string{"a goroutine"},
		},
		{
			name: "goroutine behind helper",
			source: `package sample
				type Widget struct{}
				func (w Widget) Draw() { w.prepare() }
				func (Widget) prepare() { go work() }
				func work() {}`,
			want: []string{"a goroutine"},
		},
		{
			name: "unresolved receiver helper",
			source: `package sample
				type Widget struct{}
				func (w Widget) Draw() { w.prepare() }
				func (Widget) prepare() {}`,
			eraseMethod: "prepare",
			want:        []string{"an unresolved package-local call prepare"},
		},
		{
			name: "I/O behind package helper",
			source: `package sample
				import "os"
				type Widget struct{}
				func (Widget) Measure(int) int { load(); return 1 }
				func load() { _, _ = os.ReadFile("state") }`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "I/O behind helper value",
			source: `package sample
				import "os"
				type preparer struct{}
				func (preparer) prepare() { _, _ = os.ReadFile("state") }
				type Widget struct { helper preparer }
				func (w Widget) Draw() { w.helper.prepare() }`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "clock",
			source: `package sample
				import "time"
				type Widget struct{}
				func (Widget) Draw() { _ = time.Now() }`,
			want: []string{"clock or scheduling work through time.Now"},
		},
		{
			name: "publication through interface",
			source: `package sample
				type Printer interface { Print(any) }
				type Widget struct { out Printer }
				func (w Widget) Draw() { w.out.Print(1) }`,
			want: []string{"the publication operation Print"},
		},
		{
			name: "event callback",
			source: `package sample
				type Widget struct { Check func() }
				func (w Widget) Draw() { w.Check() }`,
			callbackPhase: callbackEvent,
			want:          []string{"the event callback Check"},
		},
		{
			name: "same named method is not callback",
			source: `package sample
				type Widget struct{}
				func (Widget) Draw() { Widget{}.Check() }
				func (Widget) Check() {}`,
		},
		{
			name: "frame-local work",
			source: `package sample
				import "strings"
				type Widget struct{}
				func (Widget) Draw() { var b strings.Builder; b.WriteString("frame") }`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := renderModelFromSource(t, test.source)
			if test.eraseMethod != "" {
				for _, function := range model.functions {
					ast.Inspect(function.declaration.Body, func(node ast.Node) bool {
						selector, ok := node.(*ast.SelectorExpr)
						if !ok || selector.Sel.Name != test.eraseMethod {
							return true
						}
						delete(model.types.Selections, selector)
						delete(model.types.Uses, selector.Sel)
						return true
					})
				}
			}
			if test.callbackPhase != "" {
				for _, function := range model.functions {
					ast.Inspect(function.declaration.Body, func(node ast.Node) bool {
						selector, ok := node.(*ast.SelectorExpr)
						if !ok || selector.Sel.Name != "Check" {
							return true
						}
						selection := model.types.Selections[selector]
						if selection != nil {
							model.callbackByObject[selection.Obj().Pos()] = callbackRule{phase: test.callbackPhase}
						}
						return true
					})
				}
			}
			var got []string
			for _, function := range model.functions {
				if function.name != "Draw" && function.name != "Measure" {
					continue
				}
				for _, finding := range model.effectsFrom(function) {
					got = append(got, finding.effect)
				}
			}
			slices.Sort(got)
			slices.Sort(test.want)
			if !slices.Equal(got, test.want) {
				t.Errorf("effects = %v, want %v", got, test.want)
			}
		})
	}
}

func renderModelFromSource(t *testing.T, source string) renderPackageModel {
	t.Helper()
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "sample.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	model := renderPackageModel{
		functionByObject: make(map[token.Pos]*renderFunction),
		callbackByObject: make(map[token.Pos]callbackRule),
		localNames:       make(map[string]bool),
		types: &types.Info{
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		},
	}
	aliases := importAliases(t, file)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		renderer := &renderFunction{
			name: function.Name.Name, declaration: function, imports: aliases, set: set,
		}
		if function.Recv != nil && len(function.Recv.List) == 1 {
			renderer.receiver = renderReceiverIdentity(function.Recv.List[0].Type)
			if len(function.Recv.List[0].Names) == 1 {
				renderer.receiverName = function.Recv.List[0].Names[0].Name
			}
		}
		model.functions = append(model.functions, renderer)
		model.localNames[renderer.name] = true
	}
	var typeErrors []error
	config := types.Config{
		Importer: importer.Default(),
		Error:    func(err error) { typeErrors = append(typeErrors, err) },
	}
	_, _ = config.Check("sample", set, []*ast.File{file}, model.types)
	if len(typeErrors) > 0 {
		t.Fatalf("type-check sample: %v", typeErrors)
	}
	for _, function := range model.functions {
		if object, ok := model.types.Defs[function.declaration.Name].(*types.Func); ok {
			model.functionByObject[object.Pos()] = function
		}
	}
	if len(model.functions) == 0 {
		t.Fatal("source declared no functions")
	}
	return model
}

func renderReceiverIdentity(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.StarExpr:
		return "*" + renderReceiverIdentity(expression.X)
	case *ast.IndexExpr:
		return renderReceiverIdentity(expression.X)
	case *ast.IndexListExpr:
		return renderReceiverIdentity(expression.X)
	case *ast.Ident:
		return expression.Name
	default:
		return "<unknown>"
	}
}
