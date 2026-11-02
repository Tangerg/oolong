// Package mermaid delegates Mermaid syntax to the optional official CLI so no
// second parser dialect develops here. Applications prepare PNGs in workers and
// accept immutable results on their content owner.
//
// The application installs and versions the CLI and browser. Unix process groups
// and Windows 10+ jobs provide cancellation ownership; other platforms reject
// rendering because they cannot guarantee browser-child cleanup.
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

const (
	defaultRenderTimeout  = 30 * time.Second
	defaultMaxSourceBytes = 64 << 10
	defaultMaxOutputBytes = 8 << 20
	defaultMaxPixels      = 16_000_000
	defaultMaxEdges       = 500
	defaultViewportWidth  = 1200
	defaultViewportHeight = 800
	maxDiagnosticBytes    = 64 << 10
)

// Config treats Executable, Node and Browser as trusted application configuration because
// source must never select code to execute. Zero limits choose bounded defaults.
type Config struct {
	// Resolve the installed official mmdc package (default mmdc) and its runtime
	// (default node) so the adapter owns browser launch without interpreting shims.
	Executable string
	Node       string
	// An empty browser uses the installed headless shell to match the official CLI
	// rendering mode.
	Browser string
	Theme   string
	// The deadline includes admission waiting so queued renders cannot wait forever.
	// Unix shutdown also allows bounded graceful and forced process-group cleanup.
	Timeout        time.Duration
	MaxSourceBytes int
	MaxOutputBytes int64
	MaxPixels      int64
	MaxEdges       int
	// Puppeteer sizes content in viewport pixels before the PNG is validated.
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

var ErrLimit = errors.New("mermaid: resource limit exceeded")

// Image may outlive its browser because its immutable PNG retains no terminal handle
// or live process.
type Image struct {
	data []byte
	size image.Point
}

// PNG detaches its bytes so image consumers can retain or modify their copy independently.
func (i *Image) PNG() []byte {
	if i == nil {
		return nil
	}
	return slices.Clone(i.data)
}

func (i *Image) Size() image.Point {
	if i == nil {
		return image.Point{}
	}
	return i.size
}

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

func (c *Config) applyDefaults() {
	if c.Theme == "" {
		c.Theme = "default"
	}
	if c.Timeout == 0 {
		c.Timeout = defaultRenderTimeout
	}
	if c.MaxSourceBytes == 0 {
		c.MaxSourceBytes = defaultMaxSourceBytes
	}
	if c.MaxOutputBytes == 0 {
		c.MaxOutputBytes = defaultMaxOutputBytes
	}
	if c.MaxPixels == 0 {
		c.MaxPixels = defaultMaxPixels
	}
	if c.MaxEdges == 0 {
		c.MaxEdges = defaultMaxEdges
	}
	if c.Width == 0 {
		c.Width = defaultViewportWidth
	}
	if c.Height == 0 {
		c.Height = defaultViewportHeight
	}
}

// Validate after defaults so zero values retain their useful construction meaning.
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

// Render returns no image on failure, timeout or cancellation, while errors
// preserve cancellation and process identities. Accepted input and output are
// bounded; containing browser peak memory or disk usage requires an external sandbox.
func (r *Renderer) Render(ctx context.Context, source string) (result *Image, err error) {
	if r == nil || r.slot == nil {
		return nil, errors.New("mermaid: uninitialized renderer")
	}
	if len(source) > r.cfg.MaxSourceBytes {
		return nil, fmt.Errorf("%w: source bytes", ErrLimit)
	}
	if !utf8.ValidString(source) {
		return nil, errors.New("mermaid: source is not valid UTF-8")
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
	diagnostics := &boundedOutput{limit: maxDiagnosticBytes}
	cmd.Stdout, cmd.Stderr = diagnostics, diagnostics
	if runErr := run(ctx, cmd); runErr != nil {
		return nil, fmt.Errorf("mermaid: backend: %w: %q", runErr, diagnostics.buffer.String())
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
	reader := bytes.NewReader(data)
	if _, err := png.Decode(reader); err != nil {
		return nil, fmt.Errorf("mermaid: invalid PNG: %w", err)
	}
	if reader.Len() != 0 {
		return nil, errors.New("mermaid: trailing data after PNG")
	}
	return &Image{data: data, size: image.Pt(cfg.Width, cfg.Height)}, nil
}

type boundedOutput struct {
	buffer bytes.Buffer
	limit  int
}

func (b *boundedOutput) Write(data []byte) (int, error) {
	length := len(data)
	_, _ = b.buffer.Write(data[:min(length, max(0, b.limit-b.buffer.Len()))])
	return length, nil
}
