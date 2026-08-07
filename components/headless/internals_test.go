package headless

// Tests of what this package keeps to itself.
//
// The undo history's bound is a promise about memory in a long-lived process, not
// about behaviour a caller can see: an editor that kept every keystroke for a
// session that runs all day is a leak with a friendly name. Nothing outside can
// ask how many steps are held, so this asks from inside.

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"

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
	if len(e.undo) > maxUndo {
		t.Fatalf("history holds %d steps, want at most %d", len(e.undo), maxUndo)
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
	for i, item := range container.Items[:cap(container.Items)] {
		if i >= len(container.Items) && item.Of != nil {
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
	form := &Form{Fields: []Field{fields[0], fields[1], fields[2]}}
	form.Measure(10)
	form.Fields = []Field{fields[0]}
	form.Measure(10)
	for i, item := range form.body.Items[:cap(form.body.Items)] {
		if i >= len(form.body.Items) && item.Of != nil {
			t.Fatalf("form item %d retained a removed field", i)
		}
	}

	values := []int{1, 2, 3}
	tree := &Tree[*int]{Nodes: []Node[*int]{{Item: &values[0]}, {Item: &values[1]}, {Item: &values[2]}}}
	tree.Rows()
	tree.Nodes = tree.Nodes[:1]
	tree.Rows()
	for i, row := range tree.list.Items[:cap(tree.list.Items)] {
		if i >= len(tree.list.Items) && (row.Item != nil || row.path != "") {
			t.Fatalf("tree row %d retained a removed node", i)
		}
	}
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
