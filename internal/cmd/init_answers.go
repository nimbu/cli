package cmd

import (
	"fmt"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/bootstrap"
)

// Conflict policies for in-place init (--on-conflict).
const (
	initConflictAsk       = "ask"
	initConflictOverwrite = "overwrite"
	initConflictKeep      = "keep"
)

// initPresets are the wizard answers given up front as flags. An empty field
// means "not given": the wizard asks, or --no-input fails naming the flag.
type initPresets struct {
	Site        string
	Theme       string
	Repeatables string
	Bundles     string
	OnConflict  string
}

func (c *InitCmd) presets(flags *RootFlags) initPresets {
	p := initPresets{
		Theme:       strings.TrimSpace(c.Theme),
		Repeatables: strings.TrimSpace(c.Repeatables),
		Bundles:     strings.TrimSpace(c.Bundles),
		OnConflict:  strings.ToLower(strings.TrimSpace(c.OnConflict)),
	}
	if flags != nil {
		p.Site = strings.TrimSpace(flags.Site)
	}
	if p.OnConflict == "" {
		p.OnConflict = initConflictAsk
	}
	return p
}

// missingForNoInput names the flags --no-input needs but did not get. Site and
// theme have no safe default; everything else does (no repeatables, no
// bundles, the default directory name).
func (p initPresets) missingForNoInput() []string {
	var missing []string
	if p.Site == "" {
		missing = append(missing, "--site")
	}
	if p.Theme == "" {
		missing = append(missing, "--theme")
	}
	return missing
}

func initNoInputError(missing []string) error {
	return fmt.Errorf("init --no-input needs --site and --theme (missing %s); optional: --repeatables none|all|IDS, --bundles none|IDS, --on-conflict overwrite|keep, and a target directory argument",
		strings.Join(missing, ", "))
}

// matchInitSite finds a site by ID or subdomain (case-insensitive).
func matchInitSite(sites []api.Site, value string) (api.Site, error) {
	value = strings.TrimSpace(value)
	for _, site := range sites {
		if site.ID == value || strings.EqualFold(site.Subdomain, value) {
			return site, nil
		}
	}
	names := make([]string, 0, len(sites))
	for _, site := range sites {
		names = append(names, siteConfigValue(site))
	}
	return api.Site{}, fmt.Errorf("site %q not found among your sites (%s)", value, strings.Join(names, ", "))
}

// matchInitTheme finds a theme by ID, short name, theme short ID or name
// (case-insensitive).
func matchInitTheme(themes []api.Theme, value string) (api.Theme, error) {
	value = strings.TrimSpace(value)
	for _, theme := range themes {
		if theme.ID == value {
			return theme, nil
		}
	}
	for _, theme := range themes {
		for _, candidate := range []string{theme.Short, theme.ThemeShortID, theme.Name} {
			if candidate != "" && strings.EqualFold(strings.TrimSpace(candidate), value) {
				return theme, nil
			}
		}
	}
	names := make([]string, 0, len(themes))
	for _, theme := range themes {
		names = append(names, themeConfigValue(theme))
	}
	return api.Theme{}, fmt.Errorf("theme %q not found for this site (%s)", value, strings.Join(names, ", "))
}

// parseInitRepeatables turns --repeatables (none, all, or comma-separated ids)
// into repeatable IDs, rejecting IDs the manifest does not define.
func parseInitRepeatables(value string, options []bootstrap.Repeatable) ([]string, error) {
	return parseInitSelection("--repeatables", value, repeatableIDsForPrompt(options), true)
}

// parseInitBundles turns --bundles (none or comma-separated ids) into bundle IDs.
func parseInitBundles(value string, options []bootstrap.Bundle) ([]string, error) {
	ids := make([]string, 0, len(options))
	for _, bundle := range options {
		ids = append(ids, bundle.ID)
	}
	return parseInitSelection("--bundles", value, ids, false)
}

func parseInitSelection(flag, value string, available []string, allowAll bool) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "none":
		return nil, nil
	case "all":
		if allowAll {
			return append([]string(nil), available...), nil
		}
	}
	known := make(map[string]struct{}, len(available))
	for _, id := range available {
		known[id] = struct{}{}
	}
	ids := splitCSVValues(value)
	for _, id := range ids {
		if _, ok := known[id]; !ok {
			if len(available) == 0 {
				return nil, fmt.Errorf("%s: unknown id %q (this template defines none)", flag, id)
			}
			return nil, fmt.Errorf("%s: unknown id %q (available: %s)", flag, id, strings.Join(available, ", "))
		}
	}
	return ids, nil
}

// conflictSkipSet applies a non-ask --on-conflict policy: keep skips every
// conflicting file, overwrite skips none.
func conflictSkipSet(policy string, conflicts []string) map[string]struct{} {
	skip := map[string]struct{}{}
	if policy == initConflictKeep {
		for _, rel := range conflicts {
			skip[rel] = struct{}{}
		}
	}
	return skip
}
