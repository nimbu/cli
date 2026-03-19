package themes

import "strings"

// Kind identifies a remote theme resource collection.
type Kind string

const (
	KindAsset    Kind = "asset"
	KindLayout   Kind = "layout"
	KindSnippet  Kind = "snippet"
	KindTemplate Kind = "template"
)

func (k Kind) Collection() string {
	switch k {
	case KindLayout:
		return "layouts"
	case KindTemplate:
		return "templates"
	case KindSnippet:
		return "snippets"
	default:
		return "assets"
	}
}

func (k Kind) DisplayRoot() string {
	switch k {
	case KindLayout:
		return "layouts"
	case KindTemplate:
		return "templates"
	case KindSnippet:
		return "snippets"
	default:
		return ""
	}
}

// Resource represents one managed local or remote theme file.
type Resource struct {
	Kind        Kind   `json:"kind"`
	DisplayPath string `json:"path"`
	LocalPath   string `json:"local_path,omitempty"`
	RemoteName  string `json:"remote_name"`
	PublicURL   string `json:"public_url,omitempty"`
	AbsPath     string `json:"-"`
}

// Action records one upload or delete operation.
type Action struct {
	Kind        Kind   `json:"kind"`
	DisplayPath string `json:"path"`
	LocalPath   string `json:"local_path,omitempty"`
	RemoteName  string `json:"remote_name"`
}

// Result is emitted by push/sync commands.
type Result struct {
	Built    bool     `json:"built,omitempty"`
	Deleted  []Action `json:"deleted,omitempty"`
	DryRun   bool     `json:"dry_run,omitempty"`
	Mode     string   `json:"mode"`
	Theme    string   `json:"theme"`
	Uploaded []Action `json:"uploaded,omitempty"`
}

// BuildConfig configures an optional pre-push/pre-sync build step.
type BuildConfig struct {
	Args    []string
	CWD     string
	Command string
	Env     map[string]string
}

// RootSpec maps a local root onto a remote theme resource kind.
type RootSpec struct {
	AbsPath    string
	Kind       Kind
	LocalPath  string
	RemoteBase string
}

// Config is the resolved theme sync configuration.
type Config struct {
	Build       *BuildConfig
	Generated   []string
	Ignore      []string
	ProjectRoot string
	Roots       []RootSpec
	Theme       string
}

// Options control one push/sync run.
type Options struct {
	All        bool
	Build      bool
	DryRun     bool
	Force      bool
	Prune      bool
	Since      string
	Only       []string
	LiquidOnly bool
	CSSOnly    bool
	JSOnly     bool
	ImagesOnly bool
	FontsOnly  bool
}

type resourceKey struct {
	kind       Kind
	remoteName string
}

func keyFor(resource Resource) resourceKey {
	return resourceKey{kind: resource.Kind, remoteName: resource.RemoteName}
}

// DisplayPath returns the stable CLI-facing path for a resource.
func DisplayPath(kind Kind, remoteName string) string {
	name := normalizePath(remoteName)
	if kind == KindAsset {
		return strings.TrimPrefix(name, "/")
	}
	return kind.DisplayRoot() + "/" + name
}

// ParseCLIPath maps a CLI file path onto a resource kind and remote name.
func ParseCLIPath(raw string) (Kind, string) {
	path := normalizePath(strings.TrimPrefix(strings.TrimSpace(raw), "./"))
	path = strings.TrimPrefix(path, "/")

	switch {
	case strings.HasPrefix(path, "layouts/"):
		return KindLayout, strings.TrimPrefix(path, "layouts/")
	case strings.HasPrefix(path, "templates/"):
		return KindTemplate, strings.TrimPrefix(path, "templates/")
	case strings.HasPrefix(path, "snippets/"):
		return KindSnippet, strings.TrimPrefix(path, "snippets/")
	default:
		return KindAsset, path
	}
}
