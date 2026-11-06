package markdown

import (
	"net/mail"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/idna"

	"github.com/Tangerg/oolong/ui/core/text"
)

const (
	schemeHTTP    = "http"
	schemeHTTPS   = "https"
	schemeMailto  = "mailto"
	bareWebPrefix = "www."
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
	h := hyperlink{target: destination}
	var address addressIdentity
	target, err := url.Parse(destination)
	if err == nil {
		h.openable = openable(target)
		address = identifyAddress(target)
	}
	// Drawing may remove both controls and adjacent zero-width clusters. Trust
	// must use that same projection, not a second character-filtering policy.
	shown := text.Line{{Text: label}}.Wrap(0)[0].Line.String()
	h.disguised = disguises(shown, address)
	return h
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

// A prose link scanner may omit addresses it cannot confidently delimit. That
// omission cannot authorize a label: ambiguous address-like words must expose
// the destination, and IDNs must retain their whole host during comparison.
var labelURLPrefix = regexp.MustCompile(`(?i)(?:` + schemeHTTP + `|` + schemeHTTPS + `|` + schemeMailto + `):|` + regexp.QuoteMeta(bareWebPrefix))

var labelAddressStart = regexp.MustCompile(labelURLPrefix.String() + `|[^\s<>]*@`)

func disguises(label string, target addressIdentity) bool {
	// Quoted mailbox local-parts and recipient lists can contain whitespace.
	if named := identifyMailboxes(label); named.kind == mailAddress && !labelURLPrefix.MatchString(label) {
		return !named.matches(target)
	}
	for word := range strings.FieldsSeq(label) {
		start := labelAddressStart.FindStringIndex(word)
		if start == nil {
			continue
		}
		if !identifyLabelAddress(word[start[0]:]).matches(target) {
			return true
		}
	}
	return false
}

type addressKind uint8

const (
	unknownAddress addressKind = iota
	webAddress
	mailAddress
)

// Only the canonical host or recipient set participates in label trust. Paths
// may differ on one web host; mailbox local-parts remain case-sensitive.
type addressIdentity struct {
	kind addressKind
	name string
}

func (a addressIdentity) matches(other addressIdentity) bool {
	return a.kind != unknownAddress && a == other
}

func identifyLabelAddress(label string) addressIdentity {
	if strings.HasPrefix(strings.ToLower(label), bareWebPrefix) {
		label = schemeHTTPS + "://" + label
	}
	target, err := url.Parse(label)
	if err == nil && target.Scheme != "" {
		return identifyAddress(target)
	}
	return identifyMailboxes(label)
}

func identifyAddress(target *url.URL) addressIdentity {
	switch target.Scheme {
	case schemeHTTP, schemeHTTPS:
		if host := addressHost(target.Hostname()); host != "" {
			return addressIdentity{kind: webAddress, name: host}
		}
	case schemeMailto:
		return identifyMailto(target)
	}
	return addressIdentity{}
}

var addressDomains = idna.New(idna.MapForLookup(), idna.BidiRule(), idna.VerifyDNSLength(true))

func addressHost(host string) string {
	if address, err := netip.ParseAddr(host); err == nil {
		return address.String()
	}
	canonical, err := addressDomains.ToASCII(host)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(canonical, ".")
}

func identifyMailto(target *url.URL) addressIdentity {
	address, err := url.PathUnescape(target.Opaque)
	if err != nil || target.Fragment != "" {
		return addressIdentity{}
	}
	var recipients strings.Builder
	recipients.WriteString(address)
	for field := range strings.SplitSeq(target.RawQuery, "&") {
		name, value, _ := strings.Cut(field, "=")
		name, err = url.PathUnescape(name)
		if err != nil {
			return addressIdentity{}
		}
		switch strings.ToLower(name) {
		case "to", "cc", "bcc":
			// Mailto uses percent encoding, not form encoding: '+' belongs to a mailbox.
			value, err = url.PathUnescape(value)
			if err != nil {
				return addressIdentity{}
			}
			if value == "" {
				continue
			}
			if recipients.Len() != 0 {
				recipients.WriteByte(',')
			}
			recipients.WriteString(value)
		}
	}
	return identifyMailboxes(recipients.String())
}

func identifyMailboxes(value string) addressIdentity {
	if !mailboxAddrSpecs(value) {
		return addressIdentity{}
	}
	addresses, err := mail.ParseAddressList(value)
	if err != nil || len(addresses) == 0 {
		return addressIdentity{}
	}
	names := make([]string, 0, len(addresses))
	for _, address := range addresses {
		at := strings.LastIndexByte(address.Address, '@')
		local, domain := address.Address[:at], address.Address[at+1:]
		if !strings.HasPrefix(domain, "[") {
			domain = addressHost(domain)
		}
		if domain == "" {
			return addressIdentity{}
		}
		mailbox := mail.Address{Address: local + "@" + domain}
		names = append(names, mailbox.String())
	}
	slices.Sort(names)
	return addressIdentity{kind: mailAddress, name: strings.Join(slices.Compact(names), ",")}
}

// net/mail also accepts names, comments and groups. Those can discard a second
// visible address, so only addr-spec lists may establish a label's identity.
func mailboxAddrSpecs(value string) bool {
	var closing rune
	escaped := false
	for _, r := range value {
		if escaped {
			escaped = false
			continue
		}
		switch closing {
		case '"':
			switch r {
			case '\\':
				escaped = true
			case '"':
				closing = 0
			}
		case ']':
			if r == ']' {
				closing = 0
			}
		default:
			switch r {
			case '"':
				closing = '"'
			case '[':
				closing = ']'
			case '<', '>', ':', ';', '(', ')':
				return false
			}
		}
	}
	return closing == 0
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
