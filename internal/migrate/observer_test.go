package migrate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/observer"
)

type recordingCopyObserver struct {
	items []string
}

func (o *recordingCopyObserver) StageStart(string) {}
func (o *recordingCopyObserver) StageItem(stage, _ string, _, _ int64) {
	o.items = append(o.items, stage)
}
func (o *recordingCopyObserver) StageDone(string, string)            {}
func (o *recordingCopyObserver) StageSkip(string, string)            {}
func (o *recordingCopyObserver) SubStageDone(string, string, string) {}
func (o *recordingCopyObserver) StageWarning(string, string)         {}

func TestCopyCustomizationsUsesProvidedStageLabelForItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/customers/customizations" && r.Header.Get("X-Nimbu-Site") == "source":
			_, _ = w.Write([]byte(`[{"name":"vat_number","type":"string"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/customers/customizations":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	obs := &recordingCopyObserver{}
	ctx := observer.WithCopyObserver(context.Background(), obs)
	fromClient := api.New(srv.URL, "test-token").WithSite("source")
	toClient := api.New(srv.URL, "test-token").WithSite("target")

	_, err := CopyCustomizations(ctx, CustomizationService{Kind: CustomizationCustomers}, fromClient, toClient, SiteRef{Site: "source"}, SiteRef{Site: "target"}, true, "Customer Config")
	if err != nil {
		t.Fatalf("copy customizations: %v", err)
	}
	if len(obs.items) != 1 || obs.items[0] != "Customer Config" {
		t.Fatalf("expected item stage Customer Config, got %#v", obs.items)
	}
}

func TestCopyCustomersUsesCustomersStageForItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/customers/customizations":
			_, _ = w.Write([]byte(`[{"name":"email","type":"string"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/customers" && r.Header.Get("X-Nimbu-Site") == "source":
			_, _ = w.Write([]byte(`[{"id":"c1","email":"ada@example.test"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/customers":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	obs := &recordingCopyObserver{}
	ctx := observer.WithCopyObserver(context.Background(), obs)
	fromClient := api.New(srv.URL, "test-token").WithSite("source")
	toClient := api.New(srv.URL, "test-token").WithSite("target")

	_, err := CopyCustomers(ctx, fromClient, toClient, SiteRef{Site: "source"}, SiteRef{Site: "target"}, RecordCopyOptions{DryRun: true})
	if err != nil {
		t.Fatalf("copy customers: %v", err)
	}
	if len(obs.items) != 1 || obs.items[0] != "Customers" {
		t.Fatalf("expected item stage Customers, got %#v", obs.items)
	}
}
