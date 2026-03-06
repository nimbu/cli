package devproxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractMetadataPreservesTrailingSlash(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/foo/?q=1", nil)

	meta, err := extractMetadata(req, "nimbu-go-cli", nil)
	if err != nil {
		t.Fatalf("extract metadata: %v", err)
	}
	if meta.path != "/foo/" {
		t.Fatalf("expected path /foo/, got %q", meta.path)
	}
}

func TestSplitHostPortIPv6(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.Host = "[2001:db8::1]"

	host, port := splitHostPort(req)
	if host != "2001:db8::1" {
		t.Fatalf("host mismatch: %q", host)
	}
	if port != 80 {
		t.Fatalf("port mismatch: %d", port)
	}
}

func TestSplitHostPortIPv6WithPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.Host = "[2001:db8::1]:8443"

	host, port := splitHostPort(req)
	if host != "2001:db8::1" {
		t.Fatalf("host mismatch: %q", host)
	}
	if port != 8443 {
		t.Fatalf("port mismatch: %d", port)
	}
}

func TestSplitHostPortIPv6TLSDefault(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	req.Host = "[2001:db8::1]"

	host, port := splitHostPort(req)
	if host != "2001:db8::1" {
		t.Fatalf("host mismatch: %q", host)
	}
	if port != 443 {
		t.Fatalf("port mismatch: %d", port)
	}
}
