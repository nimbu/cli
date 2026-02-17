package cmd

import (
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

	body := map[string]any{}
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	return body, nil
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func wrapParamsBody(body map[string]any) map[string]any {
	if params, ok := body["params"]; ok && len(body) == 1 {
		return map[string]any{"params": params}
	}
	return map[string]any{"params": body}
}
