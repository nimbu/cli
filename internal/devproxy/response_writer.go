package devproxy

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

func writeSimulatorResponse(w http.ResponseWriter, response *api.SimulatorResponse) error {
	if response == nil {
		return fmt.Errorf("nil simulator response")
	}

	status := response.EffectiveStatus()
	if status <= 0 {
		status = http.StatusOK
	}

	writeFilteredHeaders(w.Header(), response.Headers)
	w.WriteHeader(status)

	body := response.Body
	if body == "" {
		return nil
	}

	decoded, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		if _, writeErr := w.Write([]byte(body)); writeErr != nil {
			return writeErr
		}
		return nil
	}

	if isBinaryContent(contentTypeFromHeaders(response.Headers)) {
		_, err = w.Write(decoded)
		return err
	}

	_, err = w.Write(decoded)
	return err
}

func contentTypeFromHeaders(headers map[string]string) string {
	if value, ok := headers["content-type"]; ok {
		return value
	}
	if value, ok := headers["Content-Type"]; ok {
		return value
	}
	return "text/html"
}

func isBinaryContent(contentType string) bool {
	contentType = strings.ToLower(contentType)
	binaryTypes := []string{
		"image/",
		"audio/",
		"video/",
		"application/pdf",
		"application/zip",
		"application/octet-stream",
		"font/",
	}
	for _, t := range binaryTypes {
		if strings.Contains(contentType, t) {
			return true
		}
	}
	return false
}

func writeFilteredHeaders(dst http.Header, headers map[string]string) {
	excluded := map[string]struct{}{
		"connection":          {},
		"keep-alive":          {},
		"proxy-authenticate":  {},
		"proxy-authorization": {},
		"te":                  {},
		"trailer":             {},
		"transfer-encoding":   {},
		"upgrade":             {},
	}

	connectionTokens := map[string]struct{}{}
	for name, value := range headers {
		if !strings.EqualFold(name, "connection") {
			continue
		}
		for _, token := range strings.Split(value, ",") {
			token = strings.ToLower(strings.TrimSpace(token))
			if token == "" {
				continue
			}
			connectionTokens[token] = struct{}{}
		}
	}

	for name, value := range headers {
		normalized := strings.ToLower(name)
		if _, skip := excluded[normalized]; skip {
			continue
		}
		if _, hopByHop := connectionTokens[normalized]; hopByHop {
			continue
		}

		if strings.EqualFold(name, "set-cookie") {
			cookies := strings.Split(value, "\n")
			if len(cookies) > 1 {
				for _, cookie := range cookies {
					cookie = strings.TrimSpace(cookie)
					if cookie == "" {
						continue
					}
					dst.Add("Set-Cookie", cookie)
				}
				continue
			}
		}

		dst.Set(name, strings.ReplaceAll(value, "\n", ""))
	}
}

func writeJSONError(w http.ResponseWriter, status int, code string, message string) {
	if status <= 0 {
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":"%s","message":%q}`+"\n", code, message)
}
