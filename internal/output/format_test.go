package output

import (
	"bytes"
	"context"
	"testing"
)

func TestFromFlagsRejectsConflict(t *testing.T) {
	_, err := FromFlags(true, true)
	if err == nil {
		t.Fatal("expected error when both --json and --plain are set")
	}
	if _, ok := err.(*ParseError); !ok {
		t.Errorf("expected *ParseError, got %T", err)
	}
}

func TestFromFlagsJSON(t *testing.T) {
	mode, err := FromFlags(true, false)
	if err != nil {
		t.Fatal(err)
	}
	if !mode.JSON {
		t.Error("expected JSON=true")
	}
	if mode.Plain {
		t.Error("expected Plain=false")
	}
}

func TestFromFlagsPlain(t *testing.T) {
	mode, err := FromFlags(false, true)
	if err != nil {
		t.Fatal(err)
	}
	if mode.JSON {
		t.Error("expected JSON=false")
	}
	if !mode.Plain {
		t.Error("expected Plain=true")
	}
}

func TestFromFlagsDefault(t *testing.T) {
	mode, err := FromFlags(false, false)
	if err != nil {
		t.Fatal(err)
	}
	if mode.JSON || mode.Plain {
		t.Error("expected default mode (neither JSON nor Plain)")
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("NIMBU_JSON", "1")
	t.Setenv("NIMBU_PLAIN", "")
	mode := FromEnv()
	if !mode.JSON {
		t.Error("expected JSON=true with NIMBU_JSON=1")
	}
	if mode.Plain {
		t.Error("expected Plain=false")
	}
}

func TestFromEnvPlain(t *testing.T) {
	t.Setenv("NIMBU_JSON", "")
	t.Setenv("NIMBU_PLAIN", "true")
	mode := FromEnv()
	if mode.JSON {
		t.Error("expected JSON=false")
	}
	if !mode.Plain {
		t.Error("expected Plain=true with NIMBU_PLAIN=true")
	}
}

func TestModeHelpers(t *testing.T) {
	tests := []struct {
		name    string
		mode    Mode
		isJSON  bool
		isPlain bool
		isHuman bool
	}{
		{"default", Mode{}, false, false, true},
		{"json", Mode{JSON: true}, true, false, false},
		{"plain", Mode{Plain: true}, false, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := WithMode(context.Background(), tt.mode)
			if got := IsJSON(ctx); got != tt.isJSON {
				t.Errorf("IsJSON=%v, want %v", got, tt.isJSON)
			}
			if got := IsPlain(ctx); got != tt.isPlain {
				t.Errorf("IsPlain=%v, want %v", got, tt.isPlain)
			}
			if got := IsHuman(ctx); got != tt.isHuman {
				t.Errorf("IsHuman=%v, want %v", got, tt.isHuman)
			}
		})
	}
}

func TestWriterFromContextReturnsDefault(t *testing.T) {
	ctx := context.Background()
	w := WriterFromContext(ctx)
	if w == nil {
		t.Fatal("expected non-nil default writer")
	}
	if w.Out == nil {
		t.Error("expected Out to be set")
	}
	if w.Err == nil {
		t.Error("expected Err to be set")
	}
}

func TestWriterUseColor(t *testing.T) {
	tests := []struct {
		color string
		want  bool
	}{
		{"always", true},
		{"never", false},
		{"auto", false}, // NoTTY=true, buffer writer → not a terminal
	}
	for _, tt := range tests {
		t.Run(tt.color, func(t *testing.T) {
			w := &Writer{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}, Color: tt.color, NoTTY: true}
			if got := w.UseColor(); got != tt.want {
				t.Errorf("UseColor(%q)=%v, want %v", tt.color, got, tt.want)
			}
		})
	}
}

func TestWriterIsTTYWithNoTTY(t *testing.T) {
	w := &Writer{Out: &bytes.Buffer{}, NoTTY: true}
	if w.IsTTY() {
		t.Error("expected IsTTY=false with NoTTY=true")
	}
}

func TestWriterIsTTYWithBuffer(t *testing.T) {
	w := &Writer{Out: &bytes.Buffer{}}
	if w.IsTTY() {
		t.Error("expected IsTTY=false for non-file writer")
	}
}
