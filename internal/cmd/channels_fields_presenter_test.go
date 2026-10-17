package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/output"
)

// Columns are padded by display width: a label with accents, CJK or emoji
// must not push the type column out of line (len() counts bytes).
func TestChannelFieldPresenterAlignsWideLabels(t *testing.T) {
	var stdout bytes.Buffer
	ctx := output.WithWriter(context.Background(), &output.Writer{Out: &stdout, Err: &bytes.Buffer{}, Color: "never"})

	err := newChannelFieldPresenter(ctx).Render(customFieldSchemaView{
		Key: "people",
		Fields: []api.CustomField{
			{Name: "first_name", Label: "Prénom", Type: "string"},
			{Name: "kanji", Label: "名前", Type: "string"},
			{Name: "mood", Label: "Mood 🙂", Type: "string"},
			{Name: "age", Label: "Age", Type: "integer"},
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	typeColumn := -1
	rows := 0
	for _, line := range strings.Split(stdout.String(), "\n") {
		idx := strings.Index(line, "string")
		if idx < 0 {
			idx = strings.Index(line, "integer")
		}
		if idx < 0 || !strings.HasPrefix(line, "  ") {
			continue
		}
		col := ansi.StringWidth(line[:idx])
		if typeColumn == -1 {
			typeColumn = col
		}
		if col != typeColumn {
			t.Fatalf("type column at cell %d, want %d:\n%s", col, typeColumn, stdout.String())
		}
		rows++
	}
	if rows != 4 {
		t.Fatalf("expected 4 field rows, got %d:\n%s", rows, stdout.String())
	}
}

func TestChannelFieldPresenterPadRightUsesDisplayWidth(t *testing.T) {
	p := &channelFieldPresenter{}
	for _, value := range []string{"Prénom", "名前", "abc"} {
		if got := ansi.StringWidth(p.padRight(value, 8)); got != 8 {
			t.Fatalf("padRight(%q, 8) is %d cells wide", value, got)
		}
	}
	if got := p.padRight("名前名前名", 4); got != "名前名前名" {
		t.Fatalf("wider values must not be cut, got %q", got)
	}
}
