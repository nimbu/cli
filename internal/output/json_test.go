package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestJSONWritesValidIndentedJSON(t *testing.T) {
	var out, errOut bytes.Buffer
	ctx := testContextWithMode(&out, &errOut, Mode{})

	data := map[string]any{"name": "Widget", "count": 42}
	if err := JSON(ctx, data); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\nraw: %s", err, out.String())
	}
	if got["name"] != "Widget" {
		t.Errorf("expected name=Widget, got %v", got["name"])
	}
	if got["count"] != float64(42) {
		t.Errorf("expected count=42, got %v", got["count"])
	}
}

func TestJSONErrWritesToStderr(t *testing.T) {
	var out, errOut bytes.Buffer
	ctx := testContextWithMode(&out, &errOut, Mode{})

	if err := JSONErr(ctx, map[string]string{"error": "boom"}); err != nil {
		t.Fatal(err)
	}

	if out.Len() != 0 {
		t.Errorf("expected nothing on stdout, got %q", out.String())
	}

	var got map[string]string
	if err := json.Unmarshal(errOut.Bytes(), &got); err != nil {
		t.Fatalf("stderr is not valid JSON: %v", err)
	}
	if got["error"] != "boom" {
		t.Errorf("expected error=boom, got %v", got["error"])
	}
}

func TestSuccessPayload(t *testing.T) {
	p := SuccessPayload("done")
	if p["status"] != "success" {
		t.Errorf("expected status=success, got %v", p["status"])
	}
	if p["message"] != "done" {
		t.Errorf("expected message=done, got %v", p["message"])
	}
}

func TestErrorPayload(t *testing.T) {
	p := ErrorPayload(errors.New("fail"))
	if p["status"] != "error" {
		t.Errorf("expected status=error, got %v", p["status"])
	}
	if p["error"] != "fail" {
		t.Errorf("expected error=fail, got %v", p["error"])
	}
}

func TestCountPayload(t *testing.T) {
	p := CountPayload(7)
	if p["count"] != 7 {
		t.Errorf("expected count=7, got %v", p["count"])
	}
}

func TestIDPayload(t *testing.T) {
	p := IDPayload("abc-123")
	if p["id"] != "abc-123" {
		t.Errorf("expected id=abc-123, got %v", p["id"])
	}
}

func TestPathPayload(t *testing.T) {
	p := PathPayload("/tmp/out")
	if p["path"] != "/tmp/out" {
		t.Errorf("expected path=/tmp/out, got %v", p["path"])
	}
}

func TestStatusPayloadOmitsEmptyMessage(t *testing.T) {
	p := StatusPayload("ok", "")
	if _, exists := p["message"]; exists {
		t.Errorf("expected no message key when empty, got %v", p["message"])
	}
}
