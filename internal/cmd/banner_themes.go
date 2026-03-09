package cmd

import "strings"

// BannerTheme holds a named color palette for the server banner.
type BannerTheme struct {
	Name    string
	Label   string
	Palette []string // one hex color per banner line
}

var bannerThemes = []BannerTheme{
	{
		Name:  "default",
		Label: "Default (white)",
		Palette: []string{
			"#ffffff", "#ffffff", "#ffffff",
			"#ffffff", "#ffffff", "#ffffff",
		},
	},
	{
		Name:  "rainbow",
		Label: "Rainbow",
		Palette: []string{
			"#ff0000", "#ff8800", "#ffff00",
			"#00cc00", "#0066ff", "#8800ff",
		},
	},
	{
		Name:  "gradient",
		Label: "Gradient",
		Palette: []string{
			"#38bdf8", "#22d3ee", "#34d399",
			"#facc15", "#fb7185", "#ef4444",
		},
	},
	{
		Name:  "ocean",
		Label: "Ocean",
		Palette: []string{
			"#1e3a5f", "#2563eb", "#3b82f6",
			"#60a5fa", "#93c5fd", "#bfdbfe",
		},
	},
	{
		Name:  "sunset",
		Label: "Sunset",
		Palette: []string{
			"#7c2d12", "#c2410c", "#ea580c",
			"#f97316", "#fb923c", "#fdba74",
		},
	},
	{
		Name:  "forest",
		Label: "Forest",
		Palette: []string{
			"#14532d", "#166534", "#15803d",
			"#22c55e", "#4ade80", "#86efac",
		},
	},
	{
		Name:  "neon",
		Label: "Neon",
		Palette: []string{
			"#f0abfc", "#c084fc", "#a78bfa",
			"#818cf8", "#6366f1", "#4f46e5",
		},
	},
	{
		Name:  "candy",
		Label: "Candy",
		Palette: []string{
			"#fca5a5", "#fdba74", "#fde68a",
			"#86efac", "#93c5fd", "#d8b4fe",
		},
	},
	{
		Name:  "cyberpunk",
		Label: "Cyberpunk",
		Palette: []string{
			"#ff003c", "#ff2a6d", "#d300c5",
			"#7b00ff", "#3c1361", "#1a1a2e",
		},
	},
	{
		Name:  "aurora",
		Label: "Aurora",
		Palette: []string{
			"#00ff87", "#00e4a0", "#00c8b8",
			"#00acd1", "#6366f1", "#a855f7",
		},
	},
	{
		Name:  "lava",
		Label: "Lava",
		Palette: []string{
			"#fff7ed", "#fed7aa", "#fdba74",
			"#f97316", "#ea580c", "#9a3412",
		},
	},
	{
		Name:  "ice",
		Label: "Ice",
		Palette: []string{
			"#f0f9ff", "#bae6fd", "#7dd3fc",
			"#38bdf8", "#0ea5e9", "#0369a1",
		},
	},
	{
		Name:  "dracula",
		Label: "Dracula",
		Palette: []string{
			"#ff79c6", "#bd93f9", "#8be9fd",
			"#50fa7b", "#f1fa8c", "#ffb86c",
		},
	},
	{
		Name:  "catppuccin",
		Label: "Catppuccin",
		Palette: []string{
			"#f38ba8", "#fab387", "#f9e2af",
			"#a6e3a1", "#89b4fa", "#cba6f7",
		},
	},
	{
		Name:  "synthwave",
		Label: "Synthwave",
		Palette: []string{
			"#f6d365", "#ff6a88", "#ff2a6d",
			"#c31dde", "#7b2fef", "#241734",
		},
	},
	{
		Name:  "nord",
		Label: "Nord",
		Palette: []string{
			"#88c0d0", "#81a1c1", "#5e81ac",
			"#b48ead", "#a3be8c", "#ebcb8b",
		},
	},
	{
		Name:  "matrix",
		Label: "Matrix",
		Palette: []string{
			"#0d3b0d", "#14691a", "#1fa12a",
			"#39e645", "#7cfc7c", "#c4ffc4",
		},
	},
	{
		Name:  "sakura",
		Label: "Sakura",
		Palette: []string{
			"#fdf2f8", "#fce7f3", "#fbcfe8",
			"#f9a8d4", "#f472b6", "#ec4899",
		},
	},
}

// BannerThemes returns the ordered list of available themes.
func BannerThemes() []BannerTheme {
	return bannerThemes
}

// BannerThemeByName looks up a theme by name (case-insensitive).
func BannerThemeByName(name string) (BannerTheme, bool) {
	lower := strings.ToLower(strings.TrimSpace(name))
	for _, t := range bannerThemes {
		if t.Name == lower {
			return t, true
		}
	}
	return BannerTheme{}, false
}

// BannerThemeNames returns the names of all available themes.
func BannerThemeNames() []string {
	names := make([]string, len(bannerThemes))
	for i, t := range bannerThemes {
		names[i] = t.Name
	}
	return names
}

// DefaultBannerPalette returns the default (white) palette.
func DefaultBannerPalette() []string {
	return bannerThemes[0].Palette
}
