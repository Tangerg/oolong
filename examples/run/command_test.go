package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

const (
	commandFixture       = "--command-fixture"
	commandFixtureEnv    = "OOLONG_COMMAND_FIXTURE"
	commandFixtureStatus = 7
)

func TestCommandPreservesInheritedIOAndExitStatus(t *testing.T) {
	t.Setenv(commandFixtureEnv, "inherited value")
	directory := t.TempDir()
	t.Chdir(directory)
	for _, mode := range []string{"success", "failure"} {
		t.Run(mode, func(t *testing.T) {
			var output bytes.Buffer
			argument := `words with spaces, "quotes", and \slashes\`
			err := runCommand(t.Context(), []string{os.Args[0], "-test.run=^TestCommandProcessFixture$", "--", commandFixture, mode, argument}, &output)
			if mode == "success" && err != nil {
				t.Fatal(err)
			}
			if mode == "failure" {
				exit, ok := errors.AsType[*exec.ExitError](err)
				if !ok || exit.ExitCode() != commandFixtureStatus {
					t.Fatalf("exit identity: %v", err)
				}
			}
			for _, want := range []string{"stdin=0", "directory=" + directory, "environment=inherited value", "argument=" + argument, "stderr retained"} {
				if !strings.Contains(output.String(), want) {
					t.Errorf("command output lacks %q: %q", want, output.String())
				}
			}
		})
	}
}

func TestCommandPreservesCancellationCause(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("command no longer belongs to the application")
	cancel(cause)
	var output bytes.Buffer
	if err := runCommand(ctx, []string{os.Args[0], "-test.run=^TestCommandProcessFixture$", "--", commandFixture, "success", "unused"}, &output); !errors.Is(err, cause) {
		t.Fatalf("cancellation cause: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("cancelled command produced output: %q", output.String())
	}
}

func TestCommandProcessFixture(_ *testing.T) {
	index := slices.Index(os.Args, commandFixture)
	if index < 0 {
		return
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(2)
	}
	directory, err := os.Getwd()
	if err != nil {
		os.Exit(3)
	}
	fmt.Printf("stdin=%d\ndirectory=%s\nenvironment=%s\nargument=%s\n", len(input), directory, os.Getenv(commandFixtureEnv), os.Args[index+2])
	fmt.Fprintln(os.Stderr, "stderr retained")
	if os.Args[index+1] == "failure" {
		os.Exit(commandFixtureStatus)
	}
	os.Exit(0)
}

func TestCancelledRunReportsItsCauseOnce(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("run no longer belongs to the application")
	cancel(cause)
	pump := &outputPump{command: []string{os.Args[0], "-test.run=^TestCommandProcessFixture$", "--", commandFixture, "success", "unused"}}
	err := pump.run(ctx)
	if !errors.Is(err, cause) || strings.Count(err.Error(), cause.Error()) != 1 {
		t.Fatalf("cancelled run reported %v", err)
	}
}
