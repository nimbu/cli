package api

import (
	"encoding/base64"
	"mime"
	"path"
)

// NewUploadCreatePayload builds the JSON upload body expected by POST /uploads.
func NewUploadCreatePayload(filename string, content []byte, contentType string) map[string]any {
	source := map[string]any{
		"__type":     "File",
		"attachment": base64.StdEncoding.EncodeToString(content),
		"filename":   path.Base(filename),
	}
	if contentType == "" {
		contentType = mime.TypeByExtension(path.Ext(filename))
	}
	if contentType != "" {
		source["content_type"] = contentType
	}
	return map[string]any{"source": source}
}
