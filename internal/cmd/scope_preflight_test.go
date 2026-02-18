package cmd

import "testing"

func TestScopeSatisfied(t *testing.T) {
	available := map[string]struct{}{
		"write_products": {},
		"read_orders":    {},
	}

	if !scopeSatisfied("read_products", available) {
		t.Fatal("write scope should satisfy read scope")
	}
	if !scopeSatisfied("read_orders", available) {
		t.Fatal("exact read scope should satisfy")
	}
	if scopeSatisfied("read_customers", available) {
		t.Fatal("unexpected satisfied scope")
	}
}
