package program

import (
	"context"
	"runtime"
	"testing"
	"weak"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestClosedScopeReleasesItsExternalContextValues(t *testing.T) {
	_, rt := taskOwner(t)
	scope, supplied, err := func() (*Scope, weak.Pointer[[64]byte], error) {
		type contextKey struct{}
		value := new([64]byte)
		external := context.WithValue(t.Context(), contextKey{}, value)
		reference := weak.Make(value)
		scope, err := rt.NewScope(ScopeConfig{Context: external})
		return scope, reference, err
	}()
	if err != nil {
		t.Fatal(err)
	}
	scope.Close()
	runtime.GC()
	retained := supplied.Value() != nil
	runtime.KeepAlive(scope)
	if retained {
		t.Fatal("closed scope retained values belonging to its external context")
	}
}

func TestScopeDoesNotRetainTheCallersAttachmentStorage(t *testing.T) {
	owner, rt := taskOwner(t)
	scope, supplied, err := func() (*Scope, weak.Pointer[scene.Attachment], error) {
		lease := new(scene.Attachment)
		*lease = owner.root.Content().Attachment()
		reference := weak.Make(lease)
		scope, err := rt.NewScope(ScopeConfig{Context: t.Context(), Attachment: lease})
		return scope, reference, err
	}()
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Close()
	runtime.GC()
	retained := supplied.Value() != nil
	runtime.KeepAlive(scope)
	if retained {
		t.Fatal("scope cancellation retained caller-owned attachment storage after copying its lease")
	}
}
