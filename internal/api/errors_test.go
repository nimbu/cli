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

	t.Run("string symbolic code", func(t *testing.T) {
		err := parseError(412, []byte(`{"code":"precondition_failed","message":"stale","current_etag":"abc"}`))
		if err.Code != "precondition_failed" {
			t.Fatalf("code = %q", err.Code)
		}
		if err.Raw["current_etag"] != "abc" {
			t.Fatalf("Raw = %#v", err.Raw)
		}
		if err.CurrentETag() != "abc" {
			t.Fatalf("CurrentETag = %q", err.CurrentETag())
		}
	})
}
