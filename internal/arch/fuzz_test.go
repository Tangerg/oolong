package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// TestEveryCommittedFuzzCorpusHasATarget keeps regression inputs attached to the
// property that replays them. Go silently ignores testdata/fuzz/<Name> when a fuzz
// target is renamed or removed, which otherwise turns a committed crasher into a
// directory that looks guarded but never runs.
func TestEveryCommittedFuzzCorpusHasATarget(t *testing.T) {
	root := repoRoot(t)
	targets := make(map[string]map[string]bool)
	checked := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if skipped(entry.Name(), path == root) {
			return filepath.SkipDir
		}
		fuzzDir := filepath.Dir(path)
		if filepath.Base(fuzzDir) != "fuzz" || filepath.Base(filepath.Dir(fuzzDir)) != "testdata" {
			return nil
		}
		corpus, readErr := os.ReadDir(path)
		if readErr != nil {
			return readErr
		}
		hasSeed := false
		for _, seed := range corpus {
			if !seed.IsDir() {
				hasSeed = true
				break
			}
		}
		if !hasSeed {
			return filepath.SkipDir
		}

		checked++
		packageDir := filepath.Dir(filepath.Dir(fuzzDir))
		if targets[packageDir] == nil {
			targets[packageDir] = packageFuzzTargets(t, packageDir)
		}
		if !targets[packageDir][entry.Name()] {
			relativePath, relativeErr := filepath.Rel(root, path)
			if relativeErr != nil {
				return relativeErr
			}
			t.Errorf("%s has committed corpus but no func %s(f *testing.F)",
				filepath.ToSlash(relativePath), entry.Name())
		}
		return filepath.SkipDir
	})
	if err != nil {
		t.Fatalf("walk fuzz corpora: %v", err)
	}
	if checked == 0 {
		t.Fatal("no committed fuzz corpus was checked, so this test proves nothing")
	}
}

func packageFuzzTargets(t *testing.T, dir string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read fuzz package %s: %v", dir, err)
	}
	targets := make(map[string]bool)
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && isFuzzTarget(function) {
				targets[function.Name.Name] = true
			}
		}
	}
	return targets
}

func isFuzzTarget(function *ast.FuncDecl) bool {
	if !isFuzzName(function.Name.Name) || function.Recv != nil ||
		function.Type.TypeParams != nil ||
		(function.Type.Results != nil && len(function.Type.Results.List) > 0) {
		return false
	}
	params := function.Type.Params
	if params == nil || len(params.List) != 1 || len(params.List[0].Names) > 1 {
		return false
	}
	pointer, ok := params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := pointer.X.(*ast.SelectorExpr)
	if ok {
		return selector.Sel.Name == "F"
	}
	name, ok := pointer.X.(*ast.Ident)
	return ok && name.Name == "F"
}

// isFuzzName matches the go command's target-name rule: Fuzz itself is valid, and
// the first rune after Fuzz may be anything except a lower-case letter.
func isFuzzName(name string) bool {
	rest, ok := strings.CutPrefix(name, "Fuzz")
	if !ok || rest == "" {
		return ok
	}
	first, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsLower(first)
}
