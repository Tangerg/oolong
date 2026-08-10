package text_test

import (
	"testing"

	"github.com/Tangerg/oolong/core/text"
)

func TestPrefersUnicodeKeepsTheModernDefaultOrRequiresUTF8(t *testing.T) {
	for _, test := range []struct {
		locale string
		want   bool
	}{
		{"en_US.UTF-8", true},
		{"zh_CN.utf8", true},
		{"en_GB.UTF8@calendar=gregorian", true},
		{"", true},
		{"C", false},
		{"POSIX", false},
		{"en_US", false},
		{"en_US.ISO-8859-1", false},
	} {
		if got := text.PrefersUnicode(test.locale); got != test.want {
			t.Errorf("PrefersUnicode(%q) = %t, want %t", test.locale, got, test.want)
		}
	}
}
