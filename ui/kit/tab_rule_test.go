package kit_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestTabDecorationAndPanelShareOneHeight(t *testing.T) {
	tabs := kit.NewTabs(kit.TabsConfig{NoRule: true, Glyphs: kit.ASCII(), Items: []base.Tab{{Title: "tab", Content: testElement(kit.Label{Text: "body"})}}})
	tabs.SetRule("-")
	rows := paintWidget(10, 4, tabs)
	if !strings.Contains(rows[1], "-") || !strings.Contains(rows[2], "body") || tabs.HeightForWidth(10) != 3 {
		t.Fatalf("rule did not move the panel: rows=%q height=%d", rows, tabs.HeightForWidth(10))
	}
}
