package cmd

import (
	"reflect"
	"testing"
)

func TestListRequestedFields(t *testing.T) {
	tests := []struct {
		name  string
		flags *QueryFlags
		want  []string
	}{
		{name: "nil flags", flags: nil, want: nil},
		{name: "empty", flags: &QueryFlags{}, want: []string{}},
		{name: "trim and drop empty", flags: &QueryFlags{Fields: " id, slug , ,name ,,"}, want: []string{"id", "slug", "name"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := listRequestedFields(tc.flags)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("listRequestedFields() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestListOutputColumns(t *testing.T) {
	defaults := []string{"id", "name"}
	headers := []string{"ID", "NAME"}

	t.Run("defaults", func(t *testing.T) {
		gotFields, gotHeaders := listOutputColumns(&QueryFlags{}, defaults, headers)
		if !reflect.DeepEqual(gotFields, defaults) {
			t.Fatalf("fields = %v, want %v", gotFields, defaults)
		}
		if !reflect.DeepEqual(gotHeaders, headers) {
			t.Fatalf("headers = %v, want %v", gotHeaders, headers)
		}
	})

	t.Run("custom fields", func(t *testing.T) {
		gotFields, gotHeaders := listOutputColumns(&QueryFlags{Fields: "id,created_at,published"}, defaults, headers)
		wantFields := []string{"id", "created_at", "published"}
		wantHeaders := []string{"ID", "CREATED AT", "PUBLISHED"}
		if !reflect.DeepEqual(gotFields, wantFields) {
			t.Fatalf("fields = %v, want %v", gotFields, wantFields)
		}
		if !reflect.DeepEqual(gotHeaders, wantHeaders) {
			t.Fatalf("headers = %v, want %v", gotHeaders, wantHeaders)
		}
	})
}
