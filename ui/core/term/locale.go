package term

import "github.com/Tangerg/oolong/ui/core/termenv"

// DetectLocale returns the locale that decides character encoding in an explicit
// terminal environment. LC_ALL overrides every category, LC_CTYPE overrides the
// language default, and LANG is the fallback. An empty string means the environment
// made no claim.
func DetectLocale(env termenv.Lookup) string {
	for _, name := range [...]string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if value := env.Value(name); value != "" {
			return value
		}
	}
	return ""
}
