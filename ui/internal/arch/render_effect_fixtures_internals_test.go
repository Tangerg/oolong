package arch

import (
	"go/ast"
	"slices"
	"testing"
)

func TestRenderEffectRuleFollowsHelpersAndDistinguishesPureCalls(t *testing.T) {
	tests := []struct {
		name          string
		source        string
		callbackPhase callbackPhase
		eraseMethod   string
		want          []string
	}{
		{
			name: "direct goroutine",
			source: `package sample
				type Widget struct{}
				func (Widget) Draw() { go work() }
				func work() {}`,
			want: []string{"a goroutine"},
		},
		{
			name: "goroutine behind helper",
			source: `package sample
				type Widget struct{}
				func (w Widget) Draw() { w.prepare() }
				func (Widget) prepare() { go work() }
				func work() {}`,
			want: []string{"a goroutine"},
		},
		{
			name: "unresolved receiver helper",
			source: `package sample
				type Widget struct{}
				func (w Widget) Draw() { w.prepare() }
				func (Widget) prepare() {}`,
			eraseMethod: "prepare",
			want:        []string{"an unresolved package-local call prepare"},
		},
		{
			name: "I/O behind package helper",
			source: `package sample
				import "os"
				type Widget struct{}
				func (Widget) HeightForWidth(int) int { load(); return 1 }
				func load() { _, _ = os.ReadFile("state") }`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "I/O behind helper value",
			source: `package sample
				import "os"
				type preparer struct{}
				func (preparer) prepare() { _, _ = os.ReadFile("state") }
				type Widget struct { helper preparer }
				func (w Widget) Draw() { w.helper.prepare() }`,
			want: []string{"I/O through os.ReadFile"},
		},
		{
			name: "clock",
			source: `package sample
				import "time"
				type Widget struct{}
				func (Widget) Draw() { _ = time.Now() }`,
			want: []string{"clock or scheduling work through time.Now"},
		},
		{
			name: "application interface keeps its own contract",
			source: `package sample
				type Printer interface { Print(any) }
				type Widget struct { out Printer }
				func (w Widget) Draw() { w.out.Print(1) }`,
		},
		{
			name: "event callback",
			source: `package sample
				type Widget struct { Check func() }
				func (w Widget) Draw() { w.Check() }`,
			callbackPhase: callbackEvent,
			want:          []string{"the event callback Check"},
		},
		{
			name: "same named method is not callback",
			source: `package sample
				type Widget struct{}
				func (Widget) Draw() { Widget{}.Check() }
				func (Widget) Check() {}`,
		},
		{
			name: "frame-local work",
			source: `package sample
				import "strings"
				type Widget struct{}
				func (Widget) Draw() { var b strings.Builder; b.WriteString("frame") }`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := renderModelFromSource(t, test.source)
			if test.eraseMethod != "" {
				for object := range model.functionByObject {
					if object.Name() == test.eraseMethod {
						delete(model.functionByObject, object)
					}
				}
			}
			if test.callbackPhase != "" {
				for _, function := range model.functions {
					ast.Inspect(function.body, func(node ast.Node) bool {
						selector, ok := node.(*ast.SelectorExpr)
						if !ok || selector.Sel.Name != "Check" {
							return true
						}
						selection := model.types.Selections[selector]
						if selection != nil {
							model.callbackByObject[selection.Obj()] = callbackRule{phase: test.callbackPhase}
						}
						return true
					})
				}
			}
			var got []string
			for _, function := range model.functions {
				if function.name != "Draw" && function.name != "HeightForWidth" {
					continue
				}
				for _, finding := range model.effectsFrom(function) {
					got = append(got, finding.effect)
				}
			}
			slices.Sort(got)
			slices.Sort(test.want)
			if !slices.Equal(got, test.want) {
				t.Errorf("effects = %v, want %v", got, test.want)
			}
		})
	}
}
