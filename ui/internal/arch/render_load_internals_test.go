package arch

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type listedRenderPackage struct {
	Dir             string
	ImportPath      string
	Export          string
	CompiledGoFiles []string
	DepOnly         bool
	Module          *struct{ GoVersion string }
	Error           *struct{ Err string }
	DepsErrors      []struct{ Err string }
}

func loadRenderPackage(t *testing.T, directory, packageKey string) renderPackageModel {
	t.Helper()
	model, err := readRenderPackage(t.Context(), directory, packageKey)
	if err != nil {
		t.Fatal(err)
	}
	return model
}

func readRenderPackage(ctx context.Context, directory, packageKey string) (renderPackageModel, error) {
	command := exec.CommandContext(ctx, "go", "list", "-deps", "-export", "-compiled", "-json", ".")
	command.Dir = directory
	output, err := command.Output()
	if err != nil {
		var detail string
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			detail = strings.TrimSpace(string(exit.Stderr))
		}
		return renderPackageModel{}, fmt.Errorf("go list rendering package %s: %w: %s", directory, err, detail)
	}
	listed, exports, err := decodeRenderPackages(output)
	if err != nil {
		return renderPackageModel{}, err
	}
	set := token.NewFileSet()
	files := make([]*ast.File, 0, len(listed.CompiledGoFiles))
	for _, name := range listed.CompiledGoFiles {
		if !filepath.IsAbs(name) {
			name = filepath.Join(listed.Dir, name)
		}
		file, err := parser.ParseFile(set, name, nil, parser.SkipObjectResolution)
		if err != nil {
			return renderPackageModel{}, fmt.Errorf("parse rendering package %s: %w", name, err)
		}
		files = append(files, file)
	}
	config := types.Config{Importer: importer.ForCompiler(set, "gc", func(path string) (io.ReadCloser, error) {
		exported, exists := exports[path]
		if !exists {
			return nil, fmt.Errorf("go list supplied no export data for rendering dependency %s", path)
		}
		//nolint:gosec // Only go list's compiler export paths reach this lookup.
		return os.Open(exported)
	})}
	if listed.Module != nil && listed.Module.GoVersion != "" {
		config.GoVersion = "go" + listed.Module.GoVersion
	}
	return checkRenderPackage(set, files, listed.ImportPath, packageKey, config)
}

func decodeRenderPackages(output []byte) (listedRenderPackage, map[string]string, error) {
	var root listedRenderPackage
	exports := make(map[string]string)
	decoder := jsontext.NewDecoder(bytes.NewReader(output))
	for {
		var listed listedRenderPackage
		if err := json.UnmarshalDecode(decoder, &listed); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return root, nil, fmt.Errorf("decode rendering package graph: %w", err)
		}
		if listed.Error != nil || len(listed.DepsErrors) > 0 {
			return root, nil, fmt.Errorf("incomplete rendering package %s: %v %v", listed.ImportPath, listed.Error, listed.DepsErrors)
		}
		if listed.Export != "" {
			exports[listed.ImportPath] = listed.Export
		}
		if !listed.DepOnly {
			if root.ImportPath != "" {
				return root, nil, fmt.Errorf("rendering package query selected both %s and %s", root.ImportPath, listed.ImportPath)
			}
			root = listed
		}
	}
	if root.ImportPath == "" || len(root.CompiledGoFiles) == 0 {
		return root, nil, errors.New("rendering package query returned no compiled source package")
	}
	return root, exports, nil
}

func checkRenderPackage(set *token.FileSet, files []*ast.File, importPath, packageKey string, config types.Config) (renderPackageModel, error) {
	info := &types.Info{
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	checked, err := config.Check(importPath, set, files, info)
	if err != nil {
		return renderPackageModel{}, fmt.Errorf("type-check rendering package %s: %w", importPath, err)
	}
	return newRenderModel(set, files, checked, info, packageKey), nil
}

func renderModelFromSource(t *testing.T, source string) renderPackageModel {
	t.Helper()
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "sample.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	model, err := checkRenderPackage(set, []*ast.File{file}, "sample", "sample", types.Config{Importer: importer.Default()})
	if err != nil {
		t.Fatal(err)
	}
	return model
}
