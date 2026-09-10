package pagepath

import (
	"errors"
	"testing"
)

func sampleSiblings() []RepeatableRef {
	return []RepeatableRef{
		{ID: "000000000000000000000001", Slug: "hero_stage", Index: 0},
		{ID: "000000000000000000000002", Slug: "proof_strip", Index: 1},
		{ID: "000000000000000000000003", Slug: "case_cards", Index: 2},
		{ID: "000000000000000000000004", Slug: "case_cards", Index: 3},
		{ID: "000000000000000000000005", Slug: "case_cards", Index: 4},
	}
}

func TestAnchorForInsert(t *testing.T) {
	siblings := sampleSiblings()
	tests := []struct {
		name     string
		position int
		want     Anchor
	}{
		{name: "beginning is explicit null", position: 0, want: Anchor{Mode: AnchorNull}},
		{name: "index 1 after first", position: 1, want: Anchor{Mode: AnchorID, ID: "000000000000000000000001"}},
		{name: "index 5 after last", position: 5, want: Anchor{Mode: AnchorID, ID: "000000000000000000000005"}},
		{name: "beyond end omits key", position: 6, want: Anchor{Mode: AnchorOmit}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AnchorForInsert(siblings, tt.position)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestAnchorForInsertEmptyCanvas(t *testing.T) {
	got, err := AnchorForInsert(nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != (Anchor{Mode: AnchorNull}) {
		t.Fatalf("empty canvas position 0 = %#v", got)
	}
	got, err = AnchorForInsert(nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got != (Anchor{Mode: AnchorOmit}) {
		t.Fatalf("empty canvas position 1 = %#v", got)
	}
}

func TestAnchorForInsertRejectsNegative(t *testing.T) {
	_, err := AnchorForInsert(sampleSiblings(), -1)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAnchorForMove(t *testing.T) {
	siblings := sampleSiblings()
	tests := []struct {
		name          string
		current, dest int
		want          Anchor
		noop          bool
	}{
		{name: "same index is noop", current: 2, dest: 2, noop: true},
		{name: "to beginning", current: 2, dest: 0, want: Anchor{Mode: AnchorNull}},
		{name: "move later uses sibling at target", current: 2, dest: 4, want: Anchor{Mode: AnchorID, ID: "000000000000000000000005"}},
		{name: "move later by one", current: 2, dest: 3, want: Anchor{Mode: AnchorID, ID: "000000000000000000000004"}},
		{name: "move earlier uses sibling before target", current: 4, dest: 1, want: Anchor{Mode: AnchorID, ID: "000000000000000000000001"}},
		{name: "move first later", current: 0, dest: 2, want: Anchor{Mode: AnchorID, ID: "000000000000000000000003"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AnchorForMove(siblings, tt.current, tt.dest)
			if tt.noop {
				if !errors.Is(err, ErrAnchorNoop) {
					t.Fatalf("err = %v, want ErrAnchorNoop", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseAnchor(t *testing.T) {
	siblings := sampleSiblings()
	tests := []struct {
		name    string
		input   string
		wantID  string
		wantErr string
	}{
		{name: "hex id", input: "000000000000000000000002", wantID: "000000000000000000000002"},
		{name: "index", input: "3", wantID: "000000000000000000000004"},
		{
			name:    "unknown id lists candidates",
			input:   "abc",
			wantErr: "path \"abc\": repeatable id \"abc\" not found; repeatables: [0] hero_stage 000000000000000000000001, [1] proof_strip 000000000000000000000002, [2] case_cards 000000000000000000000003, [3] case_cards 000000000000000000000004, [4] case_cards 000000000000000000000005",
		},
		{
			name:    "unknown 24-hex lists candidates",
			input:   "ffffffffffffffffffffffff",
			wantErr: "path \"ffffffffffffffffffffffff\": repeatable id \"ffffffffffffffffffffffff\" not found; repeatables: [0] hero_stage 000000000000000000000001, [1] proof_strip 000000000000000000000002, [2] case_cards 000000000000000000000003, [3] case_cards 000000000000000000000004, [4] case_cards 000000000000000000000005",
		},
		{
			name:    "index out of range",
			input:   "9",
			wantErr: "path \"9\": index 9 out of range; repeatables: [0] hero_stage 000000000000000000000001, [1] proof_strip 000000000000000000000002, [2] case_cards 000000000000000000000003, [3] case_cards 000000000000000000000004, [4] case_cards 000000000000000000000005",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := ParseAnchor(tt.input, siblings)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error")
				}
				var rerr *ResolveError
				if !errors.As(err, &rerr) {
					t.Fatalf("error type %T, want *ResolveError", err)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
				}
				if len(rerr.Candidates) != 5 {
					t.Fatalf("candidates = %v", rerr.Candidates)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if id != tt.wantID {
				t.Fatalf("id = %q, want %q", id, tt.wantID)
			}
		})
	}
}
