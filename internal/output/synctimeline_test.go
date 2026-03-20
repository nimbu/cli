package output

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// newTestTimeline creates a non-animated SyncTimeline writing to a buffer.
func newTestTimeline(mode, theme string, dryRun bool) (*SyncTimeline, *bytes.Buffer) {
	var buf bytes.Buffer
	ctx := context.Background()
	ctx = WithWriter(ctx, &Writer{
		Out:   &buf,
		Err:   &buf,
		NoTTY: true,
		Color: "never",
	})
	tl := NewSyncTimeline(ctx, mode, theme, dryRun)
	return tl, &buf
}

func TestSyncTimeline_HappyPush(t *testing.T) {
	tl, buf := newTestTimeline("push", "default", false)
	tl.SetCategories([]SyncCategory{
		{Label: "layouts", Count: 2},
		{Label: "snippets", Count: 3},
		{Label: "templates", Count: 4},
		{Label: "assets", Count: 1},
	})
	tl.Header()

	// Upload layouts
	tl.StartCategory(0)
	tl.SetActiveFile("layouts/blank.liquid")
	tl.FileUploaded()
	tl.SetActiveFile("layouts/default.liquid")
	tl.FileUploaded()
	tl.CategoryDone(0)

	// Upload snippets
	tl.StartCategory(1)
	for i := 0; i < 3; i++ {
		tl.FileUploaded()
	}
	tl.CategoryDone(1)

	// Upload templates
	tl.StartCategory(2)
	for i := 0; i < 4; i++ {
		tl.FileUploaded()
	}
	tl.CategoryDone(2)

	// Upload assets
	tl.StartCategory(3)
	tl.FileUploaded()
	tl.CategoryDone(3)

	// Manually set started to control elapsed time
	tl.started = time.Now().Add(-5 * time.Second)
	tl.Footer()

	output := buf.String()
	assertContains(t, output, `push theme "default"`)
	assertContains(t, output, "layouts: 2 uploaded")
	assertContains(t, output, "snippets: 3 uploaded")
	assertContains(t, output, "templates: 4 uploaded")
	assertContains(t, output, "assets: 1 uploaded")
	assertContains(t, output, "done: 10 files in")
}

func TestSyncTimeline_SyncWithDeletes(t *testing.T) {
	tl, buf := newTestTimeline("sync", "default", false)
	tl.SetCategories([]SyncCategory{
		{Label: "templates", Count: 2},
	})
	tl.Header()

	tl.StartCategory(0)
	tl.FileUploaded()
	tl.FileUploaded()
	tl.CategoryDone(0)

	tl.StartDeletes(3)
	tl.SetActiveDelete("templates/old.liquid")
	tl.FileDeleted()
	tl.SetActiveDelete("templates/unused.liquid")
	tl.FileDeleted()
	tl.SetActiveDelete("templates/dead.liquid")
	tl.FileDeleted()
	tl.DeletesDone()

	tl.Footer()

	output := buf.String()
	assertContains(t, output, `sync theme "default"`)
	assertContains(t, output, "templates: 2 uploaded")
	assertContains(t, output, "deleted: 3 files")
	assertContains(t, output, "2 uploads, 3 deletes in")
}

func TestSyncTimeline_UploadError(t *testing.T) {
	tl, buf := newTestTimeline("push", "default", false)
	tl.SetCategories([]SyncCategory{
		{Label: "layouts", Count: 2},
		{Label: "snippets", Count: 3},
		{Label: "templates", Count: 22},
		{Label: "assets", Count: 20},
	})
	tl.Header()

	tl.StartCategory(0)
	tl.FileUploaded()
	tl.FileUploaded()
	tl.CategoryDone(0)

	tl.StartCategory(1)
	tl.FileUploaded()
	tl.FileUploaded()
	tl.FileUploaded()
	tl.CategoryDone(1)

	tl.StartCategory(2)
	for i := 0; i < 14; i++ {
		tl.FileUploaded()
	}
	tl.FileFailed("templates/shop/product.liquid", "422 Unprocessable Entity — invalid liquid syntax")
	tl.ErrorFooter()

	output := buf.String()
	assertContains(t, output, "layouts: 2 uploaded")
	assertContains(t, output, "snippets: 3 uploaded")
	assertContains(t, output, "templates: failed at templates/shop/product.liquid (14/22)")
	assertContains(t, output, "422 Unprocessable Entity")
	assertContains(t, output, "19 uploaded, 1 failed, 28 skipped")
}

func TestSyncTimeline_DeleteError(t *testing.T) {
	tl, buf := newTestTimeline("sync", "default", false)
	tl.SetCategories([]SyncCategory{
		{Label: "templates", Count: 2},
	})
	tl.Header()

	tl.StartCategory(0)
	tl.FileUploaded()
	tl.FileUploaded()
	tl.CategoryDone(0)

	tl.StartDeletes(3)
	tl.FileDeleted()
	tl.DeleteFailed("templates/old.liquid", "404 Not Found")
	tl.ErrorFooter()

	output := buf.String()
	assertContains(t, output, "deleting: failed at templates/old.liquid")
	assertContains(t, output, "404 Not Found")
	assertContains(t, output, "2 uploaded, 1 failed, 2 skipped")
}

func TestSyncTimeline_DryRun(t *testing.T) {
	tl, buf := newTestTimeline("push", "default", true)
	tl.RenderPlan([]SyncCategory{
		{Label: "layouts", Count: 2},
		{Label: "snippets", Count: 18},
		{Label: "templates", Count: 22},
		{Label: "assets", Count: 20},
	}, 0)

	output := buf.String()
	assertContains(t, output, `push theme "default" [dry-run]`)
	assertContains(t, output, "layouts: 2 would upload")
	assertContains(t, output, "snippets: 18 would upload")
	assertContains(t, output, "templates: 22 would upload")
	assertContains(t, output, "assets: 20 would upload")
	assertContains(t, output, "dry run complete: 62 uploads, 0 deletes")
}

func TestSyncTimeline_DryRunWithDeletes(t *testing.T) {
	tl, buf := newTestTimeline("sync", "default", true)
	tl.RenderPlan([]SyncCategory{
		{Label: "templates", Count: 5},
	}, 3)

	output := buf.String()
	assertContains(t, output, `sync theme "default" [dry-run]`)
	assertContains(t, output, "templates: 5 would upload")
	assertContains(t, output, "deletes: 3 would delete")
	assertContains(t, output, "dry run complete: 5 uploads, 3 deletes")
}

func TestSyncTimeline_ZeroFiles(t *testing.T) {
	tl, buf := newTestTimeline("push", "default", false)
	tl.NothingToDo()

	output := buf.String()
	assertContains(t, output, `push theme "default"`)
	assertContains(t, output, "nothing to push")
}

func TestSyncTimeline_ZeroFilesSync(t *testing.T) {
	tl, buf := newTestTimeline("sync", "default", false)
	tl.NothingToDo()

	output := buf.String()
	assertContains(t, output, "nothing to sync")
}

func TestSyncTimeline_SingleFile(t *testing.T) {
	tl, buf := newTestTimeline("push", "default", false)
	tl.SetCategories([]SyncCategory{
		{Label: "templates", Count: 1},
	})
	tl.Header()
	tl.StartCategory(0)
	tl.FileUploaded()
	tl.CategoryDone(0)
	tl.Footer()

	output := buf.String()
	assertContains(t, output, "templates: 1 uploaded")
	assertContains(t, output, "done: 1 file in")
}

func TestSyncTimeline_EmptyCategoriesOmitted(t *testing.T) {
	// Only liquid files, no assets
	tl, buf := newTestTimeline("push", "default", false)
	tl.SetCategories([]SyncCategory{
		{Label: "layouts", Count: 2},
		{Label: "templates", Count: 3},
	})
	tl.Header()

	tl.StartCategory(0)
	tl.FileUploaded()
	tl.FileUploaded()
	tl.CategoryDone(0)

	tl.StartCategory(1)
	tl.FileUploaded()
	tl.FileUploaded()
	tl.FileUploaded()
	tl.CategoryDone(1)

	tl.Footer()

	output := buf.String()
	assertContains(t, output, "layouts: 2 uploaded")
	assertContains(t, output, "templates: 3 uploaded")
	assertNotContains(t, output, "assets")
	assertNotContains(t, output, "snippets")
}

func TestSyncTimeline_ElapsedPrecision(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"sub-second", 200 * time.Millisecond, "0.2s"},
		{"few seconds", 4200 * time.Millisecond, "4.2s"},
		{"boundary", 9900 * time.Millisecond, "9.9s"},
		{"ten seconds", 10 * time.Second, "10s"},
		{"over minute", 65 * time.Second, "1m05s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatElapsedPrecise(tt.duration)
			if got != tt.want {
				t.Errorf("formatElapsedPrecise(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}

func TestSyncTimeline_Plural(t *testing.T) {
	tests := []struct {
		n    int
		s, p string
		want string
	}{
		{0, "file", "files", "files"},
		{1, "file", "files", "file"},
		{2, "file", "files", "files"},
		{1, "upload", "uploads", "upload"},
		{5, "upload", "uploads", "uploads"},
	}

	for _, tt := range tests {
		got := plural(tt.n, tt.s, tt.p)
		if got != tt.want {
			t.Errorf("plural(%d, %q, %q) = %q, want %q", tt.n, tt.s, tt.p, got, tt.want)
		}
	}
}

func TestSyncTimeline_FooterIdempotent(t *testing.T) {
	tl, buf := newTestTimeline("push", "default", false)
	tl.SetCategories([]SyncCategory{{Label: "layouts", Count: 1}})
	tl.Header()
	tl.StartCategory(0)
	tl.FileUploaded()
	tl.CategoryDone(0)
	tl.Footer()

	beforeSecondFooter := buf.String()
	tl.Footer() // second call should be no-op
	afterSecondFooter := buf.String()

	if beforeSecondFooter != afterSecondFooter {
		t.Error("Footer() is not idempotent — second call produced additional output")
	}
}

func TestSyncTimeline_ErrorFooterIdempotent(t *testing.T) {
	tl, buf := newTestTimeline("push", "default", false)
	tl.SetCategories([]SyncCategory{{Label: "layouts", Count: 2}})
	tl.Header()
	tl.StartCategory(0)
	tl.FileUploaded()
	tl.FileFailed("layouts/x.liquid", "500 Internal Server Error")
	tl.ErrorFooter()

	before := buf.String()
	tl.ErrorFooter() // second call should be no-op
	after := buf.String()

	if before != after {
		t.Error("ErrorFooter() is not idempotent")
	}
}

func TestSyncTimeline_Context(t *testing.T) {
	ctx := context.Background()
	if SyncTimelineFromContext(ctx) != nil {
		t.Error("SyncTimelineFromContext should return nil for empty context")
	}

	tl, _ := newTestTimeline("push", "test", false)
	ctx = WithSyncTimeline(ctx, tl)
	got := SyncTimelineFromContext(ctx)
	if got != tl {
		t.Error("SyncTimelineFromContext should return the stored timeline")
	}
}

// --- helpers ---

func assertContains(t *testing.T, output, substr string) {
	t.Helper()
	if !strings.Contains(output, substr) {
		t.Errorf("output should contain %q, got:\n%s", substr, output)
	}
}

func assertNotContains(t *testing.T, output, substr string) {
	t.Helper()
	if strings.Contains(output, substr) {
		t.Errorf("output should NOT contain %q, got:\n%s", substr, output)
	}
}
