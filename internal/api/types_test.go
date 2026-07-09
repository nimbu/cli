package api

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCustomerUnmarshalLegacyNameFields(t *testing.T) {
	var c Customer
	err := json.Unmarshal([]byte(`{"id":"1","email":"a@b.com","firstname":"Ada","lastname":"Lovelace"}`), &c)
	if err != nil {
		t.Fatalf("unmarshal customer: %v", err)
	}

	if c.FirstName != "Ada" || c.LastName != "Lovelace" {
		t.Fatalf("unexpected names: %+v", c)
	}
}

func TestCustomerUnmarshalTimestamps(t *testing.T) {
	var c Customer
	err := json.Unmarshal([]byte(`{
		"id":"1",
		"email":"a@b.com",
		"firstname":"Ada",
		"lastname":"Lovelace",
		"created_at":"2023-01-02T03:04:05Z",
		"updated_at":"2024-05-06T07:08:09Z"
	}`), &c)
	if err != nil {
		t.Fatalf("unmarshal customer: %v", err)
	}

	if c.CreatedAt == nil || !c.CreatedAt.Equal(time.Date(2023, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("expected created_at to decode, got %v", c.CreatedAt)
	}
	if c.UpdatedAt == nil || !c.UpdatedAt.Equal(time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)) {
		t.Fatalf("expected updated_at to decode, got %v", c.UpdatedAt)
	}
}

func TestMenuUnmarshalSlugFallbackToHandle(t *testing.T) {
	var m Menu
	err := json.Unmarshal([]byte(`{"id":"1","name":"main","slug":"main"}`), &m)
	if err != nil {
		t.Fatalf("unmarshal menu: %v", err)
	}

	if m.Handle != "main" {
		t.Fatalf("expected handle from slug, got %q", m.Handle)
	}
}

func TestOrderUnmarshalStateTotalsAndCustomer(t *testing.T) {
	var o Order
	err := json.Unmarshal([]byte(`{
		"id":"1",
		"state":"completed",
		"currency":"EUR",
		"totals":{"total":15.5},
		"customer":{"id":"cust-1"}
	}`), &o)
	if err != nil {
		t.Fatalf("unmarshal order: %v", err)
	}

	if o.Status != "completed" {
		t.Fatalf("expected status from state, got %q", o.Status)
	}
	if o.Total != 15.5 {
		t.Fatalf("expected total from totals.total, got %v", o.Total)
	}
	if o.CustomerID != "cust-1" {
		t.Fatalf("expected customer id from nested customer, got %q", o.CustomerID)
	}
}

func TestOrderUnmarshalTimestamps(t *testing.T) {
	var o Order
	err := json.Unmarshal([]byte(`{
		"id":"1",
		"state":"completed",
		"currency":"EUR",
		"totals":{"total":15.5},
		"customer":{"id":"cust-1"},
		"created_at":"2023-01-02T03:04:05Z",
		"updated_at":"2024-05-06T07:08:09Z"
	}`), &o)
	if err != nil {
		t.Fatalf("unmarshal order: %v", err)
	}

	if o.CreatedAt == nil || !o.CreatedAt.Equal(time.Date(2023, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("expected created_at to decode, got %v", o.CreatedAt)
	}
	if o.UpdatedAt == nil || !o.UpdatedAt.Equal(time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)) {
		t.Fatalf("expected updated_at to decode, got %v", o.UpdatedAt)
	}
}

func TestChannelUnmarshalTrimNameAndMissingCount(t *testing.T) {
	var c Channel
	err := json.Unmarshal([]byte(`{"id":"1","slug":"x","name":"  X  "}`), &c)
	if err != nil {
		t.Fatalf("unmarshal channel: %v", err)
	}

	if c.Name != "X" {
		t.Fatalf("expected trimmed name, got %q", c.Name)
	}
	if c.EntryCount != nil {
		t.Fatal("expected nil entry count when field missing")
	}
}

func TestUploadUnmarshalNestedSourceMetadata(t *testing.T) {
	var u Upload
	err := json.Unmarshal([]byte(`{
		"id":"u1",
		"url":"https://api.example.test/uploads/u1",
		"source":{
			"filename":"hero.jpg",
			"url":"https://cdn.example.test/hero.jpg",
			"content_type":"image/jpeg",
			"size":42
		}
	}`), &u)
	if err != nil {
		t.Fatalf("unmarshal upload: %v", err)
	}

	if u.Name != "hero.jpg" {
		t.Fatalf("expected filename from source, got %q", u.Name)
	}
	if u.URL != "https://cdn.example.test/hero.jpg" {
		t.Fatalf("expected source url to override api url, got %q", u.URL)
	}
	if u.MimeType != "image/jpeg" {
		t.Fatalf("expected mime type from source, got %q", u.MimeType)
	}
	if u.Size != 42 {
		t.Fatalf("expected size from source, got %d", u.Size)
	}
}

func TestWebhookUnmarshalTargetURLFallback(t *testing.T) {
	var w Webhook
	err := json.Unmarshal([]byte(`{"id":"w1","target_url":"https://hooks.example.test","events":["order.created"]}`), &w)
	if err != nil {
		t.Fatalf("unmarshal webhook: %v", err)
	}

	if w.URL != "https://hooks.example.test" {
		t.Fatalf("expected url from target_url, got %q", w.URL)
	}
}

func TestThemeUnmarshalCDNRoot(t *testing.T) {
	var theme Theme
	err := json.Unmarshal([]byte(`{
		"id":"theme-1",
		"name":"Storefront",
		"cdn_base_path":"s/acme/themes/storefront/",
		"cdn_host":"https://cdn.example.test",
		"cdn_root":"https://cdn.example.test/s/acme/themes/storefront/",
		"site_id":"site-1",
		"site_short_id":"acme",
		"theme_short_id":"storefront"
	}`), &theme)
	if err != nil {
		t.Fatalf("unmarshal theme: %v", err)
	}

	if theme.CDNRoot != "https://cdn.example.test/s/acme/themes/storefront/" {
		t.Fatalf("expected cdn_root, got %q", theme.CDNRoot)
	}
	if theme.CDNHost != "https://cdn.example.test" || theme.CDNBasePath != "s/acme/themes/storefront/" {
		t.Fatalf("expected host/base path, got %+v", theme)
	}
	if theme.SiteID != "site-1" || theme.SiteShortID != "acme" || theme.ThemeShortID != "storefront" {
		t.Fatalf("expected site/theme ids, got %+v", theme)
	}
}

func TestProductUnmarshalCurrentAPIFields(t *testing.T) {
	var p Product
	err := json.Unmarshal([]byte(`{
		"id":"p1",
		"slug":"coffee",
		"name":"Coffee",
		"status":"active",
		"price":9.5,
		"current_stock":12,
		"digital":false,
		"requires_shipping":true,
		"on_sale":true,
		"on_sale_price":7.5
	}`), &p)
	if err != nil {
		t.Fatalf("unmarshal product: %v", err)
	}

	if p.Status != "active" {
		t.Fatalf("expected status, got %q", p.Status)
	}
	if p.CurrentStock != 12 {
		t.Fatalf("expected current stock, got %d", p.CurrentStock)
	}
	if !p.RequiresShipping {
		t.Fatal("expected requires shipping")
	}
	if !p.OnSale || p.OnSalePrice != 7.5 {
		t.Fatalf("expected on sale price, got %+v", p)
	}
}

func TestProductMarshalOmitsAbsentTimestamps(t *testing.T) {
	// A product parsed from a projected response without timestamps must not
	// emit zero-value created_at/updated_at keys (omitempty is a no-op for
	// value time.Time, so these fields are pointers).
	var p Product
	if err := json.Unmarshal([]byte(`{"id":"1","name":"Widget","slug":"widget"}`), &p); err != nil {
		t.Fatalf("unmarshal product: %v", err)
	}
	if p.CreatedAt != nil || p.UpdatedAt != nil {
		t.Fatalf("expected nil timestamps, got created=%v updated=%v", p.CreatedAt, p.UpdatedAt)
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal product: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal marshalled product: %v", err)
	}
	if _, ok := m["created_at"]; ok {
		t.Fatalf("expected created_at to be omitted, got %s", data)
	}
	if _, ok := m["updated_at"]; ok {
		t.Fatalf("expected updated_at to be omitted, got %s", data)
	}
}

func TestProductTimestampsRoundTrip(t *testing.T) {
	const created = "2023-01-02T03:04:05Z"
	const updated = "2024-05-06T07:08:09Z"
	var p Product
	if err := json.Unmarshal([]byte(`{"id":"1","name":"Widget","created_at":"`+created+`","updated_at":"`+updated+`"}`), &p); err != nil {
		t.Fatalf("unmarshal product: %v", err)
	}
	if p.CreatedAt == nil || p.UpdatedAt == nil {
		t.Fatalf("expected non-nil timestamps, got created=%v updated=%v", p.CreatedAt, p.UpdatedAt)
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal product: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal marshalled product: %v", err)
	}
	if m["created_at"] != created {
		t.Fatalf("expected created_at %q, got %v", created, m["created_at"])
	}
	if m["updated_at"] != updated {
		t.Fatalf("expected updated_at %q, got %v", updated, m["updated_at"])
	}
}

func TestEntryMarshalOmitsNilTimestamps(t *testing.T) {
	e := Entry{ID: "1", Title: "Hello"}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal marshalled entry: %v", err)
	}
	if _, ok := m["created_at"]; ok {
		t.Fatalf("expected created_at to be omitted, got %s", data)
	}
	if _, ok := m["updated_at"]; ok {
		t.Fatalf("expected updated_at to be omitted, got %s", data)
	}
}

func TestEntryMarshalIncludesPresentTimestamps(t *testing.T) {
	created := time.Date(2023, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	e := Entry{ID: "1", Title: "Hello", CreatedAt: &created, UpdatedAt: &updated}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal marshalled entry: %v", err)
	}
	if m["created_at"] != "2023-01-02T03:04:05Z" {
		t.Fatalf("expected created_at to be emitted, got %v", m["created_at"])
	}
	if m["updated_at"] != "2024-05-06T07:08:09Z" {
		t.Fatalf("expected updated_at to be emitted, got %v", m["updated_at"])
	}
}

func TestEntryMarshalMergesNonCollidingExtraFields(t *testing.T) {
	e := Entry{
		ID:    "1",
		Title: "Hello",
		Extra: map[string]any{
			"custom_field": "custom-value",
			"title":        "should not override struct field",
		},
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal marshalled entry: %v", err)
	}
	if m["custom_field"] != "custom-value" {
		t.Fatalf("expected extra custom_field to be merged, got %v", m["custom_field"])
	}
	// A struct field always wins over an Extra key of the same name, since it
	// is already present in `merged` before the Extra loop runs.
	if m["title"] != "Hello" {
		t.Fatalf("expected struct field to take precedence over Extra, got %v", m["title"])
	}
}

func TestEntryMarshalExtraCreatedAtLeaksWhenTimestampNil(t *testing.T) {
	// Current behavior (documented, not necessarily desired): when
	// e.CreatedAt is nil, omitempty drops "created_at" from the marshaled
	// alias entirely, so it is absent from `merged` by the time the Extra
	// loop runs. That makes the "already exists" check pass, and an Extra
	// key named "created_at" leaks into the output even though the struct's
	// own CreatedAt field is nil.
	e := Entry{
		ID: "1",
		Extra: map[string]any{
			"created_at": "2020-01-01T00:00:00Z",
		},
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal marshalled entry: %v", err)
	}
	if m["created_at"] != "2020-01-01T00:00:00Z" {
		t.Fatalf("expected Extra's created_at to leak through when struct field is nil, got %v", m["created_at"])
	}
}
