package cmd

import (
	"bytes"
	"testing"
)

func TestServerShortcutLinksFromSummary(t *testing.T) {
	summary := serverSummary{
		ReadyURL: "http://127.0.0.1:3456",
		SiteHost: "demo.nimbu.be",
	}

	links := serverShortcutLinksFromSummary(summary)

	if links.DevURL != "http://localhost:3456" {
		t.Fatalf("unexpected dev url: %q", links.DevURL)
	}
	if links.LiveURL != "https://demo.nimbu.be" {
		t.Fatalf("unexpected live url: %q", links.LiveURL)
	}
	if links.AdminURL != "https://demo.nimbu.be/admin" {
		t.Fatalf("unexpected admin url: %q", links.AdminURL)
	}
}

func TestDecideServerShortcutBeforeReady(t *testing.T) {
	decision := decideServerShortcut('l', false, serverShortcutLinks{
		LiveURL:  "https://demo.nimbu.io",
		AdminURL: "https://demo.nimbu.io/admin",
	})

	if !decision.Pending {
		t.Fatal("expected shortcut to be pending before ready")
	}
	if decision.Action != serverShortcutOpenLive {
		t.Fatalf("unexpected action: %v", decision.Action)
	}
}

func TestDecideServerShortcutAfterReady(t *testing.T) {
	decision := decideServerShortcut('b', true, serverShortcutLinks{
		LiveURL:  "https://demo.nimbu.io",
		AdminURL: "https://demo.nimbu.io/admin",
	})

	if decision.Pending {
		t.Fatal("did not expect pending shortcut after ready")
	}
	if decision.Action != serverShortcutOpenAdmin {
		t.Fatalf("unexpected action: %v", decision.Action)
	}
}

func TestDecideServerShortcutIgnoresUnavailableDevURL(t *testing.T) {
	decision := decideServerShortcut('o', true, serverShortcutLinks{
		LiveURL:  "https://demo.nimbu.io",
		AdminURL: "https://demo.nimbu.io/admin",
	})

	if decision.Action != serverShortcutNone {
		t.Fatalf("expected no action, got: %v", decision.Action)
	}
	if decision.Pending {
		t.Fatal("did not expect pending shortcut for unavailable target")
	}
}

func TestDecideServerShortcutUppercase(t *testing.T) {
	decision := decideServerShortcut('L', true, serverShortcutLinks{
		LiveURL:  "https://demo.nimbu.io",
		AdminURL: "https://demo.nimbu.io/admin",
	})

	if decision.Action != serverShortcutOpenLive {
		t.Fatalf("unexpected action: %v", decision.Action)
	}
	if decision.Pending {
		t.Fatal("did not expect pending shortcut after ready")
	}
}

func TestDecideServerShortcutMapsEnterToLogMarker(t *testing.T) {
	tests := []byte{'\n', '\r'}

	for _, key := range tests {
		decision := decideServerShortcut(key, false, serverShortcutLinks{
			LiveURL:  "https://demo.nimbu.io",
			AdminURL: "https://demo.nimbu.io/admin",
		})

		if decision.Action != serverShortcutLogMarker {
			t.Fatalf("key %q action = %v, want %v", key, decision.Action, serverShortcutLogMarker)
		}
		if decision.Pending {
			t.Fatalf("key %q should not be pending", key)
		}
	}
}

func TestWriteServerShortcutLogMarker(t *testing.T) {
	var buf bytes.Buffer

	if err := writeServerShortcutLogMarker(&buf); err != nil {
		t.Fatalf("writeServerShortcutLogMarker returned error: %v", err)
	}
	if got := buf.String(); got != "\n" {
		t.Fatalf("marker output = %q, want %q", got, "\n")
	}
}

func TestServerBrowserCommand(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		url      string
		wantCmd  string
		wantArgs []string
	}{
		{
			name:     "darwin",
			goos:     "darwin",
			url:      "https://demo.nimbu.io",
			wantCmd:  "open",
			wantArgs: []string{"https://demo.nimbu.io"},
		},
		{
			name:     "linux",
			goos:     "linux",
			url:      "https://demo.nimbu.io",
			wantCmd:  "xdg-open",
			wantArgs: []string{"https://demo.nimbu.io"},
		},
		{
			name:     "windows",
			goos:     "windows",
			url:      "https://demo.nimbu.io",
			wantCmd:  "rundll32",
			wantArgs: []string{"url.dll,FileProtocolHandler", "https://demo.nimbu.io"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, args, err := serverBrowserCommand(tc.goos, tc.url)
			if err != nil {
				t.Fatalf("serverBrowserCommand returned error: %v", err)
			}
			if cmd != tc.wantCmd {
				t.Fatalf("command = %q, want %q", cmd, tc.wantCmd)
			}
			if len(args) != len(tc.wantArgs) {
				t.Fatalf("args len = %d, want %d", len(args), len(tc.wantArgs))
			}
			for i := range args {
				if args[i] != tc.wantArgs[i] {
					t.Fatalf("arg[%d] = %q, want %q", i, args[i], tc.wantArgs[i])
				}
			}
		})
	}
}
