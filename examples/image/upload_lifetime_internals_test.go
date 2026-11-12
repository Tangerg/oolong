package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/uitest"
)

type cancellingImageHost struct {
	*imageHost
	cancel context.CancelFunc
}

func (h *cancellingImageHost) Transmit(ctx context.Context, data []byte) (<-chan graphics.UploadResult, error) {
	result, err := h.imageHost.Transmit(ctx, data)
	h.cancel()
	return result, err
}

func TestUploadCancellationRetainsCleanupAfterAdmission(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	host := &cancellingImageHost{
		imageHost: &imageHost{Host: uitest.New(t, uitest.Config{Width: 40, Height: 12})},
		cancel:    cancel,
	}
	if err := run(ctx, filepath.Join("../..", samplePNG), host); err != nil {
		t.Fatal(err)
	}
	if host.image.ID() == 0 {
		t.Fatal("upload never reached transport admission")
	}
	if host.image.Live() {
		t.Fatal("revoked result delivery orphaned an accepted image")
	}
}
