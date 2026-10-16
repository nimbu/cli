package cmd

func (c *ChannelsListCmd) helpExamples() []helpExample {
	return []helpExample{
		{About: "List the channels of the site in nimbu.yml", Command: "nimbu channels list"},
		{About: "List channels of another site, without entry counts", Command: "nimbu channels list --site my-site --no-entry-count"},
		{About: "Every channel as JSON", Command: "nimbu channels list --all --json"},
	}
}

func (c *ChannelEntriesListCmd) helpExamples() []helpExample {
	return []helpExample{
		{About: "List the first page of blog entries", Command: "nimbu channels entries list --channel blog"},
		{About: "Every published entry, newest first, as JSON", Command: "nimbu channels entries list --channel blog --all --filters published=true \\\n  --sort created_at:desc --json"},
	}
}

func (c *ChannelEntriesCreateCmd) helpExamples() []helpExample {
	return []helpExample{
		{About: "Create an entry with inline assignments", Command: "nimbu channels entries create --channel blog title=Hello published:=true"},
		{About: "Create an entry from a JSON file", Command: "nimbu channels entries create --channel blog --file entry.json"},
		{About: "Write localized fields for one locale", Command: "nimbu channels entries create --channel blog --locale en title=Hello"},
	}
}

func (c *ChannelEntriesUpdateCmd) helpExamples() []helpExample {
	return []helpExample{
		{About: "Change one field", Command: "nimbu channels entries update --channel blog --entry welcome title=Welcome"},
		{About: "Typed values use :=", Command: "nimbu channels entries update --channel blog --entry welcome published:=true"},
		{About: "Update the English translation from stdin", Command: "echo '{\"title\":\"Welcome\"}' | nimbu channels entries update \\\n  --channel blog --entry welcome --locale en --file -"},
	}
}
