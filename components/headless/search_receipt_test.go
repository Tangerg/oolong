package headless_test

import (
	"testing"
	"testing/synctest"

	"github.com/Tangerg/oolong/components/headless"
)

func TestReceivedSearchResultCannotPublishIntoANewerSubmission(t *testing.T) {
	for _, change := range []string{"same query", "query round trip", "mode"} {
		t.Run(change, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := searching(t)
				query := "cat"
				if change == "mode" {
					query = "cat|dog"
				}
				tr := transcriptOf(plainRows(query)...)
				s.Submit(tr, query, false)
				previous := found(t, s, query)
				copied := previous
				if !previous.Current() || !copied.Current() {
					t.Fatal("a received result was not current")
				}
				if change == "query round trip" {
					s.Submit(tr, "other", false)
				}
				tr = transcriptOf(plainRows("cat", "cat|dog")...)
				s.Submit(tr, query, change == "mode")
				if previous.Current() || copied.Current() {
					t.Fatal("a received result kept publication authority after a newer request")
				}
				latest := found(t, s, query)
				if latest.Query != previous.Query || !latest.Current() {
					t.Fatal("query text did not distinguish the submission receipt")
				}
				var applied []headless.Result
				for _, result := range []headless.Result{latest, previous} {
					if result.Current() {
						applied = append(applied, result)
					}
				}
				if len(applied) != 1 || len(applied[0].Matches) != len(latest.Matches) {
					t.Fatal("delayed publication replaced the latest result")
				}
			})
		})
	}
}

func TestSearchCancellationInvalidatesResultsAlreadyReceived(t *testing.T) {
	for _, cancel := range []string{"empty query", "nil transcript", "close"} {
		t.Run(cancel, func(t *testing.T) {
			s := searching(t)
			tr := transcriptOf(plainRows("cat")...)
			s.Submit(tr, "cat", false)
			result := found(t, s, "cat")
			switch cancel {
			case "empty query":
				s.Submit(tr, "", false)
			case "nil transcript":
				s.Submit(nil, "cat", false)
			case "close":
				s.Close()
			}
			if result.Current() {
				t.Fatal("a cancelled result retained publication authority")
			}
			if len(result.Matches) != 1 {
				t.Fatal("cancellation mutated the consumer's received snapshot")
			}
		})
	}
	if (headless.Result{}).Current() {
		t.Fatal("a result without a submission had publication authority")
	}
}

func TestNilTranscriptCancelsAnUnreadSearchAnswer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := searching(t)
		s.Submit(transcriptOf(plainRows("cat")...), "cat", false)
		synctest.Wait()
		s.Submit(nil, "cat", false)
		synctest.Wait()
		select {
		case <-s.Results():
			t.Fatal("a removed transcript left an old answer in the result stream")
		default:
		}
	})
}
