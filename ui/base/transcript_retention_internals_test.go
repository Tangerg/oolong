package base

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

type retainedBlock struct{ payload []byte }

func (b *retainedBlock) HeightForWidth(int) int { return 1 }
func (b *retainedBlock) Draw(grid.View)         {}
func (b *retainedBlock) Rows(int) []text.Row {
	return []text.Row{{Text: string(b.payload)}}
}

func TestTranscriptCommitReleasesPayloadAndPlacement(t *testing.T) {
	p := transcriptPublisher(t)
	if _, err := p.TryPublish(identityBlock{}); err != nil {
		t.Fatal(err)
	}
	transcript, err := NewTranscript(TranscriptConfig{Publisher: p, Retention: PublishCompleted, MaxItems: 1024})
	if err != nil {
		t.Fatal(err)
	}
	items := make([]*TranscriptItem, 1024)
	for i := range items {
		items[i], err = transcript.Add(&retainedBlock{payload: []byte{byte(i)}})
		if err != nil {
			t.Fatal(err)
		}
	}
	stageTranscriptForTest(transcript, 80)
	for sequence := uint64(1); sequence <= 1023; sequence++ {
		p.Commit(p.Prepare(), sequence)
		p.Written(sequence)
		p.FlushNotifications()
	}
	for _, item := range items[:1023] {
		if item.body != nil || item.document != nil || item.cancel != nil || item.State() != ContentRetired {
			t.Fatal("retired handle retains content or subscription")
		}
	}
	if transcript.Len() != 1 || transcript.Block(items[1023].ID()) == nil {
		t.Fatal("live content lost its identity")
	}
	if transcript.Block(items[0].ID()) != nil {
		t.Fatal("retired identity still resolves")
	}
	stageTranscriptForTest(transcript, 80)
	if transcript.StartRow() != 1023 || transcript.Height() != 1 {
		t.Fatalf("retained extent=%d+%d", transcript.StartRow(), transcript.Height())
	}
	if len(transcript.geometry.Value().blocks) != 1 {
		t.Fatal("new projection retains retired placements")
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
	out, err := cmd.Output()
	if err != nil {
		var diagnostic []byte
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			diagnostic = exit.Stderr
		}
		t.Fatalf("heap probe for %d blocks: %v\nstdout:\n%s\nstderr:\n%s", blocks, err, out, diagnostic)
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
	p := transcriptPublisher(t)
	transcript, err := NewTranscript(TranscriptConfig{Publisher: p, Retention: PublishCompleted})
	if err != nil {
		t.Fatal(err)
	}
	for sequence := 1; sequence <= blocks; sequence++ {
		if _, err := transcript.Add(&retainedBlock{payload: make([]byte, 64<<10)}); err != nil {
			t.Fatal(err)
		}
		p.Commit(p.Prepare(), uint64(sequence))
		p.Written(uint64(sequence))
		p.FlushNotifications()
	}
	stageTranscriptForTest(transcript, 80)
	runtime.GC()
	debug.FreeOSMemory()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	fmt.Printf("OOLONG_HEAP=%d\n", stats.HeapAlloc)
	runtime.KeepAlive(transcript)
	runtime.KeepAlive(p)
}
