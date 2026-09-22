//go:build !unix && !windows

package mermaid_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/mermaid"
)

func TestUnsupportedPlatformFailsConstruction(t *testing.T) {
	if _, err := mermaid.New(mermaid.Config{}); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("New error=%v", err)
	}
}
