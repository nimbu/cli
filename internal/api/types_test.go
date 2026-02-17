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
