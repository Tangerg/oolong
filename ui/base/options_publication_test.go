package base_test

import (
	"runtime"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func TestOptionReplacementPreparesIdentityBeforeChangingTheField(t *testing.T) {
	for _, kind := range []string{"Select", "MultiSelect"} {
		for _, ownership := range []string{"local", "controlled"} {
			for _, exit := range []string{"panic", "Goexit"} {
				t.Run(kind+"/"+ownership+"/"+exit, func(t *testing.T) {
					fail := false
					same := func(a, b string) bool {
						if fail {
							if exit == "Goexit" {
								runtime.Goexit()
							}
							panic("identity interrupted")
						}
						return a == b
					}
					var setOptions func([]base.Option[string])
					var options func() []base.Option[string]
					var answer func() []string
					if kind == "Select" {
						field := base.NewSelect(base.SelectConfig[string]{Same: same})
						if ownership == "controlled" {
							value := "a"
							field.Choose(value)
						}
						field.SetOptions(base.Options("a", "b"))
						setOptions, options = field.SetOptions, field.Options
						answer = func() []string {
							chosen, ok := field.Chosen()
							if !ok {
								return nil
							}
							return []string{chosen.Value}
						}
					} else {
						field := base.NewMultiSelect(base.MultiSelectConfig[string]{Same: same})
						field.SetOptions(base.Options("a", "b"))
						if ownership == "controlled" {
							value := []string{"a"}
							field.SetTaken(value)
						} else {
							field.Toggle()
						}
						setOptions, options, answer = field.SetOptions, field.Options, field.Taken
					}
					before := options()
					fail = true
					done := make(chan struct{})
					go func() {
						defer close(done)
						defer func() {
							if got := recover(); (exit == "panic" && got != "identity interrupted") || (exit == "Goexit" && got != nil) {
								t.Errorf("identity exit = %v", got)
							}
						}()
						setOptions(base.Options("b", "a"))
						t.Error("interrupted identity comparison returned")
					}()
					<-done
					fail = false
					if !slices.Equal(options(), before) || !slices.Equal(answer(), []string{"a"}) {
						t.Errorf("interrupted replacement published options=%v answer=%v", options(), answer())
					}
					setOptions(base.Options("b", "a"))
					if !slices.Equal(options(), base.Options("b", "a")) || !slices.Equal(answer(), []string{"a"}) {
						t.Fatalf("completed replacement lost the choice: options=%v answer=%v", options(), answer())
					}
				})
			}
		}
	}
}
