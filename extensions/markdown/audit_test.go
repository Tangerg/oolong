package markdown_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"

	"github.com/Tangerg/oolong/extensions/markdown"
)

func TestMarkdownTextFollowsSemanticNormalization(t *testing.T) {
	for _, test := range [][2]string{
		{`\*literal\* &amp; &#20013;`, "*literal* & 中"},
		{"`one\ntwo`", "one two"},
		{"`&amp;`", "&amp;"},
		{`\&amp;`, "&amp;"},
		{"![a\nb](url)", "[a b]"},
		{"&notit; &#00000065; &#x0000041;", "&notit; &#00000065; &#x0000041;"},
		{"&#65; &#x42; &amp;lt;", "A B &lt;"},
	} {
		got := render(t, 60, test[0])
		if len(got) != 1 || got[0] != test[1] {
			t.Fatalf("%q: %q, want %q", test[0], got, test[1])
		}
	}
}

func TestSourceNormalizationPrecedesEveryMarkdownSyntax(t *testing.T) {
	for _, test := range []struct{ name, source, want string }{
		{"prose", "before\x00after", "before�after"},
		{"code span", "`before\x00after`", "before�after"},
		{"fenced code", "```\nbefore\x00after\n```", "before�after"},
		{"indented code", "    before\x00after", "before�after"},
		{"image label", "![before\x00after](image.png)", "[before�after]"},
		{"reference identity", "[kept][a\x00b]\n\n[a�b]: /target", "kept"},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := render(t, 60, test.source)
			if !slices.Equal(rows, []string{test.want}) {
				t.Fatalf("Render(%q) = %q, want %q", test.source, rows, test.want)
			}
		})
	}
}

func TestExtensionPreparationReceivesTheNormalizedSource(t *testing.T) {
	for _, test := range []struct {
		name, source, info string
		kind               markdown.Extension
	}{
		{"code", "```a\x00b\r\nbefore\x00after\r\n```", "a�b", markdown.FencedCode},
		{"math", "$$\r\nbefore\x00after\r\n$$", "", markdown.DisplayMath},
	} {
		t.Run(test.name, func(t *testing.T) {
			var appearance markdown.Look
			calls := 0
			appearance.SetRenderer(test.kind, func(info, source string) (grid.Drawable, error) {
				calls++
				if info != test.info || source != "before�after" {
					t.Errorf("extension info=%q source=%q", info, source)
				}
				return nil, markdown.ErrUnhandled
			})
			_ = mustRender(t, test.source, appearance)
			if calls != 1 {
				t.Fatalf("extension calls = %d, want 1", calls)
			}
		})
	}
}

func TestFenceInfoHasSemanticTextButCodeKeepsItsSource(t *testing.T) {
	for _, info := range []string{"go&#108;ang", `go\+\+`} {
		look := look()
		var gotInfo, gotSource string
		look.SetRenderer(markdown.FencedCode, func(info, source string) (grid.Drawable, error) {
			gotInfo, gotSource = info, source
			return text.NewBlock(text.BlockConfig{}), nil
		})
		if _, err := markdown.Render("```"+info+"\n&amp; \\x\n```", look); err != nil {
			t.Fatal(err)
		}
		want := "golang"
		if info == `go\+\+` {
			want = "go++"
		}
		if gotInfo != want || gotSource != "&amp; \\x" {
			t.Fatalf("info=%q source=%q", gotInfo, gotSource)
		}
	}
}

func TestLinkedImageKeepsItsLabelAndOuterDestination(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{"[![<ada@example.test>](badge.png)](project)", "project"},
		{"![<ada@example.test>](badge.png)", "badge.png"},
		{"[![<ada@example.test>](badge.png)]()", ""},
	} {
		var doc markdown.Doc
		doc.SetBlocks(mustRender(t, tc.source, markdown.Look{}))
		rows := doc.Rows(40)
		if len(rows) != 1 || rows[0].Text != "[ada@example.test]" {
			t.Fatalf("image label: %+v", rows)
		}
		surface := grid.NewSurface(40, 1)
		doc.Draw(surface.View())
		if got := cellAt(surface, 0, 0).Link; got != tc.want {
			t.Errorf("%s points at %q, want %q", tc.source, got, tc.want)
		}
	}
}

func TestFenceInfoDispatchesOnTheFirstWord(t *testing.T) {
	type call struct {
		kind markdown.Extension
		info string
	}
	want := []call{{markdown.FencedCode, "go"}, {markdown.DisplayMath, ""}}
	for _, sep := range []string{" ", "\t"} {
		var calls []call
		var appearance markdown.Look
		for _, kind := range []markdown.Extension{markdown.FencedCode, markdown.DisplayMath} {
			appearance.SetRenderer(kind, func(info, _ string) (grid.Drawable, error) {
				calls = append(calls, call{kind, info})
				return nil, markdown.ErrUnhandled
			})
		}
		_ = mustRender(t, "~~~go"+sep+"example\nx\n~~~\n\n~~~math"+sep+"display\ny\n~~~", appearance)
		if !slices.Equal(calls, want) {
			t.Errorf("separator %q dispatched %v, want %v", sep, calls, want)
		}
	}
}
