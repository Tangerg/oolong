// Package arch holds the tests that keep the library's layering true.
//
// The layering is what the library is, and it has two axes rather than one.
//
// The first is a ladder of abstraction: primitives, then headless, then kit, each
// less general than the one below it, with a dependency edge that only ever points
// down. Primitives know what a terminal is made of. Headless knows what a list does
// and not what one looks like. Kit decides what one looks like, and is the only ring
// anybody is expected to walk away from.
//
// The second is the host. [github.com/Tangerg/oolong/program] runs an interface, and
// it is not the top of the ladder — it is beside it. It may reach down to primitives
// for cells and events, and it must never know that headless or kit exist: the loop
// drives a Component, which is a method set, and a loop that imported the widgets
// would make every interface built on it depend on this library's taste in widgets.
// That rule is the one this file exists for.
//
// A ring nothing checks is a ring that drifts — the boundary held by discipline alone
// is the one somebody crosses once, quietly, in a hurry.
//
// Each ring declares the rings it must never import, rather than the ones it may. A
// list of wrong directions stays short and stays true; a matrix of allowed edges has to
// be edited every time a legitimate one appears, which teaches people to edit the test
// instead of the code.
package arch

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const modulePath = "github.com/Tangerg/oolong"

// The rings, longest prefix first so the first match wins.
var rings = []struct {
	prefix string
	name   string
}{
	{"primitives/", "primitives"},
	{"headless/", "headless"},
	{"kit/", "kit"},
	{"program/", "program"},
	{"ptytest/", "ptytest"},
	{"examples/", "examples"},
	{"internal/", "internal"},
}

// forbidden is, for each ring, the rings it may never import.
var forbidden = map[string][]string{
	// Cells, graphemes, input, layout and the terminal. The most general layer there
	// is: it knows what a terminal is made of and nothing about what anyone builds
	// from it.
	"primitives": {"headless", "kit", "program", "ptytest", "examples"},

	// Behaviour with no appearance: a list knows what the arrow keys do and not what
	// a selected row looks like. It may draw and answer input; it may not own a
	// goroutine, a terminal, or a program, and it may not depend on the one set of
	// answers kit gives — otherwise walking away from kit would mean walking away
	// from the behaviour too.
	"headless": {"kit", "program", "ptytest", "examples"},

	// One appearance for that behaviour, and the only ring anybody is expected to
	// replace. It may not own a goroutine or a program.
	"kit": {"program", "ptytest", "examples"},

	// The host: the loop, the frame schedule, the one goroutine. It is beside the
	// ladder rather than on top of it, and it must never know the widgets exist —
	// a loop that imported them would make every interface built on it inherit this
	// library's taste.
	"program": {"headless", "kit", "ptytest", "examples"},

	// The harness. It drives a terminal program from outside and nothing in the
	// library may lean on it: a harness the thing it tests depends on is a harness
	// nobody can change.
	"ptytest": {"headless", "kit", "program", "examples"},

	// The demonstrations. Everything may be imported here and nothing may import
	// them, which is what keeps an example from quietly becoming a dependency.
	"examples": {},

	// The tests that guard the rings. They import nothing.
	"internal": {"primitives", "headless", "kit", "program", "ptytest", "examples"},
}

// dependencies are the only third-party packages this library uses.
//
// The list is short on purpose, and it is a promise: a terminal interface library that
// drags a dependency tree behind it is one people work around instead of using. Adding
// to it is a decision, which is why it is written down where a test can fail.
var dependencies = []string{
	"github.com/rivo/uniseg",
	"github.com/mattn/go-runewidth",
	"golang.org/x/term",
	"golang.org/x/sys",
}

func TestEveryImportPointsDown(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()

	checked := 0
	walk(t, root, func(dir, path string) {
		from := ringOf(dir)
		if from == "" {
			return
		}
		checked++
		for _, imported := range imports(t, fset, path) {
			rest, ok := strings.CutPrefix(imported, modulePath+"/")
			if !ok {
				continue
			}
			to := ringOf(rest)
			if to == "" || to == from {
				continue
			}
			if slices.Contains(forbidden[from], to) {
				t.Errorf("%s (%s) imports %s (%s): %s must never depend on %s",
					dir, from, rest, to, from, to)
			}
		}
	})
	if checked == 0 {
		t.Fatal("no files were checked, so this test proves nothing")
	}
}

// TestTheLibraryDependsOnNothingElse is the promise that makes this library usable.
//
// A terminal interface library that pulls in a framework, a logger and a colour
// package is one people wrap rather than adopt. The list it is checked against is
// short, and lengthening it is a decision somebody has to make on purpose.
func TestTheLibraryDependsOnNothingElse(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()

	walk(t, root, func(dir, path string) {
		for _, imported := range imports(t, fset, path) {
			if !strings.Contains(imported, ".") {
				continue // a standard library package
			}
			if strings.HasPrefix(imported, modulePath) {
				continue
			}
			allowed := false
			for _, dep := range dependencies {
				if imported == dep || strings.HasPrefix(imported, dep+"/") {
					allowed = true
					break
				}
			}
			if !allowed {
				t.Errorf("%s imports %s, which is not one of this library's dependencies",
					dir, imported)
			}
		}
	})
}

// TestOnlyPrimitivesKnowWhatDrawsTheTerminal keeps the rendering substrate replaceable.
//
// The day it is worth changing what draws — a different width table, a different
// terminal package — the work is one ring's rather than the whole library's. That stays
// true only while nothing above that ring has quietly reached for it.
func TestOnlyPrimitivesKnowWhatDrawsTheTerminal(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()

	walk(t, root, func(dir, path string) {
		// ptytest is exempt, and the exemption is narrow: this rule is about what
		// draws a terminal, and ptytest drives one from the outside. It allocates a
		// pty and spawns a process into it, which is the operating system's business
		// and not the renderer's — and it is not part of the library, which is why
		// letting it reach for a syscall costs the substrate nothing.
		if ring := ringOf(dir); ring == "primitives" || ring == "ptytest" || ring == "" {
			return
		}
		for _, imported := range imports(t, fset, path) {
			for _, dep := range dependencies {
				if imported == dep || strings.HasPrefix(imported, dep+"/") {
					t.Errorf("%s imports %s: only primitives may know what draws the terminal",
						dir, imported)
				}
			}
		}
	})
}

// TestEveryDirectoryBelongsToARing catches a directory added without a rule to govern
// it, which would be unguarded from the day it appeared — the way an unguarded boundary
// always starts.
func TestEveryDirectoryBelongsToARing(t *testing.T) {
	root := moduleRoot(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read the module: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		ring := ringOf(entry.Name() + "/")
		if ring == "" {
			t.Errorf("%s belongs to no ring: add it to the rules, or put its code in a "+
				"ring that exists", entry.Name())
			continue
		}
		if _, ruled := forbidden[ring]; !ruled {
			t.Errorf("ring %q has no rule saying what it must not import", ring)
		}
	}
}

// TestTheRulesWouldActuallyRefuseSomething is the counter-example. A guard never shown
// to fail is a guard nobody knows is wired up: were the ring table empty, or its names
// out of step with what ringOf produces, every check above would pass by finding
// nothing.
func TestTheRulesWouldActuallyRefuseSomething(t *testing.T) {
	for _, tc := range []struct {
		from, to string
		refused  bool
	}{
		// The edges that make the ladder a ladder.
		{"primitives/grid", "headless", true},
		{"primitives/layout", "kit", true},
		{"primitives/text", "program", true},
		{"headless", "kit", true},
		{"headless", "program", true},
		{"kit", "program", true},

		// The rule the host exists under: the loop must not know the widgets. It is
		// the one edge that would look reasonable to add and would cost the most.
		{"program", "headless", true},
		{"program", "kit", true},

		// An example may be imported by nothing, which is what keeps a demonstration
		// from quietly becoming a dependency.
		{"kit", "examples/streaming", true},
		{"headless", "examples/streaming", true},

		// Nothing in the library leans on the harness that tests it.
		{"program", "ptytest", true},
		{"primitives/grid", "ptytest", true},

		// The edges the rings are made of.
		{"headless", "primitives/grid", false},
		{"kit", "headless", false},
		{"kit", "primitives/layout", false},
		{"program", "primitives/term", false},
		{"primitives/text", "primitives/grid", false},
		{"examples/streaming", "kit", false},
		{"examples/streaming", "program", false},
	} {
		from, to := ringOf(tc.from), ringOf(tc.to)
		if from == "" {
			t.Fatalf("%s belongs to no ring, so nothing about it is guarded", tc.from)
		}
		if to == "" {
			t.Fatalf("%s belongs to no ring, so importing it is unguarded", tc.to)
		}
		if got := slices.Contains(forbidden[from], to); got != tc.refused {
			verb := map[bool]string{true: "refused", false: "allowed"}
			t.Errorf("%s -> %s is %s, want it %s", from, to, verb[got], verb[tc.refused])
		}
	}
}

// ringOf names the ring a module-relative directory belongs to.
func ringOf(dir string) string {
	dir = filepath.ToSlash(dir)
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	for _, r := range rings {
		if strings.HasPrefix(dir, r.prefix) {
			return r.name
		}
	}
	return ""
}

// walk visits every production Go file, reporting its directory relative to the module
// root. Test files are skipped: a test may reach across rings for a fixture, and
// constraining that would buy nothing.
func walk(t *testing.T, root string, visit func(dir, path string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == "vendor" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		dir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		visit(filepath.ToSlash(dir), path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk the module: %v", err)
	}
}

// imports reads one file's import paths.
func imports(t *testing.T, fset *token.FileSet, path string) []string {
	t.Helper()
	file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		out = append(out, strings.Trim(spec.Path.Value, `"`))
	}
	return out
}

// moduleRoot is the directory holding go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the working directory")
		}
		dir = parent
	}
}
