package pagepath

import "net/url"

// EscapeName encodes an editable name for a raw API path.
// It uses url.PathEscape (%20 for spaces, never +) so Rails CGI.unescape
// round-trips names including spaces and en-dashes.
func EscapeName(name string) string {
	return url.PathEscape(name)
}

// UnescapeName decodes a raw-path name segment.
func UnescapeName(name string) (string, error) {
	return url.PathUnescape(name)
}
