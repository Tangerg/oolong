package kit_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestEveryKitAccessorConfigHasAControlledExample is the composition-side coverage
// gate for caller-owned state. Headless tests prove controller behavior; each kit
// Config that accepts the same ownership seam must independently demonstrate that it
// passes both caller and controller transitions through without retaining a shadow.
func TestEveryKitAccessorConfigHasAControlledExample(t *testing.T) {
	configs, examples := kitAccessorConfigs(t), controlledKitExamples(t)
	for config := range configs {
		if !examples[config] {
			t.Errorf("%s accepts headless.Accessor but has no ExampleNew%s_controlled", config, strings.TrimSuffix(config, "Config"))
		}
	}
	for config := range examples {
		if !configs[config] {
			t.Errorf("controlled example for %s is stale", config)
		}
	}
}

func kitAccessorConfigs(t *testing.T) map[string]bool {
	t.Helper()
	configs := make(map[string]bool)
	for _, file := range kitPackageFiles(t, false) {
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typed, ok := specification.(*ast.TypeSpec)
				if ok && typed.Name.IsExported() && strings.HasSuffix(typed.Name.Name, "Config") &&
					structHasHeadlessAccessor(typed.Type) {
					configs[typed.Name.Name] = true
				}
			}
		}
	}
	return configs
}

func controlledKitExamples(t *testing.T) map[string]bool {
	t.Helper()
	examples := make(map[string]bool)
	for _, file := range kitPackageFiles(t, true) {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || !strings.HasPrefix(function.Name.Name, "ExampleNew") ||
				!strings.HasSuffix(function.Name.Name, "_controlled") {
				continue
			}
			name := strings.TrimSuffix(strings.TrimPrefix(function.Name.Name, "ExampleNew"), "_controlled")
			examples[name+"Config"] = true
		}
	}
	return examples
}

func kitPackageFiles(t *testing.T, tests bool) []*ast.File {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	set := token.NewFileSet()
	var files []*ast.File
	for _, entry := range entries {
		isTest := strings.HasSuffix(entry.Name(), "_test.go")
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || isTest != tests {
			continue
		}
		file, err := parser.ParseFile(set, entry.Name(), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	return files
}

func structHasHeadlessAccessor(expression ast.Expr) bool {
	structure, ok := expression.(*ast.StructType)
	if !ok {
		return false
	}
	for _, field := range structure.Fields.List {
		indexed, ok := field.Type.(*ast.IndexExpr)
		if !ok {
			continue
		}
		selector, ok := indexed.X.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		from, named := selector.X.(*ast.Ident)
		if named && from.Name == "headless" && selector.Sel.Name == "Accessor" {
			return true
		}
	}
	return false
}

func TestKitAccessorConfigRuleRecognizesDirectHeadlessOwnership(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "rule.go", `package rule
type Direct struct { Value headless.Accessor[int] }
type Local struct { Value Accessor[int] }
type Nested struct { Values []headless.Accessor[int] }
type Plain struct { Value int }
`, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"Direct": true, "Local": false, "Nested": false, "Plain": false}
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, specification := range general.Specs {
			typed := specification.(*ast.TypeSpec)
			if got := structHasHeadlessAccessor(typed.Type); got != want[typed.Name.Name] {
				t.Errorf("%s recognized = %t, want %t", typed.Name.Name, got, want[typed.Name.Name])
			}
		}
	}
}
