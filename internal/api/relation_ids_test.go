package api

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestRelationIDsUnmarshalShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    RelationIDs
		wantErr bool
	}{
		{
			name:  "string array",
			input: `["a","b"]`,
			want:  RelationIDs{"a", "b"},
		},
		{
			name:  "empty array",
			input: `[]`,
			want:  RelationIDs{},
		},
		{
			name:  "null",
			input: `null`,
			want:  nil,
		},
		{
			name:  "reference objects",
			input: `[{"__type":"Reference","className":"customer","id":"c1"},{"__type":"Reference","className":"customer","id":"c2"}]`,
			want:  RelationIDs{"c1", "c2"},
		},
		{
			name:  "mixed strings and references",
			input: `["plain",{"id":"obj"}]`,
			want:  RelationIDs{"plain", "obj"},
		},
		{
			name:  "trimmed string ids",
			input: `[" a ", "", {"id":" b "}]`,
			want:  RelationIDs{"a", "b"},
		},
		{
			name:  "legacy objectId",
			input: `[{"objectId":"legacy"}]`,
			want:  RelationIDs{"legacy"},
		},
		{
			name:  "relation wrapper with objects",
			input: `{"__type":"Relation","className":"customer","objects":[{"__type":"Reference","className":"customer","id":"c1"}]}`,
			want:  RelationIDs{"c1"},
		},
		{
			name:  "relation wrapper without objects",
			input: `{"__type":"Relation","className":"customer"}`,
			want:  RelationIDs{},
		},
		{
			name:  "relation wrapper with empty objects",
			input: `{"__type":"Relation","className":"customer","objects":[]}`,
			want:  RelationIDs{},
		},
		{
			name:    "number is rejected",
			input:   `1`,
			wantErr: true,
		},
		{
			name:    "string is rejected",
			input:   `"c1"`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got RelationIDs
			err := json.Unmarshal([]byte(tc.input), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestRelationIDsMarshalJSONEmitsStringArray(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(RelationIDs{"a", "b"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != `["a","b"]` {
		t.Fatalf("got %s", data)
	}

	data, err = json.Marshal(RelationIDs{})
	if err != nil {
		t.Fatalf("marshal empty: %v", err)
	}
	if string(data) != `[]` {
		t.Fatalf("empty got %s", data)
	}
}

func TestRoleUnmarshalRelationAndPlainMembers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want RelationIDs
	}{
		{
			name: "plain string customers",
			body: `{"id":"bingo","name":"bingo","customers":["c1","c2"]}`,
			want: RelationIDs{"c1", "c2"},
		},
		{
			name: "relation customers",
			body: `{"id":"bingo","name":"bingo","customers":{"__type":"Relation","className":"customer","objects":[{"__type":"Reference","className":"customer","id":"c1"}]},"children":{"__type":"Relation","className":"role","objects":[{"id":"child"}]},"parents":["parent"]}`,
			want: RelationIDs{"c1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var role Role
			if err := json.Unmarshal([]byte(tc.body), &role); err != nil {
				t.Fatalf("unmarshal role: %v", err)
			}
			if !slices.Equal(role.Customers, tc.want) {
				t.Fatalf("customers = %#v, want %#v", role.Customers, tc.want)
			}
			if !role.CustomersExpanded {
				t.Fatal("expected customers to be expanded")
			}
			if tc.name == "relation customers" {
				if !slices.Equal(role.Children, RelationIDs{"child"}) {
					t.Fatalf("children = %#v", role.Children)
				}
				if !slices.Equal(role.Parents, RelationIDs{"parent"}) {
					t.Fatalf("parents = %#v", role.Parents)
				}
			}
		})
	}
}

func TestRoleUnmarshalMarksUnexpandedRelation(t *testing.T) {
	t.Parallel()

	var role Role
	err := json.Unmarshal([]byte(`{"id":"bingo","name":"bingo","customers":{"__type":"Relation","className":"customer"},"children":{"__type":"Relation","className":"role","objects":[]}}`), &role)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if role.CustomersExpanded {
		t.Fatal("pointer-only customers relation must not look expanded")
	}
	if len(role.Customers) != 0 {
		t.Fatalf("customers = %#v", role.Customers)
	}
	if !role.ChildrenExpanded {
		t.Fatal("children with objects:[] is expanded")
	}
}
