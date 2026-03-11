package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestLoginWithCredentials(t *testing.T) {
	var gotAuth string
	var gotCode string
	var reqBody struct {
		Description string `json:"description"`
		ExpiresIn   int    `json:"expires_in"`
	}
	var requestCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}

		gotAuth = r.Header.Get("Authorization")
		gotCode = r.Header.Get("X-Nimbu-Two-Factor")

		if r.Header.Get("Accept") != "application/json" {
			t.Fatalf("expected Accept: application/json, got %s", r.Header.Get("Accept"))
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"tok","user":{"id":"u1","email":"me@example.com"}}`))
	}))
	defer srv.Close()

	client := api.New(srv.URL, "")
	host, err := os.Hostname()
	if err != nil {
		host = "nimbu"
	}

	resp, err := loginWithCredentials(context.Background(), client, "me@example.com", "sekret", 60, false, func(string) (string, error) {
		t.Fatalf("unexpected two-factor prompt")
		return "", nil
	})
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if resp.Token != "tok" {
		t.Fatalf("expected token tok, got %q", resp.Token)
	}
	if requestCount != 1 {
		t.Fatalf("expected 1 request, got %d", requestCount)
	}
	if gotAuth != "Basic "+base64.StdEncoding.EncodeToString([]byte("me@example.com:sekret")) {
		t.Fatalf("unexpected auth header: %s", gotAuth)
	}
	if reqBody.Description != "Nimbu login from "+host {
		t.Fatalf("unexpected description: %s", reqBody.Description)
	}
	if reqBody.ExpiresIn != 60 {
		t.Fatalf("expected expires_in 60, got %d", reqBody.ExpiresIn)
	}
	if gotCode != "" {
		t.Fatalf("did not expect two-factor header, got %q", gotCode)
	}
}

func TestLoginWithCredentialsPromptsAndRetriesTwoFactor(t *testing.T) {
	var gotCode string
	var requestCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		gotCode = r.Header.Get("X-Nimbu-Two-Factor")

		w.Header().Set("Content-Type", "application/json")

		if requestCount == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":210,"message":"two-factor required"}`))
			return
		}

		if gotCode != "123456" {
			t.Fatalf("expected two-factor header on retry, got %q", gotCode)
		}
		_, _ = w.Write([]byte(`{"token":"tok-2fa","user":{"id":"u1","email":"me@example.com"}}`))
	}))
	defer srv.Close()

	client := api.New(srv.URL, "")
	resp, err := loginWithCredentials(context.Background(), client, "me@example.com", "sekret", 120, false, func(string) (string, error) {
		return "123456", nil
	})
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if resp.Token != "tok-2fa" {
		t.Fatalf("expected token tok-2fa, got %q", resp.Token)
	}
	if requestCount != 2 {
		t.Fatalf("expected 2 requests, got %d", requestCount)
	}
}

func TestLoginWithCredentialsNoInputRejectsTwoFactor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":210,"message":"two-factor required"}`))
	}))
	defer srv.Close()

	client := api.New(srv.URL, "")
	_, err := loginWithCredentials(context.Background(), client, "me@example.com", "sekret", 120, true, func(string) (string, error) {
		t.Fatalf("should not prompt for code in no-input mode")
		return "", nil
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "two-factor code required with --no-input") {
		t.Fatalf("unexpected error: %v", err)
	}
}
