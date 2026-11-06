package kit_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestTabDecorationAndPanelShareOneHeight(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	tabs := kit.NewTabs(kit.TabsConfig{NoRule: true, Glyphs: kit.ASCII(), Items: []base.Tab{{Title: "tab", Content: scenes.Element(kit.Label{Text: "body"})}}})
	tabs.SetRule("-")
	rows := paintWidget(scenes, 10, 4, tabs)
	if !strings.Contains(rows[1], "-") || !strings.Contains(rows[2], "body") || tabs.HeightForWidth(10) != 3 {
		t.Fatalf("rule did not move the panel: rows=%q height=%d", rows, tabs.HeightForWidth(10))
	}
}

func TestTabRuleRepeatsCompleteMulticolumnPatterns(t *testing.T) {
	glyphs := kit.ASCII()
	glyphs.Rule = "界-"
	tabs := kit.NewTabs(kit.TabsConfig{Glyphs: glyphs})
	root, err := scene.NewRoot(scene.RootConfig{Element: tabs.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	var sequence uint64
	for i, tc := range []struct {
		rule string
		want string
	}{
		{rule: "界-", want: "界-界-"},
		{rule: "=-", want: "=-=-=-"},
	} {
		if i > 0 {
			tabs.SetRule(tc.rule)
		}
		rows := paint(6, 2, func(view grid.View) {
			candidate, prepareErr := root.Prepare(view)
			if prepareErr != nil {
				t.Fatal(prepareErr)
			}
			sequence++
			candidate.Commit(sequence)
			root.Wrote(sequence)
		})
		if tabs.Rule() != tc.rule || rows[1] != tc.want {
			t.Fatalf("rule=%q drawn=%q, want=%q / %q", tabs.Rule(), rows[1], tc.rule, tc.want)
		}
	}
}
