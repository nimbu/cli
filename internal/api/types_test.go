package api

import (
	"encoding/json"
	"testing"
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
		t.Fatalf("expected url from source, got %q", u.URL)
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
