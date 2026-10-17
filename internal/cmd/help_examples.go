package cmd

import (
	"github.com/alecthomas/kong"
)

// helpExample is one entry of a command's "Examples:" help section: a short
// comment and the command line. Keep 2-4 per command, simple to complex,
// using the current grammar: identity flags first, inline assignments last.
type helpExample struct {
	About   string
	Command string
}

// helpExampler is implemented by command structs that show examples.
type helpExampler interface {
	helpExamples() []helpExample
}

// helpExamplesFor returns the examples of the command struct behind node.
func helpExamplesFor(node *kong.Node) []helpExample {
	if node == nil || !node.Target.IsValid() {
		return nil
	}
	target := node.Target
	if target.CanAddr() {
		target = target.Addr()
	}
	if !target.CanInterface() {
		return nil
	}
	if provider, ok := target.Interface().(helpExampler); ok {
		return provider.helpExamples()
	}
	return nil
}

func (c *CLI) helpExamples() []helpExample {
	return []helpExample{
		{About: "Log in, then list the sites you can access", Command: "nimbu auth login\nnimbu sites list"},
		{About: "Bootstrap a theme project and run the local dev server", Command: "nimbu init my-theme\nnimbu server"},
		{About: "List the entries of a channel as JSON", Command: "nimbu channels entries list --channel blog --json"},
		{About: "Update an entry with inline assignments", Command: "nimbu channels entries update --channel blog --entry welcome title=Welcome"},
	}
}

func (c *AuthLoginCmd) helpExamples() []helpExample {
	return []helpExample{
		{About: "Log in through the browser", Command: "nimbu auth login"},
		{About: "Log in from SSH or a container with a one-time code", Command: "nimbu auth login --device"},
		{About: "Store an existing API token, read from stdin", Command: "nimbu auth login --token - < token-file"},
	}
}

func (c *SitesListCmd) helpExamples() []helpExample {
	return []helpExample{
		{About: "List the sites you can access", Command: "nimbu sites list"},
		{About: "List every site as stable TSV", Command: "nimbu sites list --all --plain"},
	}
}

func (c *InitCmd) helpExamples() []helpExample {
	return []helpExample{
		{About: "Bootstrap a project, prompting for the directory", Command: "nimbu init"},
		{About: "Bootstrap into ./my-theme", Command: "nimbu init my-theme"},
		{About: "Bootstrap from a local starterskit checkout", Command: "nimbu init my-theme --dir ../theme-starterskit"},
	}
}

func (c *ServerCmd) helpExamples() []helpExample {
	return []helpExample{
		{About: "Start the simulator proxy and the project dev server", Command: "nimbu server"},
		{About: "Run a custom dev server command (pnpm dev)", Command: "nimbu server --cmd pnpm --arg dev"},
		{About: "Browse the draft of a page on its normal URL", Command: "nimbu server --draft about"},
	}
}

func (c *ReleaseCmd) helpExamples() []helpExample {
	return []helpExample{
		{About: "Plan a release without deploying", Command: "nimbu release --env production --dry-run"},
		{About: "Release schema, cloud code and theme", Command: "nimbu release --env production"},
		{About: "Release only the theme", Command: "nimbu release --env production --only theme"},
	}
}

func (c *APICmd) helpExamples() []helpExample {
	return []helpExample{
		{About: "Fetch a resource", Command: "nimbu api get /channels"},
		{About: "Fetch every page of a list", Command: "nimbu api get /customers --all"},
		{About: "Send a JSON body from a file", Command: "nimbu api patch /channels/blog --data @payload.json"},
		{About: "Delete a resource", Command: "nimbu api delete /redirects/<id> --force"},
	}
}

func (c *CompletionCmd) helpExamples() []helpExample {
	return []helpExample{
		{About: "Load zsh completions in ~/.zshrc", Command: `eval "$(nimbu completion zsh)"`},
		{About: "Load fish completions", Command: "nimbu completion fish | source"},
	}
}
