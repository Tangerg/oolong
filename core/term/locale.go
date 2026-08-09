package term

// DetectLocaleIn returns the locale that decides character encoding in an explicit
// terminal environment. LC_ALL overrides every category, LC_CTYPE overrides the
// language default, and LANG is the fallback. An empty string means the environment
// made no claim.
func DetectLocaleIn(lookup func(string) (string, bool)) string {
	if lookup == nil {
		return ""
	}
	for _, name := range [...]string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if value, ok := lookup(name); ok && value != "" {
			return value
		}
	}
	return ""
}
