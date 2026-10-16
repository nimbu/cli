package output

import (
	"bytes"
	"testing"

	"github.com/muesli/termenv"
)

func TestColorEnabledPrecedence(t *testing.T) {
	tests := []struct {
		name string
		mode string
		tty  bool
		env  map[string]string
		want bool
	}{
		{name: "auto tty", mode: "auto", tty: true, want: true},
		{name: "auto pipe", mode: "auto", tty: false, want: false},
		{name: "always beats NO_COLOR", mode: "always", env: map[string]string{"NO_COLOR": "1"}, want: true},
		{name: "never beats tty", mode: "never", tty: true, want: false},
		{name: "NO_COLOR disables tty", mode: "auto", tty: true, env: map[string]string{"NO_COLOR": "1"}, want: false},
		{name: "empty NO_COLOR ignored", mode: "auto", tty: true, env: map[string]string{"NO_COLOR": ""}, want: true},
		{name: "TERM dumb disables", mode: "auto", tty: true, env: map[string]string{"TERM": "dumb"}, want: false},
		{name: "FORCE_COLOR does not color pipes", mode: "auto", env: map[string]string{"FORCE_COLOR": "3"}, want: false},
		{name: "FORCE_COLOR=0 disables tty", mode: "auto", tty: true, env: map[string]string{"FORCE_COLOR": "0"}, want: false},
		{name: "CLICOLOR_FORCE does not color pipes", mode: "auto", env: map[string]string{"CLICOLOR_FORCE": "1"}, want: false},
		{name: "NO_COLOR beats FORCE_COLOR on tty", mode: "auto", tty: true, env: map[string]string{"NO_COLOR": "1", "FORCE_COLOR": "1"}, want: false},
		{name: "empty mode is auto", mode: "", tty: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(k string) string { return tt.env[k] }
			if got := colorEnabled(tt.mode, tt.tty, getenv); got != tt.want {
				t.Fatalf("colorEnabled(%q, %v) = %v, want %v", tt.mode, tt.tty, got, tt.want)
			}
		})
	}
}

func TestWriterColorProfileFollowsStream(t *testing.T) {
	w := &Writer{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}, Color: "never"}
	if got := w.ErrColorProfile(); got != termenv.Ascii {
		t.Fatalf("never: ErrColorProfile = %v, want Ascii", got)
	}
	w.Color = "always"
	if got := w.ErrColorProfile(); got == termenv.Ascii {
		t.Fatalf("always: ErrColorProfile = Ascii, want a color profile")
	}
	if !w.ErrUseColor() || !w.UseColor() {
		t.Fatalf("always: expected both streams colored")
	}
}
