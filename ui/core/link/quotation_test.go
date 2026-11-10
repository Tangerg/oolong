package link_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/link"
)

// A URL inside a quoted sentence used to keep the sentence's final period.
func TestAURLInsideAQuotedSentenceLeavesItsPunctuation(t *testing.T) {
	links := link.Detect(`He said "see https://go.dev/doc." and left`, nil)
	if len(links) != 1 || links[0].Target != "https://go.dev/doc" {
		t.Fatalf("links = %+v, want the URL without the sentence's period", links)
	}
	links = link.Detect(`"https://go.dev/doc."`, nil)
	if len(links) != 1 || links[0].Target != "https://go.dev/doc." {
		t.Fatalf("links = %+v, want the whole quoted URL", links)
	}
}

// A quoted sentence used to be taken whole as a relative file name, hiding the
// rooted path inside it.
func TestARootedPathInsideAQuotedSentenceIsTheLink(t *testing.T) {
	source := `error: "cannot open /etc/app/config.yaml"`
	for name, exists := range map[string]func(string) bool{
		"unconfirmed": nil,
		"confirmed":   func(string) bool { return false },
	} {
		t.Run(name, func(t *testing.T) {
			links := link.Detect(source, exists)
			if len(links) != 1 || links[0].Kind != link.File || links[0].Target != "/etc/app/config.yaml" {
				t.Fatalf("links = %+v, want the rooted path", links)
			}
		})
	}
	links := link.Detect(`open "My Documents/report.txt" now`, nil)
	if len(links) != 1 || links[0].Target != "My Documents/report.txt" {
		t.Fatalf("links = %+v, want the quoted name with its space", links)
	}
}
