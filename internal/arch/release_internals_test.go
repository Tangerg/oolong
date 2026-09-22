package arch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseComparisonPropagatesToolFailure(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	source, err := os.ReadFile("../../scripts/release.sh")
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(source), "compatibility() {\n")
	if !ok {
		t.Fatal("missing compatibility function")
	}
	body, _, ok = strings.Cut(body, "\n}\n")
	if !ok {
		t.Fatal("missing compatibility function end")
	}
	for _, fail := range []bool{false, true} {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "core"), 0o700); err != nil {
			t.Fatal(err)
		}
		stub := "#!/usr/bin/env bash\nif [[ $1 == -base=none ]]; then exit 0; fi\n"
		if fail {
			stub += "echo comparison-failed >&2\nexit 42\n"
		} else {
			stub += "echo 'Suggested version: v0.18.0'\n"
		}
		// The fixture must be executable and is accessible only inside this test's private directory.
		//nolint:gosec // G306: owner-only executable permissions for a temporary fake tool.
		if err := os.WriteFile(filepath.Join(dir, "gorelease"), []byte(stub), 0o700); err != nil {
			t.Fatal(err)
		}
		script := "set -euo pipefail\nversion=v0.18.0\ngorelease_bin=$PWD/gorelease\ngit() { echo core/v0.17.0; }\ndie() { echo \"$*\" >&2; exit 1; }\ncompatibility() {\n" + body + "\n}\ncompatibility core\n"
		//nolint:gosec // G204: repository-owned function, fake tools, and a private working directory.
		cmd := exec.CommandContext(t.Context(), bash, "-c", script)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if fail && (err == nil || !strings.Contains(string(out), "status 42")) {
			t.Fatalf("failure lost: %s, %v", out, err)
		}
		if !fail && err != nil {
			t.Fatalf("valid comparison rejected: %s, %v", out, err)
		}
	}
}
