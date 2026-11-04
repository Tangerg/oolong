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
	"strings"
	"testing"
)

func TestRenderLoaderResolvesModuleExportsAndBuildSelection(t *testing.T) {
	t.Setenv("GOWORK", "off")
	t.Setenv("GOPROXY", "off")
	directory := t.TempDir()
	writeRenderFixture(t, directory, map[string]string{
		"go.mod": `module sample.invalid/render

go 1.27.0

require sample.invalid/projection/v2 v2.0.0
replace sample.invalid/projection/v2 => ./dependency
`,
		"dependency/go.mod": "module sample.invalid/projection/v2\n\ngo 1.27.0\n",
		"dependency/value.go": `package foreign
			type Value[T any] struct { Data T }
			func (Value[T]) Commit() {}
			func Identity[T any](value T) T { return value }`,
		"render.go": `package sample
			import "sample.invalid/projection/v2"
			type Widget struct{}
			func (Widget) Draw() { value := foreign.Value[int]{Data: foreign.Identity(1)}; value.Commit() }`,
		"excluded.go": "//go:build ignore\n\npackage sample\nvar _ = unavailableOnThisBuild\n",
	})
	model := loadRenderPackage(t, directory, "sample")
	imports := model.checkedPackage.Imports()
	if len(imports) != 1 || imports[0].Path() != "sample.invalid/projection/v2" || imports[0].Name() != "foreign" {
		t.Fatalf("resolved imports = %v", imports)
	}
	if effects := renderEffectNames(model); len(effects) != 0 {
		t.Fatalf("pure imported Commit effects = %v", effects)
	}
	if model.checkedPackage.Scope().Lookup("unavailableOnThisBuild") != nil {
		t.Fatal("loader included an excluded compilation unit")
	}
}

func TestRenderLoaderRejectsBrokenImportsAndTypes(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
	}{
		{"import", "package sample\nimport _ \"sample.invalid/unavailable\"\n"},
		{"type", "package sample\nvar _ int = \"not an integer\"\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("GOWORK", "off")
			t.Setenv("GOPROXY", "off")
			directory := t.TempDir()
			writeRenderFixture(t, directory, map[string]string{
				"go.mod":    "module sample.invalid/render\n\ngo 1.27.0\n",
				"broken.go": test.source,
			})
			if _, err := readRenderPackage(t.Context(), directory, "sample"); err == nil || !strings.Contains(err.Error(), "go list rendering package") {
				t.Fatalf("broken %s load error = %v", test.name, err)
			}
		})
	}
}

func TestRenderModelRejectsTypeCheckFailure(t *testing.T) {
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "broken.go", `package sample
		type Widget struct{}
		func (Widget) Draw() { missing() }`, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	_, err = checkRenderPackage(set, []*ast.File{file}, "sample", "sample", types.Config{Importer: importer.Default()})
	if err == nil || !strings.Contains(err.Error(), "type-check rendering package") {
		t.Fatalf("type-check error = %v", err)
	}
}

func TestRenderEffectRuleUsesDeclaredPublicationOwner(t *testing.T) {
	dependency := loadRenderPackage(t, filepath.Join(repoRoot(t), "ui/core/publication"), "ui/core/publication").checkedPackage
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "sample.go", `package sample
		import "github.com/Tangerg/oolong/ui/core/publication"
		type Widget struct { publisher *publication.Publisher }
		func (w Widget) Draw() {
			commit := w.publisher.Commit
			commit(publication.Batch{}, 1)
			configure(w.publisher.SetGeometry)
			_ = w.publisher.Prepare()
			w.Commit()
			w.Print()
		}
		func configure[T any](set func(int, T) error) { var ground T; _ = set(80, ground) }
		func (Widget) Commit() {}
		func (Widget) Print() {}`, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	model, err := checkRenderPackage(set, []*ast.File{file}, "sample", "sample", types.Config{
		Importer: renderFixtureImporter{dependency: dependency, standard: importer.Default()},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"the publication operation " + modulePath + "/ui/core/publication.Publisher.Commit",
		"the publication operation " + modulePath + "/ui/core/publication.Publisher.SetGeometry",
	}
	if got := renderEffectNames(model); !slices.Equal(got, want) {
		t.Fatalf("effects = %v, want %v", got, want)
	}
}

type renderFixtureImporter struct {
	dependency *types.Package
	standard   types.Importer
}

func (i renderFixtureImporter) Import(path string) (*types.Package, error) {
	if path == i.dependency.Path() {
		return i.dependency, nil
	}
	return i.standard.Import(path)
}

func writeRenderFixture(t *testing.T, directory string, files map[string]string) {
	t.Helper()
	for name, source := range files {
		path := filepath.Join(directory, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
