//go:build windows

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestCommandResolvesWindowsExecutableExtensions(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Dir(executable))
	withoutExtension := strings.TrimSuffix(executable, filepath.Ext(executable))
	for _, path := range []string{withoutExtension, "." + string(os.PathSeparator) + filepath.Base(withoutExtension)} {
		t.Run(path, func(t *testing.T) {
			var output bytes.Buffer
			if err := runCommand(t.Context(), []string{path, "-test.run=^TestCommandProcessFixture$", "--", commandFixture, "success", "extension resolved"}, &output); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "argument=extension resolved") {
				t.Fatalf("command did not execute: %q", output.String())
			}
		})
	}
}

func TestCommandJobDoesNotRetainAProcessOutsideItsJob(t *testing.T) {
	job, err := newCommandJob()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := job.close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	handle, owned, err := job.observeProcess(windows.GetCurrentProcessId())
	if handle != 0 {
		t.Cleanup(func() {
			if closeErr := windows.CloseHandle(handle); closeErr != nil {
				t.Error(closeErr)
			}
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	if owned || handle != 0 {
		t.Fatal("a notification for an unrelated PID acquired command wait ownership")
	}
}
