package arch

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type callbackPhase string

const (
	callbackProjection callbackPhase = "projection"
	callbackSemantic   callbackPhase = "semantic"
	callbackEvent      callbackPhase = "event"
	callbackCarried    callbackPhase = "carried"
)

type callbackRule struct {
	phase  callbackPhase
	reason string
}

// renderingCallbacks classifies every function value retained by a projection
// package. Projection callbacks are the only ones Draw or Measure may invoke. A
// semantic callback belongs to an explicit model update, an event callback belongs to
// input handling, and a carried callback is data this library returns but never calls.
// Exact source-derived keys make both a new unclassified callback and a stale rule fail.
var renderingCallbacks = map[string]callbackRule{
	"components/headless:Command.Run":       {callbackCarried, "commands carry application behavior for the caller to invoke"},
	"components/headless:Completion.Accept": {callbackEvent, "acceptance follows an input action after the completion closes"},
	"components/headless:Confirm.Check":     {callbackEvent, "validation follows an answer change or form submission"},
	"components/headless:Filter.Row":        {callbackProjection, "a matched row paints only its assigned frame"},
	"components/headless:Filter.text":       {callbackSemantic, "item text is read while the filter model is rebuilt"},
	"components/headless:Form.Check":        {callbackEvent, "whole-form validation follows submission"},
	"components/headless:Form.Done":         {callbackEvent, "completion follows successful submission"},
	"components/headless:Form.GaveUp":       {callbackEvent, "abandonment follows an input action"},
	"components/headless:List.Row":          {callbackProjection, "a list row paints only its assigned frame"},
	"components/headless:MultiSelect.Check": {callbackEvent, "validation follows a selection change"},
	"components/headless:MultiSelect.Same":  {callbackProjection, "equality is a read-only projection of two values"},
	"components/headless:Select.Check":      {callbackEvent, "validation follows a selection change"},
	"components/headless:Select.Same":       {callbackProjection, "equality is a read-only projection of two values"},
	"components/headless:Settings.Change":   {callbackEvent, "a value changes only in response to an action"},
	"components/headless:Table.less":        {callbackSemantic, "ordering is rebuilt by an explicit sort operation"},
	"components/headless:Text.Check":        {callbackEvent, "validation follows an edit or form transition"},
	"components/headless:Tree.Row":          {callbackProjection, "a tree row paints only its assigned frame"},
	"components/kit:Cell.Paint":             {callbackProjection, "a cell painter receives only its frame and base style"},
	"components/kit:LinkConfig.Exists":      {callbackSemantic, "filesystem evidence is collected by SetLinks or SetText"},
	"components/kit:Settings.Label":         {callbackProjection, "a label is a read-only visible projection of an item"},
	"components/kit:Settings.Value":         {callbackProjection, "a value is a read-only visible projection of an item"},
	"components/kit:SettingsConfig.Change":  {callbackEvent, "a value changes only in response to an action"},
	"components/kit:SettingsConfig.Label":   {callbackProjection, "construction retains the visible item projection"},
	"components/kit:SettingsConfig.Value":   {callbackProjection, "construction retains the visible item projection"},
	"components/kit:Slider.Format":          {callbackProjection, "formatting is a read-only visible projection of a value"},
	"components/kit:SliderConfig.Format":    {callbackProjection, "construction retains the visible value projection"},
	"components/kit:Table.Cell":             {callbackProjection, "cell construction supplies measurement and painting together"},
	"components/kit:Table.RowStyle":         {callbackProjection, "row appearance is a read-only projection of its index"},
	"components/kit:Table.Sorted":           {callbackProjection, "sort geometry reads caller-owned ordering without changing it"},
	"components/kit:Tree.Text":              {callbackProjection, "node text is a read-only visible projection of an item"},
	"components/kit:TreeConfig.Text":        {callbackProjection, "construction retains the visible node projection"},
	"markdown:Renderer":                     {callbackSemantic, "extensions render while source is parsed into an owned document"},
}

func TestEveryRetainedRenderingCallbackHasOnePhase(t *testing.T) {
	root := repoRoot(t)
	found := make(map[string]bool)
	for _, directory := range renderingPackages {
		entries, err := os.ReadDir(filepath.Join(root, directory))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(root, directory, entry.Name())
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			for _, declaration := range file.Decls {
				typeDeclaration, ok := declaration.(*ast.GenDecl)
				if !ok || typeDeclaration.Tok != token.TYPE {
					continue
				}
				for _, specification := range typeDeclaration.Specs {
					specification := specification.(*ast.TypeSpec)
					switch declared := specification.Type.(type) {
					case *ast.FuncType:
						found[directory+":"+specification.Name.Name] = true
					case *ast.StructType:
						for _, field := range declared.Fields.List {
							if _, ok := field.Type.(*ast.FuncType); !ok {
								continue
							}
							for _, name := range field.Names {
								found[directory+":"+specification.Name.Name+"."+name.Name] = true
							}
						}
					}
				}
			}
		}
	}
	for _, problem := range callbackPhaseProblems(found, renderingCallbacks) {
		t.Error(problem)
	}
}

func callbackPhaseProblems(found map[string]bool, rules map[string]callbackRule) []string {
	var problems []string
	for callback := range found {
		rule, ok := rules[callback]
		if !ok {
			problems = append(problems,
				fmt.Sprintf("retained rendering callback %s has no projection, semantic, event, or carried phase", callback))
			continue
		}
		if rule.reason == "" {
			problems = append(problems, fmt.Sprintf("rendering callback %s has no phase reason", callback))
		}
		switch rule.phase {
		case callbackProjection, callbackSemantic, callbackEvent, callbackCarried:
		default:
			problems = append(problems,
				fmt.Sprintf("rendering callback %s has unknown phase %q", callback, rule.phase))
		}
	}
	for callback := range rules {
		if !found[callback] {
			problems = append(problems, fmt.Sprintf("rendering callback phase %s is stale", callback))
		}
	}
	slices.Sort(problems)
	return problems
}

func TestRenderingCallbackPhaseRegistryRejectsIncompleteClaims(t *testing.T) {
	found := map[string]bool{
		"sample:Blank": true, "sample:Good": true,
		"sample:Missing": true, "sample:Unknown": true,
	}
	rules := map[string]callbackRule{
		"sample:Blank":   {phase: callbackProjection},
		"sample:Good":    {phase: callbackProjection, reason: "read-only projection"},
		"sample:Stale":   {phase: callbackEvent, reason: "no longer declared"},
		"sample:Unknown": {phase: "surprise", reason: "not a real phase"},
	}
	want := []string{
		"rendering callback phase sample:Stale is stale",
		"rendering callback sample:Blank has no phase reason",
		`rendering callback sample:Unknown has unknown phase "surprise"`,
		"retained rendering callback sample:Missing has no projection, semantic, event, or carried phase",
	}
	if got := callbackPhaseProblems(found, rules); !slices.Equal(got, want) {
		t.Fatalf("phase problems = %q, want %q", got, want)
	}
}
