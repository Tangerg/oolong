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
	commandFixture              = "--command-fixture"
	commandFixtureEnv           = "OOLONG_COMMAND_FIXTURE"
	commandFixtureStatus        = 7
	commandFixtureOutputFailure = "output-failure"
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

func TestCommandCanDiscardOutput(t *testing.T) {
	if err := runCommand(t.Context(), []string{os.Args[0], "-test.run=^TestCommandProcessFixture$", "--", commandFixture, "success", "unused"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestCommandPreservesOutputFailureAlongsideExitStatus(t *testing.T) {
	outputFailure := errors.New("output destination refused data")
	for _, test := range []struct {
		name     string
		write    func([]byte) (int, error)
		want     error
		wantText string
	}{
		{name: "refusal", write: func([]byte) (int, error) { return 0, outputFailure }, want: outputFailure},
		{name: "full write with error", write: func(data []byte) (int, error) { return len(data), outputFailure }, want: outputFailure},
		{name: "short write", write: func(data []byte) (int, error) { return len(data) - 1, nil }, want: io.ErrShortWrite},
		{name: "negative count", write: func([]byte) (int, error) { return -1, outputFailure }, want: outputFailure, wantText: "run: output returned -1 bytes"},
		{name: "excess count", write: func(data []byte) (int, error) { return len(data) + 1, nil }, wantText: "run: output returned"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := runCommand(t.Context(), []string{os.Args[0], "-test.run=^TestCommandProcessFixture$", "--", commandFixture, commandFixtureOutputFailure, "unused"}, commandTestWriter(test.write))
			if test.want != nil && (!errors.Is(err, test.want) || strings.Count(err.Error(), test.want.Error()) != 1) {
				t.Fatalf("output failure = %v, want %v exactly once", err, test.want)
			}
			if test.wantText != "" && (err == nil || !strings.Contains(err.Error(), test.wantText)) {
				t.Fatalf("output failure = %v, want %q", err, test.wantText)
			}
			if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandFixtureStatus {
				t.Fatalf("exit identity = %v", err)
			}
		})
	}
}

type commandTestWriter func([]byte) (int, error)

func (write commandTestWriter) Write(data []byte) (int, error) { return write(data) }

func TestCommandProcessFixture(t *testing.T) {
	index := slices.Index(os.Args, commandFixture)
	if index < 0 {
		return
	}
	if os.Args[index+1] == commandFixtureOutputFailure {
		// A later write could receive SIGPIPE after the parent refuses this output.
		if _, err := fmt.Fprintln(os.Stdout, "output before failure"); err != nil {
			t.Fatal(err)
		}
		os.Exit(commandFixtureStatus)
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
