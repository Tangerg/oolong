// Package arch derives import facts and checks the semantic dependency graph.
// Modules govern distribution and optional third-party dependencies; UI layers
// remain real package boundaries inside the single ui distribution module.
package arch

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const modulePath = "github.com/Tangerg/oolong"

// dependencyRules grants third-party imports to modules in scripts/modules.sh's
// inventory. Modules without a grant may import no third-party packages; a grant
// cannot establish module membership.
//
// Optional content and transport modules isolate their dependencies so core and
// components do not acquire them.
var dependencyRules = map[string][]string{
	"ui": {
		"github.com/clipperhouse/uax29/v2/graphemes",
		"github.com/mattn/go-runewidth",
		"golang.org/x/term",
		"golang.org/x/sys",
	},
	"extensions/markdown":  {"github.com/yuin/goldmark"},
	"extensions/mermaid":   {"golang.org/x/sys"},
	"extensions/highlight": {"github.com/alecthomas/chroma"},
	"extensions/latex":     {"codeberg.org/go-latex/latex"},
	"extensions/ssh":       {"charm.land/ssh"},
	"examples":             {"golang.org/x/sys"},
}

// The repository root is deliberately not a module. A module is a unit of
// distribution and the root distributes nothing, so one there would be an empty
// thing to tag and an empty page for anyone who went looking. Every Go file
// therefore belongs to a module in the derived inventory, and moduleOf says so.

// The rings, longest prefix first so the first match wins.
var rings = []struct {
	prefix string
	name   string
}{
	{"ui/uitest/ptytest/", "harness"},
	{"ui/uitest/", "testharness"},
	{"ui/core/program/", "runtime"},
	{"ui/core/scene/", "scene"},
	{"ui/core/publication/", "publication"},
	{"ui/automation/", "automation"},
	{"ui/core/term/", "infrastructure"},
	{"ui/core/text/", "model"},
	{"ui/core/content/", "content"},
	{"ui/core/keymap/", "interaction"},
	{"ui/core/input/", "protocol"},
	{"ui/core/present/", "coordination"},
	{"ui/core/", "foundation"},
	{"ui/base/", "base"},
	{"ui/kit/", "kit"},
	{"ui/internal/identity/", "componentbase"},
	{"ui/internal/focus/", "componentbase"},
	{"ui/internal/fifo/", "foundation"},
	{"extensions/markdown/", "extensions/markdown"},
	{"extensions/mermaid/", "extensions/mermaid"},
	{"extensions/highlight/", "extensions/highlight"},
	{"extensions/latex/", "extensions/latex"},
	{"extensions/ssh/", "extensions/ssh"},
	{"examples/", "examples"},
	{"ui/internal/", "internal"},
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
	"content":      {"foundation"},
	"coordination": {"foundation"},

	// The OS terminal adapts decoded protocols. Runtime composes that adapter with
	// frame coordination, without learning application interaction policy or styled
	// content.
	"infrastructure": {"protocol"},
	"runtime":        {"infrastructure", "coordination", "scene", "publication"},
	"scene":          {"protocol"},
	"publication":    {"foundation"},
	"automation":     {"scene"},

	// The in-process harness drives the public runtime from above. Product code has
	// no path back to it, while the harness may use the standard terminal writer as
	// the transport implementation hidden behind program's consumer interface.
	"testharness": {"runtime"},

	// Headless components combine interaction policy with derived text. Kit adds one
	// appearance and nothing about the host that eventually runs it.
	"componentbase": nil,
	"base":          {"componentbase", "interaction", "model", "scene", "publication", "content"},
	"kit":           {"componentbase", "base"},

	// Optional content modules terminate at the common text model and remain peers:
	// no parser, highlighter or typesetter owns another.
	"extensions/markdown":  {"model", "content"},
	"extensions/mermaid":   nil,
	"extensions/highlight": {"model"},
	"extensions/latex":     {"model"},

	// A harness is outside the product graph and may inspect terminal-neutral text
	// and ANSI syntax from above. Demonstrations are the composition root and may use
	// every public branch, but no production ring depends on either test layer.
	"harness":        {"model"},
	"extensions/ssh": {"runtime"},
	"examples":       {"extensions/mermaid", "content", "testharness", "kit", "automation", "extensions/markdown", "extensions/highlight", "extensions/latex", "harness", "extensions/ssh"},

	// The architecture module contains only tests and imports no production ring.
	"internal": nil,
}

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

// TestEachModuleDependsOnWhatItSaidItWould is the promise that makes this usable.
//
// A terminal interface library that pulls in a framework, a logger and a colour
// package is one people wrap rather than adopt. Splitting into modules made the
// promise sharper rather than looser: the list belongs to a module now, so
// `components` can promise nothing at all and the markdown module can carry
// goldmark without either of them noticing.
func TestEachModuleDependsOnWhatItSaidItWould(t *testing.T) {
	root := repoRoot(t)
	inventory := moduleScript(t, root)
	fset := token.NewFileSet()

	walk(t, root, func(dir, path string) {
		module, allowed, ok := moduleOf(dir, inventory)
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
			// These adapters own external Windows processes, not terminal text,
			// input or transport. Other packages and core dependencies stay barred.
			if imported == "golang.org/x/sys/windows" && (dir == "extensions/mermaid" || dir == "examples/mermaid") {
				continue
			}
			for _, dep := range dependencyRules["ui"] {
				if imported == dep || strings.HasPrefix(imported, dep+"/") {
					t.Errorf("%s imports %s: only core may own a core implementation dependency",
						dir, imported)
				}
			}
		}
	})
}

// TestModuleInventoryOwnsEveryManifestAndDependencyRule rejects unregistered
// manifests and stale dependency grants without maintaining another module list.
func TestModuleInventoryOwnsEveryManifestAndDependencyRule(t *testing.T) {
	root := repoRoot(t)
	inventory := moduleScript(t, root)

	var found []string
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
		found = append(found, filepath.ToSlash(dir))
		return nil
	})
	if err != nil {
		t.Fatalf("walk the repository: %v", err)
	}

	slices.Sort(found)
	if !slices.Equal(found, inventory) {
		t.Errorf("repository go.mod directories = %q, want scripts/modules.sh inventory %q", found, inventory)
	}
	for dir := range dependencyRules {
		if !slices.Contains(inventory, dir) {
			t.Errorf("dependency rule for %s has no module in scripts/modules.sh", dir)
		}
	}
}

// TestModuleScriptCoversEveryDeclaredModule verifies the executable inventory
// against Go's workspace and package facts.
func TestModuleScriptCoversEveryDeclaredModule(t *testing.T) {
	root := repoRoot(t)
	all := moduleScript(t, root)
	workspace := workspaceModules(t, root)
	if !slices.Equal(all, workspace) {
		t.Fatalf("scripts/modules.sh = %q, want every go.work use %q", all, workspace)
	}
	public := moduleScript(t, root, "--public")
	if len(public) != len(slices.Compact(slices.Clone(public))) {
		t.Fatalf("scripts/modules.sh --public contains duplicates: %q", public)
	}
	wantPublic := externallyImportableModules(t, root, all)
	if !slices.Equal(public, wantPublic) {
		t.Fatalf("scripts/modules.sh --public = %q, want modules with an externally importable production package %q", public, wantPublic)
	}
}

type workFile struct {
	Use []struct {
		DiskPath string
	}
}

func workspaceModules(t *testing.T, root string) []string {
	t.Helper()
	// The command and go.work path are fixed repository policy, not input.
	//nolint:gosec // Go's own parser is the independent authority for workspace membership.
	command := exec.CommandContext(t.Context(), "go", "work", "edit", "-json", filepath.Join(root, "go.work"))
	out, commandErr := command.Output()
	if commandErr != nil {
		var detail string
		if exit, ok := errors.AsType[*exec.ExitError](commandErr); ok {
			detail = strings.TrimSpace(string(exit.Stderr))
		}
		t.Fatalf("read go.work membership: %v: %s", commandErr, detail)
	}
	var work workFile
	if decodeErr := json.Unmarshal(out, &work); decodeErr != nil {
		t.Fatalf("decode go.work membership: %v", decodeErr)
	}
	if len(work.Use) == 0 {
		t.Fatal("go.work declares no modules")
	}

	physicalRoot, rootErr := filepath.EvalSymlinks(root)
	if rootErr != nil {
		t.Fatalf("resolve physical repository root: %v", rootErr)
	}
	found := make([]string, 0, len(work.Use))
	for _, use := range work.Use {
		diskPath := filepath.FromSlash(use.DiskPath)
		if !filepath.IsAbs(diskPath) {
			diskPath = filepath.Join(root, diskPath)
		}
		absolute, pathErr := filepath.Abs(diskPath)
		if pathErr != nil {
			t.Fatalf("resolve go.work use %q: %v", use.DiskPath, pathErr)
		}
		physical, linkErr := filepath.EvalSymlinks(absolute)
		if linkErr != nil {
			t.Fatalf("resolve physical go.work use %q: %v", use.DiskPath, linkErr)
		}
		relative, relativeErr := filepath.Rel(physicalRoot, physical)
		if relativeErr != nil {
			t.Fatalf("relativize go.work use %q: %v", use.DiskPath, relativeErr)
		}
		relative = filepath.ToSlash(relative)
		if relative == ".." || strings.HasPrefix(relative, "../") {
			t.Fatalf("go.work use %q is outside the repository", use.DiskPath)
		}
		found = append(found, relative)
	}
	slices.Sort(found)
	unique := slices.Compact(found)
	if len(unique) != len(found) {
		t.Fatal("go.work declares a module more than once")
	}
	return unique
}

func TestModuleScriptFailsClosedWhenPackageFactsAreUnavailable(t *testing.T) {
	root := repoRoot(t)
	name := filepath.Join(root, "scripts", "modules.sh")
	// The executable and argument are fixed repository policy, not input.
	//nolint:gosec // Running the module inventory is the contract under test.
	command := exec.CommandContext(t.Context(), "sh", name, "--public")
	command.Dir = root
	command.Env = append(os.Environ(), "GOFLAGS=-mod=bogus")
	out, err := command.Output()
	if err == nil {
		t.Fatal("scripts/modules.sh --public accepted unavailable package facts")
	}
	if len(out) != 0 {
		t.Fatalf("failed module inventory published partial data %q", out)
	}
	exit, ok := errors.AsType[*exec.ExitError](err)
	if !ok {
		t.Fatalf("failed module inventory error = %T %v, want an exit status", err, err)
	}
	if !strings.Contains(string(exit.Stderr), "modules.sh: cannot inspect") {
		t.Fatalf("failed module inventory diagnostic = %q", exit.Stderr)
	}
}

func TestModuleScriptResolvesItsPhysicalRepository(t *testing.T) {
	root := repoRoot(t)
	link := filepath.Join(t.TempDir(), "repository")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("cannot create repository symlink: %v", err)
	}
	got := moduleScript(t, link, "--public")
	want := moduleScript(t, root, "--public")
	if !slices.Equal(got, want) {
		t.Fatalf("module inventory through symlink = %q, want %q", got, want)
	}
	module := moduleScript(t, root)[0]
	directory := filepath.Join(link, module)
	//nolint:gosec // The repository script and derived module path are the contract under test.
	command := exec.CommandContext(t.Context(), "sh", filepath.Join(link, "scripts", "modules.sh"), "--public")
	command.Dir = directory
	command.Env = append(os.Environ(), "PWD="+directory)
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("module inventory from a symlinked module directory: %v: %s", err, out)
	}
	got = strings.Fields(string(out))
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("module inventory from a symlinked module directory = %q, want %q", got, want)
	}
}

func TestRepositoryChecksUseThePhysicalRoot(t *testing.T) {
	root := repoRoot(t)
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "repository")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("cannot create repository symlink: %v", err)
	}
	t.Chdir(filepath.Join(link, "ui", "internal", "arch"))
	if got := repoRoot(t); got != want {
		t.Fatalf("repository root through symlink = %q, want physical root %q", got, want)
	}
}

func TestModuleInventoryConsumersSeparateAcquisitionFromIteration(t *testing.T) {
	root := repoRoot(t)
	paths := []string{
		filepath.Join(root, "CONTRIBUTING.md"),
	}
	for _, directory := range []string{"scripts", ".github/workflows"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !entry.IsDir() {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	invocation := regexp.MustCompile(`(?:\$|[<>])\([^\n)]*modules\.sh`)
	assignment := regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*=\$\(`)
	for _, path := range paths {
		body, err := readRepositoryFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for number, line := range strings.Split(text, "\n") {
			if invocation.MatchString(line) && !assignment.MatchString(line) {
				t.Errorf("%s:%d consumes the module inventory inline; acquire it in a checked assignment before interpreting its data", filepath.ToSlash(path), number+1)
			}
		}
		if strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml") {
			assertInventoryAcquisitionsFailFast(t, path, text, "run: |", "set -euo pipefail", "outside a fail-fast run block")
		}
		if filepath.Base(path) == "CONTRIBUTING.md" {
			assertInventoryAcquisitionsFailFast(t, path, text, "```sh", "set -e", "without fail-fast shell semantics")
		}
	}
}

func assertInventoryAcquisitionsFailFast(t *testing.T, path, text, boundary, guard, problem string) {
	t.Helper()
	const marker = "=$(scripts/modules.sh"
	for offset := 0; ; {
		at := strings.Index(text[offset:], marker)
		if at < 0 {
			return
		}
		at += offset
		block := strings.LastIndex(text[:at], boundary)
		if block < 0 || !strings.Contains(text[block:at], guard) {
			t.Errorf("%s acquires the module inventory %s", filepath.ToSlash(path), problem)
		}
		offset = at + len(marker)
	}
}

func TestRepositoryScriptsAcquireCommandOutputBeforeInterpretingIt(t *testing.T) {
	root := repoRoot(t)
	assignment := regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*=["']?$`)
	err := filepath.WalkDir(filepath.Join(root, "scripts"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".sh" {
			return nil
		}
		body, err := readRepositoryFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		if !strings.Contains(text, "\nset -e") {
			t.Errorf("%s does not make command failure fatal", filepath.ToSlash(path))
		}
		for number, line := range strings.Split(text, "\n") {
			if at := processSubstitution(line); at >= 0 {
				t.Errorf("%s:%d uses process substitution; acquire command output in a checked assignment so producer failure cannot be detached", filepath.ToSlash(path), number+1)
				continue
			}
			at := commandSubstitution(line)
			if at >= 0 && !assignment.MatchString(strings.TrimSpace(line[:at])) {
				t.Errorf("%s:%d interprets command output before proving the command succeeded", filepath.ToSlash(path), number+1)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func processSubstitution(line string) int {
	input := strings.Index(line, "<(")
	output := strings.Index(line, ">(")
	switch {
	case input < 0:
		return output
	case output < 0:
		return input
	default:
		return min(input, output)
	}
}

func commandSubstitution(line string) int {
	for offset := 0; offset < len(line); {
		at := strings.Index(line[offset:], "$(")
		if at < 0 {
			return -1
		}
		at += offset
		if at+2 == len(line) || line[at+2] != '(' {
			return at
		}
		offset = at + 3
	}
	return -1
}

type listedPackage struct {
	ImportPath string
	Name       string
	GoFiles    []string
	CgoFiles   []string
}

func externallyImportableModules(t *testing.T, root string, modules []string) []string {
	t.Helper()
	var found []string
	for _, module := range modules {
		if moduleHasExternallyImportablePackage(t, root, module) {
			found = append(found, module)
		}
	}
	return found
}

func moduleHasExternallyImportablePackage(t *testing.T, root, module string) bool {
	t.Helper()
	found := false
	for _, goos := range [...]string{"linux", "darwin", "windows"} {
		command := exec.CommandContext(t.Context(), "go", "list", "-json", "./...")
		command.Dir = filepath.Join(root, filepath.FromSlash(module))
		command.Env = append(os.Environ(),
			"GOWORK="+filepath.Join(root, "go.work"),
			"GOOS="+goos,
			"GOARCH=amd64",
			"CGO_ENABLED=0",
		)
		out, err := command.Output()
		if err != nil {
			var detail string
			if exit, ok := errors.AsType[*exec.ExitError](err); ok {
				detail = strings.TrimSpace(string(exit.Stderr))
			}
			t.Fatalf("go list %s for %s: %v: %s", goos, module, err, detail)
		}
		// go list writes one JSON value per package rather than an array, so the
		// stream is read value by value instead of unmarshalled whole.
		decoder := jsontext.NewDecoder(bytes.NewReader(out))
		for {
			var pkg listedPackage
			if err := json.UnmarshalDecode(decoder, &pkg); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				t.Fatalf("decode go list %s for %s: %v", goos, module, err)
			}
			if pkg.Name != "main" && len(pkg.GoFiles)+len(pkg.CgoFiles) > 0 && !internalImportPath(pkg.ImportPath) {
				found = true
			}
		}
	}
	return found
}

func internalImportPath(importPath string) bool {
	return strings.Contains("/"+importPath+"/", "/internal/")
}

func moduleScript(t *testing.T, root string, args ...string) []string {
	t.Helper()
	name := filepath.Join(root, "scripts", "modules.sh")
	arguments := append([]string{name}, args...)
	// The executable and every argument are fixed repository policy, not input.
	//nolint:gosec // Running the module inventory is the contract under test.
	command := exec.CommandContext(t.Context(), "sh", arguments...)
	command.Dir = root
	out, err := command.Output()
	if err != nil {
		var detail string
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			detail = strings.TrimSpace(string(exit.Stderr))
		}
		t.Fatalf("sh %s: %v: %s", strings.Join(arguments, " "), err, detail)
	}
	modules := strings.Fields(string(out))
	if len(modules) == 0 {
		t.Fatalf("sh %s returned no modules", strings.Join(arguments, " "))
	}
	slices.Sort(modules)
	return modules
}

// TestEveryModuleSharesTheWorkspaceLanguageFloor makes the coordinated release
// train one language contract as well as one source tree. A workspace uses its own
// directive while compiling, so without this check an individual module could claim
// an older floor that its repository tests never actually exercise.
func TestEveryModuleSharesTheWorkspaceLanguageFloor(t *testing.T) {
	root := repoRoot(t)
	want := goDirective(t, filepath.Join(root, "go.work"))
	for _, dir := range moduleScript(t, root) {
		path := filepath.Join(root, dir, "go.mod")
		if got := goDirective(t, path); got != want {
			t.Errorf("%s declares Go %s; go.work and the coordinated module floor declare %s", dir, got, want)
		}
	}
}

func goDirective(t *testing.T, path string) string {
	t.Helper()
	body, err := readRepositoryFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for line := range strings.SplitSeq(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "go" {
			return fields[1]
		}
	}
	t.Fatalf("%s has no Go directive", path)
	return ""
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

func TestWhiteBoxTestsNameTheirBoundary(t *testing.T) {
	root := repoRoot(t)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if skipped(entry.Name(), path == root) {
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_internals_test.go") {
			return nil
		}
		production, ok, err := productionPackage(filepath.Dir(path))
		if err != nil || !ok || production == "main" {
			return err
		}
		tested, err := packageClause(path)
		if err != nil {
			return err
		}
		if tested == production {
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			t.Errorf("%s is a white-box test; name it *_internals_test.go", filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk tests: %v", err)
	}
}

// TestConfigurationUsesExplicitValues rejects the conventional API shapes that would
// reintroduce functional options: an Options struct, a function type named Option, or
// a named or inline function value that mutates, validates or returns configuration.
// A functional option is not just another spelling: it hides the complete configuration
// from documentation, comparison and composition, and gives one operation a second
// configuration language.
//
// Private spellings are covered too. An unexported option type behind exported With
// functions is still the same second language, and internal construction code follows
// the same repository rule. This deliberately does not guess whether every struct
// named Settings, Params or something else is configuration. Those names also describe
// real domain values; their responsibility belongs to API review, not a naming
// heuristic. Domain values such as base.Option remain structs and therefore do not
// match this rule.
func TestConfigurationUsesExplicitValues(t *testing.T) {
	root := repoRoot(t)
	walk(t, root, func(_, path string) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.TypeSpec:
				position := fset.Position(value.Pos())
				location := repositoryLocation(t, root, position.Filename)

				if strings.EqualFold(value.Name.Name, "options") {
					if _, isStruct := value.Type.(*ast.StructType); isStruct {
						t.Errorf("%s:%d declares Options as configuration; use one explicit Config value",
							location, position.Line)
					}
				}
				function, isFunction := value.Type.(*ast.FuncType)
				if isFunction && (strings.EqualFold(value.Name.Name, "option") || configures(function)) {
					t.Errorf("%s:%d declares the functional option %s; use an explicit Config struct",
						location, position.Line, value.Name.Name)
				}
			case *ast.FuncDecl:
				if value.Type.Params == nil {
					return true
				}
				for _, parameter := range value.Type.Params.List {
					function := inlineFunction(parameter.Type)
					if function == nil || !configures(function) {
						continue
					}
					position := fset.Position(parameter.Pos())
					location := repositoryLocation(t, root, position.Filename)
					t.Errorf("%s:%d accepts an inline functional option in %s; use an explicit Config struct",
						location, position.Line, value.Name.Name)
				}
			}
			return true
		})
	})
}

func repositoryLocation(t *testing.T, root, path string) string {
	t.Helper()
	relative, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatalf("make %s relative: %v", path, err)
	}
	return filepath.ToSlash(relative)
}

func inlineFunction(expression ast.Expr) *ast.FuncType {
	if variadic, ok := expression.(*ast.Ellipsis); ok {
		expression = variadic.Elt
	}
	function, _ := expression.(*ast.FuncType)
	return function
}

func configures(function *ast.FuncType) bool {
	if function.Params == nil || len(function.Params.List) != 1 {
		return false
	}
	parameter := function.Params.List[0]
	if len(parameter.Names) > 1 {
		return false
	}
	input, pointer := configType(parameter.Type)
	if !configurationName(input) {
		return false
	}
	if function.Results == nil || len(function.Results.List) == 0 {
		return pointer
	}
	if len(function.Results.List) != 1 || len(function.Results.List[0].Names) > 1 {
		return false
	}
	result := function.Results.List[0].Type
	if pointer && typeName(result) == "error" {
		return true
	}
	output, _ := configType(result)
	return output == input
}

func configType(expression ast.Expr) (string, bool) {
	pointer, ok := expression.(*ast.StarExpr)
	if ok {
		return typeName(pointer.X), true
	}
	return typeName(expression), false
}

func configurationName(name string) bool {
	name = strings.ToLower(name)
	return name == "config" || strings.HasSuffix(name, "config") || strings.HasSuffix(name, "options")
}

func TestConfigurationRuleRecognizesFunctionalOptions(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{"func(*Config)", true},
		{"func(*transport.Config)", true},
		{"func(*sessionOptions)", true},
		{"func(*Config) error", true},
		{"func(Config) Config", true},
		{"func(config) config", true},
		{"func(*Config) Widget", false},
		{"func(Config)", false},
		{"func(string)", false},
		{"func(a, b *Config)", false},
		{"func(*Widget)", false},
	}
	for _, test := range tests {
		expression, err := parser.ParseExpr(test.source)
		if err != nil {
			t.Fatalf("parse %q: %v", test.source, err)
		}
		function, ok := expression.(*ast.FuncType)
		if !ok {
			t.Fatalf("%q parsed as %T", test.source, expression)
		}
		if got := configures(function); got != test.want {
			t.Errorf("configures(%q) = %t, want %t", test.source, got, test.want)
		}
	}
}

func TestConfigurationRuleRecognizesInlineFunctionalOptions(t *testing.T) {
	function := &ast.FuncType{
		Params: &ast.FieldList{List: []*ast.Field{{Type: &ast.StarExpr{X: ast.NewIdent("Config")}}}},
	}
	if got := inlineFunction(function); got != function {
		t.Fatal("an inline function parameter was not inspected")
	}
	if got := inlineFunction(&ast.Ellipsis{Elt: function}); got != function {
		t.Fatal("a variadic inline function parameter was not inspected")
	}
	if got := inlineFunction(ast.NewIdent("Option")); got != nil {
		t.Fatalf("a named option is owned by its type declaration, got %T", got)
	}
}

type constructionEntry struct {
	name, location string
}

type constructionException struct {
	names  []string
	reason string
}

var constructionExceptions = map[string]constructionException{
	"ui/core/term:Terminal": {
		names: []string{"Open", "OpenOn"},
		reason: "Open owns the process terminal and environment; OpenOn owns caller-provided " +
			"terminal files and their environment. Moving that transport into term.Config would " +
			"leak local process resources into every adapter that shares terminal behavior settings.",
	},
}

// TestConcreteTypesHaveOneConstructor keeps ownership or configuration variants out
// of exported function names. One concrete value normally gets one conventional New
// or resource-acquiring Open entry point; optional state belongs in its Config.
// Different abstraction layers may each construct their own type, but
// NewControlledThing beside NewThing makes callers choose between two languages for
// the same value and lets those paths drift.
//
// The verbs are deliberately explicit. New constructs values and Open acquires
// stateful resources in Go's standard vocabulary. Parse, Decode and Render describe
// transformations whose result types do not by themselves identify one construction
// contract, so an AST rule must not guess about them. Multiple New/Open entries need
// an exact declaration below naming the ownership or lifecycle boundary that earns
// them; an exception is therefore reviewed and kept live rather than silently skipped.
func TestConcreteTypesHaveOneConstructor(t *testing.T) {
	root := repoRoot(t)
	constructors := make(map[string][]constructionEntry)
	walk(t, root, func(dir, path string) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			constructed := constructedType(function)
			if constructed == "" {
				continue
			}
			position := fset.Position(function.Pos())
			relative, relErr := filepath.Rel(root, position.Filename)
			if relErr != nil {
				t.Fatalf("make %s relative: %v", position.Filename, relErr)
			}
			key := dir + ":" + constructed
			constructors[key] = append(constructors[key],
				constructionEntry{function.Name.Name, filepath.ToSlash(relative)})
		}
	})
	for constructed, entries := range constructors {
		if len(entries) <= 1 || allowedConstruction(constructed, entries) {
			continue
		}
		slices.SortFunc(entries, func(a, b constructionEntry) int {
			return strings.Compare(a.name+":"+a.location, b.name+":"+b.location)
		})
		locations := make([]string, len(entries))
		for i, entry := range entries {
			locations[i] = entry.location + ":" + entry.name
		}
		t.Errorf("%s has multiple New/Open construction entries %s; keep one entry or declare the distinct ownership or lifecycle boundary",
			constructed, strings.Join(locations, ", "))
	}
	for constructed, exception := range constructionExceptions {
		entries, ok := constructors[constructed]
		if exception.reason == "" {
			t.Errorf("construction exception %s has no architectural reason", constructed)
		}
		if !ok || !allowedConstruction(constructed, entries) {
			t.Errorf("construction exception %s for %v is stale or does not match the live entries",
				constructed, exception.names)
		}
	}
}

func allowedConstruction(constructed string, entries []constructionEntry) bool {
	exception, ok := constructionExceptions[constructed]
	if !ok || exception.reason == "" || len(entries) != len(exception.names) {
		return false
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.name
	}
	slices.Sort(names)
	want := slices.Clone(exception.names)
	slices.Sort(want)
	return slices.Equal(names, want)
}

func constructedType(function *ast.FuncDecl) string {
	if function.Recv != nil || !function.Name.IsExported() ||
		!constructionVerb(function.Name.Name) || function.Type.Results == nil ||
		len(function.Type.Results.List) == 0 {
		return ""
	}
	expression := function.Type.Results.List[0].Type
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = pointer.X
	}
	switch indexed := expression.(type) {
	case *ast.IndexExpr:
		expression = indexed.X
	case *ast.IndexListExpr:
		expression = indexed.X
	}
	return typeName(expression)
}

func constructionVerb(name string) bool {
	for _, verb := range []string{"New", "Open"} {
		if name == verb {
			return true
		}
		if len(name) > len(verb) && strings.HasPrefix(name, verb) {
			next := name[len(verb)]
			return next >= 'A' && next <= 'Z'
		}
	}
	return false
}

func TestConstructorRuleRecognizesConcreteResults(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{"func NewThing() *Thing", "Thing"},
		{"func NewThing() (Thing, error)", "Thing"},
		{"func NewTree[T any]() *Tree[T]", "Tree"},
		{"func NewPair[A, B any]() *Pair[A, B]", "Pair"},
		{"func New() *Thing", "Thing"},
		{"func Open(Config) (*Thing, error)", "Thing"},
		{"func OpenOn(Source) *Thing", "Thing"},
		{"func Newline() *Thing", ""},
		{"func Reopen() *Thing", ""},
		{"func BuildThing() *Thing", ""},
		{"func (Thing) NewPart() *Part", ""},
		{"func NewNothing()", ""},
	}
	for _, test := range tests {
		file, err := parser.ParseFile(token.NewFileSet(), "rule.go", "package rule\n"+test.source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %q: %v", test.source, err)
		}
		function := file.Decls[0].(*ast.FuncDecl)
		if got := constructedType(function); got != test.want {
			t.Errorf("constructedType(%q) = %q, want %q", test.source, got, test.want)
		}
	}
}

// TestConstructorsNameRelatedSettings prevents a constructor from growing a list of
// positional settings. One or two parameters commonly name the content or owner at
// the boundary; once a constructor needs three, the complete setting vocabulary must
// be visible in an explicit Config value. This is deliberately about construction,
// not arbitrary functions whose arguments describe an operation.
func TestConstructorsNameRelatedSettings(t *testing.T) {
	root := repoRoot(t)
	walk(t, root, func(_, path string) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || constructedType(function) == "" || parameterCount(function.Type.Params) < 3 ||
				hasConfigParameter(function.Type.Params) {
				continue
			}
			position := fset.Position(function.Pos())
			relative, relErr := filepath.Rel(root, position.Filename)
			if relErr != nil {
				t.Fatalf("make %s relative: %v", position.Filename, relErr)
			}
			t.Errorf("%s:%d declares the positional constructor %s with three or more inputs; name related settings in one Config value",
				filepath.ToSlash(relative), position.Line, function.Name.Name)
		}
	})
}

func parameterCount(fields *ast.FieldList) int {
	if fields == nil {
		return 0
	}
	count := 0
	for _, field := range fields.List {
		count += max(len(field.Names), 1)
	}
	return count
}

func hasConfigParameter(fields *ast.FieldList) bool {
	if fields == nil {
		return false
	}
	for _, field := range fields.List {
		if strings.HasSuffix(typeName(field.Type), "Config") {
			return true
		}
	}
	return false
}

func TestConstructorSettingsRuleRecognizesConfiguration(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{"func NewThing(a, b, c int) *Thing", true},
		{"func NewThing(a int, b string, config Config) *Thing", false},
		{"func OpenThing(a, b int, config transport.Config) (*Thing, error)", false},
		{"func NewThing(a, b int) *Thing", false},
		{"func ParseThing(a, b, c int) *Thing", false},
	}
	for _, test := range tests {
		file, err := parser.ParseFile(token.NewFileSet(), "rule.go", "package rule\n"+test.source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %q: %v", test.source, err)
		}
		function := file.Decls[0].(*ast.FuncDecl)
		got := constructedType(function) != "" && parameterCount(function.Type.Params) >= 3 &&
			!hasConfigParameter(function.Type.Params)
		if got != test.want {
			t.Errorf("positionalConstructor(%q) = %t, want %t", test.source, got, test.want)
		}
	}
}

func typeName(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.SelectorExpr:
		return expression.Sel.Name
	default:
		return ""
	}
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
		{"ui/core/grid", "ui/base", true},
		{"ui/core/layout", "ui/kit", true},
		{"ui/base", "ui/kit", true},
		{"ui/internal/identity", "ui/core/grid", true},

		// The rule the host exists under: the runtime must not know the widgets. The
		// module graph would not catch it — core could require components and Go
		// would allow it — so this is the one that has to be checked here.
		{"ui/core/program", "ui/base", true},
		{"ui/core/program", "ui/kit", true},
		{"ui/core/program", "ui/uitest", true},

		// And the substrate must not reach for the runtime that drives it.
		{"ui/core/grid", "ui/core/program", true},
		{"ui/core/text", "ui/core/program", true},
		{"ui/core/grid", "ui/core/present", true},
		{"ui/core/input", "ui/core/keymap", true},
		{"ui/core/keymap", "ui/core/input", false},
		{"ui/core/term", "ui/core/keymap", true},
		{"ui/core/program", "ui/core/keymap", true},

		// Nor may a widget: one that needed a runtime to exist could not be tested
		// without starting one, and this is the edge the table used to allow.
		{"ui/base", "ui/core/program", true},
		{"ui/kit", "ui/core/program", true},
		{"ui/base", "ui/core/present", true},

		// Nothing that promises a short dependency list may reach for the module that
		// carries a parser, and that module may not reach back up the ladder.
		{"ui/kit", "extensions/markdown", true},
		{"ui/core/text", "extensions/markdown", true},
		{"extensions/markdown", "ui/base", true},
		{"extensions/markdown", "ui/core/program", true},
		{"extensions/markdown", "ui/core/text", false},
		{"extensions/markdown", "extensions/highlight", true},
		{"extensions/highlight", "extensions/markdown", true},
		{"extensions/highlight", "ui/core/text", false},
		{"extensions/latex", "extensions/markdown", true},
		{"extensions/markdown", "extensions/latex", true},
		{"extensions/latex", "ui/core/text", false},
		{"ui/uitest/ptytest", "ui/core/text", false},

		// Nothing leans on the harness, and nothing imports a demonstration.
		{"ui/core/program", "ui/uitest/ptytest", true},
		{"ui/kit", "ui/uitest/ptytest", true},
		{"ui/kit", "ui/uitest", true},
		{"ui/kit", "examples/streaming", true},

		// The edges the rings are made of.
		{"ui/base", "ui/core/grid", false},
		{"ui/base", "ui/internal/identity", false},
		{"ui/kit", "ui/base", false},
		{"ui/kit", "ui/internal/identity", false},
		{"ui/kit", "ui/core/layout", false},
		{"ui/core/program", "ui/core/term", false},
		{"ui/uitest", "ui/core/program", false},
		{"ui/core/text", "ui/core/grid", false},
		{"examples/streaming", "ui/kit", false},
		{"examples/streaming", "ui/core/program", false},
		{"examples/streaming", "ui/uitest/ptytest", false},
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
func moduleOf(dir string, inventory []string) (string, []string, bool) {
	dir = filepath.ToSlash(dir)
	best := ""
	for _, name := range inventory {
		if dir != name && !strings.HasPrefix(dir, name+"/") {
			continue
		}
		if len(name) > len(best) {
			best = name
		}
	}
	return best, dependencyRules[best], best != ""
}

func TestModuleMembershipComesOnlyFromTheInventory(t *testing.T) {
	for _, test := range []struct {
		name      string
		directory string
		inventory []string
		module    string
		allowed   []string
	}{
		{
			name:      "module without a dependency grant",
			directory: "extra/package", inventory: []string{"extra"}, module: "extra",
		},
		{
			name:      "dependency grant cannot create membership",
			directory: "ui/core/ansi", inventory: []string{"extra"},
		},
		{
			name:      "nested module owns its packages",
			directory: "ui/core/nested/package", inventory: []string{"ui", "ui/core/nested"}, module: "ui/core/nested",
		},
		{
			name:      "member receives its dependency grant",
			directory: "ui/core/ansi", inventory: []string{"ui"}, module: "ui", allowed: dependencyRules["ui"],
		},
		{
			name:      "directory prefix cannot create membership",
			directory: "extra-copy/package", inventory: []string{"extra"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			module, allowed, ok := moduleOf(test.directory, test.inventory)
			if module != test.module || ok != (test.module != "") || !slices.Equal(allowed, test.allowed) {
				t.Fatalf("moduleOf(%q, %q) = %q, %q, %t; want %q, %q, %t", test.directory, test.inventory,
					module, allowed, ok, test.module, test.allowed, test.module != "")
			}
		})
	}
}

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

func productionPackage(dir string) (string, bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		name, err := packageClause(filepath.Join(dir, entry.Name()))
		return name, true, err
	}
	return "", false, nil
}

func packageClause(path string) (string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.PackageClauseOnly)
	if err != nil {
		return "", err
	}
	return file.Name.Name, nil
}

// repoRoot is the directory holding go.work, which is the one thing that marks the
// top of a multi-module repository rather than the top of a module.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("physical working directory: %v", err)
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
