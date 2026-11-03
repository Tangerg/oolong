//go:build windows

package process_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/Tangerg/oolong/ui/core/process"
)

const jobFixture = "--owned-job-fixture"

func TestJobContainsDescendantsThroughLauncherExitAndTermination(t *testing.T) {
	for _, mode := range []string{"exit", "terminate", "close"} {
		t.Run(mode, func(t *testing.T) {
			job, err := process.NewJob()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if closeErr := job.Close(); closeErr != nil {
					t.Error(closeErr)
				}
			})
			input, inputWriter := jobPipe(t)
			output, outputWriter := jobPipe(t)
			child, err := job.Start(process.JobStartConfig{
				Path: os.Args[0], Args: []string{os.Args[0], "-test.run=^TestJobProcessFixture$", "--", jobFixture, "parent"},
				Stdin: input, Stdout: outputWriter, Stderr: outputWriter,
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = job.Close(); _, _ = child.Wait() })
			if err = errors.Join(input.Close(), outputWriter.Close()); err != nil {
				t.Fatal(err)
			}
			pid := jobDescendant(t, output)
			handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if closeErr := windows.CloseHandle(handle); closeErr != nil {
					t.Error(closeErr)
				}
			})
			if status, err := windows.WaitForSingleObject(handle, 0); err != nil || status != uint32(windows.WAIT_TIMEOUT) {
				t.Fatalf("fixture was not live before termination: status=%d err=%v", status, err)
			}
			switch mode {
			case "exit":
				if err := inputWriter.Close(); err != nil {
					t.Fatal(err)
				}
				state, err := child.Wait()
				if err != nil || !state.Success() {
					t.Fatalf("launcher exit: state=%v err=%v", state, err)
				}
				if err := job.Terminate(23); err != nil {
					t.Fatal(err)
				}
			case "terminate":
				if err := job.Wait(); err == nil {
					t.Fatal("a live job was reported complete")
				}
				if err := job.Terminate(23); err != nil {
					t.Fatal(err)
				}
				state, err := child.Wait()
				if err != nil || state.ExitCode() != 23 {
					t.Fatalf("forced launcher exit: state=%v err=%v", state, err)
				}
			case "close":
				if err := job.Close(); err != nil {
					t.Fatal(err)
				}
				if _, err := child.Wait(); err != nil {
					t.Fatal(err)
				}
			}
			if mode != "close" {
				if err := job.Wait(); err != nil {
					t.Fatal(err)
				}
			}
			if status, err := windows.WaitForSingleObject(handle, 0); err != nil || status != windows.WAIT_OBJECT_0 {
				t.Fatalf("job completion left a live descendant: status=%d err=%v", status, err)
			}
		})
	}
}

func TestJobLaunchPreservesCallerHandlesAndExplicitDirectory(t *testing.T) {
	t.Setenv("OOLONG_JOB_FIXTURE", "inherited environment")
	job, err := process.NewJob()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := job.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	})
	directory := t.TempDir()
	input := jobFile(t, directory, "input")
	output := jobFile(t, directory, "output")
	stderr := jobFile(t, directory, "error")
	if _, err = io.WriteString(input, "prepared input"); err != nil {
		t.Fatal(err)
	}
	if _, err = input.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	inheritance := jobHandleFlags(t, output)
	argument := `words with spaces, "quotes", and \slashes\`
	config := process.JobStartConfig{
		Path: strings.TrimSuffix(os.Args[0], filepath.Ext(os.Args[0])),
		Args: []string{os.Args[0], "-test.run=^TestJobProcessFixture$", "--", jobFixture, "io", argument}, Dir: directory,
		Stdin: input, Stdout: output, Stderr: stderr,
	}
	invalid := config
	invalid.Dir = filepath.Join(directory, "missing")
	if invalidChild, launchErr := job.Start(invalid); launchErr == nil || invalidChild != nil {
		t.Fatalf("failed launch published a process: child=%v err=%v", invalidChild, launchErr)
	}
	child, err := job.Start(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = job.Close(); _, _ = child.Wait() })
	if again, launchErr := job.Start(config); !errors.Is(launchErr, os.ErrInvalid) || again != nil {
		t.Fatalf("job admitted a second launcher: child=%v err=%v", again, launchErr)
	}
	if after := jobHandleFlags(t, output); after != inheritance {
		t.Fatalf("borrowed handle changed: before=%d after=%d", inheritance, after)
	}
	state, err := child.Wait()
	if err != nil || !state.Success() {
		t.Fatalf("launcher: state=%v err=%v", state, err)
	}
	if err = job.Wait(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"stdin=prepared input", "directory=" + directory, "environment=inherited environment", "argument=" + argument} {
		if !strings.Contains(string(data), want) {
			t.Errorf("launcher output lacks %q: %q", want, data)
		}
	}
	data, err = os.ReadFile(stderr.Name())
	if err != nil || string(data) != "separate error output\n" {
		t.Fatalf("stderr handle: %q, %v", data, err)
	}
}

func TestEmptyAndClosedJobContracts(t *testing.T) {
	var zero process.Job
	if err := zero.Close(); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("zero close: %v", err)
	}
	if err := zero.Terminate(1); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("zero termination: %v", err)
	}
	job, err := process.NewJob()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = job.Close() })
	if err := job.Wait(); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("unstarted wait: %v", err)
	}
	if child, err := job.Start(process.JobStartConfig{}); !errors.Is(err, os.ErrInvalid) || child != nil {
		t.Fatalf("incomplete launch: child=%v err=%v", child, err)
	}
	for range 2 {
		if err := job.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := job.Terminate(1); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("closed termination: %v", err)
	}
	if err := job.Wait(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("closed wait: %v", err)
	}
	if child, err := job.Start(process.JobStartConfig{}); !errors.Is(err, os.ErrClosed) || child != nil {
		t.Fatalf("closed launch: child=%v err=%v", child, err)
	}
}

func jobPipe(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	return reader, writer
}

func jobFile(t *testing.T, directory, name string) *os.File {
	t.Helper()
	file, err := os.Create(filepath.Join(directory, name)) //nolint:gosec // G304: directory and name are test-owned fixture paths.
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func jobHandleFlags(t *testing.T, file *os.File) uint32 {
	t.Helper()
	query := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetHandleInformation")
	var flags uint32
	result, _, err := query.Call(file.Fd(), uintptr(unsafe.Pointer(&flags))) //nolint:gosec // G103: exact DWORD output retained through the synchronous call.
	runtime.KeepAlive(file)
	if result == 0 {
		t.Fatal(err)
	}
	return flags
}

func jobDescendant(t *testing.T, output *os.File) uint32 {
	t.Helper()
	type announcement struct {
		text string
		err  error
	}
	ready := make(chan announcement, 1)
	go func() {
		line, err := bufio.NewReader(output).ReadString('\n')
		ready <- announcement{line, err}
	}()
	select {
	case message := <-ready:
		pid, err := strconv.ParseUint(strings.TrimSpace(message.text), 10, 32)
		if err != nil || message.err != nil || pid == 0 {
			t.Fatalf("descendant readiness: %q parse=%v read=%v", message.text, err, message.err)
		}
		return uint32(pid)
	case <-time.After(5 * time.Second):
		t.Fatal("job launcher did not announce its descendant")
		return 0
	}
}

func TestJobProcessFixture(_ *testing.T) {
	index := slices.Index(os.Args, jobFixture)
	if index < 0 {
		return
	}
	switch os.Args[index+1] {
	case "child":
		time.Sleep(time.Minute)
	case "parent":
		//nolint:noctx,gosec // This controlled helper must inherit its parent's job rather than a separate cancellation owner.
		child := exec.Command(os.Args[0], "-test.run=^TestJobProcessFixture$", "--", jobFixture, "child")
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		if _, err := fmt.Fprintln(os.Stdout, child.Process.Pid); err != nil {
			os.Exit(3)
		}
		_, _ = io.Copy(io.Discard, os.Stdin)
	case "io":
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			os.Exit(4)
		}
		directory, err := os.Getwd()
		if err != nil {
			os.Exit(5)
		}
		fmt.Printf("stdin=%s\ndirectory=%s\nenvironment=%s\nargument=%s\n", input, directory, os.Getenv("OOLONG_JOB_FIXTURE"), os.Args[index+2])
		fmt.Fprintln(os.Stderr, "separate error output")
	}
	os.Exit(0)
}
