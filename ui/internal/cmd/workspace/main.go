// Command workspace derives local version replacements from the workspace's
// member manifests. It uses only the standard library so it can repair the
// workspace before Go resolves unpublished in-repository module versions.
package main

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

const generated = "// Versioned replacements are derived by scripts/sync-workspace.sh.\n"

type workspace struct {
	Use []struct{ DiskPath string }
}

type manifest struct {
	Module  struct{ Path string }
	Require []struct{ Path, Version string }
}

func main() {
	check := flag.Bool("check", false, "fail instead of updating a stale replacement block")
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	if err := syncWorkspace(context.Background(), *root, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func syncWorkspace(ctx context.Context, root string, check bool) error {
	path := filepath.Join(root, "go.work")
	source, err := os.ReadFile(path) //nolint:gosec // The CLI explicitly selects the repository whose workspace it owns.
	if err != nil {
		return err
	}
	modules, manifests, err := readMembers(ctx, root)
	if err != nil {
		return err
	}
	replacements := make(map[string]bool)
	for _, mod := range manifests {
		for _, requirement := range mod.Require {
			if directory, local := modules[requirement.Path]; local {
				replacements[requirement.Path+" "+requirement.Version+" => "+directory] = true
			}
		}
	}
	lines := make([]string, 0, len(replacements))
	for line := range replacements {
		lines = append(lines, line)
	}
	slices.Sort(lines)
	prefix, _, found := strings.Cut(string(source), generated)
	if !found && strings.Contains(prefix, "replace") {
		return errors.New("workspace: a manual replacement block cannot be overwritten")
	}
	var result strings.Builder
	result.WriteString(strings.TrimRight(prefix, "\n"))
	result.WriteString("\n\n" + generated)
	result.WriteString("// Required metadata is local even before a module has its first tag.\nreplace (\n")
	for _, line := range lines {
		result.WriteString("\t" + line + "\n")
	}
	result.WriteString(")\n")
	if bytes.Equal(source, []byte(result.String())) {
		return nil
	}
	if check {
		return errors.New("workspace: stale replacements; run scripts/sync-workspace.sh")
	}
	return os.WriteFile(path, []byte(result.String()), 0o644) //nolint:gosec // A Go workspace manifest is public source, not a secret.
}

func readMembers(ctx context.Context, root string) (map[string]string, []manifest, error) {
	var work workspace
	if err := readGoJSON(ctx, root, &work, "work", "edit", "-json", "go.work"); err != nil {
		return nil, nil, err
	}
	if len(work.Use) == 0 {
		return nil, nil, errors.New("workspace: no member modules")
	}
	modules := make(map[string]string, len(work.Use))
	manifests := make([]manifest, 0, len(work.Use))
	for _, member := range work.Use {
		var mod manifest
		if err := readGoJSON(ctx, root, &mod, "mod", "edit", "-json", filepath.Join(member.DiskPath, "go.mod")); err != nil {
			return nil, nil, err
		}
		if mod.Module.Path == "" {
			return nil, nil, fmt.Errorf("workspace: %s has no module path", member.DiskPath)
		}
		if _, duplicate := modules[mod.Module.Path]; duplicate {
			return nil, nil, fmt.Errorf("workspace: duplicate module %s", mod.Module.Path)
		}
		modules[mod.Module.Path] = "./" + filepath.ToSlash(filepath.Clean(member.DiskPath))
		manifests = append(manifests, mod)
	}
	return modules, manifests, nil
}

func readGoJSON(ctx context.Context, root string, target any, args ...string) error {
	command := exec.CommandContext(ctx, "go", args...) //nolint:gosec // Arguments are fixed Go metadata operations, never shell input.
	command.Dir = root
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("workspace: go %s: %w\n%s", strings.Join(args, " "), err, output)
	}
	return json.Unmarshal(output, target)
}
