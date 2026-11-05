package markdown

import (
	"net/url"
	"strings"

	"github.com/Tangerg/oolong/ui/core/link"
)

const (
	schemeHTTP   = "http"
	schemeHTTPS  = "https"
	schemeMailto = "mailto"
)

// hyperlink decides what a document link may do on screen. A document is
// untrusted text, and a terminal opens whatever a hyperlink says when it is
// clicked, so the destination and the words carrying it are judged together.
type hyperlink struct {
	target string
	// openable admits only what a terminal hands to a browser or a mail client.
	// Any other scheme can name a local file or launch whichever application
	// registered for it.
	openable bool
	// disguised means the label shows an address other than the destination,
	// which is how a link misleads a reader who cannot see where it goes.
	disguised bool
}

func newHyperlink(destination, label string) hyperlink {
	target, err := url.Parse(destination)
	if err != nil {
		return hyperlink{target: destination}
	}
	return hyperlink{
		target:    destination,
		openable:  openable(target),
		disguised: disguises(label, target),
	}
}

func openable(target *url.URL) bool {
	switch target.Scheme {
	case schemeHTTP, schemeHTTPS:
		return target.Host != ""
	case schemeMailto:
		return target.Opaque != ""
	default:
		return false
	}
}

func disguises(label string, target *url.URL) bool {
	for _, shown := range link.Detect(label, nil) {
		if shown.Kind != link.URL {
			continue
		}
		named, err := url.Parse(shown.Target)
		if err != nil || !strings.EqualFold(named.Hostname(), target.Hostname()) {
			return true
		}
	}
	return false
}

// labelLink is the destination the label's words may carry. It is empty when
// the destination cannot be opened safely or the label names another address.
func (h hyperlink) labelLink() string {
	if h.openable && !h.disguised {
		return h.target
	}
	return ""
}

// addressLink is the destination the written-out address may carry.
func (h hyperlink) addressLink() string {
	if h.openable {
		return h.target
	}
	return ""
}
