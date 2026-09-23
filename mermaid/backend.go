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
	directory := filepath.Dir(shim)
	resolved, err := filepath.EvalSymlinks(shim)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	roots := []string{filepath.Join(directory, "node_modules", "@mermaid-js", "mermaid-cli"), filepath.Join(directory, "..", "@mermaid-js", "mermaid-cli")}
	if err == nil {
		roots = append(roots, filepath.Join(filepath.Dir(resolved), ".."))
	}
	for _, root := range roots {
		data, err := os.ReadFile(filepath.Join(root, "package.json")) //nolint:gosec // G304: installed backend metadata selected by trusted executable configuration.
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
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
		if metadata.Name != "@mermaid-js/mermaid-cli" || entry == "" || !filepath.IsLocal(entry) {
			return "", errors.New("mermaid: invalid official CLI entry point")
		}
		script, err := filepath.Abs(filepath.Join(root, entry))
		if err != nil {
			return "", err
		}
		info, err := os.Stat(script)
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() {
			return "", errors.New("mermaid: CLI entry point is not a regular file")
		}
		return script, nil
	}
	return "", errors.New("mermaid: npm CLI package not found beside shim; configure Executable with the installed mmdc entry point")
}
