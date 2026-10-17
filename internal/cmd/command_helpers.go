package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

const maxJSONInputBytes int64 = 10 << 20

var errNoJSONInput = errors.New("no JSON input provided")

const readonlyHint = "unset NIMBU_READONLY or remove --readonly to allow write operations"

func requireWrite(flags *RootFlags, action string) error {
	if flags != nil && flags.Readonly {
		return readonlyModeError(action)
	}
	return nil
}

// readonlyModeError is the typed cli.readonly failure for a write attempted
// with --readonly / NIMBU_READONLY.
func readonlyModeError(action string) error {
	return newHintedError(fmt.Errorf("cannot %s in readonly mode", action), errorReadonly, ExitUsage, readonlyHint)
}

func requireForce(flags *RootFlags, target string) error {
	return requireForceAction(flags, "delete "+target)
}

// requireForceAction guards a destructive action (e.g. "empty channel news")
// behind --force or --yes.
func requireForceAction(flags *RootFlags, action string) error {
	if flags != nil && !confirmedByFlag(flags) {
		return confirmationRequiredError(fmt.Sprintf("refusing to %s without --force", action), "--force")
	}
	return nil
}

// confirmedByFlag reports whether --force or --yes answered a confirmation.
// Server-side force overwrites (theme conflicts, existing files, site copy
// conflicts) read flags.Force alone: --yes must never clobber remote edits.
func confirmedByFlag(flags *RootFlags) bool {
	return flags != nil && (flags.Force || flags.Yes)
}

// forceRequiredError is the cli.confirmation_required failure for an
// overwrite only an explicit --force allows.
func forceRequiredError(message string) error {
	return newHintedError(errors.New(message), errorConfirmationRequired, ExitUsage,
		"re-run with --force to overwrite; --yes does not force overwrites")
}

// confirmationRequiredError is the typed cli.confirmation_required failure
// (exit 2). flag names the override the user must pass.
func confirmationRequiredError(message, flag string) error {
	hint := "re-run with " + flag + " to confirm"
	switch flag {
	case "--force":
		hint = "re-run with --force (or --yes) to confirm"
	case "--yes":
		hint = "re-run with --yes (or --force) to confirm"
	}
	return newHintedError(errors.New(message), errorConfirmationRequired, ExitUsage, hint)
}

// protectedConfirmationError is the confirmation failure for a protected
// environment, which only --yes confirms.
func protectedConfirmationError(message string) error {
	return newHintedError(errors.New(message), errorConfirmationRequired, ExitUsage,
		"re-run with --yes to confirm; --force does not bypass a protected environment")
}

func readJSONInput(file string) (map[string]any, error) {
	value, err := readJSONAnyInput(file)
	if err != nil {
		return nil, err
	}
	body, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("parse JSON: expected an object")
	}
	return body, nil
}

func readJSONAnyInput(file string) (any, error) {
	data, err := readJSONInputBytes(file)
	if err != nil {
		return nil, err
	}
	value, err := decodeJSONAnyUseNumber(data)
	if err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	return value, nil
}

func readJSONInputUseNumber(file string) (map[string]any, error) {
	data, err := readJSONInputBytes(file)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("parse JSON: trailing JSON value")
		}
		return nil, fmt.Errorf("parse JSON: trailing JSON data: %w", err)
	}
	if value == nil {
		return nil, fmt.Errorf("parse JSON: expected an object")
	}
	return value, nil
}

func decodeJSONAnyUseNumber(data []byte) (any, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("trailing JSON value")
		}
		return nil, fmt.Errorf("trailing JSON data: %w", err)
	}
	return value, nil
}

func readJSONInputBytes(file string) ([]byte, error) {
	var (
		data []byte
		err  error
	)

	switch file {
	case "":
		if stdinIsTerminal() {
			return nil, fmt.Errorf("%w; use --file <path> or --file - with piped stdin", errNoJSONInput)
		}
		data, err = readStdinAsPrompt(readLimitedBytes)
	case "-":
		data, err = readStdinAsPrompt(readLimitedBytes)
	default:
		data, err = readLimitedFile(file)
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errNoJSONInput
	}
	return data, nil
}

func readLimitedFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()
	return readLimitedBytes(f)
}

func readLimitedBytes(input io.Reader) ([]byte, error) {
	limited := io.LimitReader(input, maxJSONInputBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	if int64(len(data)) > maxJSONInputBytes {
		return nil, fmt.Errorf("input exceeds %d bytes", maxJSONInputBytes)
	}
	return data, nil
}

// stdinIsTerminal reports whether stdin is an interactive terminal. A
// character-device check is not enough: /dev/null (CI's usual stdin) is a
// character device too, and reading a prompt answer from it yields a bare EOF.
func stdinIsTerminal() bool {
	return fileIsTerminal(os.Stdin)
}

func fileIsTerminal(f *os.File) bool {
	return f != nil && term.IsTerminal(int(f.Fd()))
}
