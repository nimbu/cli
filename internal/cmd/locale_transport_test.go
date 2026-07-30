package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nimbu/cli/internal/output"
)

func TestLocalizedContentCommandsUseContentLocale(t *testing.T) {
	const locale = "nl-BE"
	flags := &RootFlags{Site: "demo"}

	type wireCase struct {
		name     string
		path     string
		response string
		run      func(context.Context) error
	}

	cases := []wireCase{
		{
			name: "blogs list", path: "/blogs", response: `[]`,
			run: func(ctx context.Context) error {
				return (&BlogsListCmd{QueryFlags: QueryFlags{Locale: locale}, All: true}).Run(ctx, flags)
			},
		},
		{
			name: "blogs get", path: "/blogs/news", response: `{}`,
			run: func(ctx context.Context) error {
				return (&BlogsGetCmd{Blog: "news", Locale: locale}).Run(ctx, flags)
			},
		},
		{
			name: "blogs create", path: "/blogs", response: `{}`,
			run: func(ctx context.Context) error {
				return (&BlogsCreateCmd{Locale: locale, Assignments: []string{"name=News"}}).Run(ctx, flags)
			},
		},
		{
			name: "blogs update", path: "/blogs/news", response: `{}`,
			run: func(ctx context.Context) error {
				return (&BlogsUpdateCmd{Blog: "news", Locale: locale, Assignments: []string{"name=News"}}).Run(ctx, flags)
			},
		},
		{
			name: "blogs count", path: "/blogs/count", response: `{"count":1}`,
			run: func(ctx context.Context) error {
				return (&BlogsCountCmd{CountQueryFlags: CountQueryFlags{Locale: locale}}).Run(ctx, flags)
			},
		},
		{
			name: "blog posts list", path: "/blogs/news/articles", response: `[]`,
			run: func(ctx context.Context) error {
				return (&BlogPostsListCmd{QueryFlags: QueryFlags{Locale: locale}, Blog: "news", All: true}).Run(ctx, flags)
			},
		},
		{
			name: "blog posts get", path: "/blogs/news/articles/hello", response: `{}`,
			run: func(ctx context.Context) error {
				return (&BlogPostsGetCmd{Blog: "news", Post: "hello", Locale: locale}).Run(ctx, flags)
			},
		},
		{
			name: "blog posts create", path: "/blogs/news/articles", response: `{}`,
			run: func(ctx context.Context) error {
				return (&BlogPostsCreateCmd{Blog: "news", Locale: locale, Assignments: []string{"title=Hello"}}).Run(ctx, flags)
			},
		},
		{
			name: "blog posts update", path: "/blogs/news/articles/hello", response: `{}`,
			run: func(ctx context.Context) error {
				return (&BlogPostsUpdateCmd{Blog: "news", Post: "hello", Locale: locale, Assignments: []string{"title=Hello"}}).Run(ctx, flags)
			},
		},
		{
			name: "blog posts count", path: "/blogs/news/articles/count", response: `{"count":1}`,
			run: func(ctx context.Context) error {
				return (&BlogPostsCountCmd{CountQueryFlags: CountQueryFlags{Locale: locale}, Blog: "news"}).Run(ctx, flags)
			},
		},
		{
			name: "collections list", path: "/collections", response: `[]`,
			run: func(ctx context.Context) error {
				return (&CollectionsListCmd{QueryFlags: QueryFlags{Locale: locale}, All: true}).Run(ctx, flags)
			},
		},
		{
			name: "collections get", path: "/collections/summer", response: `{}`,
			run: func(ctx context.Context) error {
				return (&CollectionsGetCmd{Collection: "summer", Locale: locale}).Run(ctx, flags)
			},
		},
		{
			name: "collections create", path: "/collections", response: `{}`,
			run: func(ctx context.Context) error {
				return (&CollectionsCreateCmd{Locale: locale, Assignments: []string{"name=Summer"}}).Run(ctx, flags)
			},
		},
		{
			name: "collections update", path: "/collections/summer", response: `{}`,
			run: func(ctx context.Context) error {
				return (&CollectionsUpdateCmd{Collection: "summer", Locale: locale, Assignments: []string{"name=Summer"}}).Run(ctx, flags)
			},
		},
		{
			name: "collections count", path: "/collections/count", response: `{"count":1}`,
			run: func(ctx context.Context) error {
				return (&CollectionsCountCmd{CountQueryFlags: CountQueryFlags{Locale: locale}}).Run(ctx, flags)
			},
		},
		{
			name: "menus list", path: "/menus", response: `[]`,
			run: func(ctx context.Context) error {
				return (&MenusListCmd{QueryFlags: QueryFlags{Locale: locale}, All: true}).Run(ctx, flags)
			},
		},
		{
			name: "menus get", path: "/menus/main", response: `{"id":"menu","slug":"main","items":[{"children":[{}]}]}`,
			run: func(ctx context.Context) error {
				return (&MenusGetCmd{Menu: "main", Locale: locale}).Run(ctx, flags)
			},
		},
		{
			name: "menus create", path: "/menus", response: `{"id":"menu","slug":"main"}`,
			run: func(ctx context.Context) error {
				return (&MenusCreateCmd{Locale: locale, Assignments: []string{"name=Main"}}).Run(ctx, flags)
			},
		},
		{
			name: "menus update", path: "/menus/main", response: `{"id":"menu","slug":"main"}`,
			run: func(ctx context.Context) error {
				return (&MenusUpdateCmd{Menu: "main", Locale: locale, Assignments: []string{"name=Main"}}).Run(ctx, flags)
			},
		},
		{
			name: "menus count", path: "/menus/count", response: `{"count":1}`,
			run: func(ctx context.Context) error {
				return (&MenusCountCmd{CountQueryFlags: CountQueryFlags{Locale: locale}}).Run(ctx, flags)
			},
		},
		{
			name: "notifications list", path: "/notifications", response: `[]`,
			run: func(ctx context.Context) error {
				return (&NotificationsListCmd{QueryFlags: QueryFlags{Locale: locale}, All: true}).Run(ctx, flags)
			},
		},
		{
			name: "notifications get", path: "/notifications/welcome", response: `{}`,
			run: func(ctx context.Context) error {
				return (&NotificationsGetCmd{Notification: "welcome", Locale: locale}).Run(ctx, flags)
			},
		},
		{
			name: "notifications create", path: "/notifications", response: `{}`,
			run: func(ctx context.Context) error {
				return (&NotificationsCreateCmd{Locale: locale, Assignments: []string{"slug=welcome"}}).Run(ctx, flags)
			},
		},
		{
			name: "notifications update", path: "/notifications/welcome", response: `{}`,
			run: func(ctx context.Context) error {
				return (&NotificationsUpdateCmd{Notification: "welcome", Locale: locale, Assignments: []string{"subject=Hello"}}).Run(ctx, flags)
			},
		},
		{
			name: "notifications count", path: "/notifications/count", response: `{"count":1}`,
			run: func(ctx context.Context) error {
				return (&NotificationsCountCmd{CountQueryFlags: CountQueryFlags{Locale: locale}}).Run(ctx, flags)
			},
		},
		{
			name: "pages list", path: "/pages", response: `[]`,
			run: func(ctx context.Context) error {
				return (&PagesListCmd{QueryFlags: QueryFlags{Locale: locale}, All: true}).Run(ctx, flags)
			},
		},
		{
			name: "pages get", path: "/pages/home", response: `{"id":"page","fullpath":"home"}`,
			run: func(ctx context.Context) error {
				return (&PagesGetCmd{QueryFlags: QueryFlags{Locale: locale}, Page: "home"}).Run(ctx, flags)
			},
		},
		{
			name: "pages create", path: "/pages", response: `{"id":"page"}`,
			run: func(ctx context.Context) error {
				return (&PagesCreateCmd{Locale: locale, Assignments: []string{"title=Home"}}).Run(ctx, flags)
			},
		},
		{
			name: "pages update", path: "/pages/home", response: `{"id":"page","fullpath":"home","title":"Home"}`,
			run: func(ctx context.Context) error {
				return (&PagesUpdateCmd{QueryFlags: QueryFlags{Locale: locale}, Page: "home", Assignments: []string{"title=Home"}}).Run(ctx, flags)
			},
		},
		{
			name: "pages count", path: "/pages/count", response: `{"count":1}`,
			run: func(ctx context.Context) error {
				return (&PagesCountCmd{CountQueryFlags: CountQueryFlags{Locale: locale}}).Run(ctx, flags)
			},
		},
		{
			name: "products list", path: "/products", response: `[]`,
			run: func(ctx context.Context) error {
				return (&ProductsListCmd{QueryFlags: QueryFlags{Locale: locale}, All: true}).Run(ctx, flags)
			},
		},
		{
			name: "products get", path: "/products/wine", response: `{}`,
			run: func(ctx context.Context) error {
				return (&ProductsGetCmd{Product: "wine", Locale: locale}).Run(ctx, flags)
			},
		},
		{
			name: "products create", path: "/products", response: `{}`,
			run: func(ctx context.Context) error {
				return (&ProductsCreateCmd{Locale: locale, Assignments: []string{"name=Wine"}}).Run(ctx, flags)
			},
		},
		{
			name: "products update", path: "/products/wine", response: `{}`,
			run: func(ctx context.Context) error {
				return (&ProductsUpdateCmd{Product: "wine", Locale: locale, Assignments: []string{"name=Wine"}}).Run(ctx, flags)
			},
		},
		{
			name: "products count", path: "/products/count", response: `{"count":1}`,
			run: func(ctx context.Context) error {
				return (&ProductsCountCmd{CountQueryFlags: CountQueryFlags{Locale: locale}}).Run(ctx, flags)
			},
		},
		{
			name: "channel entries list", path: "/channels/news/entries", response: `[]`,
			run: func(ctx context.Context) error {
				return (&ChannelEntriesListCmd{QueryFlags: QueryFlags{Locale: locale}, Channel: "news", All: true}).Run(ctx, flags)
			},
		},
		{
			name: "channel entries get", path: "/channels/news/entries/hello", response: `{"id":"entry"}`,
			run: func(ctx context.Context) error {
				return (&ChannelEntriesGetCmd{QueryFlags: QueryFlags{Locale: locale}, Channel: "news", Entry: "hello"}).Run(ctx, flags)
			},
		},
		{
			name: "channel entries create", path: "/channels/news/entries", response: `{"id":"entry"}`,
			run: func(ctx context.Context) error {
				return (&ChannelEntriesCreateCmd{Channel: "news", Locale: locale, Assignments: []string{"title=Hello"}}).Run(ctx, flags)
			},
		},
		{
			name: "channel entries update", path: "/channels/news/entries/hello", response: `{"id":"entry"}`,
			run: func(ctx context.Context) error {
				return (&ChannelEntriesUpdateCmd{Channel: "news", Entry: "hello", Locale: locale, Assignments: []string{"title=Hello"}}).Run(ctx, flags)
			},
		},
		{
			name: "channel entries count", path: "/channels/news/entries/count", response: `{"count":1}`,
			run: func(ctx context.Context) error {
				return (&ChannelEntriesCountCmd{CountQueryFlags: CountQueryFlags{Locale: locale}, Channel: "news"}).Run(ctx, flags)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			targetRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/user" {
					_, _ = w.Write([]byte(`{}`))
					return
				}
				targetRequests++
				if r.URL.Path != tc.path {
					t.Errorf("request path = %q, want %q", r.URL.Path, tc.path)
				}
				if got := r.URL.Query().Get("content_locale"); got != locale {
					t.Errorf("content_locale = %q, want %q", got, locale)
				}
				if got := r.URL.Query().Get("locale"); got != "" {
					t.Errorf("legacy locale = %q, want empty", got)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()

			ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
			if err := tc.run(ctx); err != nil {
				t.Fatalf("run command: %v", err)
			}
			if targetRequests == 0 {
				t.Fatal("command made no target request")
			}
		})
	}
}

func TestLocalizedContentPaginatedListUsesContentLocaleOnPageAndCountProbe(t *testing.T) {
	const locale = "nl-BE"
	seen := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		if got := r.URL.Query().Get("content_locale"); got != locale {
			t.Errorf("%s content_locale = %q, want %q", r.URL.Path, got, locale)
		}
		if got := r.URL.Query().Get("locale"); got != "" {
			t.Errorf("%s legacy locale = %q, want empty", r.URL.Path, got)
		}
		seen[r.URL.Path] = true
		switch r.URL.Path {
		case "/pages":
			if r.URL.Query().Get("page") != "1" || r.URL.Query().Get("per_page") != "25" {
				t.Errorf("pagination query = %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[]`))
		case "/pages/count":
			_, _ = w.Write([]byte(`{"count":0}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	command := PagesListCmd{
		QueryFlags: QueryFlags{Locale: locale},
		Page:       1,
		PerPage:    25,
	}
	if err := command.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("list pages: %v", err)
	}
	for _, path := range []string{"/pages", "/pages/count"} {
		if !seen[path] {
			t.Errorf("missing request to %s", path)
		}
	}
}

func TestProductAttachmentsListKeepsLegacyLocaleParameter(t *testing.T) {
	var gotContentLocale string
	var gotLocale string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentLocale = r.URL.Query().Get("content_locale")
		gotLocale = r.URL.Query().Get("locale")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	command := ProductAttachmentsListCmd{
		QueryFlags: QueryFlags{Locale: "nl"},
		Product:    "wine",
	}
	if err := command.Run(ctx); err != nil {
		t.Fatalf("list product attachments: %v", err)
	}
	if gotLocale != "nl" || gotContentLocale != "" {
		t.Fatalf("locale=%q content_locale=%q", gotLocale, gotContentLocale)
	}
}

func TestSiteListKeepsLegacyLocaleParameter(t *testing.T) {
	const locale = "nl"
	flags := &RootFlags{Site: "demo"}
	cases := []struct {
		name string
		path string
		run  func(context.Context) error
	}{
		{
			name: "sites",
			path: "/sites",
			run: func(ctx context.Context) error {
				return (&SitesListCmd{QueryFlags: QueryFlags{Locale: locale}, All: true}).Run(ctx, flags)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("request path = %q, want %q", r.URL.Path, tc.path)
				}
				if got := r.URL.Query().Get("locale"); got != locale {
					t.Errorf("locale = %q, want %q", got, locale)
				}
				if got := r.URL.Query().Get("content_locale"); got != "" {
					t.Errorf("content_locale = %q, want empty", got)
				}
				_, _ = w.Write([]byte(`[]`))
			}))
			defer server.Close()

			ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
			if err := tc.run(ctx); err != nil {
				t.Fatalf("run command: %v", err)
			}
		})
	}
}

func TestTranslationsCountKeepsLegacyLocaleParameter(t *testing.T) {
	var gotContentLocale string
	var gotLocale string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentLocale = r.URL.Query().Get("content_locale")
		gotLocale = r.URL.Query().Get("locale")
		_, _ = w.Write([]byte(`{"count":1}`))
	}))
	defer server.Close()

	ctx, _, _ := newAdminWorkflowTestContext(t, server.URL, output.Mode{JSON: true})
	command := TranslationsCountCmd{
		CountQueryFlags: CountQueryFlags{Locale: "nl"},
	}
	if err := command.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("count translations: %v", err)
	}
	if gotLocale != "nl" || gotContentLocale != "" {
		t.Fatalf("locale=%q content_locale=%q", gotLocale, gotContentLocale)
	}
}

func TestChannelEntriesGalleryListUsesContentLocale(t *testing.T) {
	const locale = "nl-BE"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/channels/subjects/customizations":
			_, _ = w.Write([]byte(`[{"name":"photos","type":"gallery"}]`))
		case "/channels/subjects/entries/entry-1":
			if got := r.URL.Query().Get("content_locale"); got != locale {
				t.Errorf("content_locale = %q, want %q", got, locale)
			}
			if got := r.URL.Query().Get("locale"); got != "" {
				t.Errorf("legacy locale = %q, want empty", got)
			}
			_, _ = w.Write([]byte(`{"id":"entry-1","photos":{"images":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, _, _ := newGalleryTestContext(t, server.URL, output.Mode{JSON: true})
	command := ChannelEntriesGalleryListCmd{
		Channel: "subjects",
		Entry:   "entry-1",
		Field:   "photos",
		Locale:  locale,
	}
	if err := command.Run(ctx, &RootFlags{Site: "demo"}); err != nil {
		t.Fatalf("list gallery: %v", err)
	}
}
