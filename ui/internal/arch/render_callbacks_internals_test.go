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
)

type callbackRule struct {
	phase  callbackPhase
	reason string
}

// renderingCallbacks classifies every function value retained by a projection
// package. Projection callbacks are the only ones Draw or HeightForWidth may invoke. A
// semantic callback belongs to an explicit model update, and an event callback belongs
// to input handling. Exact source-derived keys make both a new unclassified callback
// and a stale rule fail.
var renderingCallbacks = map[string]callbackRule{
	"ui/kit:InputConfig.Validate":         {callbackEvent, "the input skin configures validation on its sole base.Text controller"},
	"ui/base:Confirm.unsubscribe":         {callbackEvent, "closing the control revokes its domain owner subscription"},
	"ui/kit:ButtonConfig.Activate":        {callbackEvent, "the skin supplies activation policy to its single base action"},
	"ui/kit:ButtonConfig.Availability":    {callbackProjection, "the skin shares the base action's pure eligibility policy"},
	"ui/base:Completion.Accept":           {callbackEvent, "acceptance follows an input action after the completion closes"},
	"ui/base:Confirm.Check":               {callbackEvent, "validation follows an answer change or form submission"},
	"ui/base:Filter.Row":                  {callbackProjection, "a matched row paints only its assigned frame"},
	"ui/base:Filter.text":                 {callbackSemantic, "item text is read while the filter model is rebuilt"},
	"ui/base:ButtonConfig.Activate":       {callbackEvent, "keyboard, pointer and semantic invocation enter the same typed action"},
	"ui/base:ButtonConfig.Availability":   {callbackProjection, "appearance and invocation read the same pure eligibility policy"},
	"ui/base:Dialog.backdrop":             {callbackProjection, "the backdrop paints only its assigned frame"},
	"ui/base:Dialog.place":                {callbackProjection, "placement derives geometry without changing the dialog"},
	"ui/base:DialogConfig.Backdrop":       {callbackProjection, "construction retains the backdrop projection"},
	"ui/base:DialogConfig.Place":          {callbackProjection, "construction retains pure dialog placement"},
	"ui/base:Form.cancel":                 {callbackEvent, "cancellation follows the explicit submission identity"},
	"ui/base:Form.check":                  {callbackEvent, "whole-form validation belongs to submission"},
	"ui/base:Form.submit":                 {callbackEvent, "submission invokes application policy once per admitted intent"},
	"ui/base:Form.paintField":             {callbackProjection, "field chrome paints only its assigned frame"},
	"ui/base:FormConfig.Cancel":           {callbackEvent, "construction retains explicit submission cancellation"},
	"ui/base:FormConfig.Check":            {callbackEvent, "construction retains whole-form submission validation"},
	"ui/base:FormConfig.Submit":           {callbackEvent, "construction retains the typed submission action"},
	"ui/base:FormConfig.PaintField":       {callbackProjection, "construction retains field chrome projection"},
	"ui/base:Tabs.paintTab":               {callbackProjection, "tab chrome projects the current tab state"},
	"ui/base:TabsConfig.PaintTab":         {callbackProjection, "construction retains tab chrome projection"},
	"ui/base:Transcript.stopPublication":  {callbackEvent, "closing the transcript revokes its publication subscription"},
	"ui/base:TranscriptItem.cancel":       {callbackEvent, "removing an item revokes its document subscription"},
	"ui/base:VirtualConfig.Build":         {callbackSemantic, "explicit Resolve materializes a bounded window before preparation"},
	"ui/base:subscription.callback":       {callbackSemantic, "owner-stage changes notify dependent controller projections"},
	"ui/base:History.clone":               {callbackProjection, "pure snapshot copying also serves read-only history queries"},
	"ui/base:History.equal":               {callbackSemantic, "duplicate detection belongs to the explicit Add transition"},
	"ui/base:HistoryConfig.Clone":         {callbackProjection, "construction retains the pure value copier used by history queries"},
	"ui/base:HistoryConfig.Equal":         {callbackSemantic, "construction retains duplicate detection for explicit submissions"},
	"ui/base:Select.Row":                  {callbackProjection, "choice rows project current cursor and taken states"},
	"ui/base:MultiSelect.Row":             {callbackProjection, "choice rows project current cursor and taken states"},
	"ui/base:List.Row":                    {callbackProjection, "a list row paints only its assigned frame"},
	"ui/base:Table.Row":                   {callbackProjection, "a table row paints only its assigned frame"},
	"ui/base:MultiSelect.Check":           {callbackEvent, "validation follows a selection change"},
	"ui/base:MultiSelect.Same":            {callbackProjection, "equality is a read-only projection of two values"},
	"ui/base:Select.Check":                {callbackEvent, "validation follows a selection change"},
	"ui/base:Select.Same":                 {callbackProjection, "equality is a read-only projection of two values"},
	"ui/base:Settings.Change":             {callbackEvent, "a value changes only in response to an action"},
	"ui/base:Table.less":                  {callbackSemantic, "ordering is rebuilt by an explicit sort operation"},
	"ui/base:Text.Check":                  {callbackEvent, "validation follows an edit or form transition"},
	"ui/base:fieldCheck.current":          {callbackProjection, "a validation receipt only compares its checked answer with the current owner"},
	"ui/base:TextConfig.Check":            {callbackEvent, "construction retains checking for edits and form transitions"},
	"ui/base:SelectConfig.Check":          {callbackEvent, "construction retains checking for choice transitions"},
	"ui/base:SelectConfig.Same":           {callbackProjection, "construction retains pure choice identity"},
	"ui/base:SelectConfig.Row":            {callbackProjection, "construction retains the choice row projection"},
	"ui/base:MultiSelectConfig.Check":     {callbackEvent, "construction retains checking for selection transitions"},
	"ui/base:MultiSelectConfig.Same":      {callbackProjection, "construction retains pure choice identity"},
	"ui/base:MultiSelectConfig.Row":       {callbackProjection, "construction retains the choice row projection"},
	"ui/base:ConfirmConfig.Check":         {callbackEvent, "construction retains checking for confirmation and form transitions"},
	"ui/base:Tree.Row":                    {callbackProjection, "a tree row paints only its assigned frame"},
	"ui/kit:Cell.Paint":                   {callbackProjection, "a cell painter receives only its frame and base style"},
	"ui/kit:FormConfig.Cancel":            {callbackEvent, "the skin passes cancellation policy to its single base controller"},
	"ui/kit:FormConfig.Check":             {callbackEvent, "the skin passes validation policy to its single base controller"},
	"ui/kit:FormConfig.Submit":            {callbackEvent, "the skin passes submission policy to its single base controller"},
	"ui/kit:LinkConfig.Exists":            {callbackSemantic, "filesystem evidence is collected by SetLinks or SetText"},
	"ui/kit:Settings.Label":               {callbackProjection, "a label is a read-only visible projection of an item"},
	"ui/kit:Settings.Value":               {callbackProjection, "a value is a read-only visible projection of an item"},
	"ui/kit:SettingsConfig.Change":        {callbackEvent, "a value changes only in response to an action"},
	"ui/kit:SettingsConfig.Label":         {callbackProjection, "construction retains the visible item projection"},
	"ui/kit:SettingsConfig.Value":         {callbackProjection, "construction retains the visible item projection"},
	"ui/kit:Slider.Format":                {callbackProjection, "formatting is a read-only visible projection of a value"},
	"ui/kit:SliderConfig.Format":          {callbackProjection, "construction retains the visible value projection"},
	"ui/kit:Table.Cell":                   {callbackProjection, "cell construction supplies measurement and painting together"},
	"ui/kit:Table.RowStyle":               {callbackProjection, "row appearance is a read-only projection of its index"},
	"ui/kit:Table.Sorted":                 {callbackProjection, "sort geometry reads caller-owned ordering without changing it"},
	"ui/kit:Tree.Text":                    {callbackProjection, "node text is a read-only visible projection of an item"},
	"ui/kit:TreeConfig.Text":              {callbackProjection, "construction retains the visible node projection"},
	"ui/core/content:Renderer":            {callbackSemantic, "configured renderers prepare passive content during explicit dispatch"},
	"ui/core/content:documentListener.fn": {callbackSemantic, "owner-stage source or projection transitions notify document consumers"},
	"extensions/markdown:Renderer":        {callbackSemantic, "extensions render while source is parsed into an owned document"},
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
				fmt.Sprintf("retained rendering callback %s has no projection, semantic, or event phase", callback))
			continue
		}
		if rule.reason == "" {
			problems = append(problems, fmt.Sprintf("rendering callback %s has no phase reason", callback))
		}
		switch rule.phase {
		case callbackProjection, callbackSemantic, callbackEvent:
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
		"retained rendering callback sample:Missing has no projection, semantic, or event phase",
	}
	if got := callbackPhaseProblems(found, rules); !slices.Equal(got, want) {
		t.Fatalf("phase problems = %q, want %q", got, want)
	}
}
