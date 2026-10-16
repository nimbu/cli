package cmd

import (
	"bytes"
	"strings"
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

func TestDecideServerShortcutKeyTable(t *testing.T) {
	links := serverShortcutLinks{
		DevURL:   "http://localhost:3456",
		LiveURL:  "https://demo.nimbu.io",
		AdminURL: "https://demo.nimbu.io/admin",
	}
	tests := []struct {
		key         byte
		ready       bool
		wantAction  serverShortcutAction
		wantPending bool
	}{
		{key: '?', ready: false, wantAction: serverShortcutHelp},
		{key: 'h', ready: false, wantAction: serverShortcutHelp},
		{key: 'H', ready: true, wantAction: serverShortcutHelp},
		{key: 'q', ready: false, wantAction: serverShortcutQuit},
		{key: 'Q', ready: true, wantAction: serverShortcutQuit},
		{key: '\r', ready: false, wantAction: serverShortcutLogMarker},
		{key: 'o', ready: false, wantAction: serverShortcutOpenDev, wantPending: true},
		{key: 'o', ready: true, wantAction: serverShortcutOpenDev},
		{key: 'l', ready: true, wantAction: serverShortcutOpenLive},
		{key: 'b', ready: true, wantAction: serverShortcutOpenAdmin},
		{key: 'x', ready: true, wantAction: serverShortcutNone},
		{key: ' ', ready: true, wantAction: serverShortcutNone},
	}
	for _, tc := range tests {
		got := decideServerShortcut(tc.key, tc.ready, links)
		if got.Action != tc.wantAction || got.Pending != tc.wantPending {
			t.Errorf("key %q ready=%v: got %+v, want action %v pending %v", tc.key, tc.ready, got, tc.wantAction, tc.wantPending)
		}
	}
}

func TestServerShortcutHint(t *testing.T) {
	if got := (serverShortcutLinks{DevURL: "http://localhost:3456"}).Hint(); got != "" {
		t.Fatalf("inactive listener hint = %q, want empty", got)
	}
	if got, want := (serverShortcutLinks{Active: true}).Hint(), "[enter] mark  [?] help  [q] quit"; got != want {
		t.Fatalf("hint without links = %q, want %q", got, want)
	}
	links := serverShortcutLinks{Active: true, DevURL: "http://localhost:3456", LiveURL: "https://demo.nimbu.io", AdminURL: "https://demo.nimbu.io/admin"}
	want := "[o] open dev  [l] open live  [b] open admin  [enter] mark  [?] help  [q] quit"
	if got := links.Hint(); got != want {
		t.Fatalf("hint = %q, want %q", got, want)
	}
}

func TestServerShortcutHelpRowsDocumentEveryKey(t *testing.T) {
	rows := (serverShortcutLinks{LiveURL: "https://demo.nimbu.io", AdminURL: "https://demo.nimbu.io/admin"}).HelpRows()
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, row[0])
	}
	if got, want := strings.Join(keys, ","), "l,b,enter,? h,q"; got != want {
		t.Fatalf("help keys = %q, want %q", got, want)
	}
	if !strings.Contains(rows[0][1], "https://demo.nimbu.io") {
		t.Fatalf("live row should name its URL: %q", rows[0][1])
	}
}

func newFakeModeListener(applied, restored *int) *serverShortcutListener {
	return &serverShortcutListener{mode: serverTermMode{
		apply:   func() error { *applied++; return nil },
		restore: func() error { *restored++; return nil },
	}}
}

func TestServerShortcutListenerSuspendResume(t *testing.T) {
	var applied, restored int
	l := newFakeModeListener(&applied, &restored)

	if err := l.Suspend(); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := l.Resume(); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if applied != 1 || restored != 1 {
		t.Fatalf("applied=%d restored=%d, want 1/1", applied, restored)
	}

	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if restored != 2 {
		t.Fatalf("close should restore, restored=%d", restored)
	}
	// After Close the terminal belongs to the shell again: never re-apply.
	_ = l.Resume()
	_ = l.Suspend()
	if applied != 1 || restored != 2 {
		t.Fatalf("mode touched after close: applied=%d restored=%d", applied, restored)
	}
}

func TestServerShortcutListenerRestoresTerminalOnPanic(t *testing.T) {
	var applied, restored int
	l := newFakeModeListener(&applied, &restored)

	recovered := func() (r any) {
		defer func() { r = recover() }()
		func() {
			defer l.RestoreOnPanic()
			panic("boom")
		}()
		return nil
	}()

	if recovered != "boom" {
		t.Fatalf("panic should propagate after restore, got %v", recovered)
	}
	if restored != 1 {
		t.Fatalf("terminal not restored on panic, restored=%d", restored)
	}
}

func TestServerShortcutListenerRestoreOnPanicNilSafe(t *testing.T) {
	var l *serverShortcutListener
	func() {
		defer l.RestoreOnPanic()
	}()
	if err := l.Suspend(); err != nil {
		t.Fatalf("nil suspend: %v", err)
	}
	if err := l.Resume(); err != nil {
		t.Fatalf("nil resume: %v", err)
	}
}
