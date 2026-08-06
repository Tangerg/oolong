// Package arch holds the tests that keep the repository's layering true.
//
// There are two kinds of boundary here and they are enforced differently.
//
// A **module** boundary is enforced by Go and by what each module is allowed to
// depend on. `core` is the engine and carries the whole third-party dependency
// list; `components` is built on it and carries none at all, which is a stronger
// promise than the old one and a cheaper one to keep. `ptytest` is a harness that
// depends on neither. Splitting further would be mimicry: a module boundary costs
// version skew and buys an independent dependency set, and nothing inside `core`
// has one.
//
// A **ring** boundary is enforced here, because the compiler cannot see it. Inside
// `core` the substrate must not reach for the loop that drives it. Inside
// `components` the behaviour must not reach for the one appearance this repository
// happens to ship, or walking away from that appearance would mean walking away
// from the behaviour too.
//
// Each ring declares the rings it must never import, rather than the ones it may. A
// list of wrong directions stays short and stays true; a matrix of allowed edges has
// to be edited every time a legitimate one appears, which teaches people to edit the
// test instead of the code.
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

// modules are every module in the repository, by directory, with the third-party
// packages each is allowed to import.
//
// The lists are the promise. `core` carries all four and nothing else; a terminal
// interface library that drags a dependency tree behind it is one people work
// around instead of using. `components` carries none, which is what makes it plain
// that everything it needs is already in `core`. Anything wanting a heavy
// dependency — markdown, syntax highlighting — becomes a module of its own with
// its own list, and neither of these two is touched.
var modules = map[string][]string{
	"core": {
		"github.com/rivo/uniseg",
		"github.com/mattn/go-runewidth",
		"golang.org/x/term",
		"golang.org/x/sys",
	},
	"components": nil,
	// The modules the boundary was drawn for. Markdown needs a parser and highlighting
	// needs a lexer per language and a palette per theme; both are trees of somebody
	// else's code, and this is where they are allowed to be — which is what the two
	// modules above buy by refusing them.
	"markdown":  {"github.com/yuin/goldmark"},
	"highlight": {"github.com/alecthomas/chroma"},
	"internal":  nil,
	"ptytest":   {"golang.org/x/sys"},
	"examples":  nil,
}

// The repository root is deliberately not a module. A module is a unit of
// distribution and the root distributes nothing, so one there would be an empty
// thing to tag and an empty page for anyone who went looking. Every Go file
// therefore belongs to one of the modules above, and moduleOf says so.

// The rings, longest prefix first so the first match wins.
var rings = []struct {
	prefix string
	name   string
}{
	{"core/program/", "host"},
	// Frame pacing is the loop's business and not the terminal's: nothing else
	// uses it, and "when to draw" is a question about what someone is building
	// rather than about what a terminal is made of.
	{"core/present/", "host"},
	{"core/", "substrate"},
	{"components/headless/", "headless"},
	{"components/kit/", "kit"},
	{"markdown/", "markdown"},
	{"highlight/", "highlight"},
	{"ptytest/", "harness"},
	{"examples/", "examples"},
	{"internal/", "internal"},
}

// forbidden is, for each ring, the rings it may never import.
var forbidden = map[string][]string{
	// Cells, graphemes, input, layout and the terminal. The most general layer
	// there is: it knows what a terminal is made of and nothing about what anyone
	// builds from it — including the loop that drives it.
	"substrate": {"host", "headless", "kit", "harness", "examples", "markdown", "highlight"},

	// The loop, the frame schedule, the one goroutine. It is beside the ladder
	// rather than on top of it, and it must never know the widgets exist: it drives
	// a Component, which is a method set, and a loop that imported the widgets
	// would make every interface built on it inherit this repository's taste.
	//
	// The module graph does not catch this on its own — core could require
	// components and Go would allow it — so this is the rule this file exists for
	// above all the others.
	"host": {"headless", "kit", "harness", "examples", "markdown", "highlight"},

	// Behaviour with no appearance: a list knows what the arrow keys do and not
	// what a selected row looks like. It may not depend on the one set of answers
	// kit gives, or walking away from kit would mean walking away from the
	// behaviour too — and it may not depend on the host, because a widget that
	// needed a loop to exist could not be tested without starting one.
	"headless": {"host", "kit", "harness", "examples", "markdown", "highlight"},

	// One appearance for that behaviour, and the only ring anybody is expected to
	// replace. It may not reach for markdown either: kit is part of a module that
	// promises no dependencies at all, and importing the module that carries a parser
	// would make that promise everyone else's problem.
	"kit": {"host", "harness", "examples", "markdown", "highlight"},

	// Markdown, which is a module of its own because it carries a parser. It is
	// beside the ladder rather than on it: it turns text into the substrate's own
	// lines, so anything that can draw those can draw a document without either of
	// them knowing about the other.
	//
	// It may not import the highlighter either, and that is the point of the seam it
	// has instead: a document with no highlighter draws code in one style, and a
	// program that wants one pays for it deliberately.
	"markdown": {"host", "headless", "kit", "harness", "examples", "highlight"},

	// Highlighting, which is a module of its own for the same reason and produces the
	// same thing: the substrate's own lines.
	"highlight": {"host", "headless", "kit", "harness", "examples", "markdown"},

	// The harness. Nothing here may lean on it: a harness that the thing it tests
	// depends on is a harness nobody can change.
	"harness": {"substrate", "host", "headless", "kit", "examples", "markdown", "highlight"},

	// The demonstrations. Everything may be imported here and nothing may import
	// them, which is what keeps an example from quietly becoming a dependency.
	"examples": {},

	// The tests that guard the rings. They import nothing.
	"internal": {
		"substrate", "host", "headless", "kit", "harness", "examples", "markdown", "highlight",
	},
}

func TestEveryImportPointsDown(t *testing.T) {
	root := repoRoot(t)
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

// TestEachModuleDependsOnWhatItSaidItWould is the promise that makes this usable.
//
// A terminal interface library that pulls in a framework, a logger and a colour
// package is one people wrap rather than adopt. Splitting into modules made the
// promise sharper rather than looser: the list belongs to a module now, so
// `components` can promise nothing at all and a future markdown module can carry
// goldmark without either of them noticing.
func TestEachModuleDependsOnWhatItSaidItWould(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()

	walk(t, root, func(dir, path string) {
		module, allowed, ok := moduleOf(dir)
		if !ok {
			t.Errorf("%s is in no module, so nothing governs what it may import", dir)
			return
		}
		for _, imported := range imports(t, fset, path) {
			if !strings.Contains(imported, ".") || strings.HasPrefix(imported, modulePath) {
				continue // the standard library, or our own
			}
			if !slices.ContainsFunc(allowed, func(dep string) bool {
				return imported == dep || strings.HasPrefix(imported, dep+"/")
			}) {
				t.Errorf("%s imports %s, which the %s module does not depend on",
					dir, imported, module)
			}
		}
	})
}

// TestOnlyTheSubstrateKnowsWhatDrawsTheTerminal keeps the rendering layer
// replaceable.
//
// The day it is worth changing what draws — a different width table, a different
// terminal package — the work is one ring's rather than the whole repository's.
// That stays true only while nothing above that ring has quietly reached for it.
func TestOnlyTheSubstrateKnowsWhatDrawsTheTerminal(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()

	walk(t, root, func(dir, path string) {
		// The harness is exempt, and the exemption is narrow: this rule is about
		// what draws a terminal, and ptytest drives one from the outside. It
		// allocates a pty and spawns a process into it, which is the operating
		// system's business and not the renderer's.
		switch ringOf(dir) {
		case "substrate", "harness", "":
			return
		}
		for _, imported := range imports(t, fset, path) {
			for _, dep := range modules["core"] {
				if imported == dep || strings.HasPrefix(imported, dep+"/") {
					t.Errorf("%s imports %s: only the substrate may know what draws the terminal",
						dir, imported)
				}
			}
		}
	})
}

// TestEveryModuleIsDeclaredAndEveryDeclaredModuleExists catches a module added
// without a rule to govern its dependencies, which would be unguarded from the day
// it appeared — the way an unguarded boundary always starts.
func TestEveryModuleIsDeclaredAndEveryDeclaredModuleExists(t *testing.T) {
	root := repoRoot(t)

	found := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && skipped(d.Name(), path == root) {
			return filepath.SkipDir
		}
		if d.Name() != "go.mod" {
			return nil
		}
		dir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		found[filepath.ToSlash(dir)] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walk the repository: %v", err)
	}

	for dir := range found {
		if _, declared := modules[dir]; !declared {
			t.Errorf("the module in %s has no dependency rule: add it to the table, or "+
				"put its code in a module that exists", dir)
		}
	}
	for dir := range modules {
		if !found[dir] {
			t.Errorf("the table declares a module in %s and there is no go.mod there", dir)
		}
	}
}

// TestEveryDirectoryBelongsToARing catches a package added outside the layering.
func TestEveryDirectoryBelongsToARing(t *testing.T) {
	root := repoRoot(t)
	walk(t, root, func(dir, _ string) {
		if ringOf(dir) == "" {
			t.Errorf("%s belongs to no ring: add it to the rules, or put its code in a "+
				"ring that exists", dir)
		}
	})
}

// TestTheRulesWouldActuallyRefuseSomething is the counter-example. A guard never
// shown to fail is a guard nobody knows is wired up: were the ring table empty, or
// its names out of step with what ringOf produces, every check above would pass by
// finding nothing.
func TestTheRulesWouldActuallyRefuseSomething(t *testing.T) {
	for _, tc := range []struct {
		from, to string
		refused  bool
	}{
		// The edges that make the ladder a ladder.
		{"core/grid", "components/headless", true},
		{"core/layout", "components/kit", true},
		{"components/headless", "components/kit", true},

		// The rule the host exists under: the loop must not know the widgets. The
		// module graph would not catch it — core could require components and Go
		// would allow it — so this is the one that has to be checked here.
		{"core/program", "components/headless", true},
		{"core/program", "components/kit", true},

		// And the substrate must not reach for the loop that drives it.
		{"core/grid", "core/program", true},
		{"core/text", "core/program", true},
		{"core/grid", "core/present", true},

		// Nor may a widget: one that needed a loop to exist could not be tested
		// without starting one, and this is the edge the table used to allow.
		{"components/headless", "core/program", true},
		{"components/kit", "core/program", true},
		{"components/headless", "core/present", true},

		// Nothing that promises a short dependency list may reach for the module that
		// carries a parser, and that module may not reach back up the ladder.
		{"components/kit", "markdown", true},
		{"core/text", "markdown", true},
		{"markdown", "components/headless", true},
		{"markdown", "core/program", true},
		{"markdown", "core/text", false},
		{"markdown", "highlight", true},
		{"highlight", "markdown", true},
		{"highlight", "core/text", false},

		// Nothing leans on the harness, and nothing imports a demonstration.
		{"core/program", "ptytest", true},
		{"components/kit", "ptytest", true},
		{"components/kit", "examples/streaming", true},

		// The edges the rings are made of.
		{"components/headless", "core/grid", false},
		{"components/kit", "components/headless", false},
		{"components/kit", "core/layout", false},
		{"core/program", "core/term", false},
		{"core/text", "core/grid", false},
		{"examples/streaming", "components/kit", false},
		{"examples/streaming", "core/program", false},
		{"examples/streaming", "ptytest", false},
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
			t.Errorf("%s -> %s (%s -> %s) is %s, want it %s",
				tc.from, tc.to, from, to, verb[got], verb[tc.refused])
		}
	}
}

// ringOf names the ring a repository-relative directory belongs to.
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

// moduleOf names the module a repository-relative directory belongs to, and what
// that module may import. The longest matching prefix wins, so a module nested
// inside another is attributed to the inner one.
//
// A directory belonging to no module is reported as such rather than defaulted:
// with no module at the root there is no module for it to fall back to, and code
// nothing governs is exactly what these tests exist to notice.
func moduleOf(dir string) (string, []string, bool) {
	dir = filepath.ToSlash(dir)
	best, allowed := "", []string(nil)
	for name, deps := range modules {
		if dir != name && !strings.HasPrefix(dir, name+"/") {
			continue
		}
		if len(name) > len(best) {
			best, allowed = name, deps
		}
	}
	return best, allowed, best != ""
}

// skipped reports whether a directory is none of this test's business.
func skipped(name string, isRoot bool) bool {
	return !isRoot && (name == "vendor" || strings.HasPrefix(name, "."))
}

// walk visits every production Go file in the repository, reporting its directory
// relative to the root. Test files are skipped: a test may reach across rings for a
// fixture, and constraining that would buy nothing.
func walk(t *testing.T, root string, visit func(dir, path string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipped(d.Name(), path == root) {
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
		t.Fatalf("walk the repository: %v", err)
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

// repoRoot is the directory holding go.work, which is the one thing that marks the
// top of a multi-module repository rather than the top of a module.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.work above the working directory, so the repository has no top")
		}
		dir = parent
	}
}
