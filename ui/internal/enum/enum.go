// Package enum names enumerated values for their String methods, so a value
// observed in a test failure, a log or an automation client reads as a word and
// an unknown one still identifies itself.
package enum

import "strconv"

// Name is names[value], or kind(value) for a value with no name.
func Name[T ~uint8 | ~uint16](names []string, value T, kind string) string {
	if int(value) < len(names) && names[value] != "" {
		return names[value]
	}
	return kind + "(" + strconv.Itoa(int(value)) + ")"
}
