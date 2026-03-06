package devproxy

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/nimbu/cli/internal/api"
)

var errBodyTooLarge = errors.New("request body exceeds configured max size")

func readBody(req *http.Request, maxBytes int64) ([]byte, map[string]any, error) {
	if req == nil || req.Body == nil {
		return nil, nil, nil
	}

	if req.Method == http.MethodGet || req.Method == http.MethodHead {
		return nil, nil, nil
	}

	limited := io.LimitReader(req.Body, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, nil, errBodyTooLarge
	}

	parsed := parseBody(req.Header.Get("Content-Type"), data)
	return data, parsed, nil
}

func parseBody(contentType string, data []byte) map[string]any {
	if len(data) == 0 {
		return nil
	}

	if strings.Contains(contentType, "application/json") {
		var body any
		if err := json.Unmarshal(data, &body); err != nil {
			return map[string]any{}
		}
		if object, ok := body.(map[string]any); ok {
			return object
		}
		return map[string]any{"_": body}
	}

	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		values, err := url.ParseQuery(string(data))
		if err != nil {
			return map[string]any{}
		}
		return valuesToMap(values)
	}

	// For multipart and unknown content types, send raw body only.
	return nil
}

func valuesToMap(values url.Values) map[string]any {
	out := make(map[string]any, len(values))
	for key, items := range values {
		if len(items) == 1 {
			out[key] = items[0]
			continue
		}
		copyItems := make([]string, len(items))
		copy(copyItems, items)
		out[key] = copyItems
	}
	return out
}

func buildPayload(req *http.Request, userAgent string, templateCode string, rawBody []byte, parsedBody map[string]any) (api.SimulatorPayload, error) {
	metadata, err := extractMetadata(req, userAgent, parsedBody)
	if err != nil {
		return api.SimulatorPayload{}, err
	}

	headersJSON, err := json.Marshal(metadata.headers)
	if err != nil {
		return api.SimulatorPayload{}, fmt.Errorf("marshal headers: %w", err)
	}

	queryJSON, err := json.Marshal(metadata.query)
	if err != nil {
		return api.SimulatorPayload{}, fmt.Errorf("marshal query: %w", err)
	}

	var bodyJSON *string
	if metadata.body != nil {
		b, err := json.Marshal(metadata.body)
		if err != nil {
			return api.SimulatorPayload{}, fmt.Errorf("marshal body: %w", err)
		}
		s := string(b)
		bodyJSON = &s
	}

	var rawBodyBase64 *string
	if len(rawBody) > 0 {
		s := base64.StdEncoding.EncodeToString(rawBody)
		rawBodyBase64 = &s
	}

	return api.SimulatorPayload{
		Simulator: api.SimulatorRequest{
			Code:    templateCode,
			Method:  metadata.method,
			Path:    metadata.path,
			Version: "v3",
			Request: api.SimulatorRequestContext{
				Body:    bodyJSON,
				Headers: string(headersJSON),
				Host:    metadata.host,
				Method:  metadata.method,
				Params:  metadata.params,
				Port:    metadata.port,
				Query:   string(queryJSON),
				RawBody: rawBodyBase64,
			},
		},
	}, nil
}

type requestMetadata struct {
	body    map[string]any
	headers map[string]any
	host    string
	method  string
	params  map[string]any
	path    string
	port    int
	query   map[string]any
}

func extractMetadata(req *http.Request, userAgent string, parsedBody map[string]any) (*requestMetadata, error) {
	method := strings.ToLower(req.Method)
	path := requestPath(req)
	host, port := splitHostPort(req)
	query := valuesToMap(req.URL.Query())
	headers := extractHeaders(req, userAgent)

	params := make(map[string]any, len(query))
	for key, value := range query {
		params[key] = value
	}

	if shouldMergeBody(req.Method) && parsedBody != nil {
		for key, value := range parsedBody {
			params[key] = value
		}
	}

	return &requestMetadata{
		body:    parsedBody,
		headers: headers,
		host:    host,
		method:  method,
		params:  params,
		path:    path,
		port:    port,
		query:   query,
	}, nil
}

func shouldMergeBody(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodPatch, http.MethodPost, http.MethodPut:
		return true
	default:
		return false
	}
}

func extractHeaders(req *http.Request, userAgent string) map[string]any {
	headers := make(map[string]any, len(req.Header)+6)
	for key, values := range req.Header {
		if len(values) == 1 {
			headers[strings.ToLower(key)] = values[0]
			continue
		}
		copyValues := make([]string, len(values))
		copy(copyValues, values)
		headers[strings.ToLower(key)] = copyValues
	}

	headers["x-nimbu-simulator"] = userAgent
	headers["REQUEST_METHOD"] = strings.ToUpper(req.Method)
	headers["REQUEST_PATH"] = req.URL.Path
	headers["PATH_INFO"] = req.URL.Path
	headers["REQUEST_URI"] = req.URL.RequestURI()
	headers["QUERY_STRING"] = req.URL.RawQuery

	if cookie, ok := headers["cookie"]; ok {
		headers["HTTP_COOKIE"] = cookie
	}

	return headers
}

func splitHostPort(req *http.Request) (string, int) {
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	if host == "" {
		return "localhost", defaultPort(req)
	}

	parsedHost, parsedPort, err := net.SplitHostPort(host)
	if err == nil {
		port, parseErr := strconv.Atoi(parsedPort)
		if parseErr == nil {
			if parsedHost == "" {
				parsedHost = "localhost"
			}
			return strings.Trim(parsedHost, "[]"), port
		}
	}

	trimmed := strings.Trim(host, "[]")
	if ip := net.ParseIP(trimmed); ip != nil {
		return trimmed, defaultPort(req)
	}

	if strings.Count(host, ":") == 0 {
		if host == "" {
			host = "localhost"
		}
		return host, defaultPort(req)
	}

	// Host:port fallback for malformed values where SplitHostPort cannot parse.
	if strings.Count(host, ":") == 1 {
		parts := strings.SplitN(host, ":", 2)
		baseHost := parts[0]
		if baseHost == "" {
			baseHost = "localhost"
		}
		if port, parseErr := strconv.Atoi(parts[1]); parseErr == nil {
			return baseHost, port
		}
		return baseHost, defaultPort(req)
	}

	// Preserve full host for unknown colon-heavy values instead of truncating.
	return host, defaultPort(req)
}

func requestPath(req *http.Request) string {
	if req == nil || req.URL == nil {
		return "/"
	}

	path := req.URL.EscapedPath()
	if path == "" {
		path = req.URL.Path
	}
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func defaultPort(req *http.Request) int {
	if req != nil && req.TLS != nil {
		return 443
	}
	return 80
}
