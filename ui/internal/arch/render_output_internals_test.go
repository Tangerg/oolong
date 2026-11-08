package arch

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"testing"
)

func TestRenderingRejectsOutputAndDispatchOperations(t *testing.T) {
	for _, test := range []struct {
		path     string
		receiver string
		call     string
	}{
		{path: "ui/core/grid", receiver: "Inline", call: "Flush(nil)"},
		{path: "ui/core/grid", receiver: "Inline", call: "Finish(nil)"},
		{path: "ui/core/program", receiver: "Dispatcher", call: "Dispatch(nil, nil)"},
	} {
		t.Run(test.receiver+"."+test.call, func(t *testing.T) {
			dependency := loadRenderPackage(t, filepath.Join(repoRoot(t), test.path), test.path).checkedPackage
			set := token.NewFileSet()
			file, err := parser.ParseFile(set, "sample.go", `package sample
import owner "`+dependency.Path()+`"
type Widget struct { output *owner.`+test.receiver+` }
func (w Widget) Draw() { _ = w.output.`+test.call+` }
`, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			model, err := checkRenderPackage(set, []*ast.File{file}, "sample", "sample", types.Config{
				Importer: renderFixtureImporter{dependency: dependency, standard: importer.Default()},
			})
			if err != nil {
				t.Fatal(err)
			}
			if effects := renderEffectNames(model); len(effects) != 1 {
				t.Fatalf("%s.%s effects = %v, want one owner operation", test.receiver, test.call, effects)
			}
		})
	}
}

func TestGridExternalWriterCapabilitiesHaveEffectDeclarations(t *testing.T) {
	dependency := loadRenderPackage(t, filepath.Join(repoRoot(t), "ui/core/grid"), "ui/core/grid").checkedPackage
	var writer types.Type
	for _, imported := range dependency.Imports() {
		if imported.Path() == "io" {
			writer = imported.Scope().Lookup("Writer").Type()
			break
		}
	}
	if writer == nil {
		t.Fatal("grid exposes no io.Writer type for output capability review")
	}
	checked := false
	for _, name := range dependency.Scope().Names() {
		object, ok := dependency.Scope().Lookup(name).(*types.TypeName)
		if !ok || !object.Exported() {
			continue
		}
		methods := types.NewMethodSet(types.NewPointer(object.Type()))
		for selection := range methods.Methods() {
			method := selection.Obj().(*types.Func)
			if !method.Exported() {
				continue
			}
			parameters := method.Type().(*types.Signature).Params()
			for parameter := range parameters.Variables() {
				// PaintContext also implements io.Writer but can only change its borrowed frame.
				if types.Identical(parameter.Type(), writer) {
					checked = true
					if effectOwnedCall(method) == "" {
						t.Errorf("%s.%s accepts an output capability without an effect declaration", name, method.Name())
					}
					break
				}
			}
		}
	}
	if !checked {
		t.Fatal("no grid output capabilities were checked")
	}
}
