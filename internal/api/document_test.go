package api

import (
	"encoding/json"
	"testing"
)

func TestDocumentPreservesOriginalJSONWhileExposingTypedValue(t *testing.T) {
	input := []byte(`{"id":"p1","name":"Shirt","custom":{"color":"blue"},"variants":[{"sku":"S"}]}`)

	var document Document[Product]
	if err := json.Unmarshal(input, &document); err != nil {
		t.Fatalf("unmarshal document: %v", err)
	}

	if document.Value.ID != "p1" || document.Value.Name != "Shirt" {
		t.Fatalf("typed value = %#v", document.Value)
	}

	output, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal document: %v", err)
	}

	var got, want any
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if err := json.Unmarshal(input, &want); err != nil {
		t.Fatalf("decode input: %v", err)
	}
	if !JSONEqual(got, want) {
		t.Fatalf("round trip lost data:\n got: %s\nwant: %s", output, input)
	}
}

func JSONEqual(a, b any) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(left) == string(right)
}
