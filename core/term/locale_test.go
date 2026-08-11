package term_test

import (
	"testing"

	"github.com/Tangerg/oolong/core/term"
)

func TestLocaleFollowsCharacterEnvironmentPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values map[string]string
		want   string
	}{
		{name: "nothing said", values: map[string]string{}},
		{name: "language fallback", values: map[string]string{"LANG": "en_US.UTF-8"}, want: "en_US.UTF-8"},
		{
			name:   "character category outranks language",
			values: map[string]string{"LC_CTYPE": "C", "LANG": "en_US.UTF-8"},
			want:   "C",
		},
		{
			name:   "all categories outrank character category",
			values: map[string]string{"LC_ALL": "POSIX", "LC_CTYPE": "C", "LANG": "en_US.UTF-8"},
			want:   "POSIX",
		},
		{
			name:   "an empty override is absent",
			values: map[string]string{"LC_ALL": "", "LC_CTYPE": "en_GB.UTF-8", "LANG": "C"},
			want:   "en_GB.UTF-8",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookup := func(name string) (string, bool) {
				value, ok := tc.values[name]
				return value, ok
			}
			if got := term.DetectLocale(lookup); got != tc.want {
				t.Fatalf("DetectLocale = %q, want %q", got, tc.want)
			}
		})
	}
	if got := term.DetectLocale(nil); got != "" {
		t.Fatalf("DetectLocale(nil) = %q, want no claim", got)
	}
}
