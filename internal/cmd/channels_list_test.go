package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestFetchChannelEntryCounts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/channels/news/entries/count":
			_, _ = w.Write([]byte(`{"count":12}`))
		case "/channels/abc123/entries/count":
			_, _ = w.Write([]byte(`{"count":5}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	channels := []api.Channel{
		{ID: "1", Slug: "news"},
		{ID: "abc123"},
	}

	fetchChannelEntryCounts(context.Background(), api.New(srv.URL, ""), channels)

	if channels[0].EntryCount == nil || *channels[0].EntryCount != 12 {
		t.Fatalf("unexpected first channel count: %#v", channels[0].EntryCount)
	}
	if channels[1].EntryCount == nil || *channels[1].EntryCount != 5 {
		t.Fatalf("unexpected second channel count: %#v", channels[1].EntryCount)
	}
}

func TestFetchChannelEntryCountsSkipsFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	channels := []api.Channel{{ID: "1", Slug: "news"}}
	fetchChannelEntryCounts(context.Background(), api.New(srv.URL, ""), channels)

	if channels[0].EntryCount != nil {
		t.Fatalf("expected nil entry count on failure, got %#v", channels[0].EntryCount)
	}
}
