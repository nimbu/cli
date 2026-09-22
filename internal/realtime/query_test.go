package realtime

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateQueryAccepts(t *testing.T) {
	queries := []map[string]string{
		nil,
		{"_status": "published"},
		{"slug": "hello", "title.contains": "foo"},
		{"where": `{"and":[{"a":1}]}`},
		{"color.title": "red", "tags.in": "a,b"},
		{"title.ne": "x"},
		{"count.gte": "3", "count.lt": "10"},
		{"body.exists": "true", "name.start": "ab", "name.end": "yz", "name.matches": "a*"},
	}
	for _, query := range queries {
		if err := ValidateQuery(query); err != nil {
			t.Fatalf("ValidateQuery(%v) = %v, want nil", query, err)
		}
	}
}

func TestValidateQueryRejects(t *testing.T) {
	tests := []struct {
		name  string
		query map[string]string
		want  string
	}{
		{
			name:  "reserved key",
			query: map[string]string{"sort": "created_at"},
			want:  "watch does not support `sort`: pagination, sort, projection and search are not available on live queries",
		},
		{
			name:  "reserved key as base segment",
			query: map[string]string{"fields.title": "x"},
			want:  "watch does not support `fields`: pagination, sort, projection and search are not available on live queries",
		},
		{
			name:  "regex operator",
			query: map[string]string{"title.regex": "^a"},
			want:  "`regex` is not supported on live queries; use contains, start or end",
		},
		{
			name:  "geo operator",
			query: map[string]string{"location.near": "1,2"},
			want:  "geo operators (near, geoWithin, geoIntersects) are not supported on live queries",
		},
		{
			name:  "acl key",
			query: map[string]string{"_acl.read": "x"},
			want:  "filtering on _acl/_owner is not allowed",
		},
		{
			name:  "owner key",
			query: map[string]string{"_owner": "x"},
			want:  "filtering on _acl/_owner is not allowed",
		},
		{
			name:  "inverse relation",
			query: map[string]string{"inverse_of_products": "x"},
			want:  "inverse_of_* relations are not supported on live queries",
		},
		{
			name:  "empty segment",
			query: map[string]string{"color..title": "x"},
			want:  "invalid filter key `color..title`",
		},
		{
			name:  "dollar segment",
			query: map[string]string{"$where": "x"},
			want:  "invalid filter key `$where`",
		},
		{
			name:  "negated sub-field path",
			query: map[string]string{"color.title.ne": "red"},
			want:  "negated operators on sub-field paths (`color.title.ne`) are not supported on live queries; use the positive form",
		},
		{
			name:  "value too long",
			query: map[string]string{"title": strings.Repeat("a", 1001)},
			want:  "filter value for `title` is too long (max 1000 characters)",
		},
		{
			name:  "key too long",
			query: map[string]string{strings.Repeat("a", 101): "x"},
			want:  "filter key `" + strings.Repeat("a", 101) + "` is too long (max 100 characters)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateQuery(tc.query)
			if err == nil {
				t.Fatalf("ValidateQuery(%v) = nil, want error", tc.query)
			}
			var queryErr *QueryError
			if !errors.As(err, &queryErr) {
				t.Fatalf("error %T is not *QueryError", err)
			}
			if err.Error() != tc.want {
				t.Fatalf("message = %q, want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestValidateQueryTooManyKeys(t *testing.T) {
	query := map[string]string{}
	for i := range 21 {
		query[string(rune('a'+i))+"field"] = "x"
	}
	err := ValidateQuery(query)
	if err == nil || err.Error() != "live queries support at most 20 filter keys" {
		t.Fatalf("err = %v, want max keys error", err)
	}
}

func TestValidateQueryTooLarge(t *testing.T) {
	query := map[string]string{}
	for i := range 10 {
		query[string(rune('a'+i))+"field"] = strings.Repeat("v", 500)
	}
	err := ValidateQuery(query)
	if err == nil || err.Error() != "filter is too large (max 4096 bytes)" {
		t.Fatalf("err = %v, want size error", err)
	}
}
