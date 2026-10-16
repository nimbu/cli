package cmd

import (
	"strings"

	"github.com/alecthomas/kong"
)

// rootHelpSection is one titled block of top-level commands in root help.
// Commands opt in with a `section:"<key>"` tag on their CLI field.
type rootHelpSection struct {
	Key   string
	Title string
}

var rootHelpSections = []rootHelpSection{
	{Key: "start", Title: "Getting started"},
	{Key: "content", Title: "Content"},
	{Key: "commerce", Title: "Commerce"},
	{Key: "code", Title: "Theme & code"},
	{Key: "admin", Title: "Admin"},
	{Key: "advanced", Title: "Advanced"},
}

// suppressedFlagGroups are hidden from default help and shown by --help-all.
var suppressedFlagGroups = map[string]bool{
	"Output":     true,
	"Agent/CI":   true,
	"Connection": true,
}

const (
	forceFlagName  = "force"
	dryRunFlagName = "dry-run"
)

// requiresForce reports whether a command's help says it needs --force.
// Those commands show --force (and --dry-run) under "Safety:" in default
// help instead of hiding it in the Agent/CI group.
func requiresForce(node *kong.Node) bool {
	if node == nil || node.Type == kong.ApplicationNode {
		return false
	}
	return strings.Contains(node.Help, "--force") || strings.Contains(node.Detail, "--force")
}

// flagSections classifies every visible flag by the Kong model, not by the
// rendered text: command flags, command groups (Query), Safety, Global Flags
// (all application-level flags, --env included), then the groups only shown
// with --help-all.
func flagSections(app *kong.Application, node *kong.Node, opts helpRenderOptions) []helpSection {
	appFlags := map[*kong.Flag]bool{}
	for _, flag := range app.Flags {
		appFlags[flag] = true
	}
	safetyMode := requiresForce(node)

	var command, safety, global []*kong.Flag
	var commandGroups, appGroups []string
	grouped := map[string][]*kong.Flag{}
	titles := map[string]string{}

	for _, level := range node.AllFlags(true) {
		for _, flag := range level {
			isApp := appFlags[flag]
			switch {
			case safetyMode && flag.Name == forceFlagName && isApp:
				safety = append(safety, flag)
			case safetyMode && flag.Name == dryRunFlagName && !isApp:
				safety = append(safety, flag)
			case flag.Group != nil:
				key := flag.Group.Key
				if suppressedFlagGroups[key] && !opts.ShowAll {
					continue
				}
				if _, seen := grouped[key]; !seen {
					titles[key] = flag.Group.Title
					if isApp {
						appGroups = append(appGroups, key)
					} else {
						commandGroups = append(commandGroups, key)
					}
				}
				grouped[key] = append(grouped[key], flag)
			case isApp:
				global = append(global, flag)
			default:
				command = append(command, flag)
			}
		}
	}

	all := append(append(append([]*kong.Flag{}, command...), safety...), global...)
	for _, flags := range grouped {
		all = append(all, flags...)
	}
	haveShort := false
	for _, flag := range all {
		if flag.Short != 0 {
			haveShort = true
			break
		}
	}
	rows := func(flags []*kong.Flag) []helpRow {
		out := make([]helpRow, 0, len(flags))
		for _, flag := range flags {
			out = append(out, helpRow{left: formatHelpFlag(haveShort, flag), right: kong.DefaultHelpValueFormatter(flag.Value), style: opts.Style.flag})
		}
		return out
	}
	groupTitle := func(key string) string {
		if title := titles[key]; title != "" {
			return title
		}
		return key + ":"
	}

	sections := []helpSection{{title: "Flags:", rows: rows(command)}}
	for _, key := range commandGroups {
		sections = append(sections, helpSection{title: groupTitle(key), rows: rows(grouped[key])})
	}
	sections = append(sections,
		helpSection{title: "Safety:", rows: rows(safety)},
		helpSection{title: "Global Flags:", rows: rows(global)},
	)
	for _, key := range appGroups {
		sections = append(sections, helpSection{title: groupTitle(key), rows: rows(grouped[key])})
	}
	return sections
}

// formatHelpFlag mirrors Kong's flag column: "-s, --name=PLACEHOLDER".
func formatHelpFlag(haveShort bool, flag *kong.Flag) string {
	name := flag.Name
	isBool := flag.IsBool()
	short := ""
	if flag.Short != 0 {
		short = "-" + string(flag.Short) + ", "
	} else if haveShort {
		short = "    "
	}
	switch {
	case isBool && flag.Tag.Negatable == "_":
		name = "[no-]" + name
	case isBool && flag.Tag.Negatable != "":
		name += "/" + flag.Tag.Negatable
	}
	out := short + "--" + name
	if !isBool && !flag.IsCounter() {
		out += "=" + flag.FormatPlaceHolder()
	}
	return out
}
