package api

import "testing"

func TestParseErrorCode(t *testing.T) {
	t.Run("string code", func(t *testing.T) {
		err := parseError(400, []byte(`{"code":"210","message":"two factor required"}`))
		if err.Code != "210" {
			t.Fatalf("expected code 210, got %q", err.Code)
		}
	})

	t.Run("numeric code", func(t *testing.T) {
		err := parseError(400, []byte(`{"code":210,"message":"two factor required"}`))
		if err.Code != "210" {
			t.Fatalf("expected code 210, got %q", err.Code)
		}
	})
}
