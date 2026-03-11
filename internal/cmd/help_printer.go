package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/muesli/termenv"
)

// helpOptions returns Kong help configuration options.
func helpOptions() kong.HelpOptions {
	return kong.HelpOptions{
		NoExpandSubcommands: true,
	}
}

// Color palette (matches gogcli/frontappcli).
const (
	colorUsage   = "#60a5fa" // blue - Usage heading
	colorSection = "#a78bfa" // purple - Flags, Commands, Arguments
	colorCommand = "#38bdf8" // cyan - command names
	colorDim     = "#9ca3af" // gray - brackets, flags
	colorDesc    = "#f8fafc" // near-white - command descriptions
)

// helpPrinter returns a custom HelpPrinter that colorizes output.
func helpPrinter() kong.HelpPrinter {
	return func(options kong.HelpOptions, ctx *kong.Context) error {
		// Capture default help to buffer
		var buf bytes.Buffer
		origWriter := ctx.Stdout
		ctx.Stdout = &buf

		if err := kong.DefaultHelpPrinter(options, ctx); err != nil {
			return err
		}
		ctx.Stdout = origWriter

		raw := appendRootInlinePayloadFooter(buf.String())
		raw = compactCommandsSection(raw)

		// Colorize and write
		output := colorizeHelp(raw)
		_, err := io.WriteString(origWriter, output)
		return err
	}
}

func appendRootInlinePayloadFooter(text string) string {
	if !strings.HasPrefix(text, "Usage: nimbu <command> [flags]") {
		return text
	}
	if !strings.Contains(text, "\nCommands:\n") {
		return text
	}
	if strings.Contains(text, "Create/Update supports inline payloads using:") {
		return text
	}

	footer := "\nCreate/Update supports inline payloads using: key=value, key:=json, key=@file.txt or key:=@file.json\n"
	if strings.HasSuffix(text, "\n") {
		return text + footer
	}
	return text + "\n" + footer
}

func compactCommandsSection(text string) string {
	type commandRow struct {
		cmd  string
		desc string
	}

	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if line != "Commands:" {
			out = append(out, line)
			continue
		}

		out = append(out, line)
		i++

		rows := make([]commandRow, 0)
		for i < len(lines) {
			if lines[i] == "" {
				i++
				continue
			}

			if !strings.HasPrefix(lines[i], "  ") || strings.HasPrefix(lines[i], "    ") {
				break
			}

			cmd := strings.TrimSpace(lines[i])
			descParts := make([]string, 0, 1)
			for i+1 < len(lines) && strings.HasPrefix(lines[i+1], "    ") {
				descPart := strings.TrimSpace(lines[i+1])
				if descPart != "" {
					descParts = append(descParts, descPart)
				}
				i++
			}

			rows = append(rows, commandRow{cmd: cmd, desc: strings.Join(descParts, " ")})
			i++
		}

		maxCmdLen := 0
		for _, row := range rows {
			if len(row.cmd) > maxCmdLen {
				maxCmdLen = len(row.cmd)
			}
		}

		for _, row := range rows {
			if row.desc == "" {
				out = append(out, "  "+row.cmd)
				continue
			}
			leaderLen := maxCmdLen - len(row.cmd) + 2
			if leaderLen < 2 {
				leaderLen = 2
			}
			leader := strings.Repeat("·", leaderLen)
			out = append(out, "  "+row.cmd+" "+leader+" "+row.desc)
		}
		if i < len(lines) && lines[i] != "" {
			out = append(out, "")
		}
		i--
	}

	return strings.Join(out, "\n")
}

// helpColorMode determines color mode from CLI args and environment.
// Called BEFORE Kong parsing since --color flag not yet available.
func helpColorMode() string {
	// Check NO_COLOR first (standard)
	if os.Getenv("NO_COLOR") != "" {
		return "never"
	}

	// Check NIMBU_COLOR env
	if v := os.Getenv("NIMBU_COLOR"); v != "" {
		return strings.ToLower(strings.TrimSpace(v))
	}

	// Check --color flag in args (pre-parse)
	for i, arg := range os.Args {
		if arg == "--color" && i+1 < len(os.Args) {
			return strings.ToLower(strings.TrimSpace(os.Args[i+1]))
		}
		if strings.HasPrefix(arg, "--color=") {
			return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(arg, "--color=")))
		}
	}

	// Check --json/--plain (disable colors)
	for _, arg := range os.Args {
		if arg == "--json" || arg == "--plain" {
			return "never"
		}
	}

	return "auto"
}

// helpProfile returns termenv profile based on color mode.
func helpProfile() termenv.Profile {
	mode := helpColorMode()

	switch mode {
	case "never":
		return termenv.Ascii
	case "always":
		return termenv.TrueColor
	default: // "auto"
		return termenv.EnvColorProfile()
	}
}

// colorizeHelp applies colors to help text.
func colorizeHelp(text string) string {
	profile := helpProfile()
	if profile == termenv.Ascii {
		return text // No colors
	}

	out := termenv.NewOutput(os.Stdout, termenv.WithProfile(profile))

	// Style functions
	heading := func(s string) string {
		return out.String(s).Foreground(out.Color(colorUsage)).Bold().String()
	}
	section := func(s string) string {
		return out.String(s).Foreground(out.Color(colorSection)).Bold().String()
	}
	cmdStyle := func(s string) string {
		return out.String(s).Foreground(out.Color(colorCommand)).Bold().String()
	}
	descStyle := func(s string) string {
		return out.String(s).Foreground(out.Color(colorDesc)).String()
	}
	dim := func(s string) string {
		return out.String(s).Foreground(out.Color(colorDim)).String()
	}

	lines := strings.Split(text, "\n")
	inCommands := false

	for i, line := range lines {
		if line == "" {
			inCommands = false
		}

		// Usage: line
		if strings.HasPrefix(line, "Usage:") {
			lines[i] = heading("Usage:") + strings.TrimPrefix(line, "Usage:")
			continue
		}

		// Section headers
		if line == "Flags:" || line == "Commands:" || line == "Arguments:" {
			lines[i] = section(line)
			if line == "Commands:" {
				inCommands = true
			}
			continue
		}

		// Command lines (2-space indent, command name)
		if inCommands && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
			trimmed := strings.TrimPrefix(line, "  ")
			if trimmed != "" {
				if idx := strings.Index(trimmed, " ·"); idx > 0 {
					cmdPart := styleCmdPart(trimmed[:idx], cmdStyle, dim)
					rest := strings.TrimSpace(trimmed[idx+1:])
					leader := rest
					desc := ""
					if sp := strings.Index(rest, " "); sp > 0 {
						leader = rest[:sp]
						desc = strings.TrimSpace(rest[sp+1:])
					}

					if desc != "" {
						lines[i] = "  " + cmdPart + " " + dim(leader) + " " + descStyle(desc)
					} else {
						lines[i] = "  " + cmdPart + " " + dim(leader)
					}
					continue
				}

				// Split command from description
				parts := strings.SplitN(trimmed, "  ", 2)
				cmdPart := parts[0]

				// Style command name, dim brackets/flags
				cmdPart = styleCmdPart(cmdPart, cmdStyle, dim)

				if len(parts) > 1 {
					rest := strings.TrimSpace(parts[1])
					leader := ""
					desc := ""
					if sp := strings.Index(rest, " "); sp > 0 {
						leader = rest[:sp]
						desc = strings.TrimSpace(rest[sp+1:])
					} else {
						desc = rest
					}

					if leader != "" {
						lines[i] = "  " + cmdPart + "  " + dim(leader) + " " + descStyle(desc)
					} else {
						lines[i] = "  " + cmdPart + "  " + descStyle(desc)
					}
				} else {
					lines[i] = "  " + cmdPart
				}
			}
			continue
		}

		// Command description continuation (4-space indent)
		if inCommands && strings.HasPrefix(line, "    ") {
			// Keep description as-is
			continue
		}

		// Flag lines - dim the flag part
		if strings.HasPrefix(line, "  -") || strings.HasPrefix(line, "      --") {
			lines[i] = colorizeFlag(line, dim)
			continue
		}
	}

	return strings.Join(lines, "\n")
}

// styleCmdPart styles command part with brackets dimmed.
func styleCmdPart(cmdPart string, cmdStyle, dim func(string) string) string {
	// Handle patterns like "auth login [flags]" or "channels entries <channel>"
	var result strings.Builder
	words := strings.Fields(cmdPart)

	for j, word := range words {
		if j > 0 {
			result.WriteString(" ")
		}

		if strings.HasPrefix(word, "[") || strings.HasPrefix(word, "<") {
			// Dim brackets and their contents
			result.WriteString(dim(word))
		} else {
			// Bold command words
			result.WriteString(cmdStyle(word))
		}
	}

	return result.String()
}

// colorizeFlag dims the flag definition part.
func colorizeFlag(line string, dim func(string) string) string {
	// Find where description starts (after double-space separator)
	// Examples: "  -h, --help      Show help"
	//           "      --site=STRING   Site ID"

	// Look for pattern: flag part followed by 2+ spaces then description
	// We want to dim the flag part and keep description normal

	// Find first non-space after leading indent
	trimmed := strings.TrimLeft(line, " ")
	indent := len(line) - len(trimmed)

	// Find the double-space separator between flag and description
	// Skip the flag definition (which may contain single spaces)
	parts := strings.SplitN(trimmed, "  ", 2)
	if len(parts) < 2 {
		// No description found, dim entire line
		return dim(line)
	}

	flagPart := strings.Repeat(" ", indent) + parts[0]
	descPart := strings.TrimLeft(parts[1], " ")

	if descPart == "" {
		return dim(line)
	}

	// Calculate spacing between flag and description
	spacing := len(line) - len(flagPart) - len(descPart)
	if spacing < 2 {
		spacing = 2
	}

	return dim(flagPart) + strings.Repeat(" ", spacing) + descPart
}
