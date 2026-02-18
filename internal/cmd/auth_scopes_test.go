package cmd

import "testing"

func TestParseScopesHeader(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty", raw: "", want: []string{}},
		{name: "comma separated", raw: "read_orders, write_products", want: []string{"read_orders", "write_products"}},
		{name: "space separated", raw: "read_orders write_products", want: []string{"read_orders", "write_products"}},
		{name: "dedupe and trim", raw: " read_orders,read_orders , write_products ", want: []string{"read_orders", "write_products"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseScopesHeader(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("len mismatch: got=%v want=%v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("scopes mismatch: got=%v want=%v", got, tt.want)
				}
			}
		})
	}
}
