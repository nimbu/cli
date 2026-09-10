package cmd

import (
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/nimbu/cli/internal/pagepath"
)

const maxSurgicalFileBytes = 25 << 20

func coerceSetValue(resolved pagepath.Resolved, raw string) (any, error) {
	if resolved.RawPath == "/published" || resolved.Type == "switch" {
		return parseBoolish(raw)
	}
	return raw, nil
}

func parseBoolish(raw string) (any, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		return nil, fmt.Errorf("invalid switch value %q; use true/false/1/0/yes/no", raw)
	}
}

// encodeLocalFileValue mirrors pages update's attachment_path expansion for a
// one-off batch set. The TypeCoercer accepts {data, filename, content_type};
// a separate upload + FileRef is not needed.
func encodeLocalFileValue(path string) (map[string]any, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", path, err)
	}
	if info.Size() > maxSurgicalFileBytes {
		return nil, fmt.Errorf("file %q is larger than 25 MB", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", path, err)
	}
	if int64(len(data)) > maxSurgicalFileBytes {
		return nil, fmt.Errorf("file %q is larger than 25 MB", path)
	}
	filename := filepath.Base(path)
	return map[string]any{
		"data":         base64.StdEncoding.EncodeToString(data),
		"filename":     filename,
		"content_type": detectFileContentType(filename, data),
	}, nil
}

func detectFileContentType(filename string, data []byte) string {
	if ext := filepath.Ext(filename); ext != "" {
		if typ := mime.TypeByExtension(ext); typ != "" {
			return typ
		}
	}
	return http.DetectContentType(data)
}

func expandInsertFileValue(raw any) (any, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return raw, nil
	}
	if path := stringAny(m["attachment_path"]); path != "" {
		return encodeLocalFileValue(path)
	}
	source := stringAny(m["source"])
	if source == "" {
		source = stringAny(m["attachment_url"])
	}
	if source != "" {
		if strings.HasPrefix(source, "nimbu://") {
			return map[string]any{"__type": "FileRef", "source": source}, nil
		}
		return map[string]any{"url": source}, nil
	}
	return m, nil
}

func isFileEditablePayload(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"data", "__type", "url", "source", "attachment_path", "attachment_url"} {
		if _, ok := m[key]; ok {
			return true
		}
	}
	return false
}

func exclusiveValueSource(value, file, fromFile string) error {
	n := 0
	if value != "" {
		n++
	}
	if file != "" {
		n++
	}
	if fromFile != "" {
		n++
	}
	if n == 0 {
		return fmt.Errorf("provide a value, --file, or --from-file")
	}
	if n > 1 {
		return fmt.Errorf("value, --file, and --from-file are mutually exclusive")
	}
	return nil
}
