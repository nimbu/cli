package migrate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

type shippingRateSkipObserver struct {
	skips    map[string]string
	warnings map[string][]string
}

func (o *shippingRateSkipObserver) StageStart(string)                      {}
func (o *shippingRateSkipObserver) StageItem(string, string, int64, int64) {}
func (o *shippingRateSkipObserver) StageDone(string, string)               {}
func (o *shippingRateSkipObserver) SubStageDone(string, string, string)    {}
func (o *shippingRateSkipObserver) StageSkip(stage, reason string)         { o.skips[stage] = reason }
func (o *shippingRateSkipObserver) StageWarning(stage, warning string) {
	o.warnings[stage] = append(o.warnings[stage], warning)
}

func TestInspectShippingRatesForSiteCopyReportsCountWithoutTargetWrite(t *testing.T) {
	targetRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Nimbu-Site") == "target" {
			targetRequests++
			http.Error(w, "target must not be inspected or written", http.StatusInternalServerError)
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/shipping_rates" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"id":"one"},{"id":"two"}]`))
	}))
	defer server.Close()

	observer := &shippingRateSkipObserver{skips: map[string]string{}, warnings: map[string][]string{}}
	ctx := WithCopyObserver(context.Background(), observer)
	result := SiteCopyResult{}
	fromClient := api.New(server.URL, "token").WithSite("source")

	inspectShippingRatesForSiteCopy(ctx, fromClient, &result)

	if targetRequests != 0 {
		t.Fatalf("target requests = %d, want 0", targetRequests)
	}
	if result.ShippingRates.Skipped != 2 {
		t.Fatalf("skipped = %d, want 2", result.ShippingRates.Skipped)
	}
	reason := observer.skips["Shipping Rates"]
	if !strings.Contains(reason, "2") || !strings.Contains(reason, "region") || !strings.Contains(reason, "site-specific") {
		t.Fatalf("skip reason = %q", reason)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "not copied") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
	if len(observer.warnings["Shipping Rates"]) != 1 {
		t.Fatalf("observer warnings = %#v", observer.warnings)
	}
}

func TestInspectShippingRatesForSiteCopySkipsQuietlyWhenEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	observer := &shippingRateSkipObserver{skips: map[string]string{}, warnings: map[string][]string{}}
	ctx := WithCopyObserver(context.Background(), observer)
	result := SiteCopyResult{}
	inspectShippingRatesForSiteCopy(ctx, api.New(server.URL, "token").WithSite("source"), &result)

	if len(result.Warnings) != 0 || len(observer.warnings) != 0 {
		t.Fatalf("unexpected warnings: result=%#v observer=%#v", result.Warnings, observer.warnings)
	}
	if reason := observer.skips["Shipping Rates"]; reason == "" {
		t.Fatal("expected non-warning skipped timeline stage")
	}
}

func TestInspectShippingRatesForSiteCopyWarnsAndContinuesOnInspectionFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	observer := &shippingRateSkipObserver{skips: map[string]string{}, warnings: map[string][]string{}}
	ctx := WithCopyObserver(context.Background(), observer)
	result := SiteCopyResult{}
	inspectShippingRatesForSiteCopy(ctx, api.New(server.URL, "token").WithSite("source"), &result)

	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "inspect") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
	if len(observer.warnings["Shipping Rates"]) != 1 {
		t.Fatalf("observer warnings = %#v", observer.warnings)
	}
	if reason := observer.skips["Shipping Rates"]; !strings.Contains(reason, "inspection failed") {
		t.Fatalf("skip reason = %q", reason)
	}
}
