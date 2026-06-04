package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

func TestGallerySetDryRunBuildsCanonicalPayload(t *testing.T) {
	imagePath := writeGalleryTestFile(t, "hero.jpg", "image-bytes")
	server := galleryTestServer(t, galleryServerHooks{})
	defer server.Close()
	ctx, stdout, _ := newGalleryTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := ChannelEntriesGallerySetCmd{
		Channel:  "subjects",
		Entry:    "entry-1",
		Field:    "photos",
		Images:   []string{imagePath},
		Captions: []string{"Hero"},
		DryRun:   true,
	}

	if err := cmd.Run(ctx, &RootFlags{APIURL: server.URL}); err != nil {
		t.Fatalf("run: %v", err)
	}

	payload := decodeGalleryPayload(t, stdout.Bytes())
	photos := payload["photos"].(map[string]any)
	if photos["__type"] != "Gallery" {
		t.Fatalf("gallery type = %#v", photos["__type"])
	}
	images := photos["images"].([]any)
	if len(images) != 1 {
		t.Fatalf("images len = %d", len(images))
	}
	image := images[0].(map[string]any)
	if image["__type"] != "GalleryImage" || image["caption"] != "Hero" || image["position"] != float64(0) {
		t.Fatalf("unexpected image payload: %#v", image)
	}
	file := image["file"].(map[string]any)
	if file["__type"] != "File" || file["filename"] != "hero.jpg" {
		t.Fatalf("unexpected file payload: %#v", file)
	}
	if file["attachment"] != base64.StdEncoding.EncodeToString([]byte("image-bytes")) {
		t.Fatalf("unexpected attachment: %#v", file["attachment"])
	}
}

func TestGalleryUpdateCaptionSendsTargetedImagePatch(t *testing.T) {
	var updateBody map[string]any
	server := galleryTestServer(t, galleryServerHooks{
		onUpdate: func(body map[string]any) {
			updateBody = body
		},
	})
	defer server.Close()

	ctx, _, _ := newGalleryTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := ChannelEntriesGalleryUpdateCmd{
		Channel: "subjects",
		Entry:   "entry-1",
		Field:   "photos",
		ImageID: "img-1",
		Caption: stringPtr("Updated"),
	}

	if err := cmd.Run(ctx, &RootFlags{APIURL: server.URL}); err != nil {
		t.Fatalf("run: %v", err)
	}

	image := firstGalleryImage(t, updateBody, "photos")
	if image["id"] != "img-1" || image["caption"] != "Updated" {
		t.Fatalf("unexpected image patch: %#v", image)
	}
	if _, ok := image["position"]; ok {
		t.Fatalf("position should be omitted: %#v", image)
	}
}

func TestGalleryUpdateEmptyCaptionSendsClearPatch(t *testing.T) {
	var updateBody map[string]any
	server := galleryTestServer(t, galleryServerHooks{
		onUpdate: func(body map[string]any) {
			updateBody = body
		},
	})
	defer server.Close()

	ctx, _, _ := newGalleryTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := ChannelEntriesGalleryUpdateCmd{
		Channel: "subjects",
		Entry:   "entry-1",
		Field:   "photos",
		ImageID: "img-1",
		Caption: stringPtr(""),
	}

	if err := cmd.Run(ctx, &RootFlags{APIURL: server.URL}); err != nil {
		t.Fatalf("run: %v", err)
	}

	image := firstGalleryImage(t, updateBody, "photos")
	if caption, ok := image["caption"]; !ok || caption != "" {
		t.Fatalf("expected empty caption patch, got %#v", image)
	}
}

func TestGalleryUpdateRejectsUnknownImageID(t *testing.T) {
	updateCount := 0
	server := galleryTestServer(t, galleryServerHooks{
		onUpdate: func(body map[string]any) {
			updateCount++
		},
	})
	defer server.Close()

	ctx, _, _ := newGalleryTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := ChannelEntriesGalleryUpdateCmd{
		Channel: "subjects",
		Entry:   "entry-1",
		Field:   "photos",
		ImageID: "missing",
		Caption: stringPtr("Updated"),
	}

	err := cmd.Run(ctx, &RootFlags{APIURL: server.URL})
	if err == nil || !strings.Contains(err.Error(), `unknown image id "missing"`) {
		t.Fatalf("expected unknown image id error, got %v", err)
	}
	if updateCount != 0 {
		t.Fatalf("unexpected update count: %d", updateCount)
	}
}

func TestGalleryListOutputsCurrentImages(t *testing.T) {
	server := galleryTestServer(t, galleryServerHooks{})
	defer server.Close()

	ctx, stdout, _ := newGalleryTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := ChannelEntriesGalleryListCmd{
		Channel: "subjects",
		Entry:   "entry-1",
		Field:   "photos",
	}

	if err := cmd.Run(ctx, &RootFlags{APIURL: server.URL}); err != nil {
		t.Fatalf("run: %v", err)
	}

	var rows []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatalf("unmarshal rows: %v\nraw: %s", err, stdout.String())
	}
	if len(rows) != 2 {
		t.Fatalf("rows len = %d", len(rows))
	}
	if rows[0]["id"] != "img-1" || rows[0]["position"] != float64(0) || rows[0]["caption"] != "One" || rows[0]["filename"] != "one.jpg" {
		t.Fatalf("unexpected first row: %#v", rows[0])
	}
}

func TestGalleryAddStartsAfterCurrentMaxPosition(t *testing.T) {
	imagePath := writeGalleryTestFile(t, "new.jpg", "new-image")
	var updateBody map[string]any
	server := galleryTestServer(t, galleryServerHooks{
		onUpdate: func(body map[string]any) {
			updateBody = body
		},
	})
	defer server.Close()

	ctx, _, _ := newGalleryTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := ChannelEntriesGalleryAddCmd{
		Channel: "subjects",
		Entry:   "entry-1",
		Field:   "photos",
		Images:  []string{imagePath},
	}

	if err := cmd.Run(ctx, &RootFlags{APIURL: server.URL}); err != nil {
		t.Fatalf("run: %v", err)
	}

	image := firstGalleryImage(t, updateBody, "photos")
	if image["position"] != float64(2) {
		t.Fatalf("position = %#v, want 2", image["position"])
	}
	if _, ok := image["id"]; ok {
		t.Fatalf("new image should not include id: %#v", image)
	}
}

func TestGalleryRemoveWithForceSendsRemovePatch(t *testing.T) {
	var updateBody map[string]any
	server := galleryTestServer(t, galleryServerHooks{
		onUpdate: func(body map[string]any) {
			updateBody = body
		},
	})
	defer server.Close()

	ctx, _, _ := newGalleryTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := ChannelEntriesGalleryRemoveCmd{
		Channel:  "subjects",
		Entry:    "entry-1",
		Field:    "photos",
		ImageIDs: []string{"img-2"},
	}

	if err := cmd.Run(ctx, &RootFlags{APIURL: server.URL, Force: true}); err != nil {
		t.Fatalf("run: %v", err)
	}

	image := firstGalleryImage(t, updateBody, "photos")
	if image["id"] != "img-2" || image["remove"] != true {
		t.Fatalf("unexpected remove patch: %#v", image)
	}
}

func TestGalleryRemoveRejectsUnknownImageID(t *testing.T) {
	updateCount := 0
	server := galleryTestServer(t, galleryServerHooks{
		onUpdate: func(body map[string]any) {
			updateCount++
		},
	})
	defer server.Close()

	ctx, _, _ := newGalleryTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := ChannelEntriesGalleryRemoveCmd{
		Channel:  "subjects",
		Entry:    "entry-1",
		Field:    "photos",
		ImageIDs: []string{"missing"},
	}

	err := cmd.Run(ctx, &RootFlags{APIURL: server.URL, Force: true})
	if err == nil || !strings.Contains(err.Error(), `unknown image id "missing"`) {
		t.Fatalf("expected unknown image id error, got %v", err)
	}
	if updateCount != 0 {
		t.Fatalf("unexpected update count: %d", updateCount)
	}
}

func TestGalleryReorderSendsPositionPatches(t *testing.T) {
	var updateBody map[string]any
	server := galleryTestServer(t, galleryServerHooks{
		onUpdate: func(body map[string]any) {
			updateBody = body
		},
	})
	defer server.Close()

	ctx, _, _ := newGalleryTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := ChannelEntriesGalleryReorderCmd{
		Channel: "subjects",
		Entry:   "entry-1",
		Field:   "photos",
		Order:   "img-2,img-1",
	}

	if err := cmd.Run(ctx, &RootFlags{APIURL: server.URL}); err != nil {
		t.Fatalf("run: %v", err)
	}

	gallery := updateBody["photos"].(map[string]any)
	images := gallery["images"].([]any)
	if len(images) != 2 {
		t.Fatalf("images len = %d", len(images))
	}
	first := images[0].(map[string]any)
	second := images[1].(map[string]any)
	if first["id"] != "img-2" || first["position"] != float64(0) || second["id"] != "img-1" || second["position"] != float64(1) {
		t.Fatalf("unexpected reorder payload: %#v", images)
	}
}

func TestGalleryRemoveRequiresForce(t *testing.T) {
	ctx, _, _ := newGalleryTestContext(t, "https://api.example.test", output.Mode{JSON: true})
	cmd := ChannelEntriesGalleryRemoveCmd{
		Channel:  "subjects",
		Entry:    "entry-1",
		Field:    "photos",
		ImageIDs: []string{"img-1"},
	}

	err := cmd.Run(ctx, &RootFlags{})
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force error, got %v", err)
	}
}

func TestGalleryReorderRejectsMissingIDs(t *testing.T) {
	server := galleryTestServer(t, galleryServerHooks{})
	defer server.Close()

	ctx, _, _ := newGalleryTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := ChannelEntriesGalleryReorderCmd{
		Channel: "subjects",
		Entry:   "entry-1",
		Field:   "photos",
		Order:   "img-2",
	}

	err := cmd.Run(ctx, &RootFlags{APIURL: server.URL})
	if err == nil || !strings.Contains(err.Error(), "missing image id") {
		t.Fatalf("expected missing image id error, got %v", err)
	}
}

func TestGalleryListRejectsNonGalleryField(t *testing.T) {
	server := galleryTestServer(t, galleryServerHooks{fieldType: "string"})
	defer server.Close()

	ctx, _, _ := newGalleryTestContext(t, server.URL, output.Mode{JSON: true})
	cmd := ChannelEntriesGalleryListCmd{
		Channel: "subjects",
		Entry:   "entry-1",
		Field:   "photos",
	}

	err := cmd.Run(ctx, &RootFlags{APIURL: server.URL})
	if err == nil || !strings.Contains(err.Error(), `field "photos" is type string, expected gallery`) {
		t.Fatalf("expected non-gallery error, got %v", err)
	}
}

type galleryServerHooks struct {
	fieldType string
	onUpdate  func(map[string]any)
}

func galleryTestServer(t *testing.T, hooks galleryServerHooks) *httptest.Server {
	t.Helper()
	fieldType := hooks.fieldType
	if fieldType == "" {
		fieldType = "gallery"
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/channels/subjects/customizations":
			_, _ = w.Write([]byte(`[{"name":"photos","type":"` + fieldType + `"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/channels/subjects/entries/entry-1":
			_, _ = w.Write([]byte(`{"id":"entry-1","photos":{"images":[{"id":"img-1","position":0,"caption":"One","file":{"filename":"one.jpg","url":"https://cdn.test/one.jpg"}},{"id":"img-2","position":1,"caption":"Two","file":{"filename":"two.jpg","url":"https://cdn.test/two.jpg"}}]}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/channels/subjects/entries/entry-1":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode update: %v", err)
			}
			if hooks.onUpdate != nil {
				hooks.onUpdate(body)
			}
			_, _ = w.Write([]byte(`{"id":"entry-1"}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
}

func newGalleryTestContext(t *testing.T, apiURL string, mode output.Mode) (context.Context, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	t.Setenv("NIMBU_TOKEN", "test-token")
	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	ctx = output.WithMode(ctx, mode)
	ctx = output.WithWriter(ctx, &output.Writer{
		Out:   &stdout,
		Err:   &stderr,
		Mode:  mode,
		Color: "never",
		NoTTY: true,
	})
	ctx = context.WithValue(ctx, rootFlagsKey{}, &RootFlags{
		APIURL:  apiURL,
		Site:    "demo",
		Timeout: 2 * time.Second,
	})
	ctx = context.WithValue(ctx, configKey{}, &config.Config{})
	return ctx, &stdout, &stderr
}

func writeGalleryTestFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	return path
}

func decodeGalleryPayload(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v\nraw: %s", err, string(data))
	}
	return payload
}

func firstGalleryImage(t *testing.T, payload map[string]any, field string) map[string]any {
	t.Helper()
	gallery := payload[field].(map[string]any)
	images := gallery["images"].([]any)
	if len(images) != 1 {
		t.Fatalf("images len = %d, want 1", len(images))
	}
	return images[0].(map[string]any)
}

func stringPtr(value string) *string {
	return &value
}
