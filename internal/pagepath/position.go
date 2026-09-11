package pagepath

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrAnchorNoop is returned by AnchorForMove when the item is already at target.
var ErrAnchorNoop = errors.New("move is a no-op")

// Anchor modes distinguish omit / JSON null / id / no-op.
const (
	AnchorOmit = iota
	AnchorNull
	AnchorID
	AnchorNoop
)

// Anchor is a tri-state insert/move cursor.
type Anchor struct {
	Mode int
	ID   string
}

// RepeatableRef is one repeatable in position order.
type RepeatableRef struct {
	ID    string
	Slug  string
	Index int
}

// AnchorForInsert returns the batch `after` value for inserting at a 0-based index.
// 0 → explicit null (beginning); N → id of sibling N-1; N > len → omit (append).
func AnchorForInsert(siblings []RepeatableRef, position int) (Anchor, error) {
	if position < 0 {
		return Anchor{}, fmt.Errorf("negative insert position %d", position)
	}
	if position == 0 {
		return Anchor{Mode: AnchorNull}, nil
	}
	if position > len(siblings) {
		return Anchor{Mode: AnchorOmit}, nil
	}
	return Anchor{Mode: AnchorID, ID: siblings[position-1].ID}, nil
}

// AnchorForMove returns the batch `after` value for moving currentIndex to targetIndex.
func AnchorForMove(siblings []RepeatableRef, currentIndex, targetIndex int) (Anchor, error) {
	if currentIndex < 0 || currentIndex >= len(siblings) || targetIndex < 0 || targetIndex >= len(siblings) {
		return Anchor{}, fmt.Errorf("move index out of range")
	}
	if currentIndex == targetIndex {
		return Anchor{Mode: AnchorNoop}, ErrAnchorNoop
	}
	if targetIndex == 0 {
		return Anchor{Mode: AnchorNull}, nil
	}
	if targetIndex > currentIndex {
		return Anchor{Mode: AnchorID, ID: siblings[targetIndex].ID}, nil
	}
	return Anchor{Mode: AnchorID, ID: siblings[targetIndex-1].ID}, nil
}

// ParseAnchor resolves a 24-hex id or integer index to a sibling id.
func ParseAnchor(s string, siblings []RepeatableRef) (string, error) {
	s = strings.TrimSpace(s)
	if isHex24(s) {
		for _, sib := range siblings {
			if sib.ID == s {
				return s, nil
			}
		}
		return "", repeatableIDError(s, s, "", siblings)
	}
	if idx, err := strconv.Atoi(s); err == nil {
		if idx >= 0 && idx < len(siblings) {
			return siblings[idx].ID, nil
		}
		cands := formatRepeatables(siblings)
		return "", &ResolveError{
			Path:       s,
			Kind:       ResolveKindIndex,
			Candidates: cands,
			msg:        fmt.Sprintf("path %q: index %d out of range; repeatables: %s", s, idx, strings.Join(cands, ", ")),
		}
	}
	return "", repeatableIDError(s, s, "", siblings)
}

func isHex24(s string) bool {
	if len(s) != 24 {
		return false
	}
	for i := 0; i < 24; i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}
