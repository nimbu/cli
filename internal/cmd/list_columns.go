package cmd

import "strings"

func listRequestedFields(flags *RootFlags) []string {
	if flags == nil {
		return nil
	}

	parts := strings.Split(flags.Fields, ",")
	fields := make([]string, 0, len(parts))
	for _, part := range parts {
		field := strings.TrimSpace(part)
		if field == "" {
			continue
		}
		fields = append(fields, field)
	}

	return fields
}

func listOutputFields(flags *RootFlags, defaults []string) []string {
	if fields := listRequestedFields(flags); len(fields) > 0 {
		return fields
	}
	return defaults
}

func listOutputColumns(flags *RootFlags, defaultFields, defaultHeaders []string) ([]string, []string) {
	if fields := listRequestedFields(flags); len(fields) > 0 {
		return fields, listHeadersFromFields(fields)
	}
	return defaultFields, defaultHeaders
}

func listHeadersFromFields(fields []string) []string {
	headers := make([]string, len(fields))
	for i, field := range fields {
		headers[i] = strings.ToUpper(strings.ReplaceAll(field, "_", " "))
	}
	return headers
}
