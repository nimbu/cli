package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadURLDoesNotChangeExternalSignedURL(t *testing.T) {
	var receivedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		_, _ = w.Write([]byte("file"))
	}))
	defer server.Close()

	client := New("https://api.nimbu.test", "secret")
	response, resolved, err := client.DownloadURL(context.Background(), server.URL+"/asset?signature=abc&expires=123")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	_ = response.Body.Close()

	if receivedQuery != "signature=abc&expires=123" {
		t.Fatalf("signed query changed to %q", receivedQuery)
	}
	if resolved != server.URL+"/asset?signature=abc&expires=123" {
		t.Fatalf("resolved URL = %q", resolved)
	}
}

func TestDownloadURLDoesNotSendCredentialsAcrossSchemeDowngrade(t *testing.T) {
	var authorization, site, query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		site = r.Header.Get("X-Nimbu-Site")
		query = r.URL.RawQuery
		_, _ = w.Write([]byte("file"))
	}))
	defer server.Close()

	client := New("https://"+server.Listener.Addr().String(), "secret").WithSite("demo")
	response, _, err := client.DownloadURL(context.Background(), server.URL+"/asset?signature=abc")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	_ = response.Body.Close()

	if authorization != "" || site != "" {
		t.Fatalf("credentials leaked across scheme downgrade: authorization=%q site=%q", authorization, site)
	}
	if query != "signature=abc" {
		t.Fatalf("signed query changed to %q", query)
	}
}
