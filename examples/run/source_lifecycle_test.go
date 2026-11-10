package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/examples/internal/drawn"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/uitest"
)

const (
	pipeHolderFixture      = "--pipe-holder"
	pipeHolderReady        = "ready\n"
	pipeHolderAnnouncement = "retained-stdout:"
)

func TestCommandDescendantsStopOnCancellationAndParentExit(t *testing.T) {
	for _, ending := range []string{"exit", "cancel"} {
		t.Run(ending, func(t *testing.T) {
			listener, listenErr := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if listenErr != nil {
				t.Fatal(listenErr)
			}
			t.Cleanup(func() { _ = listener.Close() })
			if err := listener.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatal(err)
			}
			host := uitest.New(t, uitest.Config{Width: 40, Height: 4})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			ready := make(chan *program.Task, 1)
			programDone := make(chan error, 1)
			go func() {
				app, err := program.NewInline(program.InlineConfig{Host: host, Build: func(runtime *program.InlineRuntime) (scene.Element, error) { //nolint:contextcheck // Run(ctx) parents this scope; cancellation through the runtime is under test.
					r := &runner{
						keys:    runnerKeys(),
						out:     new(text.Decoder),
						runtime: runtime,
						command: []string{os.Args[0], "-test.run=^TestCommandPipeHolder$", "--", pipeHolderFixture, ending, listener.Addr().String()},
					}
					r.element = drawn.Element("run", r)
					scope, err := runtime.NewScope(program.ScopeConfig{})
					if err != nil {
						return scene.Element{}, err
					}
					task, err := r.start(scope)
					ready <- task
					if err != nil {
						return scene.Element{}, err
					}
					return r.Element(), nil
				}})
				if err == nil {
					err = app.Run(ctx)
				}
				programDone <- err
			}()
			t.Cleanup(func() {
				cancel()
				if err := <-programDone; err != nil {
					t.Errorf("run output program: %v", err)
				}
			})
			task := <-ready
			sourceDone := task.Done()
			connection, err := listener.AcceptTCP()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = connection.Close() })
			if err = connection.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
				t.Fatal(err)
			}
			readyLine, err := bufio.NewReader(connection).ReadString('\n')
			if err != nil || readyLine != pipeHolderReady {
				t.Fatalf("descendant readiness: %q, %v", readyLine, err)
			}
			host.Until(t, "child to announce its pipe holder", func() bool {
				return strings.Contains(host.Frames(), pipeHolderAnnouncement)
			})
			if ending == "cancel" {
				cancel()
			}
			select {
			case <-sourceDone:
			case <-time.After(3 * time.Second):
				t.Fatal("command source kept waiting for a pipe its process no longer owns")
			}
			if err := task.Snapshot().Err; ending == "exit" && err != nil || ending == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("command %s: %v", ending, err)
			}
			// The fixture keeps this connection for its lifetime. Forced process exit
			// may reset TCP instead of sending FIN, especially on Windows.
			if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			var tail [1]byte
			if n, err := connection.Read(tail[:]); !connectionEnded(n, err) {
				t.Fatalf("command task finished without evidence of descendant connection closure: %d, %v", n, err)
			}
		})
	}
}

func connectionEnded(n int, err error) bool {
	return n == 0 && (errors.Is(err, io.EOF) || connectionReset(err))
}

func TestDescendantConnectionClosureRejectsLiveConnections(t *testing.T) {
	for _, mode := range []string{"closed", "reset", "live", "data"} {
		t.Run(mode, func(t *testing.T) {
			listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			client, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			connection, err := listener.AcceptTCP()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = connection.Close() })
			if err = connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "closed":
				err = client.Close()
			case "reset":
				if err = client.(*net.TCPConn).SetLinger(0); err == nil {
					err = client.Close()
				}
			case "live":
				err = connection.SetReadDeadline(time.Now())
			case "data":
				_, err = client.Write([]byte("still running"))
			}
			if err != nil {
				t.Fatal(err)
			}
			var tail [1]byte
			n, err := connection.Read(tail[:])
			wantClosed := mode == "closed" || mode == "reset"
			if connectionEnded(n, err) != wantClosed {
				t.Fatalf("closure evidence: n=%d err=%v, want closed=%v", n, err, wantClosed)
			}
		})
	}
	if connectionEnded(0, errors.New("unrelated I/O failure")) {
		t.Fatal("an unknown failure was accepted as termination evidence")
	}
}

func TestCommandPipeHolder(t *testing.T) {
	index := slices.Index(os.Args, pipeHolderFixture)
	if index < 0 {
		return
	}
	mode, endpoint := os.Args[index+1], os.Args[index+2]
	if mode == "hold" {
		connection, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", endpoint)
		if err != nil {
			os.Exit(2)
		}
		if _, err := io.WriteString(connection, pipeHolderReady); err != nil {
			os.Exit(3)
		}
		fmt.Printf("%s%d\n", pipeHolderAnnouncement, os.Getpid())
		_, _ = io.Copy(io.Discard, connection)
		_ = connection.Close()
		os.Exit(0)
	}
	//nolint:gosec // G204: this controlled test subprocess retains the inherited pipe.
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestCommandPipeHolder$", "--", pipeHolderFixture, "hold", endpoint)
	child.Stderr = os.Stderr
	output, err := child.StdoutPipe()
	if err != nil {
		os.Exit(4)
	}
	if err = child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	announcement, err := bufio.NewReader(output).ReadString('\n')
	if err != nil {
		os.Exit(5)
	}
	fmt.Print(announcement)
	if mode == "cancel" {
		_ = child.Wait()
	}
	os.Exit(0)
}
