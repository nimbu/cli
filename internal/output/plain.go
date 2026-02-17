package output

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// Plain writes values as tab-separated text to the writer.
func Plain(ctx context.Context, values ...any) error {
	w := WriterFromContext(ctx)
	return WritePlain(w.Out, values...)
}

// WritePlain writes values as tab-separated line.
func WritePlain(w io.Writer, values ...any) error {
	strs := make([]string, len(values))
	for i, v := range values {
		strs[i] = fmt.Sprint(v)
	}
	_, err := fmt.Fprintln(w, strings.Join(strs, "\t"))
	return err
}

// PlainRows writes multiple rows as TSV.
func PlainRows(ctx context.Context, rows [][]any) error {
	w := WriterFromContext(ctx)
	for _, row := range rows {
		if err := WritePlain(w.Out, row...); err != nil {
			return err
		}
	}
	return nil
}

// PlainFromStruct extracts specified fields from a struct and writes as TSV.
func PlainFromStruct(ctx context.Context, v any, fields []string) error {
	w := WriterFromContext(ctx)
	values, err := extractFields(v, fields)
	if err != nil {
		return err
	}
	return WritePlain(w.Out, values...)
}

// PlainFromSlice writes each struct in a slice as a TSV row.
func PlainFromSlice(ctx context.Context, slice any, fields []string) error {
	w := WriterFromContext(ctx)

	rv := reflect.ValueOf(slice)
	if rv.Kind() != reflect.Slice {
		return fmt.Errorf("expected slice, got %T", slice)
	}

	if err := validateFieldsForSliceType(rv.Type(), fields); err != nil {
		return err
	}

	for i := 0; i < rv.Len(); i++ {
		values, err := extractFields(rv.Index(i).Interface(), fields)
		if err != nil {
			return err
		}
		if err := WritePlain(w.Out, values...); err != nil {
			return err
		}
	}

	return nil
}

func validateFieldsForSliceType(rt reflect.Type, fields []string) error {
	if rt.Kind() != reflect.Slice {
		return nil
	}

	elem := rt.Elem()
	if elem.Kind() == reflect.Ptr {
		elem = elem.Elem()
	}

	if elem.Kind() != reflect.Struct {
		return nil
	}

	return validateFieldNames(elem, fields)
}

func validateFieldNames(rt reflect.Type, fields []string) error {
	for _, fieldName := range fields {
		found := false
		for i := 0; i < rt.NumField(); i++ {
			sf := rt.Field(i)

			jsonTag := sf.Tag.Get("json")
			if jsonTag != "" {
				jsonName := strings.Split(jsonTag, ",")[0]
				if jsonName == fieldName {
					found = true
					break
				}
			}

			if strings.EqualFold(sf.Name, fieldName) {
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("unknown field %q", fieldName)
		}
	}

	return nil
}

func extractFields(v any, fields []string) ([]any, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct {
		return []any{fmt.Sprint(v)}, nil
	}

	if err := validateFieldNames(rv.Type(), fields); err != nil {
		return nil, err
	}

	rt := rv.Type()
	values := make([]any, 0, len(fields))

	for _, fieldName := range fields {
		found := false
		for i := 0; i < rt.NumField(); i++ {
			sf := rt.Field(i)

			// Check JSON tag first
			jsonTag := sf.Tag.Get("json")
			if jsonTag != "" {
				jsonName := strings.Split(jsonTag, ",")[0]
				if jsonName == fieldName {
					values = append(values, rv.Field(i).Interface())
					found = true
					break
				}
			}

			// Check field name
			if strings.EqualFold(sf.Name, fieldName) {
				values = append(values, rv.Field(i).Interface())
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("unknown field %q", fieldName)
		}
	}

	return values, nil
}
