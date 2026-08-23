package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
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
