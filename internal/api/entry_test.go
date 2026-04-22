package api

import (
	"encoding/json"
	"testing"
)

func TestEntryJSONPreservesExtraAndOmitsZeroTimes(t *testing.T) {
	var entry Entry
	if err := json.Unmarshal([]byte(`{"id":"entry-1","slug":"start","subtitle":"Active Listening"}`), &entry); err != nil {
		t.Fatalf("unmarshal entry: %v", err)
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode marshaled entry: %v", err)
	}
	if got["subtitle"] != "Active Listening" {
		t.Fatalf("expected extra subtitle preserved, got %#v", got)
	}
	if _, ok := got["created_at"]; ok {
		t.Fatalf("did not expect zero created_at in JSON: %#v", got)
	}
	if _, ok := got["updated_at"]; ok {
		t.Fatalf("did not expect zero updated_at in JSON: %#v", got)
	}
}
