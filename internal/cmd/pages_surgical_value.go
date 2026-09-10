package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
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

func (s *surgicalSession) fileRefs() *api.FileRefNormalizer {
	if s == nil {
		return &api.FileRefNormalizer{}
	}
	if s.fileRef == nil {
		s.fileRef = &api.FileRefNormalizer{Client: s.client}
	}
	return s.fileRef
}

func (s *surgicalSession) warnFileRef(warning string) {
	if warning == "" || s == nil {
		return
	}
	_, _ = fmt.Fprintf(output.WriterFromContext(s.ctx).Err, "warning: %s\n", warning)
}

func pageAttachmentFileRefOptions(ctx context.Context, client *api.Client, extra api.PageAttachmentExpansionOptions) api.PageAttachmentExpansionOptions {
	extra.Context = ctx
	extra.FileRef = &api.FileRefNormalizer{Client: client}
	extra.Warn = func(msg string) {
		_, _ = fmt.Fprintf(output.WriterFromContext(ctx).Err, "warning: %s\n", msg)
	}
	return extra
}

func expandInsertFileValue(session *surgicalSession, raw any) (any, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return raw, nil
	}
	if path := stringAny(m["attachment_path"]); path != "" {
		return encodeLocalFileValue(path)
	}
	if stringAny(m["__type"]) == "FileRef" {
		source := firstNonBlank(stringAny(m["source"]), stringAny(m["attachment_url"]), stringAny(m["url"]))
		return map[string]any{"__type": "FileRef", "source": source}, nil
	}
	source := firstNonBlank(stringAny(m["source"]), stringAny(m["attachment_url"]), stringAny(m["url"]))
	if source == "" {
		return m, nil
	}
	if strings.HasPrefix(source, "nimbu://") {
		return map[string]any{"__type": "FileRef", "source": source}, nil
	}
	ctx := context.Background()
	if session != nil {
		ctx = session.ctx
	}
	payload, warning, err := session.fileRefs().NormalizeURL(ctx, source)
	if err != nil {
		return nil, err
	}
	session.warnFileRef(warning)
	return surgicalInlineFile(payload), nil
}

func surgicalInlineFile(payload map[string]any) map[string]any {
	if stringAny(payload["__type"]) != "File" {
		return payload
	}
	filename := stringAny(payload["filename"])
	encoded := stringAny(payload["attachment"])
	contentType := stringAny(payload["content_type"])
	if contentType == "" {
		raw, _ := base64.StdEncoding.DecodeString(encoded)
		contentType = detectFileContentType(filename, raw)
	}
	return map[string]any{
		"data":         encoded,
		"filename":     filename,
		"content_type": contentType,
	}
}

func expandSetFileValue(session *surgicalSession, raw any) (any, error) {
	if !isFileEditablePayload(raw) {
		return raw, nil
	}
	return expandInsertFileValue(session, raw)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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
