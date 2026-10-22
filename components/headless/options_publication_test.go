package headless_test

import (
	"runtime"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
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
					var setOptions func([]headless.Option[string])
					var options func() []headless.Option[string]
					var answer func() []string
					if kind == "Select" {
						field := headless.NewSelect(headless.SelectConfig[string]{Same: same})
						if ownership == "controlled" {
							value := "a"
							field.SetBinding(headless.Bind(&value))
						}
						field.SetOptions(headless.Options("a", "b"))
						setOptions, options = field.SetOptions, field.Options
						answer = func() []string {
							chosen, ok := field.Chosen()
							if !ok {
								return nil
							}
							return []string{chosen.Value}
						}
					} else {
						field := headless.NewMultiSelect(headless.MultiSelectConfig[string]{Same: same})
						field.SetOptions(headless.Options("a", "b"))
						if ownership == "controlled" {
							value := []string{"a"}
							field.SetBinding(headless.Bind(&value))
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
						setOptions(headless.Options("b", "a"))
						t.Error("interrupted identity comparison returned")
					}()
					<-done
					fail = false
					if !slices.Equal(options(), before) || !slices.Equal(answer(), []string{"a"}) {
						t.Errorf("interrupted replacement published options=%v answer=%v", options(), answer())
					}
					setOptions(headless.Options("b", "a"))
					if !slices.Equal(options(), headless.Options("b", "a")) || !slices.Equal(answer(), []string{"a"}) {
						t.Fatalf("completed replacement lost the choice: options=%v answer=%v", options(), answer())
					}
				})
			}
		}
	}
}
