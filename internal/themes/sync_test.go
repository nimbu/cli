package themes

import "testing"

func TestRemoteInManagedScope(t *testing.T) {
	cfg := Config{
		Roots: []RootSpec{
			{Kind: KindLayout, LocalPath: "layouts"},
			{Kind: KindAsset, LocalPath: "images", RemoteBase: "images"},
			{Kind: KindAsset, LocalPath: "fonts", RemoteBase: "fonts"},
		},
	}

	if !remoteInManagedScope(cfg, Resource{Kind: KindLayout, RemoteName: "default.liquid"}) {
		t.Fatal("expected layout in scope")
	}
	if !remoteInManagedScope(cfg, Resource{Kind: KindAsset, RemoteName: "fonts/app.woff2"}) {
		t.Fatal("expected font asset in scope")
	}
	if remoteInManagedScope(cfg, Resource{Kind: KindAsset, RemoteName: "javascripts/app.js"}) {
		t.Fatal("unexpected asset scope match")
	}
}

func TestScopeUsesAllFiles(t *testing.T) {
	tests := []struct {
		name        string
		opts        Options
		hasCategory bool
		want        bool
	}{
		{name: "default no flags", opts: Options{}, hasCategory: false, want: false},
		{name: "all flag", opts: Options{All: true}, hasCategory: false, want: true},
		{name: "only flag", opts: Options{Only: []string{"templates/page.liquid"}}, hasCategory: false, want: true},
		{name: "category alone", opts: Options{}, hasCategory: true, want: true},
		{name: "category with since", opts: Options{Since: "origin/main"}, hasCategory: true, want: false},
		{name: "only with since", opts: Options{Only: []string{"templates/page.liquid"}, Since: "origin/main"}, hasCategory: false, want: true},
		{name: "since alone", opts: Options{Since: "origin/main"}, hasCategory: false, want: false},
		{name: "all with since", opts: Options{All: true, Since: "origin/main"}, hasCategory: false, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scopeUsesAllFiles(tt.opts, tt.hasCategory)
			if got != tt.want {
				t.Fatalf("scopeUsesAllFiles() = %v, want %v", got, tt.want)
			}
		})
	}
}
