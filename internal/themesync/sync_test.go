package themesync

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
