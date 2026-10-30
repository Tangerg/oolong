package kit_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestParagraphDetectionFailurePreservesThePublishedTextAndPolicy(t *testing.T) {
	for _, operation := range []string{"text", "links"} {
		t.Run(operation, func(t *testing.T) {
			p := kit.NewParagraph("old.go", grid.Style{})
			fail := false
			p.SetLinks(kit.LinkConfig{Enabled: true, Exists: func(string) bool {
				if fail {
					panic("detection failed")
				}
				return true
			}})
			assertParagraphPublication(t, p, "old.go")
			func() {
				defer func() {
					if got := recover(); got != "detection failed" {
						t.Fatalf("detection panic = %v", got)
					}
				}()
				if operation == "text" {
					fail = true
					p.SetText([]text.Line{text.Of("new.go", grid.Style{})})
				} else {
					p.SetLinks(kit.LinkConfig{Enabled: true, Exists: func(string) bool { panic("detection failed") }})
				}
			}()
			fail = false
			assertParagraphPublication(t, p, "old.go")
			defer func() {
				if got := recover(); got != nil {
					t.Fatalf("failed detection replaced the previous policy: %v", got)
				}
			}()
			p.SetText([]text.Line{text.Of("new.go", grid.Style{})})
			assertParagraphPublication(t, p, "new.go")
		})
	}
}

func TestParagraphDetectionCannotReplaceItsOwnInputs(t *testing.T) {
	for _, operation := range []string{"text", "links"} {
		t.Run(operation, func(t *testing.T) {
			p := kit.NewParagraph("old.go", grid.Style{})
			mutate, entered := false, false
			p.SetLinks(kit.LinkConfig{Enabled: true, Exists: func(string) bool {
				if mutate && !entered {
					entered = true
					if operation == "text" {
						p.SetText([]text.Line{text.Of("other.go", grid.Style{})})
					} else {
						p.SetLinks(kit.LinkConfig{})
					}
				}
				return true
			}})
			assertParagraphPublication(t, p, "old.go")
			mutate = true
			func() {
				defer func() {
					if recover() == nil {
						t.Error("link detection independently replaced its own inputs")
					}
				}()
				p.SetText([]text.Line{text.Of("new.go", grid.Style{})})
			}()
			assertParagraphPublication(t, p, "old.go")
			p.SetText([]text.Line{text.Of("new.go", grid.Style{})})
			assertParagraphPublication(t, p, "new.go")
		})
	}
}

func TestParagraphDetectionReadsTheLastCompletePublication(t *testing.T) {
	p := kit.NewParagraph("old.go", grid.Style{})
	checking := false
	project := func(string) bool {
		if checking {
			assertParagraphPublication(t, p, "old.go")
		}
		return true
	}
	p.SetLinks(kit.LinkConfig{Enabled: true, Exists: project})
	checking = true
	p.SetLinks(kit.LinkConfig{Enabled: true, Exists: project})
	p.SetText([]text.Line{text.Of("new.go", grid.Style{})})
	assertParagraphPublication(t, p, "new.go")
}

func assertParagraphPublication(t *testing.T, p *kit.Paragraph, want string) {
	t.Helper()
	lines, rows := p.Lines(), p.Rows(40)
	if len(lines) != 1 || lines[0].String() != want || len(rows) != 1 || rows[0].Text != want {
		t.Fatalf("inconsistent text publication: lines=%v rows=%v, want %q", lines, rows, want)
	}
	if destination, ok := p.LinkAt(0, 0, 40); !ok || destination.Target != want {
		t.Fatalf("link = %v (%v), want %q", destination, ok, want)
	}
}
