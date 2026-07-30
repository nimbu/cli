package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

const maxJSONInputBytes int64 = 10 << 20

var errNoJSONInput = errors.New("no JSON input provided")

func requireWrite(flags *RootFlags, action string) error {
	if flags != nil && flags.Readonly {
		return fmt.Errorf("cannot %s in readonly mode", action)
	}
	return nil
}

func requireForce(flags *RootFlags, target string) error {
	if flags != nil && !flags.Force {
		return fmt.Errorf("use --force to confirm deletion of %s", target)
	}
	return nil
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
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
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

func readJSONInputBytes(file string) ([]byte, error) {
	var input io.Reader

	switch file {
	case "":
		if stdinIsTerminal() {
			return nil, fmt.Errorf("%w; use --file <path> or --file - with piped stdin", errNoJSONInput)
		}
		input = os.Stdin
	case "-":
		input = os.Stdin
	default:
		f, err := os.Open(file)
		if err != nil {
			return nil, fmt.Errorf("open file: %w", err)
		}
		defer func() { _ = f.Close() }()
		input = f
	}

	limited := io.LimitReader(input, maxJSONInputBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	if int64(len(data)) > maxJSONInputBytes {
		return nil, fmt.Errorf("input exceeds %d bytes", maxJSONInputBytes)
	}
	if len(data) == 0 {
		return nil, errNoJSONInput
	}
	return data, nil
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
