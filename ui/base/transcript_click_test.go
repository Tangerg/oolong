package base_test

import (
	"image"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestTranscriptClickRunsBelongToThePresentedTextMapping(t *testing.T) {
	for _, test := range []struct {
		name      string
		point     image.Point
		nextWidth int
		scroll    int
		want      string
	}{
		{"repaint", image.Pt(1, 0), 20, 0, "alpha"},
		{"scroll", image.Pt(1, 0), 20, 1, "a"},
		{"reflow", image.Pt(1, 1), 6, 0, "e"},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := transcriptModel(t, base.TranscriptConfig{})
			transcriptAdd(t, model, transcriptBody("alpha beta\ngamma delta\nomega tail"))
			model.Scroll().ToTop()
			root := transcriptScene(t, model)
			transcriptCommit(t, root, 20, 2, 1)
			started := time.Now()
			click := func(elapsed time.Duration) {
				press := input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: test.point, At: started.Add(elapsed)}
				if !root.Handle(press) {
					t.Fatal("presented text refused the click")
				}
				press.Action = input.MouseUp
				root.Handle(press)
			}
			click(0)
			model.Scroll().By(test.scroll)
			transcriptCommit(t, root, test.nextWidth, 2, 2)
			click(50 * time.Millisecond)
			got, err := model.Selection().Text(model.Layout())
			if err != nil || got != test.want {
				t.Fatalf("selected %q, want %q: %v", got, test.want, err)
			}
		})
	}
}
