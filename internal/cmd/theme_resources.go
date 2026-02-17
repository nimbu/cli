package cmd

import (
	"fmt"
	"io"
	"os"
)

func readThemeContent(file string, inline string) ([]byte, error) {
	switch {
	case inline != "":
		return []byte(inline), nil
	case file != "":
		f, err := os.Open(file)
		if err != nil {
			return nil, fmt.Errorf("open file: %w", err)
		}
		defer func() { _ = f.Close() }()

		content, err := io.ReadAll(f)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}
		return content, nil
	default:
		content, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return content, nil
	}
}
