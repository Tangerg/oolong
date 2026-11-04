package graphics_test

import (
	"errors"
	"runtime"
	"sync"
	"testing"
	"weak"

	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/grid"
)

type imageHost struct {
	owner graphics.ImageOwner
	// Avoid the tiny allocator: weak pointers to its shared blocks may remain live.
	_ [1024]byte
}

func retiredImage(t *testing.T, revoke bool) (graphics.Resource, weak.Pointer[imageHost]) {
	t.Helper()
	host := new(imageHost)
	resource := host.owner.Bind(graphics.Image{ID: 1})
	if revoke {
		host.owner.RevokeAll()
	} else if err := host.owner.Release(resource, func(graphics.Image) error { return nil }); err != nil {
		t.Fatal(err)
	}
	return resource, weak.Make(host)
}

func TestRetiredImageHandlesDoNotRetainTheirHost(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		for _, placement := range []bool{false, true} {
			name := "release/resource"
			if revoke {
				name = "revoke/resource"
			}
			if placement {
				name += "-placement"
			}
			t.Run(name, func(t *testing.T) {
				resource, host := retiredImage(t, revoke)
				var handle any = resource
				if placement {
					handle = resource.Placement(1)
				}
				runtime.GC()
				if host.Value() != nil {
					t.Fatal("retired image retained its enclosing host")
				}
				switch handle := handle.(type) {
				case graphics.Resource:
					if handle.Live() {
						t.Fatal("retired resource became live")
					}
				case graphics.ResourcePlacement:
					if err := handle.CheckTarget(grid.NativeTarget{}); !errors.Is(err, graphics.ErrImageReleased) {
						t.Fatalf("retired placement: %v", err)
					}
				}
				runtime.KeepAlive(handle)
			})
		}
	}
}

func TestImageOwnerConcurrentFirstUseSharesOneLedger(t *testing.T) {
	var owner graphics.ImageOwner
	var targets [16]grid.NativeTarget
	var resources [len(targets)]graphics.Resource
	var workers sync.WaitGroup
	for i := range uint32(len(targets)) {
		workers.Go(func() {
			resources[i] = owner.Bind(graphics.Image{ID: i + 1})
			targets[i] = owner.NativeTarget()
		})
	}
	workers.Wait()
	for i, target := range targets {
		if target != targets[0] || !owner.Contains(resources[i].ID()) {
			t.Fatal("concurrent acquisition split image ownership")
		}
	}
	owner.RevokeAll()
	for _, resource := range resources {
		if resource.Live() {
			t.Fatal("revocation missed an acquired resource")
		}
	}
}
