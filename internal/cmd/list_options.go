package cmd

import (
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

func listRequestOptions(flags *QueryFlags, extra ...api.RequestOption) ([]api.RequestOption, error) {
	opts := make([]api.RequestOption, 0, len(extra)+4)

	if flags != nil {
		if flags.Fields != "" {
			opts = append(opts, api.WithFields(flags.Fields))
		}

		if flags.Locale != "" {
			opts = append(opts, api.WithLocale(flags.Locale))
		}

		if flags.Include != "" {
			opts = append(opts, api.WithInclude(flags.Include))
		}

		if flags.Sort != "" {
			sort, err := parseSort(flags.Sort)
			if err != nil {
				return nil, fmt.Errorf("invalid --sort: %w", err)
			}
			opts = append(opts, api.WithParam("sort", sort))
		}

		for _, raw := range flags.Filters {
			k, v, err := parseFilter(raw)
			if err != nil {
				return nil, fmt.Errorf("invalid --filter: %w", err)
			}
			opts = append(opts, api.WithParam(k, v))
		}
	}

	opts = append(opts, extra...)
	return opts, nil
}

func parseSort(sort string) (string, error) {
	sort = strings.TrimSpace(sort)
	if sort == "" {
		return "", fmt.Errorf("expected field or field:asc|field:desc")
	}

	parts := strings.SplitN(sort, ":", 2)
	field := strings.TrimSpace(parts[0])
	if field == "" {
		return "", fmt.Errorf("expected field or field:asc|field:desc")
	}

	if len(parts) == 1 {
		return field, nil
	}

	direction := strings.ToLower(strings.TrimSpace(parts[1]))
	switch direction {
	case "", "asc":
		return field, nil
	case "desc":
		return "-" + field, nil
	default:
		return "", fmt.Errorf("invalid sort direction %q, expected asc or desc", direction)
	}
}

func parseFilter(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	parts := strings.SplitN(raw, "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected key=value")
	}

	key := strings.TrimSpace(parts[0])
	if key == "" {
		return "", "", fmt.Errorf("expected key=value")
	}

	return key, strings.TrimSpace(parts[1]), nil
}
