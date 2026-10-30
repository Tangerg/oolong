package kit_test

import (
	"errors"
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestTranscriptRetainsNativeImagesThatScrollbackCannotOwn(t *testing.T) {
	publisher, err := publication.New(publication.Config{Width: 20})
	if err != nil {
		t.Fatal(err)
	}
	skin, root := transcriptSkin(t, kit.TranscriptConfig{Publisher: publisher, Retention: base.PublishCompleted})
	model := skin.Controller()
	t.Cleanup(model.Close)
	first, err := model.Add(&said{rows: []string{"accepted"}})
	if err != nil {
		t.Fatal(err)
	}
	native, err := model.Add(kit.Image{Of: graphics.Image{ID: 1, Size: image.Pt(20, 20)}, Cell: image.Pt(10, 10)})
	if err != nil {
		t.Fatal(err)
	}
	last, err := model.Add(&said{rows: []string{"retained suffix"}})
	if err != nil {
		t.Fatal(err)
	}
	if first.State() != base.ContentRetired || native.State() != base.ContentRetained || last.State() != base.ContentRetained || model.Len() != 2 {
		t.Fatal("unsupported image or its suffix transferred without a scrollback owner")
	}
	if !errors.Is(native.Err(), errors.ErrUnsupported) || native.Receipt().Snapshot().ID != 0 {
		t.Fatalf("native publication error=%v receipt=%+v", native.Err(), native.Receipt().Snapshot())
	}
	publisher.Commit(publisher.Prepare(), 1)
	publisher.Written(1)
	publisher.FlushNotifications()
	if items, _ := publisher.Pending(); items != 0 || model.Len() != 2 || model.FirstBlock() != native.ID() {
		t.Fatal("capacity progress let the suffix overtake unsupported content")
	}
	surface := transcriptPaint(t, root, 20, 3, 1)
	if row := transcriptRow(surface.View(), 2); row != "retained suffix" {
		t.Fatalf("unsupported native content lost the live suffix: %q", row)
	}
}
