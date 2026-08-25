// Package apiledger verifies that every incompatible published Go API change is
// named in the release's migration ledger.
package apiledger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

var platforms = [...]string{"linux", "darwin", "windows"}

const (
	goWorkEnv = "GOWORK"
	goWorkOff = "off"
)

// Config identifies the repository and release ledger to check. Empty fields use
// the current directory, the Unreleased section, apidiff from PATH, and discard
// informational progress.
type Config struct {
	Root    string
	Section string
	APIDiff string
	Log     io.Writer
}

// Check compares every released public workspace module with its latest immutable
// tag and requires each incompatible symbol to be named in that module's changelog
// ledger. A public module with no immutable tag is reported to Config.Log because it
// has no compatibility baseline to compare.
func Check(ctx context.Context, cfg Config) error {
	return newChecker(cfg, osRunner{}).check(ctx)
}

type checker struct {
	root    string
	section string
	apidiff string
	log     io.Writer
	runner  runner
}

func newChecker(cfg Config, runner runner) *checker {
	root := cfg.Root
	if root == "" {
		root = "."
	}
	section := cfg.Section
	if section == "" {
		section = "Unreleased"
	}
	apidiff := cfg.APIDiff
	if apidiff == "" {
		apidiff = "apidiff"
	}
	log := cfg.Log
	if log == nil {
		log = io.Discard
	}
	return &checker{root: root, section: section, apidiff: apidiff, log: log, runner: runner}
}

func (c *checker) check(ctx context.Context) error {
	document, err := os.ReadFile(filepath.Join(c.root, "CHANGELOG.md"))
	if err != nil {
		return fmt.Errorf("read CHANGELOG.md: %w", err)
	}
	changes, err := releaseSection(string(document), c.section)
	if err != nil {
		return err
	}
	modules, err := c.publicModules(ctx)
	if err != nil {
		return err
	}
	scratch, err := os.MkdirTemp("", "oolong-api-ledger-")
	if err != nil {
		return fmt.Errorf("create API ledger workspace: %w", err)
	}
	defer os.RemoveAll(scratch) //nolint:errcheck // A temporary audit directory has no recovery action.

	var problems []string
	for _, module := range modules {
		moduleProblems, checkErr := c.checkModule(ctx, scratch, changes, module)
		if checkErr != nil {
			return checkErr
		}
		problems = append(problems, moduleProblems...)
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	return nil
}

func (c *checker) publicModules(ctx context.Context) ([]string, error) {
	root, err := filepath.Abs(c.root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	script := filepath.Join(root, "scripts", "modules.sh")
	out, err := c.run(ctx, command{dir: root, name: "sh", args: []string{script, "--public"}})
	if err != nil {
		return nil, err
	}
	var modules []string
	for line := range strings.Lines(out) {
		clean := filepath.Clean(strings.TrimSpace(line))
		module := filepath.ToSlash(clean)
		if module == "." {
			continue
		}
		if filepath.IsAbs(clean) || module == ".." || strings.HasPrefix(module, "../") {
			return nil, fmt.Errorf("public module %q is outside the repository", line)
		}
		modules = append(modules, module)
	}
	slices.Sort(modules)
	unique := slices.Compact(modules)
	if len(unique) != len(modules) {
		return nil, errors.New("scripts/modules.sh --public returned duplicate modules")
	}
	if len(unique) == 0 {
		return nil, errors.New("scripts/modules.sh --public returned no modules")
	}
	return unique, nil
}

func (c *checker) checkModule(ctx context.Context, scratch, changes, module string) ([]string, error) {
	release, found, err := c.moduleRelease(ctx, module)
	if err != nil {
		return nil, err
	}
	if !found {
		if _, writeErr := fmt.Fprintf(c.log, "%s: no released baseline; skipped\n", module); writeErr != nil {
			return nil, fmt.Errorf("report skipped module %s: %w", module, writeErr)
		}
		return nil, nil
	}
	symbols, err := c.breakingSymbols(ctx, scratch, release)
	if err != nil || len(symbols) == 0 {
		return nil, err
	}
	return release.migrationProblems(changes, c.section, symbols), nil
}

type moduleRelease struct {
	name        string
	path        string
	currentDir  string
	previousDir string
	tag         string
	version     string
}

func (c *checker) moduleRelease(ctx context.Context, module string) (moduleRelease, bool, error) {
	previous, err := c.latestTag(ctx, module)
	if err != nil || previous == "" {
		return moduleRelease{}, false, err
	}
	version := strings.TrimPrefix(previous, module+"/")
	if version == previous {
		return moduleRelease{}, false, fmt.Errorf("tag %q does not belong to module %s", previous, module)
	}
	moduleDir := filepath.Join(c.root, filepath.FromSlash(module))
	modulePath, err := c.run(ctx, command{
		dir: moduleDir, name: "go", args: []string{"list", "-m", "-f", "{{.Path}}"},
		env: map[string]string{goWorkEnv: goWorkOff},
	})
	if err != nil {
		return moduleRelease{}, false, err
	}
	modulePath = strings.TrimSpace(modulePath)
	if modulePath == "" {
		return moduleRelease{}, false, fmt.Errorf("%s/go.mod has no module path", module)
	}
	if _, downloadErr := c.run(ctx, command{
		dir: c.root, name: "go", args: []string{"mod", "download", modulePath + "@" + version},
		env: map[string]string{goWorkEnv: goWorkOff},
	}); downloadErr != nil {
		return moduleRelease{}, false, downloadErr
	}
	oldDir, err := c.run(ctx, command{
		dir: c.root, name: "go", args: []string{"list", "-m", "-f", "{{.Dir}}", modulePath + "@" + version},
		env: map[string]string{goWorkEnv: goWorkOff},
	})
	if err != nil {
		return moduleRelease{}, false, err
	}
	return moduleRelease{
		name:        module,
		path:        modulePath,
		currentDir:  moduleDir,
		previousDir: strings.TrimSpace(oldDir),
		tag:         previous,
		version:     version,
	}, true, nil
}

func (c *checker) breakingSymbols(ctx context.Context, scratch string, release moduleRelease) ([]string, error) {
	moduleScratch := filepath.Join(scratch, filepath.FromSlash(release.name))
	if err := os.MkdirAll(moduleScratch, 0o700); err != nil {
		return nil, fmt.Errorf("create %s API workspace: %w", release.name, err)
	}
	set := make(map[string]struct{})
	for _, goos := range platforms {
		oldAPI := filepath.Join(moduleScratch, goos+"-old.api")
		newAPI := filepath.Join(moduleScratch, goos+"-new.api")
		if err := c.export(ctx, release.previousDir, oldAPI, release.path, goos); err != nil {
			return nil, err
		}
		if err := c.export(ctx, release.currentDir, newAPI, release.path, goos); err != nil {
			return nil, err
		}
		out, err := c.run(ctx, command{dir: c.root, name: c.apidiff, args: []string{"-m", oldAPI, newAPI}})
		if err != nil {
			return nil, err
		}
		for _, symbol := range incompatibleSymbols(out) {
			set[symbol] = struct{}{}
		}
	}
	symbols := make([]string, 0, len(set))
	for symbol := range set {
		symbols = append(symbols, symbol)
	}
	slices.Sort(symbols)
	return symbols, nil
}

func (m moduleRelease) migrationProblems(changes, section string, symbols []string) []string {
	ledger, ok := moduleLedger(changes, m.name)
	if !ok {
		return []string{fmt.Sprintf(
			"CHANGELOG.md has incompatible API changes from %s/%s but no '#### %s' ledger in [%s]",
			m.name, m.version, m.name, section,
		)}
	}
	var problems []string
	for _, symbol := range symbols {
		if !m.namesSymbol(ledger, symbol) {
			problems = append(problems, fmt.Sprintf(
				"CHANGELOG.md does not name incompatible API %s/%s (baseline %s)",
				m.name, symbol, m.tag,
			))
		}
	}
	return problems
}

// namesSymbol reports whether a ledger names one API exactly as apidiff reports
// it or qualifies that name with the module's ledger name. Root-package modules
// otherwise force migration prose such as `GlyphsFor`, which loses the package
// context readers use in Go source. Requiring backticks and the exact module
// qualifier keeps the second spelling just as precise as the first.
func (m moduleRelease) namesSymbol(ledger, symbol string) bool {
	for _, name := range [...]string{symbol, path.Base(m.name) + "." + symbol} {
		if strings.Contains(ledger, "`"+name+"`") {
			return true
		}
	}
	return false
}

func (c *checker) latestTag(ctx context.Context, module string) (string, error) {
	out, err := c.run(ctx, command{
		dir: c.root, name: "git", args: []string{"tag", "--list", module + "/v*", "--sort=-v:refname"},
	})
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(out) {
		if tag := strings.TrimSpace(line); tag != "" {
			return tag, nil
		}
	}
	return "", nil
}

func (c *checker) export(ctx context.Context, directory, output, modulePath, goos string) error {
	_, err := c.run(ctx, command{
		dir: directory, name: c.apidiff, args: []string{"-m", "-w", output, modulePath},
		env: map[string]string{"GOOS": goos, "CGO_ENABLED": "0"},
	})
	return err
}

func (c *checker) run(ctx context.Context, command command) (string, error) {
	out, err := c.runner.run(ctx, command)
	if err == nil {
		return out, nil
	}
	return "", fmt.Errorf("%s: %w%s", command.description(), err, outputSuffix(out))
}

func releaseSection(document, name string) (string, error) {
	want := "## [" + name + "]"
	lines := strings.SplitAfter(document, "\n")
	from := -1
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, want) && (len(line) == len(want) || line[len(want)] == ' ') {
			from = i + 1
			break
		}
	}
	if from < 0 {
		return "", fmt.Errorf("CHANGELOG.md has no [%s] section", name)
	}
	to := len(lines)
	for i := from; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			to = i
			break
		}
	}
	return strings.Join(lines[from:to], ""), nil
}

func moduleLedger(section, module string) (string, bool) {
	want := "#### " + module
	lines := strings.SplitAfter(section, "\n")
	from := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == want {
			from = i + 1
			break
		}
	}
	if from < 0 {
		return "", false
	}
	to := len(lines)
	for i := from; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "### ") || strings.HasPrefix(line, "#### ") {
			to = i
			break
		}
	}
	return strings.Join(lines[from:to], ""), true
}

func incompatibleSymbols(report string) []string {
	inside := false
	var symbols []string
	for line := range strings.Lines(report) {
		line = strings.TrimSpace(line)
		switch line {
		case "Incompatible changes:":
			inside = true
			continue
		case "Compatible changes:":
			inside = false
			continue
		}
		if !inside || !strings.HasPrefix(line, "- ") {
			continue
		}
		entry := strings.TrimPrefix(line, "- ")
		entry = strings.TrimPrefix(entry, "./")
		if symbol, _, ok := strings.Cut(entry, ":"); ok && symbol != "" {
			symbols = append(symbols, symbol)
		}
	}
	return symbols
}

type command struct {
	dir  string
	name string
	args []string
	env  map[string]string
}

func (c command) description() string {
	return strings.Join(append([]string{c.name}, c.args...), " ")
}

type runner interface {
	run(ctx context.Context, cmd command) (string, error)
}

type osRunner struct{}

func (osRunner) run(ctx context.Context, command command) (string, error) {
	// command is assembled only from this repository's fixed audit protocol and
	// explicit CLI configuration; no application or network input reaches it.
	//nolint:gosec // Running the selected Go, git and apidiff tools is this command's purpose.
	cmd := exec.CommandContext(ctx, command.name, command.args...)
	cmd.Dir = command.dir
	cmd.Env = os.Environ()
	for name, value := range command.env {
		cmd.Env = append(cmd.Env, name+"="+value)
	}
	out, err := cmd.Output()
	if err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok && len(exit.Stderr) > 0 {
			return string(exit.Stderr), err
		}
	}
	return string(out), err
}

func outputSuffix(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	return ": " + output
}
