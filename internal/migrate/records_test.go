package migrate

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type failingEntropyReader struct{}

func (failingEntropyReader) Read(_ []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestDownloadBinaryRejectsOversizedContentLength(t *testing.T) {
	client := api.New("https://example.test", "")
	client.HTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				ContentLength: maxRecordAttachmentBytes + 1,
				Body:          io.NopCloser(strings.NewReader("x")),
				Header:        make(http.Header),
				Request:       req,
			}, nil
		}),
	}

	_, err := downloadBinary(context.Background(), client, "https://example.test/file.bin")
	if err == nil {
		t.Fatal("expected oversized attachment error")
	}
	if !strings.Contains(err.Error(), "attachment exceeds") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDownloadBinaryRejectsOversizedUnknownLength(t *testing.T) {
	body := bytes.NewReader(bytes.Repeat([]byte("a"), int(maxRecordAttachmentBytes)+1))
	client := api.New("https://example.test", "")
	client.HTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				ContentLength: -1,
				Body:          io.NopCloser(body),
				Header:        make(http.Header),
				Request:       req,
			}, nil
		}),
	}

	_, err := downloadBinary(context.Background(), client, "https://example.test/file.bin")
	if err == nil {
		t.Fatal("expected oversized attachment error")
	}
	if !strings.Contains(err.Error(), "attachment exceeds") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCopyCustomersFailsWhenPasswordEntropyFails(t *testing.T) {
	originalReader := passwordRandReader
	passwordRandReader = failingEntropyReader{}
	defer func() {
		passwordRandReader = originalReader
	}()

	var writes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/customers/customizations":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/customers" && r.Header.Get("X-Nimbu-Site") == "source":
			_, _ = w.Write([]byte(`[{"id":"cust_1","email":"hello@example.com"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/customers" && r.Header.Get("X-Nimbu-Site") == "target":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/customers":
			writes++
			http.Error(w, "unexpected write", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	fromClient := api.New(srv.URL, "").WithSite("source")
	toClient := api.New(srv.URL, "").WithSite("target")

	_, err := CopyCustomers(
		context.Background(),
		fromClient,
		toClient,
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		RecordCopyOptions{},
	)
	if err == nil {
		t.Fatal("expected password generation error")
	}
	if !strings.Contains(err.Error(), "generate customer password") {
		t.Fatalf("unexpected error: %v", err)
	}
	if writes != 0 {
		t.Fatalf("expected no customer writes, got %d", writes)
	}
}
