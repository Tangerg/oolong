// Package arch holds the tests that keep the repository's layering true.
//
// There are two kinds of boundary here and they are enforced differently.
//
// A **module** boundary is enforced by Go and by what each module is allowed to
// depend on. `core` is the engine and carries the whole third-party dependency
// list; `components` is built on it and carries none at all, which is a stronger
// promise than the old one and a cheaper one to keep. `ptytest` is a harness above
// the product graph: its screen assertion reuses core's terminal-neutral ANSI and
// text primitives, while no production package can reach back to it. Splitting
// `ssh` is an optional transport above the runtime: it isolates the server stack so
// neither core nor applications that stay local acquire it. Splitting further would
// be mimicry: a module boundary costs version skew and buys an independent dependency
// set, and nothing inside `core` has one.
//
// A **ring** boundary is enforced here, because the compiler cannot see semantic
// direction inside a module. Core is a partial order: foundations, decoded
// protocols, interaction policy, derived models, host infrastructure and runtime
// orchestration. Inside `components`, behaviour must not reach for the one appearance
// this repository happens to ship.
//
// Each ring declares only its direct dependencies. Imports may follow that DAG
// transitively and every other direction is refused. The graph is both the design
// and the check: adding a ring means adding one node and its immediate lower edges,
// not teaching every existing ring the new name.
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
// dependency — markdown, syntax highlighting, mathematics — becomes a module of its own with
// its own list, and neither of these two is touched.
var modules = map[string][]string{
	"core": {
		"github.com/rivo/uniseg",
		"github.com/mattn/go-runewidth",
		"golang.org/x/term",
		"golang.org/x/sys",
	},
	"components": nil,
	// The modules the boundary was drawn for. Markdown and mathematics need parsers,
	// and highlighting needs a lexer per language and a palette per theme; each is a tree of somebody
	// else's code, and this is where they are allowed to be — which is what the two
	// modules above buy by refusing them.
	"markdown":  {"github.com/yuin/goldmark"},
	"highlight": {"github.com/alecthomas/chroma"},
	"latex":     {"codeberg.org/go-latex/latex"},
	"internal":  nil,
	"ptytest":   {"golang.org/x/sys"},
	"ssh":       {"charm.land/ssh"},
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
	{"core/programtest/", "testharness"},
	{"core/program/", "runtime"},
	{"core/term/", "infrastructure"},
	{"core/text/", "model"},
	{"core/keymap/", "interaction"},
	{"core/input/", "protocol"},
	{"core/present/", "coordination"},
	{"core/", "foundation"},
	{"components/headless/", "headless"},
	{"components/kit/", "kit"},
	{"markdown/", "markdown"},
	{"highlight/", "highlight"},
	{"latex/", "latex"},
	{"ptytest/", "harness"},
	{"ssh/", "ssh"},
	{"examples/", "examples"},
	{"internal/", "internal"},
}

// dependencies is the repository's semantic DAG. Each entry names immediate
// dependencies only; mayImport computes the transitive closure. An empty entry is
// an explicit promise that the ring imports no repository code.
var dependencies = map[string][]string{
	// Algorithms, escape encodings, geometry and cell values know nothing above
	// them.
	"foundation": nil,

	// Decoded input is built from foundational byte protocols. Key maps add caller
	// policy, but terminal adapters and runtime orchestration never acquire it.
	"protocol":    {"foundation"},
	"interaction": {"protocol"},

	// Styled text and frame coordination are independent derivations over the
	// foundation, not vocabulary for one another.
	"model":        {"foundation"},
	"coordination": {"foundation"},

	// The OS terminal adapts decoded protocols. Runtime composes that adapter with
	// frame coordination, without learning application interaction policy or styled
	// content.
	"infrastructure": {"protocol"},
	"runtime":        {"infrastructure", "coordination"},

	// The in-process harness drives the public runtime from above. Product code has
	// no path back to it, while the harness may use the standard terminal writer as
	// the transport implementation hidden behind program's consumer interface.
	"testharness": {"runtime"},

	// Headless components combine interaction policy with derived text. Kit adds one
	// appearance and nothing about the host that eventually runs it.
	"headless": {"interaction", "model"},
	"kit":      {"headless"},

	// Optional content modules terminate at the common text model and remain peers:
	// no parser, highlighter or typesetter owns another.
	"markdown":  {"model"},
	"highlight": {"model"},
	"latex":     {"model"},

	// A harness is outside the product graph and may inspect terminal-neutral text
	// and ANSI syntax from above. Demonstrations are the composition root and may use
	// every public branch, but no production ring depends on either test layer.
	"harness":  {"model"},
	"ssh":      {"runtime"},
	"examples": {"testharness", "kit", "markdown", "highlight", "latex", "harness", "ssh"},

	// The architecture module contains only tests and imports no production ring.
	"internal": nil,
}

// mayImport reports whether from can reach to by following dependency edges.
func mayImport(from, to string) bool {
	if from == to {
		return true
	}
	seen := map[string]bool{from: true}
	var reaches func(string) bool
	reaches = func(ring string) bool {
		for _, dependency := range dependencies[ring] {
			if dependency == to {
				return true
			}
			if seen[dependency] {
				continue
			}
			seen[dependency] = true
			if reaches(dependency) {
				return true
			}
		}
		return false
	}
	return reaches(from)
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
			if !mayImport(from, to) {
				t.Errorf("%s (%s) imports %s (%s): %s has no dependency path to %s",
					dir, from, rest, to, from, to)
			}
		}
	})
	if checked == 0 {
		t.Fatal("no files were checked, so this test proves nothing")
	}
}

// TestDocumentationPointsDown applies the same knowledge rule to API prose. An
// import cycle is not required for a lower package to become coupled to an upper
// concrete type: naming that type in its contract makes renames and replacements
// flow downward just as surely.
func TestDocumentationPointsDown(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	targets := []string{
		"core/anim", "core/ansi", "core/clipboard", "core/diff", "core/fuzzy",
		"core/graphics", "core/grid", "core/input", "core/layout", "core/link",
		"core/keymap", "core/present", "core/program", "core/programtest", "core/term", "core/text",
		"components/headless", "components/kit", "markdown", "highlight", "latex", "ptytest", "ssh",
	}

	walk(t, root, func(dir, path string) {
		from := ringOf(dir)
		for _, comment := range comments(t, fset, path) {
			for _, target := range targets {
				to := ringOf(target)
				if to == "" || mayImport(from, to) {
					continue
				}
				name := filepath.Base(target)
				if strings.Contains(comment, "["+name+".") ||
					strings.Contains(comment, modulePath+"/"+target) {
					t.Errorf("%s documents %s (%s -> %s): lower contracts must not name upper packages",
						dir, target, from, to)
				}
			}
		}
	})
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

// TestOnlyCoreOwnsCoreDependencies keeps implementation dependencies below the
// public component and extension modules.
//
// The day it is worth changing what draws — a different width table, a different
// terminal package — the work is one ring's rather than the whole repository's.
// That stays true only while nothing above that ring has quietly reached for it.
func TestOnlyCoreOwnsCoreDependencies(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()

	walk(t, root, func(dir, path string) {
		// The harness is exempt, and the exemption is narrow: this rule is about
		// what draws a terminal, and ptytest drives one from the outside. It
		// allocates a pty and spawns a process into it, which is the operating
		// system's business and not the renderer's.
		switch ringOf(dir) {
		case "foundation", "protocol", "interaction", "model", "coordination", "infrastructure", "runtime", "harness", "":
			return
		}
		for _, imported := range imports(t, fset, path) {
			for _, dep := range modules["core"] {
				if imported == dep || strings.HasPrefix(imported, dep+"/") {
					t.Errorf("%s imports %s: only core may own a core implementation dependency",
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

func TestDependencyGraphIsCompleteAndAcyclic(t *testing.T) {
	known := make(map[string]bool, len(rings))
	for _, ring := range rings {
		known[ring.name] = true
	}
	for ring := range known {
		if _, declared := dependencies[ring]; !declared {
			t.Errorf("ring %s has no dependency declaration", ring)
		}
	}
	for ring, direct := range dependencies {
		if !known[ring] {
			t.Errorf("dependency graph declares unknown ring %s", ring)
		}
		for _, dependency := range direct {
			if !known[dependency] {
				t.Errorf("%s depends on unknown ring %s", ring, dependency)
			}
		}
	}

	const (
		visiting = 1
		visited  = 2
	)
	state := make(map[string]uint8, len(dependencies))
	var visit func(string)
	visit = func(ring string) {
		switch state[ring] {
		case visiting:
			t.Errorf("dependency graph contains a cycle through %s", ring)
			return
		case visited:
			return
		}
		state[ring] = visiting
		for _, dependency := range dependencies[ring] {
			visit(dependency)
		}
		state[ring] = visited
	}
	for ring := range dependencies {
		visit(ring)
	}
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

		// The rule the host exists under: the runtime must not know the widgets. The
		// module graph would not catch it — core could require components and Go
		// would allow it — so this is the one that has to be checked here.
		{"core/program", "components/headless", true},
		{"core/program", "components/kit", true},
		{"core/program", "core/programtest", true},

		// And the substrate must not reach for the runtime that drives it.
		{"core/grid", "core/program", true},
		{"core/text", "core/program", true},
		{"core/grid", "core/present", true},
		{"core/input", "core/keymap", true},
		{"core/keymap", "core/input", false},
		{"core/term", "core/keymap", true},
		{"core/program", "core/keymap", true},

		// Nor may a widget: one that needed a runtime to exist could not be tested
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
		{"latex", "markdown", true},
		{"markdown", "latex", true},
		{"latex", "core/text", false},
		{"ptytest", "core/text", false},

		// Nothing leans on the harness, and nothing imports a demonstration.
		{"core/program", "ptytest", true},
		{"components/kit", "ptytest", true},
		{"components/kit", "core/programtest", true},
		{"components/kit", "examples/streaming", true},

		// The edges the rings are made of.
		{"components/headless", "core/grid", false},
		{"components/kit", "components/headless", false},
		{"components/kit", "core/layout", false},
		{"core/program", "core/term", false},
		{"core/programtest", "core/program", false},
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
		if got := !mayImport(from, to); got != tc.refused {
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
	return !isRoot && (name == "node_modules" || name == "vendor" || strings.HasPrefix(name, "."))
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

// comments returns each comment group in a production file without code or string
// literals, so architectural prose checks do not mistake examples for imports.
func comments(t *testing.T, fset *token.FileSet, path string) []string {
	t.Helper()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := make([]string, len(file.Comments))
	for i, group := range file.Comments {
		out[i] = group.Text()
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
