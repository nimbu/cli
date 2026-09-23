package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/pagepath"
)

const (
	surgicalPageID       = "6a6c60362e4510dd4a459982"
	surgicalUpdatedAt    = "2026-09-10T14:31:12.408Z"
	surgicalBlockID      = "6a6c699f0655c6dcb4dd854c"
	surgicalBlockID2     = "6a6c699f0655c6dcb4dd854d"
	surgicalDraftID      = "6a6d00000000000000000001"
	surgicalDraftOnlyID  = "6a6c699f0655c6dcb4dd854e"
	surgicalDraftUpdated = "2026-09-10T15:30:00.000Z"
)

func surgicalPageETag(t *testing.T) string {
	t.Helper()
	etag, err := api.PageETag(surgicalPageID, surgicalUpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	return etag
}

func surgicalPageJSON() string {
	return `{
		"id":"` + surgicalPageID + `",
		"fullpath":"about",
		"updated_at":"` + surgicalUpdatedAt + `",
		"title":"About",
		"published":true,
		"items":{
			"Theme":{"type":"select","content":"Light"},
			"Blokken":{
				"type":"canvas",
				"repeatables":[
					{
						"id":"` + surgicalBlockID + `",
						"slug":"hero_stage",
						"position":1,
						"items":{
							"Title":{"type":"text","content":"Hero"},
							"Enabled":{"type":"switch","content":false},
							"Image":{"type":"file","file":{"url":"https://cdn.example.test/a.jpg"}},
							"Ref":{"type":"reference"}
						}
					},
					{
						"id":"` + surgicalBlockID2 + `",
						"slug":"proof_strip",
						"position":2,
						"items":{"Quote":{"type":"text","content":"Hi"}}
					}
				]
			}
		}
	}`
}

func surgicalSchemaJSON() string {
	return `{
		"template":{"id":"t1","name":"home"},
		"available_blocks":{
			"Blokken":[
				{"slug":"hero_stage","label":"Hero","fields":[
					{"slug":"Title","type":"text"},
					{"slug":"Enabled","type":"switch"},
					{"slug":"Image","type":"file"},
					{"slug":"Ref","type":"reference"}
				]},
				{"slug":"proof_strip","label":"Proof","fields":[
					{"slug":"Quote","type":"text"}
				]}
			]
		},
		"select_options":{
			"Theme":[{"label":"Light","value":"light"},{"label":"Dark","value":"dark"},{"label":"Auto","value":"auto"}]
		}
	}`
}

type surgicalServer struct {
	gets         int
	posts        int
	draftGets    int
	draftPosts   int
	lastReq      *http.Request
	lastBody     map[string]any
	ifMatch      []string
	queries      []string
	paths        []string
	pageJSON     string
	schemaJSON   string
	draftJSON    string
	noDraft      bool
	draftsOff    bool
	batchFn      func(http.ResponseWriter, *http.Request, int)
	draftBatchFn func(http.ResponseWriter, *http.Request, int)
	// otherFn serves routes the fake does not know (e.g. /themes, /uploads);
	// it returns false to fall through to 404.
	otherFn func(http.ResponseWriter, *http.Request) bool
}

func (s *surgicalServer) pageBody() string {
	if s.pageJSON != "" {
		return s.pageJSON
	}
	return surgicalPageJSON()
}

func (s *surgicalServer) schemaBody() string {
	if s.schemaJSON != "" {
		return s.schemaJSON
	}
	return surgicalSchemaJSON()
}

func (s *surgicalServer) draftBody() string {
	if s.draftJSON != "" {
		return s.draftJSON
	}
	return surgicalDraftJSON()
}

func surgicalDraftJSON() string {
	return `{
		"id":"` + surgicalDraftID + `",
		"page_id":"` + surgicalPageID + `",
		"reserved_fullpath":"about",
		"updated_at":"` + surgicalDraftUpdated + `",
		"content":{
			"title":"About draft",
			"page_items":[
				{"slug":"Theme","type":"select","content":"Light"},
				{"slug":"Blokken","type":"canvas","repeatables":[
					{"_id":"` + surgicalBlockID + `","slug":"hero_stage","position":1,"page_items":[
						{"slug":"Title","type":"text","content":"Hero"},
						{"slug":"Enabled","type":"switch","content":"false"},
						{"slug":"Image","type":"file","content":"https://cdn.example.test/a.jpg"}
					]},
					{"_id":"` + surgicalBlockID2 + `","slug":"proof_strip","position":2,"page_items":[
						{"slug":"Quote","type":"text","content":"Hi"}
					]},
					{"_id":"` + surgicalDraftOnlyID + `","slug":"proof_strip","position":3,"page_items":[
						{"slug":"Quote","type":"text","content":"Draft only"}
					]}
				]}
			]
		}
	}`
}

func (s *surgicalServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && (r.URL.Path == "/pages/about" || r.URL.Path == "/pages/"+surgicalPageID):
			s.gets++
			_, _ = w.Write([]byte(s.pageBody()))
		case r.Method == http.MethodGet && r.URL.Path == "/pages/"+surgicalPageID+"/schema":
			_, _ = w.Write([]byte(s.schemaBody()))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/pages/"+surgicalPageID+"/items/"):
			_, _ = w.Write([]byte(`{"path":"/items/Blokken/repeatables/` + surgicalBlockID + `/items/Title","parent_path":"/items/Blokken/repeatables/` + surgicalBlockID + `","position":1,"siblings_count":3,"type":"item","data":{"type":"text","content":"Hero"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/pages/"+surgicalPageID+"/draft":
			s.draftGets++
			s.paths = append(s.paths, r.URL.Path)
			if s.draftsOff {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"Page drafts are not enabled"}`))
				return
			}
			if s.noDraft {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"Not Found","code":101}`))
				return
			}
			_, _ = w.Write([]byte(s.draftBody()))
		case r.Method == http.MethodPost && r.URL.Path == "/pages/"+surgicalPageID+"/draft/batch":
			s.draftPosts++
			s.lastReq = r
			s.paths = append(s.paths, r.URL.Path)
			s.ifMatch = append(s.ifMatch, r.Header.Get("If-Match"))
			s.queries = append(s.queries, r.URL.RawQuery)
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &s.lastBody); err != nil {
				t.Fatalf("decode draft batch body: %v", err)
			}
			if s.draftBatchFn != nil {
				s.draftBatchFn(w, r, s.draftPosts)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"results":[{"index":0,"status":"ok","path":"/items/Blokken/repeatables/` + surgicalBlockID + `/items/Title","id":"` + surgicalBlockID + `"}],
				"draft":` + s.draftBody() + `
			}`))
		case r.Method == http.MethodPost && r.URL.Path == "/pages/"+surgicalPageID+"/batch":
			s.posts++
			s.lastReq = r
			s.paths = append(s.paths, r.URL.Path)
			s.ifMatch = append(s.ifMatch, r.Header.Get("If-Match"))
			s.queries = append(s.queries, r.URL.RawQuery)
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &s.lastBody); err != nil {
				t.Fatalf("decode batch body: %v", err)
			}
			if s.batchFn != nil {
				s.batchFn(w, r, s.posts)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"results":[{"index":0,"status":"ok","path":"/items/Blokken/repeatables/` + surgicalBlockID + `/items/Title","id":"` + surgicalBlockID + `"}],
				"etag":"newetag12",
				"updated_at":"2026-09-10T15:00:00.000Z",
				"page":` + surgicalPageJSON() + `
			}`))
		default:
			if s.otherFn != nil && s.otherFn(w, r) {
				return
			}
			http.NotFound(w, r)
		}
	}))
}

func TestWriteBatchResultLinesUnescapesPath(t *testing.T) {
	var buf strings.Builder
	writeBatchResultLines(&buf, []plannedOp{{Op: api.BatchOperation{Op: "set"}}}, []api.BatchOpResult{
		{Status: "ok", Path: "/items/Navigation%20on%20dark%20background"},
	})
	got := buf.String()
	if !strings.Contains(got, "/items/Navigation on dark background") {
		t.Fatalf("output = %q", got)
	}
	if strings.Contains(got, "%20") {
		t.Fatalf("path still escaped: %q", got)
	}
}

func TestCoerceSetValue(t *testing.T) {
	got, err := coerceSetValue(pagepath.Resolved{Type: "switch", RawPath: "/items/X"}, "1")
	if err != nil || got != true {
		t.Fatalf("switch 1 = %#v %v", got, err)
	}
	if _, err := coerceSetValue(pagepath.Resolved{Type: "switch"}, "maybe"); err == nil {
		t.Fatal("expected switch error")
	}
	got, err = coerceSetValue(pagepath.Resolved{Type: "reference", RawPath: "/items/R"}, "abc")
	if err != nil || got != "abc" {
		t.Fatalf("reference = %#v %v", got, err)
	}
}
