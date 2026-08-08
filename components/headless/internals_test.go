package headless

// Tests of what this package keeps to itself.
//
// The undo history's bound is a promise about memory in a long-lived process, not
// about behaviour a caller can see: an editor that kept every keystroke for a
// session that runs all day is a leak with a friendly name. Nothing outside can
// ask how many steps are held, so this asks from inside.

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

func TestEditorUndoHistoryIsBounded(t *testing.T) {
	// An unbounded history in a long-lived process is a leak with a friendly name.
	e := NewEditor()
	for i := range maxUndo + 50 {
		e.Insert("x")
		e.MoveLeft()
		_ = i
	}
	if len(e.history.undo) > maxUndo {
		t.Fatalf("history holds %d steps, want at most %d", len(e.history.undo), maxUndo)
	}
}

func TestEditorHistoryClearsPoppedSnapshots(t *testing.T) {
	history := editorHistory{}
	history.record(editorState{lines: []string{"before"}, marks: []text.Mark{{ID: 1}}})
	if _, ok := history.back(editorState{lines: []string{"after"}}); !ok {
		t.Fatal("undo stack reported empty")
	}
	for i, state := range history.undo[:cap(history.undo)] {
		if state.lines != nil || state.marks != nil {
			t.Fatalf("popped undo slot %d retained %+v", i, state)
		}
	}
	if _, ok := history.forward(editorState{lines: []string{"current"}}); !ok {
		t.Fatal("redo stack reported empty")
	}
	for i, state := range history.redo[:cap(history.redo)] {
		if state.lines != nil || state.marks != nil {
			t.Fatalf("popped redo slot %d retained %+v", i, state)
		}
	}
}

type retainedWidget struct{ payload []byte }

func (*retainedWidget) Draw(Frame)      {}
func (*retainedWidget) Measure(int) int { return 1 }

type retainedField struct{ retainedWidget }

func (*retainedField) Handle(input.Event) bool { return false }
func (*retainedField) Focus(bool)              {}
func (*retainedField) Prompt() string          { return "" }
func (*retainedField) Validate() error         { return nil }
func (*retainedField) Error() error            { return nil }

type retainedModal struct{ retainedWidget }

func (*retainedModal) Handle(input.Event) bool            { return false }
func (*retainedModal) Place(image.Point) layout.Placement { return layout.Placement{} }

func TestComponentCachesReleaseRemovedChildren(t *testing.T) {
	children := []*retainedWidget{{payload: []byte("one")}, {payload: []byte("two")}, {payload: []byte("three")}}
	container := Rows(
		Item{Size: layout.Measured(1, 0), Of: children[0]},
		Item{Size: layout.Measured(1, 0), Of: children[1]},
		Item{Size: layout.Measured(1, 0), Of: children[2]},
	)
	container.Measure(10)
	container.Set(Item{Size: layout.Measured(1, 0), Of: children[0]})
	container.Measure(10)
	for i, item := range container.items[:cap(container.items)] {
		if i >= len(container.items) && item.Of != nil {
			t.Fatalf("container item %d retained a removed child", i)
		}
	}
	for i, slot := range container.slots[:cap(container.slots)] {
		if i >= len(container.slots) && slot.Of != nil {
			t.Fatalf("container slot %d retained a removed child", i)
		}
	}

	tabs := NewTabs(
		Tab{Title: "one", Of: children[0]},
		Tab{Title: "two", Of: children[1]},
		Tab{Title: "three", Of: children[2]},
	)
	tabs.Set(Tab{Title: "one", Of: children[0]})
	for i, tab := range tabs.items[:cap(tabs.items)] {
		if i >= len(tabs.items) && tab.Of != nil {
			t.Fatalf("tab item %d retained a removed child", i)
		}
	}

	fields := []*retainedField{{}, {}, {}}
	form := NewForm(fields[0], fields[1], fields[2])
	form.Measure(10)
	form.Set(fields[0])
	form.Measure(10)
	for i, item := range form.body.items[:cap(form.body.items)] {
		if i >= len(form.body.items) && item.Of != nil {
			t.Fatalf("form item %d retained a removed field", i)
		}
	}

	values := []int{1, 2, 3}
	tree := NewTree(Node[*int]{Item: &values[0]}, Node[*int]{Item: &values[1]}, Node[*int]{Item: &values[2]})
	tree.Rows()
	tree.SetNodes(tree.Nodes()[:1])
	tree.Rows()
	for i, row := range tree.list.items[:cap(tree.list.items)] {
		if i >= len(tree.list.items) && (row.Item != nil || row.id != 0) {
			t.Fatalf("tree row %d retained a removed node", i)
		}
	}
}

func TestTreeReleasesOpenIdentitiesRemovedFromItsHierarchy(t *testing.T) {
	tree := NewTree(Node[string]{
		Item: "root",
		Children: []Node[string]{{
			Item: "branch", Children: []Node[string]{{Item: "leaf"}},
		}},
	})
	tree.Open(0)
	tree.Open(1)
	if len(tree.open) != 2 {
		t.Fatalf("open identities = %d, want 2", len(tree.open))
	}

	tree.SetNodes([]Node[string]{{Item: "replacement"}})
	if len(tree.open) != 0 {
		t.Fatalf("removed hierarchy retained open identities: %v", tree.open)
	}
}

func TestOwnedCollectionsReleaseOversizedBackingStorage(t *testing.T) {
	children := make([]*retainedWidget, 1024)
	items := make([]Item, len(children))
	for i := range children {
		children[i] = &retainedWidget{payload: []byte{byte(i)}}
		items[i] = Item{Size: layout.Fixed(1), Of: children[i]}
	}
	one := items[:1]

	var container Container
	container.Set(items...)
	container.Measure(10)
	container.Set(one...)
	container.Measure(10)
	if cap(container.items) > 2*len(container.items)+16 {
		t.Fatalf("container retains capacity %d for %d child", cap(container.items), len(container.items))
	}
	if cap(container.slots) > 2*len(container.slots)+16 {
		t.Fatalf("container retains capacity %d for %d slot", cap(container.slots), len(container.slots))
	}

	tabs := NewTabs(makeTabs(children)...)
	tabs.Set(Tab{Title: "one", Of: children[0]})
	if cap(tabs.items) > 2*len(tabs.items)+16 {
		t.Fatalf("tabs retain capacity %d for %d item", cap(tabs.items), len(tabs.items))
	}

	var list List[*retainedWidget]
	list.SetItems(children)
	list.SetItems(children[:1])
	if cap(list.items) > 2*len(list.items)+16 {
		t.Fatalf("list retains capacity %d for %d item", cap(list.items), len(list.items))
	}

	var filter Filter[*retainedWidget]
	filter.SetText(func(*retainedWidget) string { return "item" })
	filter.SetItems(children)
	filter.SetItems(children[:1])
	if cap(filter.items) > 2*len(filter.items)+16 {
		t.Fatalf("filter retains capacity %d for %d item", cap(filter.items), len(filter.items))
	}
	if cap(filter.list.items) > 2*len(filter.list.items)+16 {
		t.Fatalf("filtered list retains capacity %d for %d hit", cap(filter.list.items), len(filter.list.items))
	}

	fields := make([]Field, len(children))
	for i := range fields {
		fields[i] = &retainedField{retainedWidget{payload: []byte{byte(i)}}}
	}
	form := NewForm(fields...)
	form.Measure(10)
	form.Set(fields[0])
	form.Measure(10)
	if cap(form.fields) > 2*len(form.fields)+16 {
		t.Fatalf("form retains capacity %d for %d field", cap(form.fields), len(form.fields))
	}
	if cap(form.body.items) > 2*len(form.body.items)+16 {
		t.Fatalf("form container retains capacity %d for %d field", cap(form.body.items), len(form.body.items))
	}

	nodes := make([]Node[*retainedWidget], len(children))
	for i, child := range children {
		nodes[i] = Node[*retainedWidget]{Item: child}
	}
	tree := NewTree(nodes...)
	tree.Rows()
	tree.SetNodes(nodes[:1])
	tree.Rows()
	if cap(tree.list.items) > 2*len(tree.list.items)+16 {
		t.Fatalf("tree retains capacity %d for %d row", cap(tree.list.items), len(tree.list.items))
	}
}

func TestLongLivedModelsDetachConcreteStrings(t *testing.T) {
	backing := strings.Repeat("discarded ", 1024) + "owned"
	source := backing[len(backing)-len("owned"):]
	assertDetached := func(name, got string) {
		t.Helper()
		if unsafe.StringData(got) == unsafe.StringData(source) { //nolint:gosec // compare ownership; no pointer is dereferenced.
			t.Errorf("%s retained the caller's backing string", name)
		}
	}

	container := Rows(Item{Key: source})
	assertDetached("container key", container.Items()[0].Key)

	tabs := NewTabs(Tab{Title: source})
	assertDetached("tab title", tabs.Items()[0].Title)

	dialog := NewDialog(&Stack{}, source, &retainedModal{})
	assertDetached("dialog title", dialog.Title())
	dialog.SetDescription(source)
	assertDetached("dialog description", dialog.Description())

	slider := NewSlider(0, 1)
	slider.SetLabel(source)
	assertDetached("slider label", slider.Label())

	var filter Filter[string]
	filter.SetText(func(item string) string { return item })
	filter.SetPattern(source)
	assertDetached("filter pattern", filter.Pattern())

	var editor Editor
	editor.SetText(source)
	assertDetached("editor text", editor.Text())

	var selectField Select[string]
	selectField.SetOptions([]Option[string]{{Label: source, Value: "value"}})
	assertDetached("option label", selectField.Options()[0].Label)
}

func makeTabs(children []*retainedWidget) []Tab {
	tabs := make([]Tab, len(children))
	for i, child := range children {
		tabs[i] = Tab{Title: strconv.Itoa(i), Of: child}
	}
	return tabs
}

type retainedBlock struct{ payload []byte }

func (b *retainedBlock) Measure(int) int { return 1 }
func (b *retainedBlock) Draw(grid.View)  {}
func (b *retainedBlock) Rows(int) []text.Row {
	return []text.Row{{Text: string(b.payload)}}
}

func TestTranscriptCommitReleasesPayloadAndPlacement(t *testing.T) {
	var transcript Transcript
	transcript.Resize(80)
	ids := make([]BlockID, 1024)
	for i := range ids {
		ids[i] = transcript.Append(&retainedBlock{payload: []byte{byte(i)}})
		if i < len(ids)-1 {
			transcript.Finish(ids[i])
		}
	}

	storage := transcript.blocks
	if got := transcript.Commit(func(Block, int) bool { return true }); got != len(ids)-1 {
		t.Fatalf("committed %d blocks, want %d", got, len(ids)-1)
	}
	for i := range len(ids) - 1 {
		if storage[i] != (placed{}) {
			t.Fatalf("committed placement %d still holds %+v", i, storage[i])
		}
	}
	if len(transcript.blocks) != 1 {
		t.Fatalf("kept %d placements, want one", len(transcript.blocks))
	}
	if cap(transcript.blocks) > 2*len(transcript.blocks)+64 {
		t.Fatalf("placement storage has capacity %d for %d live block", cap(transcript.blocks), len(transcript.blocks))
	}
	if got := transcript.Block(ids[len(ids)-1]); got == nil {
		t.Fatal("the live block lost its stable identity")
	}
	if got := transcript.Block(ids[0]); got != nil {
		t.Fatal("a committed identity still resolves to a payload")
	}
	if got := transcript.StartRow(); got != len(ids)-1 {
		t.Fatalf("live rows start at %d, want %d", got, len(ids)-1)
	}
	if got := transcript.Height(); got != 1 {
		t.Fatalf("live height is %d, want one", got)
	}
}

func TestTranscriptCommittedHeapDoesNotFollowSessionAge(t *testing.T) {
	if os.Getenv("OOLONG_TRANSCRIPT_HEAP_PROBE") != "" {
		transcriptHeapProbe(t)
		return
	}

	small := transcriptHeap(t, 512)
	large := transcriptHeap(t, 1024)
	const noise = 8 << 20
	if large > small+noise {
		t.Fatalf("live heap grew with committed history: 512 blocks=%d bytes, 1024 blocks=%d bytes", small, large)
	}
}

func TestSearchOwnsPendingInputAndCloseReleasesIt(t *testing.T) {
	// There is deliberately no worker: this test is about the private ownership cut,
	// and keeping the mailbox pending makes that cut deterministic.
	s := &Search{
		wake:    make(chan struct{}, 1),
		results: make(chan Result, 1),
		stop:    make(chan struct{}),
	}
	backing := strings.Repeat("discarded ", 1024) + "needle"
	query := backing[len(backing)-len("needle"):]
	var transcript Transcript
	transcript.Resize(80)
	transcript.Append(&retainedBlock{payload: []byte("needle")})
	s.Submit(&transcript, query, false)
	if unsafe.StringData(s.next.query) == unsafe.StringData(query) { //nolint:gosec // compare ownership; no pointer is dereferenced.
		t.Fatal("pending query still shares the caller's backing string")
	}
	s.results <- Result{Query: strings.Repeat("answer", 1024)}

	s.Close()
	if !s.closed || s.hasNext || s.next.query != "" || s.next.regex || s.next.start != 0 || s.next.corpus != nil || s.next.generation != 0 {
		t.Fatalf("closed search retained pending state: closed=%t hasNext=%t next=%+v", s.closed, s.hasNext, s.next)
	}
	if len(s.results) != 0 {
		t.Fatal("closed search retained an unread result")
	}
}

func transcriptHeap(t *testing.T, blocks int) uint64 {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The executable is the already-running test binary returned by the runtime, not
	// user input. A fresh process is what keeps one probe's heap from biasing the next.
	cmd := exec.CommandContext(t.Context(), exe, "-test.run=^TestTranscriptCommittedHeapDoesNotFollowSessionAge$") //nolint:gosec // exe is os.Executable, never user input.
	cmd.Env = append(os.Environ(),
		"OOLONG_TRANSCRIPT_HEAP_PROBE=1",
		"OOLONG_TRANSCRIPT_BLOCKS="+strconv.Itoa(blocks),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("heap probe for %d blocks: %v\n%s", blocks, err, out)
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		if value, ok := strings.CutPrefix(line, "OOLONG_HEAP="); ok {
			n, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				t.Fatalf("heap probe returned %q: %v", value, err)
			}
			return n
		}
	}
	t.Fatalf("heap probe returned no measurement:\n%s", out)
	return 0
}

func transcriptHeapProbe(t *testing.T) {
	t.Helper()
	blocks, err := strconv.Atoi(os.Getenv("OOLONG_TRANSCRIPT_BLOCKS"))
	if err != nil || blocks <= 0 {
		t.Fatalf("invalid block count %q", os.Getenv("OOLONG_TRANSCRIPT_BLOCKS"))
	}
	var transcript Transcript
	transcript.Resize(80)
	for range blocks {
		id := transcript.Append(&retainedBlock{payload: make([]byte, 64<<10)})
		transcript.Finish(id)
		transcript.Commit(func(Block, int) bool { return true })
	}
	runtime.GC()
	debug.FreeOSMemory()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	fmt.Printf("OOLONG_HEAP=%d\n", stats.HeapAlloc)
	runtime.KeepAlive(&transcript)
}
