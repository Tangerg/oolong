package setting_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/internal/setting"
)

type owner struct {
	refusal       error
	invalidations int
}

func (o *owner) CheckMutation() error { return o.refusal }

func (o *owner) Invalidate() { o.invalidations++ }

func TestAssignInvalidatesOnlyAChange(t *testing.T) {
	var o owner
	value := 1
	setting.Assign(&o, &value, 1)
	setting.Assign(&o, &value, 2)
	if value != 2 || o.invalidations != 1 {
		t.Fatalf("value = %d after %d invalidations, want 2 after 1", value, o.invalidations)
	}
}

func TestAssignRefusesBeforeChanging(t *testing.T) {
	refusal := errors.New("busy")
	o := owner{refusal: refusal}
	value := 1
	defer func() {
		got, _ := recover().(error)
		if !errors.Is(got, refusal) || value != 1 || o.invalidations != 0 {
			t.Fatalf("panic = %v with value %d and %d invalidations", got, value, o.invalidations)
		}
	}()
	setting.Assign(&o, &value, 2)
}

func TestReplaceInvalidatesEveryReplacement(t *testing.T) {
	var o owner
	callback := func() {}
	setting.Replace(&o, &callback, callback)
	setting.Replace(&o, &callback, nil)
	if callback != nil || o.invalidations != 2 {
		t.Fatalf("callback replaced with %d invalidations, want nil after 2", o.invalidations)
	}
}
