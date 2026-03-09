package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muesli/termenv"
	"golang.org/x/term"

	"github.com/nimbu/cli/internal/config"
	"github.com/nimbu/cli/internal/output"
)

// ConfigBannerCmd lets users pick a banner theme interactively.
type ConfigBannerCmd struct{}

func (c *ConfigBannerCmd) Run(ctx context.Context) error {
	writer := output.WriterFromContext(ctx)
	if !writer.UseColor() {
		return fmt.Errorf("interactive banner picker requires a color terminal; use 'nimbu-cli config set banner_theme <name>' instead\navailable themes: %s", strings.Join(BannerThemeNames(), ", "))
	}

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("interactive banner picker requires a terminal; use 'nimbu-cli config set banner_theme <name>' instead")
	}

	cfg, err := config.Read()
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	themes := BannerThemes()
	selected := 0
	if cfg.BannerTheme != "" {
		for i, t := range themes {
			if t.Name == cfg.BannerTheme {
				selected = i
				break
			}
		}
	}

	out := writer.Err
	if out == nil {
		out = os.Stderr
	}
	termOut := termenv.NewOutput(out, termenv.WithProfile(termenv.TrueColor))
	bannerLines := strings.Split(strings.Trim(serverBanner, "\n"), "\n")
	pickerHeight := bannerPickerLines(bannerLines)

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("enable raw mode: %w", err)
	}

	restore := func() {
		_, _ = fmt.Fprint(out, "\033[?25h") // show cursor
		_ = term.Restore(fd, oldState)
	}
	defer restore()

	// Hide cursor
	_, _ = fmt.Fprint(out, "\033[?25l")

	renderBannerPicker(out, termOut, themes, bannerLines, selected, true, pickerHeight)

	var buf [3]byte
	for {
		n, err := os.Stdin.Read(buf[:])
		if err != nil {
			return nil
		}
		if n == 0 {
			continue
		}

		// Esc (0x1b) is also the first byte of arrow key sequences.
		// If we got only 1 byte and it's 0x1b, wait briefly for more
		// bytes to arrive before treating it as a standalone Esc press.
		if n == 1 && buf[0] == 0x1b {
			if extra, ok := readEscapeTrail(fd); ok {
				// Reassemble as if we read all bytes at once.
				n = 1 + copy(buf[1:], extra)
			}
		}

		switch {
		case n == 1 && buf[0] == 'q':
			clearBannerPickerLines(out, pickerHeight)
			_, _ = fmt.Fprint(out, "Cancelled.\r\n")
			return nil

		case n == 1 && buf[0] == 0x1b:
			// Standalone Esc (no trailing bytes within timeout) — cancel
			clearBannerPickerLines(out, pickerHeight)
			_, _ = fmt.Fprint(out, "Cancelled.\r\n")
			return nil

		case n == 1 && (buf[0] == '\r' || buf[0] == '\n'):
			clearBannerPickerLines(out, pickerHeight)
			theme := themes[selected]
			// Restore terminal before writing config so any error
			// message from the caller prints in cooked mode.
			restore()
			if err := cfg.Set("banner_theme", theme.Name); err != nil {
				return err
			}
			if err := config.Write(cfg); err != nil {
				return fmt.Errorf("write config: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Banner theme set to %s.\n", theme.Name)
			return nil

		case n == 3 && buf[0] == 0x1b && buf[1] == '[' && buf[2] == 'A',
			n == 1 && buf[0] == 'k':
			if selected > 0 {
				selected--
				renderBannerPicker(out, termOut, themes, bannerLines, selected, false, pickerHeight)
			}

		case n == 3 && buf[0] == 0x1b && buf[1] == '[' && buf[2] == 'B',
			n == 1 && buf[0] == 'j':
			if selected < len(themes)-1 {
				selected++
				renderBannerPicker(out, termOut, themes, bannerLines, selected, false, pickerHeight)
			}
		}
	}
}

// readEscapeTrail tries to read the trailing bytes of an escape sequence
// (e.g. "[A" for arrow-up) within a short window. Returns the extra bytes
// and true if any were read, or nil and false on timeout / error.
func readEscapeTrail(fd int) ([]byte, bool) {
	// Set a short read deadline so we don't block forever.
	// On most terminals arrow keys arrive as 3 bytes in one shot,
	// but some slow connections may split them.
	deadline := time.After(50 * time.Millisecond)
	ch := make(chan []byte, 1)
	go func() {
		var extra [2]byte
		n, err := os.Stdin.Read(extra[:])
		if err != nil || n == 0 {
			ch <- nil
			return
		}
		ch <- extra[:n]
	}()
	select {
	case data := <-ch:
		if len(data) == 0 {
			return nil, false
		}
		return data, true
	case <-deadline:
		return nil, false
	}
}

// bannerPickerLines returns total lines rendered by the picker.
// header + theme name + blank + N banner lines + blank + hint
func bannerPickerLines(bannerLines []string) int {
	return 2 + 1 + len(bannerLines) + 1 + 1
}

func renderBannerPicker(out io.Writer, termOut *termenv.Output, themes []BannerTheme, bannerLines []string, selected int, first bool, height int) {
	if !first {
		clearBannerPickerLines(out, height)
	}

	theme := themes[selected]

	header := termOut.String("Select a banner theme").Bold().String()
	hint := termOut.String("  ↑/↓ browse · Enter confirm · Esc cancel").Foreground(termOut.Color("#64748b")).String()

	nameDisplay := fmt.Sprintf("  %s  (%d/%d)", theme.Label, selected+1, len(themes))
	styledName := termOut.String(nameDisplay).Foreground(termOut.Color("#22c55e")).Bold().String()

	_, _ = fmt.Fprintf(out, "%s\r\n", header)
	_, _ = fmt.Fprintf(out, "%s\r\n", styledName)
	_, _ = fmt.Fprint(out, "\r\n")

	contentWidth := 0
	for _, line := range bannerLines {
		if w := utf8.RuneCountInString(line); w > contentWidth {
			contentWidth = w
		}
	}

	for i, line := range bannerLines {
		padded := line + strings.Repeat(" ", contentWidth-utf8.RuneCountInString(line))
		color := theme.Palette[i%len(theme.Palette)]
		styled := termOut.String(padded).Foreground(termOut.Color(color)).Bold().String()
		_, _ = fmt.Fprintf(out, "  %s\r\n", styled)
	}

	_, _ = fmt.Fprint(out, "\r\n")
	_, _ = fmt.Fprintf(out, "%s\r\n", hint)
}

func clearBannerPickerLines(out io.Writer, height int) {
	for range height {
		_, _ = fmt.Fprint(out, "\033[A\033[2K")
	}
	_, _ = fmt.Fprint(out, "\r")
}
