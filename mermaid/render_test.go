//go:build unix || windows

package mermaid_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
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
func backendConfig(t *testing.T) mermaid.Config {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required for backend contract tests")
	}
	root := t.TempDir()
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
	write("node_modules/@mermaid-js/mermaid-cli/index.js", `import {writeFile} from 'node:fs/promises';
 export async function renderMermaid(browser,source) {
 if(source.startsWith('wait:')) {await writeFile(source.slice(5),'ready'); await new Promise(()=>{setInterval(()=>{},1000)});}
 if(source.startsWith('spin:')) {process.on('SIGINT',()=>{});await writeFile(source.slice(5),String(globalThis.browserPID));while(true){}}
 if(source==='fail') throw Error('intentional backend diagnostic');
 return {data:Buffer.from('`+base64.StdEncoding.EncodeToString(data.Bytes())+`','base64')};}`)
	write("node_modules/puppeteer/package.json", `{"name":"puppeteer","type":"module","exports":"./index.js"}`)
	write("node_modules/puppeteer/index.js", `export default {executablePath:()=>process.execPath,defaultArgs:()=>[],connect:async()=>({close:async()=>{}})};`)
	write("node_modules/@puppeteer/browsers/package.json", `{"name":"@puppeteer/browsers","type":"module","exports":"./index.js"}`)
	write("node_modules/@puppeteer/browsers/index.js", `import {spawn} from 'node:child_process'; export const CDP_WEBSOCKET_ENDPOINT_REGEX=/./; export function launch(options) {if(options.detached!==false)throw Error('detached browser'); const child=spawn(process.execPath,['-e',"process.on('SIGINT',()=>{});setInterval(()=>{},1000)"],{detached:options.detached,stdio:'ignore'}); globalThis.browserPID=child.pid; const exited=new Promise(resolve=>child.on('exit',resolve)); return {waitForLineOutput:async()=>'',close:async()=>{child.kill();await exited}};}`)
	bin := "mmdc"
	if runtime.GOOS == "windows" {
		bin += ".cmd"
	}
	write("node_modules/.bin/"+bin, "#!/bin/sh\nexit 1\n")
	return mermaid.Config{Executable: filepath.Join(root, "node_modules", ".bin", bin), Node: node, Timeout: 3 * time.Second}
}

func TestPublicBackendConfigurationAndResults(t *testing.T) {
	cfg := backendConfig(t)
	renderer, err := mermaid.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	result, err := renderer.Render(t.Context(), "png")
	if err != nil || result.Size() != image.Pt(3, 2) {
		t.Fatalf("result=%v err=%v", result, err)
	}
	if _, renderErr := renderer.Render(t.Context(), "fail"); renderErr == nil || !strings.Contains(renderErr.Error(), "intentional backend diagnostic") {
		t.Fatal(renderErr)
	}
	cfg.MaxSourceBytes = 3
	limited, err := mermaid.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, renderErr := limited.Render(t.Context(), "long"); !errors.Is(renderErr, mermaid.ErrLimit) {
		t.Fatal(renderErr)
	}
	if _, renderErr := renderer.Render(t.Context(), string([]byte{255})); renderErr == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	cfg.MaxSourceBytes = 0
	cfg.Timeout = 50 * time.Millisecond
	timed, err := mermaid.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := timed.Render(t.Context(), "wait:"+filepath.Join(t.TempDir(), "ready")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestQueuedCancellationAndSlotReuseThroughPublicRender(t *testing.T) {
	cfg := backendConfig(t)
	renderer, err := mermaid.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ready := filepath.Join(t.TempDir(), "ready")
	done := make(chan error, 1)
	go func() {
		_, renderErr := renderer.Render(ctx, "wait:"+ready)
		done <- renderErr
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("backend never ready")
		}
		time.Sleep(time.Millisecond)
	}
	queued, stop := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer stop()
	if _, err := renderer.Render(queued, "png"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued=%v", err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := renderer.Render(t.Context(), "png"); err != nil {
		t.Fatal(err)
	}
}
