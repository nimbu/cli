package migrate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestCopySiteConflictResolverCanSkipAllExistingTypes(t *testing.T) {
	var resolverCalls int
	var writes []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			writes = append(writes, r.Method+" "+r.URL.Path)
			http.Error(w, "unexpected write", http.StatusInternalServerError)
			return
		}

		site := r.Header.Get("X-Nimbu-Site")
		switch {
		case r.URL.Path == "/channels" && site == "source":
			_, _ = w.Write([]byte(`[{"id":"src-channel","slug":"articles","name":"Articles","customizations":[{"id":"src-title","name":"title","type":"string"}]}]`))
		case r.URL.Path == "/channels" && site == "target":
			_, _ = w.Write([]byte(`[{"id":"target-channel","slug":"articles","name":"Articles","customizations":[{"id":"target-title","name":"title","type":"string"}]}]`))
		case r.URL.Path == "/channels/articles" && site == "target":
			_, _ = w.Write([]byte(`{"id":"target-channel","slug":"articles","name":"Articles","customizations":[{"id":"target-title","name":"title","type":"string"}]}`))
		case r.URL.Path == "/uploads":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/customers/customizations" && site == "source":
			_, _ = w.Write([]byte(`[{"name":"vat_number","type":"string"}]`))
		case r.URL.Path == "/customers/customizations" && site == "target":
			_, _ = w.Write([]byte(`[{"name":"vat_number","type":"string"}]`))
		case r.URL.Path == "/products/customizations" && site == "source":
			_, _ = w.Write([]byte(`[{"name":"subtitle","type":"string"}]`))
		case r.URL.Path == "/products/customizations" && site == "target":
			_, _ = w.Write([]byte(`[{"name":"subtitle","type":"string"}]`))
		case r.URL.Path == "/roles":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/products":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/collections":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/themes":
			_, _ = w.Write([]byte(`[{"id":"t1","name":"Storefront","active":true}]`))
		case r.URL.Path == "/themes/t1":
			_, _ = w.Write([]byte(`{"assets":[],"layouts":[],"snippets":[],"templates":[]}`))
		case r.URL.Path == "/pages":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/menus" && site == "source":
			_, _ = w.Write([]byte(`[{"slug":"main","items":[{"title":"Home","url":"/"}]}]`))
		case r.URL.Path == "/menus" && site == "target":
			_, _ = w.Write([]byte(`[{"slug":"main","items":[{"title":"Existing","url":"/"}]}]`))
		case r.URL.Path == "/menus/main" && site == "target":
			_, _ = w.Write([]byte(`{"slug":"main","items":[{"title":"Existing","url":"/"}]}`))
		case r.URL.Path == "/blogs":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/notifications":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/redirects":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/translations":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	result, err := CopySite(
		context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		SiteCopyOptions{
			ConflictResolver: func(context.Context, ExistingContentPrompt) (ExistingContentDecision, error) {
				resolverCalls++
				return ExistingContentDecision{Action: ExistingContentSkip, ApplyToAll: true}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("copy site: %v", err)
	}
	if resolverCalls != 1 {
		t.Fatalf("expected one resolver call, got %d", resolverCalls)
	}
	if len(writes) != 0 {
		t.Fatalf("expected no writes after skip-all decision, got %#v", writes)
	}
	if got := result.Channels.Items[0].Action; got != "skip" {
		t.Fatalf("channel action = %q, want skip", got)
	}
	if got := result.CustomerConfig.Action; got != "skip" {
		t.Fatalf("customer config action = %q, want skip", got)
	}
	if got := result.ProductConfig.Action; got != "skip" {
		t.Fatalf("product config action = %q, want skip", got)
	}
	if got := result.Menus.Items[0].Action; got != "skip" {
		t.Fatalf("menu action = %q, want skip", got)
	}
}

func TestCopySiteConflictResolverCanReviewExistingChannels(t *testing.T) {
	var patched []string
	var unexpectedWrites []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("X-Nimbu-Site")
		if r.Method == http.MethodPatch {
			patched = append(patched, r.URL.Path)
			_, _ = w.Write([]byte(`{"id":"target-channel","slug":"articles"}`))
			return
		}
		if r.Method == http.MethodPost && (r.URL.Path == "/customers/customizations" || r.URL.Path == "/products/customizations") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			unexpectedWrites = append(unexpectedWrites, r.Method+" "+r.URL.Path)
			http.Error(w, "unexpected write", http.StatusInternalServerError)
			return
		}

		switch {
		case r.URL.Path == "/channels" && site == "source":
			_, _ = w.Write([]byte(`[
				{"id":"src-articles","slug":"articles","name":"Articles","customizations":[{"id":"src-title","name":"title","type":"string"}]},
				{"id":"src-events","slug":"events","name":"Events","customizations":[{"id":"src-title","name":"title","type":"string"}]}
			]`))
		case r.URL.Path == "/channels" && site == "target":
			_, _ = w.Write([]byte(`[
				{"id":"target-articles","slug":"articles","name":"Articles","customizations":[{"id":"target-title","name":"title","type":"string"}]},
				{"id":"target-events","slug":"events","name":"Events","customizations":[{"id":"target-title","name":"title","type":"string"}]}
			]`))
		case r.URL.Path == "/channels/articles" && site == "target":
			_, _ = w.Write([]byte(`{"id":"target-articles","slug":"articles","name":"Articles","customizations":[{"id":"target-title","name":"title","type":"string"}]}`))
		case r.URL.Path == "/channels/events" && site == "target":
			_, _ = w.Write([]byte(`{"id":"target-events","slug":"events","name":"Events","customizations":[{"id":"target-title","name":"title","type":"string"}]}`))
		case r.URL.Path == "/uploads":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/customers/customizations":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/products/customizations":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/roles":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/products":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/collections":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/themes":
			_, _ = w.Write([]byte(`[{"id":"t1","name":"Storefront","active":true}]`))
		case r.URL.Path == "/themes/t1":
			_, _ = w.Write([]byte(`{"assets":[],"layouts":[],"snippets":[],"templates":[]}`))
		case r.URL.Path == "/pages":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/menus":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/blogs":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/notifications":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/redirects":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/translations":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var prompts []ExistingContentPrompt
	result, err := CopySite(
		context.Background(),
		api.New(srv.URL, "").WithSite("source"),
		api.New(srv.URL, "").WithSite("target"),
		SiteRef{Site: "source"},
		SiteRef{Site: "target"},
		SiteCopyOptions{
			ConflictResolver: func(_ context.Context, prompt ExistingContentPrompt) (ExistingContentDecision, error) {
				prompts = append(prompts, prompt)
				switch prompt.Item {
				case "":
					return ExistingContentDecision{Action: ExistingContentReview}, nil
				case "articles":
					return ExistingContentDecision{Action: ExistingContentUpdate}, nil
				case "events":
					return ExistingContentDecision{Action: ExistingContentSkip}, nil
				default:
					t.Fatalf("unexpected prompt: %#v", prompt)
					return ExistingContentDecision{}, nil
				}
			},
		},
	)
	if err != nil {
		if len(unexpectedWrites) > 0 {
			t.Fatalf("copy site: %v; unexpected writes: %#v", err, unexpectedWrites)
		}
		t.Fatalf("copy site: %v", err)
	}
	if len(prompts) != 3 {
		t.Fatalf("expected type prompt plus two item prompts, got %#v", prompts)
	}
	if len(patched) != 1 || patched[0] != "/channels/articles" {
		t.Fatalf("patched = %#v, want only /channels/articles", patched)
	}
	actions := map[string]string{}
	for _, item := range result.Channels.Items {
		actions[item.Source] = item.Action
	}
	if actions["articles"] != "update" || actions["events"] != "skip" {
		t.Fatalf("channel actions = %#v", actions)
	}
}
