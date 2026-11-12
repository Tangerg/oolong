package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"sort"
	"strings"
	"testing"
)

// internalAssertions are the panics no caller argument can reach, with the guard that
// makes each one unreachable. They are invariant assertions rather than preconditions,
// so they are not part of any exported contract and belong here instead of in a doc
// comment that would tell callers about a state they cannot produce.
//
// The table is keyed by the function the panic is in, which is where a reviewer looks.
// Every entry must match a real site and every unexported site must have an entry, so
// a panic that moves into caller-reachable code fails this test until it is either
// documented on the exported API or explained here.
var internalAssertions = map[string]string{
	"ui/core/keymap:Map.checkSubscribers":                     "SubscriptionConfig.Check documents consumer refusal before binding or policy mutation; the original refusal is propagated unchanged",
	"ui/core/keymap:Map.requireIdle":                          "Map and SubscriptionConfig prohibit reentrant mutation or subscription during consumer checks and publication; the phase guard runs before any binding, policy or subscriber change",
	"extensions/markdown:blockNestingContext.SetOpenedBlocks": "Goldmark has no error return here; parse catches this private budget termination and public renderers return ErrBlockNestingLimit",
	"extensions/markdown:parse":                               "only unexpected panic values are rethrown; the private block nesting termination becomes an explicit parse error",
	"ui/core/anim:requireFinite":                              "Transition.To, Timeline.SetFrames and Value, and Spring.To, Set and Tick document non-finite input or unrepresentable motion refusal",
	"ui/core/ansi:Scanner.requireIdle":                        "Scanner documents that its active visitor cannot reenter Feed or Reset",
	"ui/core/publication:Publisher.checkPrefix":               "Commit and FailAdmission require the current pending prefix issued by this publisher",
	"ui/kit:mustCellGlyph":                                    "only ASCII and Unicode built-in glyph literals reach this assertion; caller-provided glyphs use NewCellGlyph's error result",

	// Dimensions are validated in one place for both of a renderer's buffers. The
	// exported callers, NewSurface and Surface.Resize, document the panic themselves.
	"extensions/markdown:Snapshot.prepareLayout": "Doc and Snapshot measurement, drawing, row projection and Snapshot.PrepareWidth document refusal when combined block heights cannot fit an int",
	"ui/core/grid:surfaceSize":                   "NewSurface and Surface.Resize document the overflow panic",
	"ui/core/grid:normalizedOpacity":             "RGB.Blend, Color.Blend, View.Blend and View.Fade document NaN refusal before changing colors or cells",
	"ui/core/grid:frameBuffer.setDepth":          "Screen.SetDepth and Inline.SetDepth document invalid Depth refusal; Depth.Validate runs before either canvas changes its encoding baseline",
	"ui/core/grid:publication.requireIdle":       "Screen and Inline document that publication callbacks cannot reenter mutation or publication on the same canvas",
	"ui/core/grid:viewScope.requireWritable":     "View, Screen.Frame and Inline.Frame document expired drawing and publication boundaries",
	"ui/core/grid:CursorStyle.normalized":        "View.PlaceCursor documents invalid CursorShape refusal and normalizes before changing the frame cursor",
	"ui/core/grid:CursorStyle.selector":          "CursorStyle.Sequence documents invalid Shape refusal; selector is its only encoding",

	// Collection checks run inside the exported operations that accept the collection,
	// and those operations document the rule.
	"ui/base:Transcript.assertMutable":             "Transcript documents its closed lifetime and uses the scene-owned read-only phase during content capture",
	"ui/kit:Paragraph.requireIdle":                 "Paragraph, SetText and SetLinks document that link detection cannot reenter either setter",
	"ui/base:requireMutable":                       "all live controllers require owner-phase mutation; this common guard checks scene phase before any controller state changes",
	"ui/base:requireProjection":                    "Stage and drawing require the controller's mounted scene and refuse geometry writes during its read-only phase",
	"ui/base:requireStickyOrder":                   "Sticky.SetBlocks and Add document strict nonzero identity ordering and validate the entire batch before mutation",
	"ui/base:Selection.setRange":                   "Selection.Begin and Set document invalid Affinity refusal; both endpoints are checked before anchors or gesture state change",
	"ui/kit:Box.paint":                             "Box.Draw and Panel.Element document invalid title or footer alignment refusal before cells change",
	"ui/base:subscriptions.add":                    "public Subscribe operations reject nil callbacks before entering the subscription owner",
	"ui/kit:newPanel":                              "NewPanel documents invalid alignment, Box.Theme and competing palette refusal; Dialog supplies its own scope and a title source with an empty Box.Title",
	"ui/kit:replaceGlyphs":                         "every SetGlyphs documents scene.ErrBusy outside owner mutation, the only refusal this shared guard raises",
	"ui/base:requireNonNegative":                   "each constructor and setter that takes a count documents negative refusal; this guard words the one rule they share",
	"ui/core/content:StreamDocument.assertMutable": "StreamDocument documents construction and prohibits reentrant source mutation from its synchronous change notification",
	"ui/core/publication:Publisher.assertIdle":     "Publisher documents that capture callbacks cannot reenter publication mutation",

	// Frame discipline. The transaction is reachable only through a Frame the running
	// Root handed out, and both of those types document what violating it costs.
	"ui/core/scene:Staged.valid":               "Staged documents the issuing paint callback and its nested read-only boundary",
	"ui/core/scene:Root.project":               "Element.ReadOnly and Measure document refusal before callbacks while a frame candidate awaits settlement",
	"ui/core/scene:Prepared.requireSettlement": "Prepared.Commit and Prepared.Abort document the live candidate identity and pure-phase preconditions",
	"ui/core/scene:Snapshot.commit":            "Snapshot.Stage enlists a snapshot at most once in its owning transaction; only that transaction can commit it",
	"ui/core/scene:Group.rectangles":           "GroupConfig.Layout documents exactly one rectangle per item; preparation validates that contract before use",

	"ui/core/program:program.frameReady":    "Runtime.SetFramePreparation documents that reentering rendering from its callback panics; the owner flag prevents a second readiness invocation before the first returns",
	"ui/core/program:program.answerRepaint": "wait disables input while a repaint is claimed, and completeRepaint clears that claim before invoking its reply, so the owner cannot accept two pending requests",
	"ui/core/program:inputPipe.read":        "only rethrows an existing reader panic on the Run owner after closing the input stream; FrameWriter.WrittenAt documents this cleanup-preserving transfer",

	"ui/core/grid/testdata/inline-reflow:frameCursor.place": "the fixture consumes only literal cursor shapes and a seeded generator choosing block, underline or bar; it never parses caller-supplied shapes",

	// Identity counters. These are uint64 and are consumed once per operation, so
	// exhausting one is not a state a caller can arrange within a session; the
	// exported operations that can plausibly approach one say so anyway.
	"ui/base:EditSession.requireRevision": "revisions are uint64 and consumed one per content change",
	"ui/base:EditSession.setPolicy":       "Editor.SetMask documents refusal before changing an invalid mask policy",
	"ui/base:EditSession.insertElement":   "Editor.InsertElement documents identity exhaustion before mutation",
	"ui/base:Transcript.requireRevisions": "Transcript documents refusal to wrap its content revision and revive an old selection",
	"ui/base:Transcript.publish":          "Transcript documents refusal of unrepresentable retired row coordinates",
	"ui/core/scene:next":                  "scene identity and revision counters never wrap because wrapping could revive stale handles or snapshots",
	"ui/core/term:Writer.enqueue":         "Queue and QueueControl document sequence exhaustion; accepted frame identities may never wrap",
	"ui/base:nextRevision":                "confirmation, choice and collection owners document revision exhaustion and reserve the next revision before mutation",

	// Cycle detection during the ownership copy, which Tree.SetNodes documents.
	"ui/base:treeCopy.copyNext": "Tree.SetNodes documents the cyclic-graph panic",
}

// TestEveryPanickingPreconditionIsDocumented makes the panic principle executable.
//
// A panic in this repository says the caller wrote their program wrong. The caller can
// only act on that if the rule is visible before they run it, so a precondition on an
// exported function has to be in that function's doc comment — where `go doc` shows it
// — and not only in the message the crash prints. Panics a caller cannot reach are
// declared in internalAssertions with the guard that makes them unreachable.
//
// The test fails in both directions: an undocumented exported precondition, and a
// table entry whose site has gone. The second half is what stops the table from
// becoming a list of exemptions nobody rechecks.
func TestEveryPanickingPreconditionIsDocumented(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	seen := map[string]bool{}

	walk(t, root, func(dir, path string) {
		if !publicPackageDir(dir) {
			return
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", relative(root, path), err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || !panics(function) {
				continue
			}
			name := functionName(function)
			key := dir + ":" + name
			if exportedAPI(function) {
				if !strings.Contains(strings.ToLower(documentation(function)), "panic") {
					t.Errorf("%s: %s panics but its doc comment does not say so; a caller "+
						"who cannot see the rule in go doc learns it at runtime, which is "+
						"what the panic was meant to prevent", relative(root, path), name)
				}
				continue
			}
			seen[key] = true
			if _, declared := internalAssertions[key]; !declared {
				t.Errorf("%s: %s panics from unexported code with no entry in "+
					"internalAssertions; either document the rule on the exported API "+
					"that has it, or record the guard that makes it unreachable",
					relative(root, path), name)
			}
		}
	})

	stale := make([]string, 0, len(internalAssertions))
	for key := range internalAssertions {
		if !seen[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	for _, key := range stale {
		t.Errorf("internalAssertions names %s, which no longer panics: remove the entry "+
			"rather than leaving an exemption for code that is gone", key)
	}
}

// publicPackageDir reports whether a directory holds a package this repository
// publishes. Internal packages, the workspace tooling and the examples are governed by
// other rules and are not part of anyone's contract.
func publicPackageDir(dir string) bool {
	if dir == "." || strings.HasPrefix(dir, "examples") {
		return false
	}
	return !slices.Contains(strings.Split(dir, "/"), "internal")
}

// panics reports whether the function body calls panic directly. A helper that panics
// is judged where it is written, not at every call site, which is the same place a
// reviewer reads the guard.
func panics(function *ast.FuncDecl) bool {
	found := false
	ast.Inspect(function, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		identifier, ok := call.Fun.(*ast.Ident)
		if ok && identifier.Name == "panic" {
			found = true
			return false
		}
		return true
	})
	return found
}

// exportedAPI reports whether a caller outside the package can name this function. An
// exported method on an unexported type is not reachable and is judged as internal.
func exportedAPI(function *ast.FuncDecl) bool {
	if !function.Name.IsExported() {
		return false
	}
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return true
	}
	return ast.IsExported(receiverName(function))
}

func functionName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return function.Name.Name
	}
	return receiverName(function) + "." + function.Name.Name
}

func receiverName(function *ast.FuncDecl) string {
	expression := function.Recv.List[0].Type
	if star, ok := expression.(*ast.StarExpr); ok {
		expression = star.X
	}
	if index, ok := expression.(*ast.IndexExpr); ok {
		expression = index.X
	}
	if list, ok := expression.(*ast.IndexListExpr); ok {
		expression = list.X
	}
	if identifier, ok := expression.(*ast.Ident); ok {
		return identifier.Name
	}
	return ""
}

func documentation(function *ast.FuncDecl) string {
	if function.Doc == nil {
		return ""
	}
	return function.Doc.Text()
}
