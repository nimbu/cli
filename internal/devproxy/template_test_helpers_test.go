package devproxy

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"io"
	"testing"
)

func decodeCompressedTemplatesForTest(t *testing.T, compressed string) map[string]map[string]string {
	t.Helper()

	data, err := base64.StdEncoding.DecodeString(compressed)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	zr, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("open zlib reader: %v", err)
	}
	defer func() { _ = zr.Close() }()

	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read zlib payload: %v", err)
	}

	var templates map[string]map[string]string
	if err := json.Unmarshal(raw, &templates); err != nil {
		t.Fatalf("unmarshal templates: %v", err)
	}
	return templates
}
