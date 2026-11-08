package arch

import (
	"go/types"
	"slices"
	"testing"
)

func TestRenderEffectRuleTracksFunctionValues(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{
			name: "generic helper",
			source: `package sample
				import "os"
				type Widget struct{}
				func (Widget) Draw() { load[int, string]() }
				func load[A, B any]() { _, _ = os.ReadFile("state") }`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "generic method alias",
			source: `package sample
				import "os"
				type Widget struct{}
				func (w Widget) Draw() { load := w.load[int]; load() }
				func (Widget) load[T any]() { _, _ = os.ReadFile("state") }`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "instantiated receiver method",
			source: `package sample
				import "os"
				type Worker[T any] struct{}
				func (Worker[T]) load() { _, _ = os.ReadFile("state") }
				type Widget struct{}
				func (Widget) Draw() { Worker[int]{}.load() }`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "method expression alias",
			source: `package sample
				import "os"
				type Widget struct{}
				func (w Widget) Draw() { run := Widget.invoke; run(w, load) }
				func (Widget) invoke(f func()) { f() }
				func load() { _, _ = os.ReadFile("state") }`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "local alias chain",
			source: `package sample
				import "os"
				type Widget struct{}
				func (Widget) Draw() { first := load; second := first; third := second; third() }
				func load() { _, _ = os.ReadFile("state") }`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "reassigned alias",
			source: `package sample
				import "os"
				type Widget struct{}
				func (Widget) Draw() { run := pure; run = load; alias := run; alias() }
				func pure() {}
				func load() { _, _ = os.ReadFile("state") }`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "parallel assignment alias",
			source: `package sample
				import "os"
				type Widget struct{}
				func (Widget) Draw() { first, second := pure, load; first, second = second, first; first() }
				func pure() {}
				func load() { _, _ = os.ReadFile("state") }`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "imported function alias",
			source: `package sample
				import "os"
				type Widget struct{}
				func (Widget) Draw() { read := os.ReadFile; alias := read; _, _ = alias("state") }`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "imported method alias",
			source: `package sample
				import "os"
				type Widget struct { file *os.File }
				func (w Widget) Draw() { read := w.file.Read; _, _ = read(nil) }`,
			want: []string{"I/O through os.Read"},
		},
		{
			name: "immediate closure",
			source: `package sample
				import "os"
				type Widget struct{}
				func (Widget) Draw() { func() { _, _ = os.ReadFile("state") }() }`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "returned closure captures callback",
			source: `package sample
				import "os"
				type Widget struct{}
				func (Widget) Draw() { run := wrap(load); run() }
				func wrap(f func()) func() { return func() { f() } }
				func load() { _, _ = os.ReadFile("state") }`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "tuple returned aliases",
			source: `package sample
				import "os"
				type Widget struct{}
				func (Widget) Draw() { _, run := pair(); run() }
				func pair() (func(), func()) { return pure, load }
				func pure() {}
				func load() { _, _ = os.ReadFile("state") }`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "package initializer captures callback",
			source: `package sample
				import "os"
				var run = wrap(load)
				type Widget struct{}
				func (Widget) Draw() { run() }
				func wrap(f func()) func() { _ = os.Getenv("setup"); return func() { f() } }
				func load() { _, _ = os.ReadFile("state") }`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "range invokes local closure",
			source: `package sample
				import "os"
				type Widget struct{}
				func (Widget) Draw() {
					for range func(yield func() bool) { _, _ = os.ReadFile("state"); yield() } {}
				}`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "closure passed to local invoker",
			source: `package sample
				import "os"
				type Widget struct{}
				func (Widget) Draw() { invoke(func() { _, _ = os.ReadFile("state") }) }
				func invoke(f func()) { f() }`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "closure passed to imported invoker",
			source: `package sample
				import ("os"; "slices")
				type Widget struct{}
				func (Widget) Draw() {
					slices.SortFunc([]int{}, func(a, b int) int { _, _ = os.ReadFile("state"); return a-b })
				}`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "same named pure methods",
			source: `package sample
				type Widget struct{}
				func (w Widget) Draw() { print := w.Print; print(); w.Commit(); w.Finish(); w.Dispatch() }
				func (Widget) Print() {}
				func (Widget) Commit() {}
				func (Widget) Finish() {}
				func (Widget) Dispatch() {}`,
		},
		{
			name: "uninvoked closure",
			source: `package sample
				import "os"
				type Widget struct{}
				func (Widget) Draw() { unused := func() { _, _ = os.ReadFile("state") }; _ = unused }`,
		},
		{
			name: "unrelated event call does not taint helper",
			source: `package sample
				import "os"
				type Widget struct{}
				func (Widget) Draw() { invoke(pure) }
				func (Widget) Handle() { invoke(load) }
				func invoke(f func()) { f() }
				func pure() {}
				func load() { _, _ = os.ReadFile("state") }`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := renderModelFromSource(t, test.source)
			got := renderEffectNames(model)
			slices.Sort(test.want)
			if !slices.Equal(got, test.want) {
				t.Errorf("effects = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRenderEffectRuleRetainsCallbackPhasesThroughAliases(t *testing.T) {
	for _, test := range []struct {
		name        string
		source      string
		declaration string
		phase       callbackPhase
		want        []string
	}{
		{
			name: "instantiated event field",
			source: `package sample
				type Widget[T any] struct { Check func(T) }
				func (w Widget[T]) Draw() { var value T; check := w.Check; alias := check; alias(value) }`,
			declaration: "Check", phase: callbackEvent,
			want: []string{"the event callback Check"},
		},
		{
			name: "named semantic function",
			source: `package sample
				type Renderer func()
				type Widget struct { render Renderer }
				func (w Widget) Draw() { render := w.render; render() }`,
			declaration: "Renderer", phase: callbackSemantic,
			want: []string{"the semantic callback Renderer"},
		},
		{
			name: "projection callback",
			source: `package sample
				type Widget struct { Paint func() }
				func (w Widget) Draw() { paint := w.Paint; paint() }`,
			declaration: "Paint", phase: callbackProjection,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := renderModelFromSource(t, test.source)
			for identifier, object := range model.types.Defs {
				if identifier.Name != test.declaration {
					continue
				}
				if variable, ok := object.(*types.Var); ok {
					object = variable.Origin()
				}
				model.callbackByObject[object] = callbackRule{phase: test.phase}
			}
			if got := renderEffectNames(model); !slices.Equal(got, test.want) {
				t.Fatalf("effects = %v, want %v", got, test.want)
			}
		})
	}
}

func renderEffectNames(model renderPackageModel) []string {
	var effects []string
	for _, function := range model.functions {
		if function.name != "Draw" && function.name != "HeightForWidth" {
			continue
		}
		for _, finding := range model.effectsFrom(function) {
			effects = append(effects, finding.effect)
		}
	}
	slices.Sort(effects)
	return effects
}
