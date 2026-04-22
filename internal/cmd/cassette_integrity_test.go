package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNimbuAPICassettesAreSanitized(t *testing.T) {
	root := "../testdata/nimbu_api"
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatalf("decode cassette %s: %v", path, err)
		}
		assertCassetteSanitized(t, path, value)
		return nil
	})
	if err != nil {
		t.Fatalf("walk cassette dir: %v", err)
	}
}

func assertCassetteSanitized(t *testing.T, path string, value any) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if sensitiveCassetteKey(key) {
				if text, ok := nested.(string); ok && text != "" && text != "fixture-redacted" {
					t.Fatalf("cassette contains sensitive value at %s.%s", path, key)
				}
			}
			assertCassetteSanitized(t, path+"."+key, nested)
		}
	case []any:
		for _, nested := range typed {
			assertCassetteSanitized(t, path+"[]", nested)
		}
	case string:
		if len(typed) == 24 && isLowerHexCassetteValue(typed) {
			t.Fatalf("cassette contains Mongo-like ID at %s: %s", path, typed)
		}
	}
}

func sensitiveCassetteKey(key string) bool {
	key = strings.ToLower(key)
	for _, needle := range []string{"token", "secret", "password", "authorization"} {
		if strings.Contains(key, needle) {
			return true
		}
	}
	return false
}

func isLowerHexCassetteValue(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
