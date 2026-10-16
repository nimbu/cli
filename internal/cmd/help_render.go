package cmd

import (
	"fmt"
	"go/doc/comment"
	"strings"

	"github.com/alecthomas/kong"
)

// Layout constants shared with Kong's default printer so help keeps the
// familiar shape.
const (
	helpIndent        = "  "
	helpColumnPadding = 4
)

type helpRenderOptions struct {
	Summary bool // one-paragraph usage (Kong's short form)
	ShowAll bool // --help-all: include Output/Agent-CI/Connection flags
	Width   int
	Style   helpStyle
}

// helpRow is one two-column row; left is styled by kind after padding.
type helpRow struct {
	left  string
	right string
	style func(string) string
}

// helpSection is a titled block of rows. Rows of every section in one table
// share a left column width so the descriptions line up across sections.
type helpSection struct {
	title string
	rows  []helpRow
}

type helpBuilder struct {
	lines []string
	opts  helpRenderOptions
}

func (b *helpBuilder) blank() {
	if len(b.lines) > 0 && b.lines[len(b.lines)-1] != "" {
		b.lines = append(b.lines, "")
	}
}

func (b *helpBuilder) line(s string) {
	b.lines = append(b.lines, strings.TrimRight(s, " "))
}

// docText formats text as a Go doc comment, like Kong's help: indented
// blocks stay preformatted (prefixed by codePrefix), paragraphs are re-flowed
// to width.
func docText(text, codePrefix string, width int) string {
	var parser comment.Parser
	printer := comment.Printer{TextCodePrefix: codePrefix, TextWidth: width}
	return string(printer.Text(parser.Parse(text)))
}

// wrap writes prose re-flowed to the help width.
func (b *helpBuilder) wrap(text string) {
	for _, l := range strings.Split(strings.TrimSpace(docText(strings.TrimSpace(text), "    ", b.opts.Width)), "\n") {
		b.line(l)
	}
}

func (b *helpBuilder) String() string {
	for len(b.lines) > 0 && b.lines[len(b.lines)-1] == "" {
		b.lines = b.lines[:len(b.lines)-1]
	}
	return strings.Join(b.lines, "\n") + "\n"
}

// table writes sections whose rows share one left column width, wrapping
// the right column like Kong's two-column layout.
func (b *helpBuilder) table(sections []helpSection) {
	width := b.opts.Width - len(helpIndent)
	maxLeft := 375 * width / 1000
	if maxLeft < 30 {
		maxLeft = 30
	}
	leftSize := 0
	for _, section := range sections {
		for _, row := range section.rows {
			if n := len(row.left); n > leftSize && n < maxLeft {
				leftSize = n
			}
		}
	}
	offset := strings.Repeat(" ", leftSize+helpColumnPadding)

	for _, section := range sections {
		if len(section.rows) == 0 {
			continue
		}
		b.blank()
		b.line(b.opts.Style.section(section.title))
		for _, row := range section.rows {
			wrapped := docText(row.right, helpIndent, width-leftSize-helpColumnPadding)
			right := strings.Split(strings.TrimRight(wrapped, "\n"), "\n")
			style := row.style
			if style == nil {
				style = func(s string) string { return s }
			}
			left := helpIndent + style(row.left)
			if len(row.left) < maxLeft && row.right != "" {
				pad := strings.Repeat(" ", leftSize-len(row.left)+helpColumnPadding)
				b.line(left + pad + right[0])
				right = right[1:]
			} else {
				b.line(left)
				if row.right == "" {
					right = nil
				}
			}
			for _, l := range right {
				b.line(helpIndent + offset + l)
			}
		}
	}
}

// renderHelp renders help for the selected node from the Kong model.
func renderHelp(ctx *kong.Context, opts helpRenderOptions) string {
	app := ctx.Model
	node := ctx.Selected()
	if node == nil {
		node = app.Node
	}
	b := &helpBuilder{opts: opts}
	isRoot := node == app.Node

	usage := strings.TrimSpace(node.Summary())
	if isRoot {
		usage = strings.TrimSpace(app.Summary())
	}
	b.line(opts.Style.usage("Usage:") + " " + strings.TrimSpace(app.Name+" "+usage))

	if node.Help != "" {
		b.blank()
		b.wrap(node.Help)
	}
	if opts.Summary {
		b.blank()
		b.line(fmt.Sprintf("Run %q for more information.", node.FullPath()+" --help"))
		return b.String()
	}
	if node.Detail != "" {
		b.blank()
		b.wrap(node.Detail)
	}

	if len(node.Positional) > 0 {
		rows := make([]helpRow, 0, len(node.Positional))
		for _, arg := range node.Positional {
			rows = append(rows, helpRow{left: arg.Summary(), right: kong.DefaultHelpValueFormatter(arg), style: opts.Style.flag})
		}
		b.table([]helpSection{{title: "Arguments:", rows: rows}})
	}

	b.table(commandSections(node, isRoot, opts.Style))
	b.table(flagSections(app, node, opts))

	if examples := helpExamplesFor(node); len(examples) > 0 {
		b.blank()
		b.line(opts.Style.section("Examples:"))
		for i, example := range examples {
			if i > 0 {
				b.line("")
			}
			if example.About != "" {
				b.line(helpIndent + opts.Style.comment("# "+example.About))
			}
			for _, l := range strings.Split(example.Command, "\n") {
				b.line(helpIndent + l)
			}
		}
	}

	b.blank()
	switch {
	case hasVisibleCommands(node) && isRoot:
		b.wrap(fmt.Sprintf("Run %q for more information on a command.", app.Name+" help <command>"))
	case hasVisibleCommands(node):
		b.wrap(fmt.Sprintf("Run %q for more information on a command.", node.FullPath()+" <command> --help"))
	}
	if isRoot {
		b.wrap("Create/Update supports inline payloads using: key=value, key:=json, key=@file.txt or key:=@file.json")
	}
	if !opts.ShowAll {
		b.wrap(fmt.Sprintf("Run %q for all flags.", node.FullPath()+" --help-all"))
	}
	return b.String()
}

func hasVisibleCommands(node *kong.Node) bool {
	for _, child := range node.Children {
		if !child.Hidden {
			return true
		}
	}
	return false
}

// commandSections lists the node's subcommands: grouped into the root help
// sections at the top level, one "Commands:" block elsewhere.
func commandSections(node *kong.Node, isRoot bool, style helpStyle) []helpSection {
	if !isRoot {
		rows := []helpRow{}
		for _, child := range node.Children {
			if !child.Hidden {
				rows = append(rows, commandRow(child, style))
			}
		}
		return []helpSection{{title: "Commands:", rows: rows}}
	}

	sections := make([]helpSection, len(rootHelpSections))
	index := map[string]int{}
	for i, s := range rootHelpSections {
		sections[i] = helpSection{title: s.Title + ":"}
		index[s.Key] = i
	}
	other := helpSection{title: "Other commands:"}
	for _, child := range node.Children {
		if child.Hidden {
			continue
		}
		if i, ok := index[commandHelpSection(child)]; ok {
			sections[i].rows = append(sections[i].rows, commandRow(child, style))
		} else {
			other.rows = append(other.rows, commandRow(child, style))
		}
	}
	return append(sections, other)
}

func commandRow(node *kong.Node, style helpStyle) helpRow {
	name := node.Name
	if node.Type == kong.ArgumentNode {
		name = "<" + node.Name + ">"
	}
	if len(node.Aliases) > 0 {
		name += " (" + strings.Join(node.Aliases, ", ") + ")"
	}
	return helpRow{left: name, right: node.Help, style: style.command}
}

// commandHelpSection is the root help section a command is listed under,
// from its `section` struct tag.
func commandHelpSection(node *kong.Node) string {
	if node.Tag == nil {
		return ""
	}
	return node.Tag.Get("section")
}
