package pagepath

import "fmt"

// ParseError is a structured failure while parsing a human or raw page path.
type ParseError struct {
	Input   string
	Message string
}

func (e *ParseError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("path %q: %s", e.Input, e.Message)
}

// ResolveError kinds used in CLI JSON details.
const (
	ResolveKindIndex    = "index_out_of_range"
	ResolveKindEditable = "editable_not_found"
	ResolveKindPage     = "page_editable_not_found"
	ResolveKindSlug     = "ambiguous_slug"
	ResolveKindID       = "repeatable_id_not_found"
	ResolveKindDepth    = "depth_exceeded"
)

// ResolveError lists candidates so an agent can correct a path in one step.
type ResolveError struct {
	Path       string
	Kind       string
	Candidates []string
	msg        string
}

func (e *ResolveError) Error() string {
	if e == nil {
		return ""
	}
	if e.msg != "" {
		return e.msg
	}
	return fmt.Sprintf("path %q: resolve failed", e.Path)
}

func parseErrorf(input, format string, args ...any) *ParseError {
	return &ParseError{Input: input, Message: fmt.Sprintf(format, args...)}
}
