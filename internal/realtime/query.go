package realtime

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	maxQueryKeys      = 20
	maxQueryKeyLen    = 100
	maxQueryValueLen  = 1000
	maxQueryJSONBytes = 4096
)

// QueryError reports a live-query filter that the server would reject.
type QueryError struct {
	Key    string
	Reason string
}

func (e *QueryError) Error() string {
	return e.Reason
}

var reservedQueryKeys = map[string]bool{
	"_status_id":     true,
	"_xdr":           true,
	"access_token":   true,
	"content_locale": true,
	"direction":      true,
	"explain":        true,
	"fields":         true,
	"include_slugs":  true,
	"limit":          true,
	"only":           true,
	"page":           true,
	"per_page":       true,
	"resolve":        true,
	"resolve_depth":  true,
	"search":         true,
	"signature":      true,
	"site":           true,
	"site_id":        true,
	"skip":           true,
	"sort":           true,
	"t":              true,
	"use_acl":        true,
	"x-cdn-expires":  true,
}

var geoOperators = map[string]bool{
	"geoIntersects": true,
	"geoWithin":     true,
	"maxDistance":   true,
	"minDistance":   true,
	"near":          true,
}

var negatedOperators = map[string]bool{
	"ne":           true,
	"nin":          true,
	"not_contains": true,
}

// ValidateQuery checks a live-query filter map against the rules the server
// applies, so the CLI can fail with a precise message before connecting.
func ValidateQuery(q map[string]string) error {
	if len(q) == 0 {
		return nil
	}
	if len(q) > maxQueryKeys {
		return &QueryError{Reason: fmt.Sprintf("live queries support at most %d filter keys", maxQueryKeys)}
	}

	keys := make([]string, 0, len(q))
	for key := range q {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if err := validateQueryKey(key); err != nil {
			return err
		}
		if len(q[key]) > maxQueryValueLen {
			return &QueryError{
				Key:    key,
				Reason: fmt.Sprintf("filter value for `%s` is too long (max %d characters)", key, maxQueryValueLen),
			}
		}
	}

	encoded, err := json.Marshal(q)
	if err != nil {
		return &QueryError{Reason: fmt.Sprintf("filter cannot be encoded: %v", err)}
	}
	if len(encoded) > maxQueryJSONBytes {
		return &QueryError{Reason: fmt.Sprintf("filter is too large (max %d bytes)", maxQueryJSONBytes)}
	}
	return nil
}

func validateQueryKey(key string) error {
	if len(key) > maxQueryKeyLen {
		return &QueryError{
			Key:    key,
			Reason: fmt.Sprintf("filter key `%s` is too long (max %d characters)", key, maxQueryKeyLen),
		}
	}

	segments := strings.Split(key, ".")
	for _, segment := range segments {
		if segment == "" || strings.HasPrefix(segment, "$") {
			return &QueryError{Key: key, Reason: fmt.Sprintf("invalid filter key `%s`", key)}
		}
	}

	base := segments[0]
	if base == "_acl" || base == "_owner" {
		return &QueryError{Key: key, Reason: "filtering on _acl/_owner is not allowed"}
	}
	if reservedQueryKeys[base] {
		return &QueryError{
			Key:    key,
			Reason: fmt.Sprintf("watch does not support `%s`: pagination, sort, projection and search are not available on live queries", base),
		}
	}
	if strings.HasPrefix(base, "inverse_of_") {
		return &QueryError{Key: key, Reason: "inverse_of_* relations are not supported on live queries"}
	}

	if len(segments) < 2 {
		return nil
	}

	last := segments[len(segments)-1]
	if last == "regex" {
		return &QueryError{Key: key, Reason: "`regex` is not supported on live queries; use contains, start or end"}
	}
	if geoOperators[last] {
		return &QueryError{
			Key:    key,
			Reason: "geo operators (near, geoWithin, geoIntersects) are not supported on live queries",
		}
	}
	if len(segments) >= 3 && negatedOperators[last] {
		return &QueryError{
			Key:    key,
			Reason: "negated operators on sub-field paths (`color.title.ne`) are not supported on live queries; use the positive form",
		}
	}
	return nil
}
