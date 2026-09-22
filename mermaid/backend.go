package mermaid

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func resolveBackend(cfg Config) (Config, error) {
	if cfg.Executable == "" {
		cfg.Executable = "mmdc"
	}
	executable, err := exec.LookPath(cfg.Executable)
	if err != nil {
		return cfg, fmt.Errorf("mermaid: locate backend: %w", err)
	}
	cfg.Executable, err = filepath.Abs(executable)
	if err != nil {
		return cfg, err
	}
	extension := strings.ToLower(filepath.Ext(cfg.Executable))
	if runtime.GOOS != "windows" || extension != ".cmd" && extension != ".bat" {
		return cfg, nil
	}
	script, err := npmCLI(cfg.Executable)
	if err != nil {
		return cfg, err
	}
	node, err := exec.LookPath("node.exe")
	if err != nil {
		return cfg, fmt.Errorf("mermaid: locate Node.js: %w", err)
	}
	cfg.Executable, err = filepath.Abs(node)
	if err != nil {
		return cfg, err
	}
	cfg.Arguments = append([]string{script}, cfg.Arguments...)
	return cfg, nil
}

// npm shims are shell programs. Resolve the official package's declared entry
// point instead of feeding command arguments through cmd.exe or parsing a shim.
func npmCLI(shim string) (string, error) {
	directory := filepath.Dir(shim)
	roots := []string{filepath.Join(directory, "node_modules", "@mermaid-js", "mermaid-cli"), filepath.Join(directory, "..", "@mermaid-js", "mermaid-cli")}
	for _, root := range roots {
		data, err := os.ReadFile(filepath.Join(root, "package.json")) //nolint:gosec // G304: installed backend metadata selected by trusted executable configuration.
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		var metadata struct {
			Name string
			Bin  map[string]string
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
	return "", errors.New("mermaid: npm CLI package not found beside shim; configure node.exe with an absolute CLI script in Arguments")
}
