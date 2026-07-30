package api

import (
	"bytes"
	"encoding/json"
)

// Document keeps the original API representation alongside a typed view.
// JSON output is therefore lossless while human-oriented commands can use Value.
type Document[T any] struct {
	Value T
	raw   json.RawMessage
}

func (d *Document[T]) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &d.Value); err != nil {
		return err
	}
	d.raw = bytes.Clone(data)
	return nil
}

func (d Document[T]) MarshalJSON() ([]byte, error) {
	if len(d.raw) > 0 {
		return bytes.Clone(d.raw), nil
	}
	return json.Marshal(d.Value)
}

// DocumentValues returns the typed views used by table and plain renderers.
func DocumentValues[T any](documents []Document[T]) []T {
	values := make([]T, len(documents))
	for i := range documents {
		values[i] = documents[i].Value
	}
	return values
}
