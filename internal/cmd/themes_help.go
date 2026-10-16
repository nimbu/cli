package cmd

func (c *ThemePushCmd) helpExamples() []helpExample {
	return []helpExample{
		{About: "Upload files with uncommitted git changes", Command: "nimbu themes push"},
		{About: "Build first, then upload only Liquid files", Command: "nimbu themes push --build --liquid-only"},
		{About: "Upload selected files", Command: "nimbu themes push --only snippets/header.liquid --only stylesheets/theme.css"},
		{About: "Preview a full upload without changing anything", Command: "nimbu themes push --all --dry-run"},
	}
}

func (c *ThemeSyncCmd) helpExamples() []helpExample {
	return []helpExample{
		{About: "Upload files with uncommitted git changes", Command: "nimbu themes sync"},
		{About: "Preview a full sync that also deletes remote extras", Command: "nimbu themes sync --all --prune --dry-run"},
		{About: "Build, upload everything and delete remote extras", Command: "nimbu themes sync --build --all --prune"},
	}
}

func (c *ThemePullCmd) helpExamples() []helpExample {
	return []helpExample{
		{About: "Download the theme in nimbu.yml", Command: "nimbu themes pull"},
		{About: "Download only the Liquid files of another theme", Command: "nimbu themes pull --theme storefront --liquid-only"},
	}
}
