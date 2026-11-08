package arch

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type pinnedTool struct {
	name, module, version, path string
}

func pinnedTools(t *testing.T) []pinnedTool {
	t.Helper()
	command := exec.CommandContext(t.Context(), "bash", filepath.Join(repoRoot(t), "scripts", "tools.sh"), "list") //nolint:gosec // Executable arguments come from the committed pin manifest and isolated test fixtures, never external input.
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("tool pins: %v\n%s", err, output)
	}
	var tools []pinnedTool
	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 {
			t.Fatalf("invalid tool pin: %q", line)
		}
		tools = append(tools, pinnedTool{fields[0], fields[1], fields[2], fields[3]})
	}
	return tools
}

func (tool pinnedTool) buildInfo() string {
	return fmt.Sprintf("fixture: go1.27.0\n\tpath\t%s\n\tmod\t%s\t%s\th1:fixture\n", tool.path, tool.module, tool.version)
}

func writeToolBuildInfo(t *testing.T, bin string) {
	t.Helper()
	for _, tool := range pinnedTools(t) {
		writeReleaseFixture(t, filepath.Join(bin, tool.name+".buildinfo"), tool.buildInfo())
	}
}

func TestPinnedToolsRejectUnverifiableBuilds(t *testing.T) {
	tool := pinnedTools(t)[0]
	for _, test := range []struct {
		name, metadata string
	}{
		{"script without build identity", ""},
		{"different command", strings.Replace(tool.buildInfo(), "\tpath\t"+tool.path, "\tpath\texample.invalid/other", 1)},
		{"different module", strings.Replace(tool.buildInfo(), "\tmod\t"+tool.module, "\tmod\texample.invalid/other", 1)},
		{"different version", strings.Replace(tool.buildInfo(), tool.version, "v0.0.1", 1)},
		{"local replacement", tool.buildInfo() + "\t=>\t../local-checker\t(devel)\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			bin := t.TempDir()
			writeReleaseFixture(t, filepath.Join(bin, tool.name), "#!/bin/sh\nexit 0\n")
			writeReleaseFixture(t, filepath.Join(bin, tool.name+".buildinfo"), test.metadata)
			writeReleaseFixture(t, filepath.Join(bin, "go"), "#!/bin/sh\nset -eu\ntest \"$1 $2\" = 'version -m'\ncat \"$3.buildinfo\"\n")
			command := exec.CommandContext(t.Context(), "bash", filepath.Join(repoRoot(t), "scripts", "tools.sh"), "check", tool.name) //nolint:gosec // Executable arguments come from the committed pin manifest and isolated test fixtures, never external input.
			command.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), tool.path+"@"+tool.version) {
				t.Fatalf("unverifiable tool admitted: %v\n%s", err, output)
			}
		})
	}
}

func TestPinnedToolsInstallAndCheckOneVersion(t *testing.T) {
	var tool pinnedTool
	for _, candidate := range pinnedTools(t) {
		if candidate.path == candidate.module {
			tool = candidate
			break
		}
	}
	if tool.name == "" {
		t.Fatal("fixture needs a pinned command at its module root")
	}
	proxy, bin := t.TempDir(), filepath.Join(t.TempDir(), "tool binaries")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	const wrongVersion = "v0.0.1"
	for _, version := range []string{tool.version, wrongVersion} {
		publishFixtureModule(t, proxy, tool.module, version, "package main\n\nfunc main() {}\n")
	}
	writeReleaseFixture(t, filepath.Join(proxy, filepath.FromSlash(tool.module), "@v", "list"), tool.version+"\n"+wrongVersion+"\n")
	proxyURL := url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(proxy), "/")}
	environment := append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GOBIN="+bin, "GOMODCACHE="+t.TempDir(), "GOWORK=off", "GOFLAGS=-modcacherw",
		"GOPROXY="+proxyURL.String(), "GONOPROXY=none", "GOSUMDB=off")
	for _, action := range []string{"install", "check"} {
		command := exec.CommandContext(t.Context(), "bash", filepath.Join(repoRoot(t), "scripts", "tools.sh"), action, tool.name) //nolint:gosec // Executable arguments come from the committed pin manifest and isolated test fixtures, never external input.
		command.Env = environment
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%s pinned tool: %v\n%s", action, err, output)
		}
	}
	command := exec.CommandContext(t.Context(), "go", "install", tool.path+"@"+wrongVersion) //nolint:gosec // Executable arguments come from the committed pin manifest and isolated test fixtures, never external input.
	command.Env = environment
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("install another version: %v\n%s", err, output)
	}
	command = exec.CommandContext(t.Context(), "bash", filepath.Join(repoRoot(t), "scripts", "tools.sh"), "check", tool.name) //nolint:gosec // Executable arguments come from the committed pin manifest and isolated test fixtures, never external input.
	command.Env = environment
	if output, err := command.CombinedOutput(); err == nil || !strings.Contains(string(output), tool.path+"@"+tool.version) {
		t.Fatalf("another installed version admitted: %v\n%s", err, output)
	}
}

func TestPinnedToolsRejectUnknownNamesBeforeInstallation(t *testing.T) {
	bin, marker := t.TempDir(), filepath.Join(t.TempDir(), "installed")
	writeReleaseFixture(t, filepath.Join(bin, "go"), "#!/bin/sh\ntouch \"$INSTALL_MARKER\"\n")
	command := exec.CommandContext(t.Context(), "bash", filepath.Join(repoRoot(t), "scripts", "tools.sh"), "install", pinnedTools(t)[0].name, "unknown-tool") //nolint:gosec // Executable arguments come from the committed pin manifest and isolated test fixtures, never external input.
	command.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "INSTALL_MARKER="+marker)
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "unknown pinned tool: unknown-tool") {
		t.Fatalf("unknown tool admitted: %v\n%s", err, output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("invalid selection started installation: %v", err)
	}
}

func TestReleaseRejectsMismatchedCheckerVersion(t *testing.T) {
	fixture := newReleaseSnapshot(t, true)
	tool := pinnedTools(t)[0]
	writeReleaseFixture(t, filepath.Join(fixture.bin, tool.name+".buildinfo"), strings.Replace(tool.buildInfo(), tool.version, "v0.0.1", 1))
	command := exec.CommandContext(t.Context(), "bash", "scripts/release.sh", "ui", "0.1.0")
	command.Dir = fixture.root
	command.Env = append(os.Environ(), "PATH="+fixture.bin+string(os.PathListSeparator)+os.Getenv("PATH"), "FIXTURE_GO="+fixture.goBin, "GOWORK=off")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "release checkers do not match the committed tool pins") {
		t.Fatalf("release admitted an unpinned checker: %v\n%s", err, output)
	}
}
