package base_test

import (
	"errors"
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestFieldValidationRevocationSurvivesInterruptedCallbacks(t *testing.T) {
	for _, kind := range []string{"text", "select", "multi-select", "confirm"} {
		for _, operation := range []string{"validate", "reply"} {
			for _, exit := range []string{"panic", "Goexit"} {
				t.Run(kind+"/"+operation+"/"+exit, func(t *testing.T) {
					field, install := fieldWithPolicy(kind)
					problem := errors.New("invalid answer")
					interrupt := false
					install(func() error {
						if interrupt {
							if exit == "Goexit" {
								runtime.Goexit()
							}
							panic("interrupted validation")
						}
						return problem
					})
					root := semanticOwner(t, field.Element())
					if err := field.Validate(); !errors.Is(err, problem) {
						t.Fatal(err)
					}
					receipt := field.Validation()
					if receipt == nil || !receipt() {
						t.Fatal("completed validation has no receipt")
					}
					before, changed := root.Progress().ModelRevision, root.Changes()
					interrupt = true
					invoke := func() { _ = field.Validate() }
					if operation == "reply" {
						answer := "1"
						switch kind {
						case "text":
							answer = "answer"
						case "confirm":
							answer = "yes"
						}
						invoke = func() { _ = field.(interface{ Reply(said string) error }).Reply(answer) }
					}
					done := make(chan struct{})
					go func() {
						defer close(done)
						defer func() {
							failure := recover()
							if exit == "panic" && failure != "interrupted validation" || exit == "Goexit" && failure != nil {
								t.Errorf("callback exit=%v", failure)
							}
						}()
						invoke()
						t.Error("interrupted validation returned")
					}()
					<-done
					if field.Error() != nil || field.Validation() != nil || receipt() {
						t.Fatal("interrupted validation retained its predecessor")
					}
					select {
					case <-changed:
					default:
						t.Fatal("revocation did not wake its scene")
					}
					observation, err := root.Observe(scene.Query{})
					if err != nil || observation.Progress.ModelRevision <= before {
						t.Fatalf("revocation did not advance observation: %+v, %v", observation.Progress, err)
					}
					for _, node := range observation.Nodes {
						if node.State.Has(scene.StateInvalid) {
							t.Fatal("revoked validation remained semantically invalid")
						}
					}
				})
			}
		}
	}
}
