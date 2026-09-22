package arch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestReleaseTagPushes(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	source, err := os.ReadFile("../../scripts/release.sh")
	if err != nil {
		t.Fatal(err)
	}
	_, execution, ok := strings.Cut(strings.ReplaceAll(string(source), "\r\n", "\n"), "# Execution.")
	if !ok {
		t.Fatal("missing release execution section")
	}
	_, execution, ok = strings.Cut(execution, "for ((phase = 1; phase <= phases; phase++)); do\n")
	if !ok {
		t.Fatal("missing release phase loop")
	}
	execution, _, ok = strings.Cut(execution, "# Verify what this release actually controls.")
	if !ok {
		t.Fatal("missing release verification section")
	}
	execution = "for ((phase = 1; phase <= phases; phase++)); do\n" + execution

	for _, count := range []int{1, 3, 4, 6} {
		for _, failure := range []string{"none", "compatibility", "push"} {
			t.Run(fmt.Sprintf("%d/%s", count, failure), func(t *testing.T) {
				dir := t.TempDir()
				modules := []string{"foundation"}
				phases := []string{"1"}
				for i := range count {
					modules = append(modules, fmt.Sprintf("module%d", i))
					phases = append(phases, "2")
				}
				for _, module := range append(slices.Clone(modules), "application") {
					if err := os.Mkdir(filepath.Join(dir, module), 0o700); err != nil {
						t.Fatal(err)
					}
				}
				failModule := modules[len(modules)-1]
				if failure == "push" {
					failModule = modules[max(1, len(modules)/2)]
				}
				script := fmt.Sprintf(`set -euo pipefail
version=v0.19.0
MODULE_PATH=example.invalid/release-fixture
order=(%s application)
PUBLIC_MODULES=(%s)
phase_of=(%s 3)
phases=3
failure=%s
fail_module=%s
export trace=$PWD/events
step() { printf 'phase %%s\n' "$phase" >> "$trace"; }
note() { :; }
die() { echo "$*" >&2; exit 1; }
oolong_deps() {
  case "$1" in
    foundation) ;;
    application) echo module0 ;;
    *) echo foundation ;;
  esac
}
compatibility() {
  printf 'check %%s\n' "$1" >> "$trace"
  if [[ $failure == compatibility && $1 == "$fail_module" ]]; then
    die comparison-failed
  fi
}
go() { :; }
git() {
  case "$1" in
    rev-parse) echo fixture-commit ;;
    tag)
      printf 'tag %%s\n' "$3" >> "$trace"
      ;;
    push)
      shift
      printf 'push' >> "$trace"
      printf ' %%s' "$@" >> "$trace"
      printf '\n' >> "$trace"
      if [[ $failure == push && " $* " == *" $fail_module/$version "* ]]; then
        echo push-failed >&2
        return 42
      fi
      ;;
    add|commit) ;;
    *) echo "unexpected git operation: $*" >&2; exit 1 ;;
  esac
}
`, strings.Join(modules, " "), strings.Join(modules, " "), strings.Join(phases, " "), failure, failModule)
				// All state-changing tools in the production phase loop are private shell
				// fixtures: no Git repository, remote, or module cache can be modified.
				//nolint:gosec // G204: repository-owned script with fake tools in a private directory.
				cmd := exec.CommandContext(t.Context(), bash, "-c", script+execution)
				cmd.Dir = dir
				out, runErr := cmd.CombinedOutput()
				if (runErr != nil) != (failure != "none") {
					t.Fatalf("release result: %v\n%s", runErr, out)
				}
				if failure != "none" {
					message := "push-failed"
					if failure == "compatibility" {
						message = "comparison-failed"
					}
					if !strings.Contains(string(out), message) {
						t.Fatalf("failure diagnostic lost: %s", out)
					}
				}
				//nolint:gosec // G304: fixed fixture filename inside this test's private directory.
				trace, err := os.ReadFile(filepath.Join(dir, "events"))
				if err != nil {
					t.Fatal(err)
				}
				lines := strings.Split(strings.TrimSpace(string(trace)), "\n")
				var pushed []string
				for at, line := range lines {
					args, push := strings.CutPrefix(line, "push --quiet origin ")
					if !push || args == "main" {
						continue
					}
					tags := strings.Fields(args)
					if len(tags) > 3 {
						t.Errorf("push suppresses GitHub tag events: %s", line)
					}
					for _, tag := range tags {
						module := strings.TrimSuffix(tag, "/v0.19.0")
						prepared := modules[1:]
						if module == "foundation" {
							prepared = modules[:1]
						}
						for _, peer := range prepared {
							if !slices.Contains(lines[:at], "tag "+peer+"/v0.19.0") ||
								(peer != "foundation" && !slices.Contains(lines[:at], "check "+peer)) {
								t.Errorf("pushed %s before its phase prepared %s", tag, peer)
							}
						}
						pushed = append(pushed, module)
					}
					if strings.Contains(args, failModule+"/") && failure == "push" && at != len(lines)-1 {
						t.Error("release continued after a failed push")
					}
				}
				want := modules
				switch failure {
				case "compatibility":
					want = modules[:1]
				case "push":
					want = modules[:slices.Index(modules, failModule)+1]
				}
				if !slices.Equal(pushed, want) {
					t.Errorf("published modules = %v, want %v exactly once in dependency order", pushed, want)
				}
				if slices.Contains(lines, "phase 3") != (failure == "none") {
					t.Errorf("downstream phase ran without all dependency tags: %s", trace)
				}
			})
		}
	}
}

func TestReleaseComparisonPropagatesToolFailure(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	source, err := os.ReadFile("../../scripts/release.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, newline := range []string{"\n", "\r\n"} {
		fixture := strings.ReplaceAll(strings.ReplaceAll(string(source), "\r\n", "\n"), "\n", newline)
		_, body, ok := strings.Cut(strings.ReplaceAll(fixture, "\r\n", "\n"), "compatibility() {\n")
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
}
