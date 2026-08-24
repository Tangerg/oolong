package arch

import (
	"cmp"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestNonCopyableOwnersStateAndEnforceTheirContract keeps public ownership and its
// mechanical representation in both directions. A comment alone cannot stop a value
// copy from giving two mutable owners the same slice, tree or terminal model, while a
// lock hidden in an exported struct makes ordinary assignment invalid whether or not
// its documentation warned the caller. A direct noCopy or synchronization field
// makes the standard copylocks analyzer reject that copy; the matching public promise
// tells users why the pointer identity matters.
func TestNonCopyableOwnersStateAndEnforceTheirContract(t *testing.T) {
	root := repoRoot(t)
	markerPackages := make(map[string]bool)
	walk(t, root, func(_, path string) {
		set := token.NewFileSet()
		file, err := parser.ParseFile(set, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		imports := importNames(file)
		for _, declaration := range file.Decls {
			group, ok := declaration.(*ast.GenDecl)
			if !ok || group.Tok != token.TYPE {
				continue
			}
			for _, raw := range group.Specs {
				spec, ok := raw.(*ast.TypeSpec)
				if !ok {
					continue
				}
				documented := declaresNoCopyContract(typeDocumentation(group, spec))
				structure, ok := spec.Type.(*ast.StructType)
				protected, marker := noCopyStructProtection(structure, imports)
				if marker && documented {
					directory := filepath.Dir(path)
					defined, known := markerPackages[directory]
					if !known {
						defined = packageDefinesNoCopyMarker(t, directory)
						markerPackages[directory] = defined
					}
					protected = protected && defined
				}
				position := set.Position(spec.Pos())
				switch nonCopyContractProblem(ast.IsExported(spec.Name.Name), documented, ok && protected) {
				case noNonCopyProblem:
				case missingProtection:
					t.Errorf("%s: %s promises not to be copied but has no direct go vet copylocks marker",
						position, spec.Name.Name)
				case missingDocumentation:
					t.Errorf("%s: %s has a direct go vet copylocks marker but does not state that it must not be copied",
						position, spec.Name.Name)
				}
			}
		}
	})
}

// TestExportedOwnersDoNotHideNestedCopylocks closes the transitive half of the
// ownership contract. go vet follows value fields recursively: a List already became
// non-copyable through Scroll and Matcher even when List had neither its own marker
// nor any public warning. Every exported owner that contains that fact therefore
// states it directly; the test above then keeps its marker and documentation paired.
func TestExportedOwnersDoNotHideNestedCopylocks(t *testing.T) {
	types := repositoryOwnershipTypes(t, repoRoot(t))
	copylocked := copylockedOwnershipTypes(types)
	for _, key := range sortedOwnershipKeys(types) {
		owner := types[key]
		if owner.exported && copylocked[key] && !owner.direct {
			t.Errorf("%s: %s contains a copylocked owner but has no direct marker; state its own ownership contract",
				owner.position, key.name)
		}
	}
}

type ownershipTypeKey struct {
	dir  string
	name string
}

type ownershipType struct {
	position token.Position
	exported bool
	direct   bool
	contains []ownershipTypeKey
}

func repositoryOwnershipTypes(t *testing.T, root string) map[ownershipTypeKey]ownershipType {
	t.Helper()
	types := make(map[ownershipTypeKey]ownershipType)
	walk(t, root, func(dir, path string) {
		set := token.NewFileSet()
		file, err := parser.ParseFile(set, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		imports := importNames(file)
		for _, declaration := range file.Decls {
			group, ok := declaration.(*ast.GenDecl)
			if !ok || group.Tok != token.TYPE {
				continue
			}
			for _, raw := range group.Specs {
				spec := raw.(*ast.TypeSpec)
				structure, ok := spec.Type.(*ast.StructType)
				if !ok {
					continue
				}
				direct, _ := noCopyStructProtection(structure, imports)
				key := ownershipTypeKey{dir: dir, name: spec.Name.Name}
				types[key] = ownershipType{
					position: set.Position(spec.Pos()),
					exported: spec.Name.IsExported(),
					direct:   direct,
					contains: ownershipValueReferences(structure, dir, imports),
				}
			}
		}
	})
	return types
}

func ownershipValueReferences(structure *ast.StructType, dir string, imports map[string]string) []ownershipTypeKey {
	var references []ownershipTypeKey
	for _, field := range structure.Fields.List {
		references = append(references, ownershipValueReference(field.Type, dir, imports)...)
	}
	return references
}

func ownershipValueReference(expression ast.Expr, dir string, imports map[string]string) []ownershipTypeKey {
	switch expression := expression.(type) {
	case *ast.Ident:
		return []ownershipTypeKey{{dir: dir, name: expression.Name}}
	case *ast.SelectorExpr:
		qualifier, ok := expression.X.(*ast.Ident)
		if !ok {
			return nil
		}
		imported, ok := imports[qualifier.Name]
		if !ok {
			return nil
		}
		return repositoryOwnershipReference(imported, expression.Sel.Name)
	case *ast.IndexExpr:
		return ownershipValueReference(expression.X, dir, imports)
	case *ast.IndexListExpr:
		return ownershipValueReference(expression.X, dir, imports)
	case *ast.ArrayType:
		if expression.Len == nil {
			return nil
		}
		return ownershipValueReference(expression.Elt, dir, imports)
	case *ast.ParenExpr:
		return ownershipValueReference(expression.X, dir, imports)
	case *ast.StructType:
		return ownershipValueReferences(expression, dir, imports)
	default:
		// Pointers, slices, maps, channels, functions and interfaces are safe to copy
		// even when the value they refer to is not. This is the same boundary go vet's
		// copylocks analyzer uses.
		return nil
	}
}

func repositoryOwnershipReference(imported, name string) []ownershipTypeKey {
	prefix := modulePath + "/"
	dir, ok := strings.CutPrefix(imported, prefix)
	if !ok || dir == "" {
		return nil
	}
	return []ownershipTypeKey{{dir: dir, name: name}}
}

func copylockedOwnershipTypes(types map[ownershipTypeKey]ownershipType) map[ownershipTypeKey]bool {
	copylocked := make(map[ownershipTypeKey]bool)
	for key, owner := range types {
		copylocked[key] = owner.direct
	}
	for changed := true; changed; {
		changed = false
		for key, owner := range types {
			if copylocked[key] {
				continue
			}
			for _, contained := range owner.contains {
				if copylocked[contained] {
					copylocked[key], changed = true, true
					break
				}
			}
		}
	}
	return copylocked
}

func sortedOwnershipKeys(types map[ownershipTypeKey]ownershipType) []ownershipTypeKey {
	return slices.SortedFunc(maps.Keys(types), func(a, b ownershipTypeKey) int {
		return cmp.Or(cmp.Compare(a.dir, b.dir), cmp.Compare(a.name, b.name))
	})
}

func TestNestedCopylockRuleRecognizesValueOwnership(t *testing.T) {
	locked := ownershipTypeKey{dir: "sample", name: "Locked"}
	middle := ownershipTypeKey{dir: "sample", name: "Middle"}
	outer := ownershipTypeKey{dir: "sample", name: "Outer"}
	plain := ownershipTypeKey{dir: "sample", name: "Plain"}
	got := copylockedOwnershipTypes(map[ownershipTypeKey]ownershipType{
		locked: {direct: true},
		middle: {contains: []ownershipTypeKey{locked}},
		outer:  {contains: []ownershipTypeKey{middle}},
		plain:  {},
	})
	for key, want := range map[ownershipTypeKey]bool{locked: true, middle: true, outer: true, plain: false} {
		if got[key] != want {
			t.Errorf("%s copylocked = %t, want %t", key.name, got[key], want)
		}
	}
}

func TestNestedCopylockRuleDistinguishesValuesFromReferences(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "sample.go", `package sample
import "github.com/Tangerg/oolong/core/keymap"
type Local struct{}
type Box[T any] struct{ Value T }
type Owner struct {
	Value Local
	Pointer *Local
	Slice []Local
	Array [1]Local
	Box Box[int]
	Matcher keymap.Matcher
}
`, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	group := file.Decls[len(file.Decls)-1].(*ast.GenDecl)
	structure := group.Specs[0].(*ast.TypeSpec).Type.(*ast.StructType)
	got := make(map[ownershipTypeKey]bool)
	for _, reference := range ownershipValueReferences(structure, "components/headless", importNames(file)) {
		got[reference] = true
	}
	want := []ownershipTypeKey{
		{dir: "components/headless", name: "Local"},
		{dir: "components/headless", name: "Box"},
		{dir: "core/keymap", name: "Matcher"},
	}
	for _, reference := range want {
		if !got[reference] {
			t.Errorf("value reference %s:%s was not recognized", reference.dir, reference.name)
		}
	}
	if len(got) != len(want) {
		t.Errorf("recognized references = %v, want only %v", got, want)
	}
}

type nonCopyProblem uint8

const (
	noNonCopyProblem nonCopyProblem = iota
	missingProtection
	missingDocumentation
)

func nonCopyContractProblem(exported, documented, protected bool) nonCopyProblem {
	if documented && !protected {
		return missingProtection
	}
	if exported && protected && !documented {
		return missingDocumentation
	}
	return noNonCopyProblem
}

func declaresNoCopyContract(doc string) bool {
	doc = strings.ToLower(strings.Join(strings.Fields(doc), " "))
	return strings.Contains(doc, "must not be copied")
}

func typeDocumentation(group *ast.GenDecl, spec *ast.TypeSpec) string {
	if spec.Doc != nil {
		return spec.Doc.Text()
	}
	if len(group.Specs) == 1 && group.Doc != nil {
		return group.Doc.Text()
	}
	return ""
}

func importNames(file *ast.File) map[string]string {
	imports := make(map[string]string, len(file.Imports))
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imports[name] = path
	}
	return imports
}

func noCopyStructProtection(structure *ast.StructType, imports map[string]string) (protected, marker bool) {
	if structure == nil {
		return false, false
	}
	for _, field := range structure.Fields.List {
		switch fieldType := field.Type.(type) {
		case *ast.Ident:
			if fieldType.Name == "noCopy" {
				return true, true
			}
		case *ast.SelectorExpr:
			packageName, ok := fieldType.X.(*ast.Ident)
			if !ok {
				continue
			}
			switch imports[packageName.Name] {
			case "sync":
				if isCopylockSyncType(fieldType.Sel.Name) {
					return true, false
				}
			case "sync/atomic":
				return true, false
			}
		}
	}
	return false, false
}

func isCopylockSyncType(name string) bool {
	switch name {
	case "Cond", "Map", "Mutex", "Once", "Pool", "RWMutex", "WaitGroup":
		return true
	default:
		return false
	}
}

func packageDefinesNoCopyMarker(t *testing.T, directory string) bool {
	t.Helper()
	methods := make(map[string]bool, 2)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read %s: %v", directory, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(directory, entry.Name()), nil,
			parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", filepath.Join(directory, entry.Name()), err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || !isNoCopyReceiver(function.Recv) ||
				(function.Name.Name != "Lock" && function.Name.Name != "Unlock") ||
				function.Type.Params.NumFields() != 0 || function.Type.Results != nil {
				continue
			}
			methods[function.Name.Name] = true
		}
	}
	return methods["Lock"] && methods["Unlock"]
}

func isNoCopyReceiver(receivers *ast.FieldList) bool {
	if receivers == nil || len(receivers.List) != 1 {
		return false
	}
	pointer, ok := receivers.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	name, ok := pointer.X.(*ast.Ident)
	return ok && name.Name == "noCopy"
}

func TestNonCopyableOwnerRuleRecognizesProtection(t *testing.T) {
	for _, test := range []struct {
		doc  string
		want bool
	}{
		{doc: "An Owner must not be copied after first use.", want: true},
		{doc: "An Owner must not be copied after its first use.", want: true},
		{doc: "An Owner is not safe for concurrent use.", want: false},
	} {
		if got := declaresNoCopyContract(test.doc); got != test.want {
			t.Errorf("contract %q = %v, want %v", test.doc, got, test.want)
		}
	}

	tests := []struct {
		name   string
		source string
		want   bool
	}{
		{name: "marker", source: "type Owner struct { guard noCopy }", want: true},
		{name: "mutex", source: "type Owner struct { mu sync.Mutex }", want: true},
		{name: "atomic", source: "type Owner struct { generation atomic.Uint64 }", want: true},
		{name: "pointer is copyable", source: "type Owner struct { mu *sync.Mutex }", want: false},
		{name: "unprotected storage", source: "type Owner struct { values []int }", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := "package rule\nimport (\"sync\"; \"sync/atomic\")\n" + test.source
			file, err := parser.ParseFile(token.NewFileSet(), "rule.go", source, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parse rule: %v", err)
			}
			group := file.Decls[len(file.Decls)-1].(*ast.GenDecl)
			structure := group.Specs[0].(*ast.TypeSpec).Type.(*ast.StructType)
			got, _ := noCopyStructProtection(structure, importNames(file))
			if got != test.want {
				t.Errorf("protected = %v, want %v", got, test.want)
			}
		})
	}

	for _, test := range []struct {
		name                            string
		exported, documented, protected bool
		want                            nonCopyProblem
	}{
		{name: "public contract and marker", exported: true, documented: true, protected: true},
		{name: "contract without marker", exported: true, documented: true, want: missingProtection},
		{name: "marker without public contract", exported: true, protected: true, want: missingDocumentation},
		{name: "private implementation lock", protected: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := nonCopyContractProblem(test.exported, test.documented, test.protected); got != test.want {
				t.Errorf("problem = %v, want %v", got, test.want)
			}
		})
	}
}
