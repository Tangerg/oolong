// Package content dispatches explicit format names to application-configured
// renderers. It imports no format implementation and has no global registry.
package content

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/Tangerg/oolong/core/grid"
)

// Format is an explicit content label, normalized by trimming space and folding
// ASCII letters to lowercase. Labels contain letters, digits, '.', '+' or '-'.
type Format string

// Renderer prepares passive content. The caller chooses the execution goroutine.
// A readable result and an error may coexist. Results must remain semantically
// stable while displayed; context does not make a renderer cancellable by itself.
type Renderer func(ctx context.Context, source string) (grid.Drawable, error)

// Binding associates one format with its sole renderer.
type Binding struct {
	Format Format
	Render Renderer
}

// Config contains the complete instance-local set of bindings.
type Config struct{ Bindings []Binding }

// Registry owns an immutable dispatch table. Concurrent calls are safe only when
// the configured renderers support them; returned content has its own ownership.
// The zero Registry is empty.
type Registry struct{ renderers map[Format]Renderer }

// ErrUnknownFormat means that this instance has no implementation for a format.
var ErrUnknownFormat = errors.New("content: unknown format")

// ErrInvalidResult means a renderer returned nil content without an error.
var ErrInvalidResult = errors.New("content: renderer returned no content")

// New validates and copies the bindings. Duplicate normalized names, invalid
// names and nil renderers fail construction; configuration order never wins.
func New(cfg Config) (*Registry, error) {
	r := &Registry{renderers: make(map[Format]Renderer, len(cfg.Bindings))}
	for _, binding := range cfg.Bindings {
		name, err := normalize(binding.Format)
		if err != nil {
			return nil, err
		}
		if binding.Render == nil {
			return nil, fmt.Errorf("content: nil renderer for %q", name)
		}
		if _, exists := r.renderers[name]; exists {
			return nil, fmt.Errorf("content: duplicate format %q", name)
		}
		r.renderers[name] = binding.Render
	}
	return r, nil
}

// Render selects exactly one renderer. Cancellation before dispatch prevents the
// call; after dispatch the renderer owns cancellation. Errors preserve their
// identity for errors.Is/As. No alternative renderer is tried after failure.
func (r *Registry) Render(ctx context.Context, format Format, source string) (grid.Drawable, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	name, err := normalize(format)
	if err != nil {
		return nil, err
	}
	var render Renderer
	if r != nil {
		render = r.renderers[name]
	}
	if render == nil {
		return nil, fmt.Errorf("%w: %q", ErrUnknownFormat, name)
	}
	result, err := render(ctx, source)
	if nilContent(result) {
		result = nil
		if err == nil {
			err = ErrInvalidResult
		}
	}
	if err != nil {
		return result, fmt.Errorf("content: render %q: %w", name, err)
	}
	return result, nil
}

func normalize(format Format) (Format, error) {
	name := strings.TrimSpace(string(format))
	if name == "" {
		return "", errors.New("content: empty format")
	}
	for _, r := range name {
		valid := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '+' || r == '-'
		if !valid {
			return "", fmt.Errorf("content: invalid format %q", format)
		}
	}
	return Format(strings.ToLower(name)), nil
}

func nilContent(value grid.Drawable) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
