// Package mermaid prepares neutral PNG diagrams using the optional official
// Mermaid CLI. It does not parse a Mermaid subset, upload terminal images, or do
// work during drawing. Applications run Render in a worker and accept its result
// on their content owner before binding it to a terminal.
//
// The CLI and its browser are installed and versioned by the application. This
// backend supports Unix process groups and Windows 10+ Job Objects; other platforms report ErrUnsupported
// instead of promising cancellation that leaves browser children running.
package mermaid

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json/v2"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Config describes one backend instance. Zero limits choose documented defaults.
// Executable, Node and Browser are trusted application configuration, never
// diagram input. There is no shell interpolation or automatic package download.
type Config struct {
	// Executable locates the installed official mmdc package (default mmdc).
	// Node selects its JavaScript runtime (default node). Arbitrary CLI launchers
	// and argument injection are not supported; the renderer owns browser launch.
	Executable string
	Node       string
	// Browser optionally selects a Chromium executable for Puppeteer. Empty uses
	// its installed headless shell, matching the official CLI browser mode.
	Browser string
	// Theme is default, dark, forest, neutral or base. Empty selects default.
	Theme string
	// Timeout defaults to 30 seconds. Unix shutdown allows one second for graceful
	// exit and one second to confirm forced process-group termination.
	Timeout time.Duration
	// MaxSourceBytes defaults to 64 KiB; MaxOutputBytes to 8 MiB.
	MaxSourceBytes int
	MaxOutputBytes int64
	// MaxPixels defaults to 16 million decoded pixels. MaxEdges defaults to 500.
	MaxPixels int64
	MaxEdges  int
	// Width and Height are browser viewport pixels, defaulting to 1200 by 800.
	Width, Height int
}

// Renderer owns immutable configuration and serializes generation per instance.
// Waiting calls respect their contexts. Every call owns and removes its temporary
// directory and owns browser launch and shutdown independently of CLI signal handlers.
type Renderer struct {
	cfg     Config
	backend backend
	slot    chan struct{}
}

// ErrLimit identifies rejected source or output resource limits.
var ErrLimit = errors.New("mermaid: resource limit exceeded")

// Image is an owned neutral PNG; it carries no terminal handle or live process.
type Image struct {
	data []byte
	size image.Point
}

// PNG returns an independent copy suitable for Runtime.Images().Transmit.
func (i *Image) PNG() []byte {
	if i == nil {
		return nil
	}
	return slices.Clone(i.data)
}

// Size reports the PNG dimensions validated during preparation.
func (i *Image) Size() image.Point {
	if i == nil {
		return image.Point{}
	}
	return i.size
}

// New snapshots configuration and resolves the explicitly installed backend.
func New(cfg Config) (*Renderer, error) {
	if err := platformSupported(); err != nil {
		return nil, err
	}
	resolved, err := resolveBackend(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.Browser, err = absoluteBrowser(cfg.Browser); err != nil {
		return nil, err
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Renderer{cfg: cfg, backend: resolved, slot: make(chan struct{}, 1)}, nil
}

// absoluteBrowser resolves a configured browser once, at construction, so that a
// later change to PATH cannot make two renders run two different programs.
func absoluteBrowser(name string) (string, error) {
	if name == "" {
		return "", nil
	}
	found, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("mermaid: locate browser: %w", err)
	}
	absolute, err := filepath.Abs(found)
	if err != nil {
		return "", fmt.Errorf("mermaid: browser path: %w", err)
	}
	return absolute, nil
}

// applyDefaults gives every zero field the meaning [Config] states for it.
func (c *Config) applyDefaults() {
	if c.Theme == "" {
		c.Theme = "default"
	}
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
	if c.MaxSourceBytes == 0 {
		c.MaxSourceBytes = 64 << 10
	}
	if c.MaxOutputBytes == 0 {
		c.MaxOutputBytes = 8 << 20
	}
	if c.MaxPixels == 0 {
		c.MaxPixels = 16_000_000
	}
	if c.MaxEdges == 0 {
		c.MaxEdges = 500
	}
	if c.Width == 0 {
		c.Width = 1200
	}
	if c.Height == 0 {
		c.Height = 800
	}
}

// validate reports a configuration no renderer could honour. It runs after the
// defaults, so what it refuses is what the caller actually asked for.
func (c *Config) validate() error {
	switch c.Theme {
	case "default", "dark", "forest", "neutral", "base":
	default:
		return fmt.Errorf("mermaid: invalid theme %q", c.Theme)
	}
	if c.Timeout < 0 || c.MaxSourceBytes < 0 ||
		c.MaxOutputBytes < 0 || c.MaxOutputBytes == math.MaxInt64 ||
		c.MaxPixels < 0 || c.MaxEdges < 0 || c.Width < 0 || c.Height < 0 {
		return errors.New("mermaid: limits and dimensions must be positive")
	}
	if int64(c.Width) > c.MaxPixels/int64(c.Height) {
		return fmt.Errorf("%w: viewport pixels", ErrLimit)
	}
	return nil
}

// Render generates and validates PNG output. Source and diagnostics are bounded.
// A backend error, invalid PNG, timeout or cancellation returns no image. Errors
// preserve context cancellation and process exit identities with errors.Is/As.
// Limits bound accepted input/output, not the browser's peak memory or disk usage;
// an application needing an adversarial sandbox must supply one around the CLI.
func (r *Renderer) Render(ctx context.Context, source string) (result *Image, err error) {
	if r == nil || r.slot == nil {
		return nil, errors.New("mermaid: uninitialized renderer")
	}
	if !utf8.ValidString(source) {
		return nil, errors.New("mermaid: source is not valid UTF-8")
	}
	if len(source) > r.cfg.MaxSourceBytes {
		return nil, fmt.Errorf("%w: source bytes", ErrLimit)
	}
	ctx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()
	select {
	case r.slot <- struct{}{}:
		defer func() { <-r.slot }()
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	dir, err := os.MkdirTemp("", "oolong-mermaid-")
	if err != nil {
		return nil, fmt.Errorf("mermaid: temporary directory: %w", err)
	}
	defer func() {
		err = errors.Join(err, os.RemoveAll(dir), context.Cause(ctx))
		if err != nil {
			result = nil
		}
	}()
	args, output, err := r.arguments(dir)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, r.backend.node, args...) //nolint:gosec // Trusted backend configuration, fixed flags, source only on stdin.
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(source)
	diagnostics := &boundedOutput{limit: 64 << 10}
	cmd.Stdout, cmd.Stderr = diagnostics, diagnostics
	if runErr := run(ctx, cmd); runErr != nil {
		return nil, fmt.Errorf("mermaid: backend: %w: %s", runErr, diagnostics.String())
	}
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	return readImage(ctx, output, r.cfg.MaxOutputBytes, r.cfg.MaxPixels)
}

//go:embed backend.mjs
var backendSource string

func (r *Renderer) arguments(dir string) ([]string, string, error) {
	script := filepath.Join(dir, "backend.mjs")
	if err := os.WriteFile(script, []byte(backendSource), 0o600); err != nil {
		return nil, "", err
	}
	config := struct {
		Browser string `json:"browser"`
		Profile string `json:"profile"`
		Timeout int64  `json:"timeout"`
		Width   int    `json:"width"`
		Height  int    `json:"height"`
		Mermaid struct {
			SecurityLevel string `json:"securityLevel"`
			MaxTextSize   int    `json:"maxTextSize"`
			MaxEdges      int    `json:"maxEdges"`
			Theme         string `json:"theme"`
		} `json:"mermaid"`
	}{Browser: r.cfg.Browser, Profile: filepath.Join(dir, "browser"), Timeout: r.cfg.Timeout.Milliseconds(), Width: r.cfg.Width, Height: r.cfg.Height}
	config.Mermaid.SecurityLevel = "strict"
	config.Mermaid.MaxTextSize = r.cfg.MaxSourceBytes
	config.Mermaid.MaxEdges = r.cfg.MaxEdges
	config.Mermaid.Theme = r.cfg.Theme
	configPath := filepath.Join(dir, "config.json")
	if err := writeJSON(configPath, config); err != nil {
		return nil, "", err
	}
	output := filepath.Join(dir, "diagram.png")
	return []string{script, r.backend.entry, configPath, output}, output, nil
}

func writeJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func readImage(ctx context.Context, path string, maxBytes, maxPixels int64) (result *Image, err error) {
	defer func() {
		if cause := context.Cause(ctx); cause != nil {
			result = nil
			err = errors.Join(err, cause)
		}
	}()
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	data, err := readBounded(path, maxBytes)
	if err != nil {
		return nil, err
	}
	return decodePNG(data, maxPixels)
}

// readBounded reads a regular file whose size is checked both before and after it is
// opened, so a path replaced between the two cannot deliver more than the bound.
func readBounded(path string, maxBytes int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("mermaid: inspect output: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, fmt.Errorf("%w: output must be a bounded regular file", ErrLimit)
	}
	file, err := os.Open(path) //nolint:gosec // Fixed output name under the render's private directory.
	if err != nil {
		return nil, fmt.Errorf("mermaid: read output: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err = file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, fmt.Errorf("%w: output bytes", ErrLimit)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w: output bytes", ErrLimit)
	}
	return data, nil
}

// decodePNG reads the dimensions before the pixels, so an image claiming more of them
// than the bound allows is refused rather than decoded and then measured.
func decodePNG(data []byte, maxPixels int64) (*Image, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("mermaid: invalid PNG: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width) > maxPixels/int64(cfg.Height) {
		return nil, fmt.Errorf("%w: output pixels", ErrLimit)
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("mermaid: invalid PNG: %w", err)
	}
	return &Image{data: data, size: image.Pt(cfg.Width, cfg.Height)}, nil
}

type boundedOutput struct {
	bytes.Buffer
	limit int
}

func (b *boundedOutput) Write(data []byte) (int, error) {
	length := len(data)
	_, _ = b.Buffer.Write(data[:min(length, max(0, b.limit-b.Len()))])
	return length, nil
}
