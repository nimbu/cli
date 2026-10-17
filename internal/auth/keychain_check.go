package auth

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ErrNoDefaultKeychain means macOS has no default keychain for this user, as
// with a scratch HOME or a bare CI image. The Keychain backend would then
// block on a "keychain not found" dialog instead of returning an error.
var ErrNoDefaultKeychain = errors.New("no default macOS keychain")

const securityTool = "/usr/bin/security"

// defaultKeychainPath asks macOS for the user's default keychain without
// touching it, so it never shows a dialog; swapped in tests.
var defaultKeychainPath = func() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, securityTool, "default-keychain", "-d", "user").Output()
	if ctxErr := ctx.Err(); ctxErr != nil {
		// A killed process is an ExitError too; a hang says nothing about
		// whether the keychain exists.
		return "", ctxErr
	}
	return strings.Trim(strings.TrimSpace(string(out)), `"`), err
}

// checkDefaultKeychain fails fast when backend would use the macOS Keychain
// but the user has no default keychain to store items in.
func checkDefaultKeychain(goos, backend string) error {
	if goos != "darwin" || (backend != "auto" && backend != "keychain") {
		return nil
	}
	path, err := defaultKeychainPath()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return nil // could not ask; let the backend report its own error
		}
		return noDefaultKeychainError()
	}
	if path == "" || (!fileExists(path) && !fileExists(path+"-db")) {
		return noDefaultKeychainError()
	}
	return nil
}

func noDefaultKeychainError() error {
	home, _ := os.UserHomeDir()
	return fmt.Errorf("%w for HOME=%s", ErrNoDefaultKeychain, home)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !errors.Is(err, fs.ErrNotExist)
}
