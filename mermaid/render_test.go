//go:build unix || windows

package mermaid_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/mermaid"
)

// The fixture exercises the public configuration and real Node/process boundary
// with small installed-package doubles. Real Mermaid/Chromium has a separate test.
func backendConfig(t *testing.T, startupDelay time.Duration) mermaid.Config {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required for backend contract tests")
	}
	root := t.TempDir()
	t.Logf("backend fixture created at %s", time.Now().UTC().Format(time.RFC3339Nano))
	write := func(name, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		//nolint:gosec // G306: executable fixtures in a private temporary directory.
		if err := os.WriteFile(p, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	write("node_modules/@mermaid-js/mermaid-cli/package.json", `{"name":"@mermaid-js/mermaid-cli","type":"module","exports":"./index.js","bin":{"mmdc":"cli.js"}}`)
	write("node_modules/@mermaid-js/mermaid-cli/cli.js", "// entry locator")
	write("node_modules/@mermaid-js/mermaid-cli/index.js", fmt.Sprintf(`import {appendFileSync} from 'node:fs';
 const stages = %q;
 const stage = name => appendFileSync(stages, JSON.stringify({pid:process.pid,stage:name,time:Date.now()})+'\n');
 globalThis.backendStage = stage;
 stage('module loaded');
 await new Promise(resolve=>setTimeout(resolve,%d));
 const ready = async (url, value) => {
   const response = await fetch(url+'?ready='+encodeURIComponent(value));
   if (!response.ok) throw Error('readiness delivery failed');
   stage('ready');
 };
 process.on('exit',()=>stage('launcher exit'));
 export async function renderMermaid(browser,source) {
 stage('render entered');
 if(source.startsWith('wait:')) {await ready(source.slice(5),'ready'); await new Promise(()=>{setInterval(()=>{},1000)});}
 if(source.startsWith('spin:')) {process.on('SIGINT',()=>{});await ready(source.slice(5),String(globalThis.browserPID));while(true){}}
 if(source==='fail') throw Error('intentional backend diagnostic');
 return {data:Buffer.from('%s','base64')};}`, filepath.Join(root, "stages.jsonl"), startupDelay.Milliseconds(), base64.StdEncoding.EncodeToString(data.Bytes())))
	write("node_modules/puppeteer/package.json", `{"name":"puppeteer","type":"module","exports":"./index.js"}`)
	write("node_modules/puppeteer/index.js", `export default {
 executablePath: options => {
   if (options.headless !== 'shell') throw Error('wrong browser mode');
   return process.execPath;
 },
 defaultArgs: options => {
   if (options.headless !== 'shell' || !options.userDataDir) throw Error('wrong browser arguments');
   return [];
 },
 connect: async () => ({on: () => {}, close: async () => {}}),
};`)
	write("node_modules/@puppeteer/browsers/package.json", `{"name":"@puppeteer/browsers","type":"module","exports":"./index.js"}`)
	write("node_modules/@puppeteer/browsers/index.js", `import {spawn} from 'node:child_process';
 export const CDP_WEBSOCKET_ENDPOINT_REGEX = /./;
 export function launch(options) {
   if (options.detached !== false) throw Error('detached browser');
   const child = spawn(process.execPath, ['-e', "process.on('SIGINT',()=>{});setInterval(()=>{},1000)"], {
     detached: options.detached, stdio: 'ignore',
   });
   globalThis.browserPID = child.pid;
   globalThis.backendStage('browser spawned');
   const exited = new Promise(resolve => child.on('exit', () => {
     globalThis.backendStage('browser exited');
     resolve();
   }));
   return {
     nodeProcess: child,
     waitForLineOutput: async () => '',
     close: async () => {child.kill(); await exited},
   };
 }`)

	t.Cleanup(func() {
		data, err := os.ReadFile(filepath.Join(root, "stages.jsonl")) //nolint:gosec // G304: fixture-owned stage diagnostics.
		if err == nil {
			t.Logf("backend stages: %s", data)
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Error(err)
		}
	})
	bin := "mmdc"
	if runtime.GOOS == "windows" {
		bin += ".cmd"
	}
	write("node_modules/.bin/"+bin, "#!/bin/sh\nexit 1\n")
	return mermaid.Config{Executable: filepath.Join(root, "node_modules", ".bin", bin), Node: node, Timeout: 30 * time.Second}
}

func TestPublicBackendConfigurationAndResults(t *testing.T) {
	cfg := backendConfig(t, 0)
	renderer, err := mermaid.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), cfg.Timeout)
	defer cancel()
	result, err := renderer.Render(ctx, "png")
	if err != nil || result.Size() != image.Pt(3, 2) {
		t.Fatalf("result=%v err=%v", result, err)
	}
	if _, renderErr := renderer.Render(ctx, "fail"); renderErr == nil || !strings.Contains(renderErr.Error(), "intentional backend diagnostic") {
		t.Fatal(renderErr)
	}
	cfg.MaxSourceBytes = 3
	limited, err := mermaid.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, renderErr := limited.Render(ctx, "long"); !errors.Is(renderErr, mermaid.ErrLimit) {
		t.Fatal(renderErr)
	}
	if _, renderErr := renderer.Render(ctx, string([]byte{255})); renderErr == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}

func TestRenderTimeoutIncludesBackendStartup(t *testing.T) {
	cfg := backendConfig(t, time.Second)
	cfg.Timeout = 50 * time.Millisecond
	timed, err := mermaid.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := timed.Render(t.Context(), "png"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

// renderCall joins before fixture cleanup even when a readiness assertion fails.
// Closing done publishes err and permits both readiness and cleanup to observe it.
type renderCall struct {
	done    chan struct{}
	cancel  context.CancelFunc
	err     error
	elapsed time.Duration
}

func startRender(ctx context.Context, t *testing.T, renderer *mermaid.Renderer, source string) *renderCall {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	call := &renderCall{done: make(chan struct{}), cancel: cancel}
	started := time.Now()
	go func() {
		_, call.err = renderer.Render(ctx, source)
		call.elapsed = time.Since(started)
		close(call.done)
	}()
	t.Cleanup(func() {
		cancel()
		<-call.done
		t.Logf("Render finished after %s; cleanup joined: %v", call.elapsed, call.err)
	})
	return call
}

// A loopback request publishes readiness without opening a file that another
// process may still hold during a Windows rename. Render remains the result owner.
func readinessServer(t *testing.T) (string, <-chan []byte) {
	t.Helper()
	ready := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		ready <- []byte(request.URL.Query().Get("ready"))
	}))
	t.Cleanup(server.Close)
	return server.URL, ready
}

func waitReady(ctx context.Context, ready <-chan []byte, call *renderCall) ([]byte, error) {
	select {
	case <-call.done:
		return nil, errors.Join(errors.New("backend ended before readiness"), call.err)
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	case data := <-ready:
		return data, nil
	}
}

func TestQueuedCancellationAndSlotReuseThroughPublicRender(t *testing.T) {
	for _, delay := range []time.Duration{0, 2250 * time.Millisecond} {
		t.Run(delay.String(), func(t *testing.T) {
			cfg := backendConfig(t, delay)
			renderer, err := mermaid.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), cfg.Timeout)
			defer cancel()
			endpoint, ready := readinessServer(t)
			call := startRender(ctx, t, renderer, "wait:"+endpoint)
			if data, err := waitReady(ctx, ready, call); err != nil || string(data) != "ready" {
				t.Fatalf("readiness=%q error=%v", data, err)
			}
			queued, stop := context.WithTimeout(ctx, 30*time.Millisecond)
			defer stop()
			if _, err := renderer.Render(queued, "png"); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("queued=%v", err)
			}
			call.cancel()
			<-call.done
			if !errors.Is(call.err, context.Canceled) {
				t.Fatal(call.err)
			}
			if _, err := renderer.Render(ctx, "png"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReadinessReportsBackendFailureAndCancellation(t *testing.T) {
	for _, mode := range []string{"fail", "cancel", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			cfg := backendConfig(t, 0)
			renderer, err := mermaid.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), cfg.Timeout)
			defer cancel()
			endpoint, ready := readinessServer(t)
			source := "wait:" + endpoint
			if mode == "fail" {
				source = "fail"
			}
			call := startRender(ctx, t, renderer, source)
			if mode == "fail" {
				if _, err := waitReady(ctx, ready, call); err == nil || !strings.Contains(err.Error(), "intentional backend diagnostic") {
					t.Fatalf("startup failure: %v", err)
				}
				return
			}
			if _, err := waitReady(ctx, ready, call); err != nil {
				t.Fatal(err)
			}
			waiting, stop := context.WithCancel(ctx)
			want := context.Canceled
			if mode == "deadline" {
				stop()
				waiting, stop = context.WithDeadline(ctx, time.Now())
				want = context.DeadlineExceeded
			} else {
				stop()
			}
			defer stop()
			if _, err := waitReady(waiting, nil, call); !errors.Is(err, want) {
				t.Fatalf("readiness: %v", err)
			}
			// Cleanup cancels and joins the still-running render before its files disappear.
		})
	}
}
