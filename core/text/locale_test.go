package text_test

import (
	"testing"

	"github.com/Tangerg/oolong/core/text"
)

func TestUTF8LocaleRequiresAnExplicitUTF8Encoding(t *testing.T) {
	for _, test := range []struct {
		locale string
		want   bool
	}{
		{"en_US.UTF-8", true},
		{"zh_CN.utf8", true},
		{"en_GB.UTF8@calendar=gregorian", true},
		{"", false},
		{"C", false},
		{"POSIX", false},
		{"en_US", false},
		{"en_US.ISO-8859-1", false},
	} {
		if got := text.UTF8Locale(test.locale); got != test.want {
			t.Errorf("UTF8Locale(%q) = %t, want %t", test.locale, got, test.want)
		}
	}
}
