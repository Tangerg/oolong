package headless_test

import (
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/layout"
)

func TestAnswerPublicationCannotRefuseAFieldFocusTransfer(t *testing.T) {
	for _, kind := range []string{"Text", "MultiSelect"} {
		for _, gaining := range []bool{false, true} {
			for _, contained := range []bool{false, true} {
				t.Run(kind+map[bool]string{false: "/blur", true: "/focus"}[gaining]+map[bool]string{false: "/direct", true: "/container"}[contained], func(t *testing.T) {
					checks := 0
					var field headless.Field
					var changeAnswer func()
					switch kind {
					case "Text":
						value := &publishingValue[string]{value: "initial"}
						text := headless.NewText(headless.TextConfig{Value: value, Check: func(string) error { checks++; return nil }})
						field = text
						changeAnswer = func() {
							value.value = "first\nsecond"
							value.set = func(string) { value.set = nil; text.SetText("latest") }
						}
					case "MultiSelect":
						value := &publishingValue[[]string]{value: []string{"a"}}
						choices := headless.NewMultiSelect(headless.MultiSelectConfig[string]{
							Value: value, Same: headless.Equal[string], Check: func([]string) error { checks++; return nil },
						})
						choices.SetOptions(headless.Options("a", "b"))
						field = choices
						changeAnswer = func() {
							value.value = []string{"a", "unavailable"}
							value.set = func([]string) {
								value.set = nil
								choices.Do(headless.SelectNext)
								choices.Toggle()
							}
						}
					}
					other := &reentrant{name: "other"}
					focus := field.Focus
					var container *headless.Container
					if contained {
						container = headless.NewContainer(layout.Down, headless.Item{Of: field}, headless.Item{Of: other})
						focus = func(has bool) {
							if has {
								container.FocusIndex(0)
							} else {
								container.FocusIndex(1)
							}
						}
					} else {
						focus(true)
					}
					if gaining {
						focus(false)
						checks = 0
						changeAnswer()
						focus(true)
					} else {
						changeAnswer()
					}
					focus(false)
					if checks != 1 {
						t.Fatalf("focus transfer lost blur validation: checks=%d, want 1", checks)
					}
					if contained && (container.Focused() != other || !other.focused) {
						t.Fatal("focus transfer did not settle on the other child")
					}
				})
			}
		}
	}
}

func TestOnlyANewerFocusTransitionCanRevokeBlurValidation(t *testing.T) {
	for _, kind := range []string{"Text", "MultiSelect"} {
		for _, returning := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: "/repeated blur", true: "/away and back"}[returning], func(t *testing.T) {
				checks := 0
				var field headless.Field
				var publish func(func())
				switch kind {
				case "Text":
					value := &publishingValue[string]{value: "initial"}
					field = headless.NewText(headless.TextConfig{Value: value, Check: func(string) error { checks++; return nil }})
					publish = func(callback func()) {
						value.value = "first\nsecond"
						value.set = func(string) { value.set = nil; callback() }
					}
				case "MultiSelect":
					value := &publishingValue[[]string]{value: []string{"a"}}
					choices := headless.NewMultiSelect(headless.MultiSelectConfig[string]{
						Value: value, Same: headless.Equal[string],
						Check: func([]string) error { checks++; return nil },
					})
					choices.SetOptions(headless.Options("a", "b"))
					field = choices
					publish = func(callback func()) {
						value.value = []string{"a", "unavailable"}
						value.set = func([]string) { value.set = nil; callback() }
					}
				}
				field.Focus(true)
				publish(func() {
					if returning {
						field.Focus(true)
					}
					field.Focus(false)
				})
				field.Focus(false)
				if checks != 1 {
					t.Fatalf("blur validated %d times, want only its current transition", checks)
				}
			})
		}
	}
}
