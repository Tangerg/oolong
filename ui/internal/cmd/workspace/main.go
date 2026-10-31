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

const (
	generated       = "// Versioned replacements are derived by scripts/sync-workspace.sh."
	replacementNote = "// Required metadata is local even before a module has its first tag."
)

type workspace struct {
	Use []struct{ DiskPath string }
}

type manifest struct {
	Module  struct{ Path string }
	Require []struct{ Path, Version string }
}

func main() {
	check := flag.Bool("check", false, "fail instead of updating a stale replacement block")
	members := flag.Bool("members", false, "print physical member directories for scripts/modules.sh")
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	ctx := context.Background()
	var err error
	if *members {
		var directories []string
		directories, err = memberDirectories(ctx, *root)
		if err == nil {
			_, err = fmt.Fprintln(os.Stdout, strings.Join(directories, "\n"))
		}
	} else {
		err = syncWorkspace(ctx, *root, *check)
	}
	if err != nil {
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
	prefix, suffix, newline, err := surroundingDirectives(string(source))
	if err != nil {
		return err
	}
	var result strings.Builder
	result.WriteString(prefix)
	result.WriteString(generated + newline + replacementNote + newline + "replace (" + newline)
	for _, line := range lines {
		result.WriteString("\t" + line + newline)
	}
	result.WriteString(")" + newline)
	result.WriteString(suffix)
	if bytes.Equal(source, []byte(result.String())) {
		return nil
	}
	if check {
		return errors.New("workspace: stale replacements; run scripts/sync-workspace.sh")
	}
	return os.WriteFile(path, []byte(result.String()), 0o644) //nolint:gosec // A Go workspace manifest is public source, not a secret.
}

// Only the marked replace directive is generated. Other directives, including
// newly added members after it, retain their original text and ownership.
func surroundingDirectives(source string) (prefix, suffix, newline string, err error) {
	prefix, newline = source, lineEnding(source)
	offset := 0
	for line := range strings.Lines(source) {
		if strings.TrimRight(line, "\r\n") == generated {
			prefix, newline = source[:offset], lineEnding(line)
			suffix, err = afterGeneratedReplace(source[offset+len(line):])
			if err != nil {
				return "", "", "", err
			}
			break
		}
		offset += len(line)
	}
	if offset == len(source) {
		if !strings.HasSuffix(prefix, "\n") {
			prefix += newline
		}
		if !strings.HasSuffix(prefix, newline+newline) {
			prefix += newline
		}
	}
	for _, outside := range []string{prefix, suffix} {
		for line := range strings.Lines(outside) {
			if fields := strings.Fields(line); len(fields) > 0 && fields[0] == "replace" {
				return "", "", "", errors.New("workspace: a manual replacement cannot be overwritten")
			}
		}
	}
	return prefix, suffix, newline, nil
}

func lineEnding(source string) string {
	if at := strings.IndexByte(source, '\n'); at > 0 && source[at-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}

func afterGeneratedReplace(tail string) (string, error) {
	note, directives, _ := strings.Cut(tail, "\n")
	line, remainder, _ := strings.Cut(directives, "\n")
	fields := strings.Fields(line)
	if strings.TrimSuffix(note, "\r") != replacementNote || len(fields) < 2 || fields[0] != "replace" {
		return "", errors.New("workspace: missing marked replacement directive")
	}
	if fields[1] != "(" {
		return remainder, nil
	}
	suffix := remainder
	for next := range strings.Lines(remainder) {
		suffix = suffix[len(next):]
		if strings.HasPrefix(strings.TrimSpace(next), ")") {
			return suffix, nil
		}
	}
	return "", errors.New("workspace: unterminated generated replacement block")
}

func memberDirectories(ctx context.Context, root string) ([]string, error) {
	var work workspace
	if err := readGoJSON(ctx, root, &work, "work", "edit", "-json", "go.work"); err != nil {
		return nil, err
	}
	if len(work.Use) == 0 {
		return nil, errors.New("workspace: no member modules")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	var directories []string
	for _, use := range work.Use {
		directory := filepath.FromSlash(use.DiskPath)
		if !filepath.IsAbs(directory) {
			directory = filepath.Join(root, directory)
		}
		physical, linkErr := filepath.EvalSymlinks(directory)
		if linkErr != nil {
			return nil, linkErr
		}
		relative, relErr := filepath.Rel(root, physical)
		if relErr != nil || !filepath.IsLocal(relative) {
			return nil, fmt.Errorf("workspace: member %q is outside the repository", use.DiskPath)
		}
		directory = filepath.ToSlash(relative)
		if len(strings.Fields(directory)) != 1 || strings.TrimSpace(directory) != directory {
			return nil, fmt.Errorf("workspace: module directory %q contains whitespace", directory)
		}
		if _, statErr := os.Stat(filepath.Join(physical, "go.mod")); statErr != nil {
			return nil, statErr
		}
		directories = append(directories, directory)
	}
	slices.Sort(directories)
	if len(slices.Compact(directories)) != len(directories) {
		return nil, errors.New("workspace: duplicate member directory")
	}
	return directories, nil
}

func readMembers(ctx context.Context, root string) (map[string]string, []manifest, error) {
	directories, err := memberDirectories(ctx, root)
	if err != nil {
		return nil, nil, err
	}
	modules := make(map[string]string, len(directories))
	manifests := make([]manifest, 0, len(directories))
	for _, directory := range directories {
		var mod manifest
		if err := readGoJSON(ctx, root, &mod, "mod", "edit", "-json", filepath.Join(directory, "go.mod")); err != nil {
			return nil, nil, err
		}
		if mod.Module.Path == "" {
			return nil, nil, fmt.Errorf("workspace: %s has no module path", directory)
		}
		if _, duplicate := modules[mod.Module.Path]; duplicate {
			return nil, nil, fmt.Errorf("workspace: duplicate module %s", mod.Module.Path)
		}
		modules[mod.Module.Path] = "./" + directory
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
