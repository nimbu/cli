package themesync

import "testing"

func TestDisplayPathAndParseCLIPath(t *testing.T) {
	tests := []struct {
		input      string
		kind       Kind
		remoteName string
		display    string
	}{
		{input: "layouts/default.liquid", kind: KindLayout, remoteName: "default.liquid", display: "layouts/default.liquid"},
		{input: "./templates/customers/login.liquid", kind: KindTemplate, remoteName: "customers/login.liquid", display: "templates/customers/login.liquid"},
		{input: "snippets/repeatables/header.liquid", kind: KindSnippet, remoteName: "repeatables/header.liquid", display: "snippets/repeatables/header.liquid"},
		{input: "images/logo.svg", kind: KindAsset, remoteName: "images/logo.svg", display: "images/logo.svg"},
	}

	for _, test := range tests {
		kind, remoteName := ParseCLIPath(test.input)
		if kind != test.kind || remoteName != test.remoteName {
			t.Fatalf("%s => (%s, %s), want (%s, %s)", test.input, kind, remoteName, test.kind, test.remoteName)
		}
		if got := DisplayPath(kind, remoteName); got != test.display {
			t.Fatalf("%s => display %q, want %q", test.input, got, test.display)
		}
	}
}
