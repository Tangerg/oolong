package mermaid

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type backend struct{ node, entry string }

func resolveBackend(cfg Config) (backend, error) {
	executable := cfg.Executable
	if executable == "" {
		executable = "mmdc"
	}
	resolved, err := exec.LookPath(executable)
	if err != nil {
		return backend{}, fmt.Errorf("mermaid: locate installed CLI: %w", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return backend{}, err
	}
	entry, err := npmCLI(resolved)
	if err != nil {
		return backend{}, err
	}
	node := cfg.Node
	if node == "" {
		node = "node"
	}
	node, err = exec.LookPath(node)
	if err != nil {
		return backend{}, fmt.Errorf("mermaid: locate Node.js: %w", err)
	}
	node, err = filepath.Abs(node)
	return backend{node: node, entry: entry}, err
}

// npm shims are shell programs. Resolve the official package's declared entry
// point instead of feeding command arguments through cmd.exe or parsing a shim.
func npmCLI(shim string) (string, error) {
	entry, err := filepath.EvalSymlinks(shim)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(entry)
	// An npm-generated launcher lives in .bin (local) or beside node_modules
	// (global). A symlink instead selects its target's own installation.
	switch filepath.Base(entry) {
	case "mmdc", "mmdc.cmd", "mmdc.ps1":
		modules := filepath.Join(dir, "node_modules")
		if filepath.Base(dir) == ".bin" {
			modules = filepath.Dir(dir)
		}
		root := filepath.Join(modules, "@mermaid-js", "mermaid-cli")
		if _, statErr := os.Stat(filepath.Join(root, "package.json")); !errors.Is(statErr, os.ErrNotExist) {
			return cliEntry(root)
		}
	}

	// The declared binary may be nested at any depth within its package.
	for root := dir; ; root = filepath.Dir(root) {
		if _, statErr := os.Stat(filepath.Join(root, "package.json")); !errors.Is(statErr, os.ErrNotExist) {
			return cliEntry(root)
		}
		if filepath.Dir(root) == root {
			return "", errors.New("mermaid: no npm package owns the selected mmdc entry point")
		}
	}
}

func cliEntry(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "package.json")) //nolint:gosec // G304: trusted installed backend metadata.
	if err != nil {
		return "", err
	}
	var metadata struct {
		Name string            `json:"name"`
		Bin  map[string]string `json:"bin"`
	}
	if err = json.Unmarshal(data, &metadata); err != nil {
		return "", fmt.Errorf("mermaid: CLI package metadata: %w", err)
	}
	entry := metadata.Bin["mmdc"]
	if metadata.Name != "@mermaid-js/mermaid-cli" || !filepath.IsLocal(entry) {
		return "", errors.New("mermaid: invalid official CLI entry point")
	}
	script := filepath.Join(root, entry)
	info, err := os.Stat(script)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("mermaid: CLI entry point is not a regular file")
	}
	return script, nil
}
