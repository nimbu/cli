// Package envutil reads process environment conventions shared across nimbu.
package envutil

import "strings"

// IsCI reports whether CI marks a CI runner: set to anything but an empty
// value, 0, false or no (case-insensitive, surrounding space ignored).
func IsCI(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv("CI"))) {
	case "", "0", "false", "no":
		return false
	default:
		return true
	}
}
