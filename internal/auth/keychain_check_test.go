package auth

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func stubDefaultKeychainPath(t *testing.T, path string, err error) *int {
	t.Helper()
	calls := 0
	old := defaultKeychainPath
	defaultKeychainPath = func() (string, error) {
		calls++
		return path, err
	}
	t.Cleanup(func() { defaultKeychainPath = old })
	return &calls
}

func TestCheckDefaultKeychain(t *testing.T) {
	dir := t.TempDir()
	login := filepath.Join(dir, "login.keychain-db")
	if err := os.WriteFile(login, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		goos    string
		backend string
		path    string
		err     error
		wantErr bool
	}{
		{name: "existing keychain", goos: "darwin", backend: "auto", path: login},
		{name: "explicit keychain backend", goos: "darwin", backend: "keychain", path: login},
		{name: "legacy name resolves to -db file", goos: "darwin", backend: "auto", path: filepath.Join(dir, "login.keychain")},
		{name: "security reports none", goos: "darwin", backend: "auto", err: &exec.ExitError{}, wantErr: true},
		{name: "default points at a deleted file", goos: "darwin", backend: "auto", path: filepath.Join(dir, "gone.keychain-db"), wantErr: true},
		{name: "empty answer", goos: "darwin", backend: "auto", wantErr: true},
		{name: "security tool unusable", goos: "darwin", backend: "auto", err: exec.ErrNotFound},
		{name: "security timed out", goos: "darwin", backend: "auto", err: context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubDefaultKeychainPath(t, tt.path, tt.err)
			err := checkDefaultKeychain(tt.goos, tt.backend)
			if got := errors.Is(err, ErrNoDefaultKeychain); got != tt.wantErr {
				t.Fatalf("checkDefaultKeychain() = %v, want ErrNoDefaultKeychain: %v", err, tt.wantErr)
			}
		})
	}
}

// Other platforms and explicit non-Keychain backends never ask macOS.
func TestCheckDefaultKeychainSkipsOtherBackends(t *testing.T) {
	calls := stubDefaultKeychainPath(t, "", &exec.ExitError{})
	for _, tc := range []struct{ goos, backend string }{
		{"darwin", "file"},
		{"linux", "auto"},
		{"windows", "auto"},
	} {
		if err := checkDefaultKeychain(tc.goos, tc.backend); err != nil {
			t.Fatalf("%s/%s: unexpected error %v", tc.goos, tc.backend, err)
		}
	}
	if *calls != 0 {
		t.Fatalf("asked macOS %d times, want 0", *calls)
	}
}
