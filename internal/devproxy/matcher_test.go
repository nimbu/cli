package devproxy

import "testing"

func TestMatcherBuiltins(t *testing.T) {
	m := NewMatcher(nil, nil)

	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{method: "GET", path: "/", want: true},
		{method: "GET", path: "/ons-werk", want: true},
		{method: "GET", path: "/images/logo.svg", want: false},
		{method: "GET", path: "/javascripts/app.js", want: false},
		{method: "GET", path: "/__webpack_hmr", want: false},
		{method: "GET", path: "/sockjs-node/info", want: false},
	}

	for _, tc := range tests {
		got := m.ShouldProxy(tc.method, tc.path, false)
		if got != tc.want {
			t.Fatalf("ShouldProxy(%q,%q)=%v want %v", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestMatcherIncludeOverride(t *testing.T) {
	m := NewMatcher([]string{"GET /images/*"}, nil)
	if !m.ShouldProxy("GET", "/images/logo.svg", false) {
		t.Fatal("include rule should force proxying")
	}
}

func TestMatcherExcludeRule(t *testing.T) {
	m := NewMatcher(nil, []string{"POST /.well-known/*"})
	if m.ShouldProxy("POST", "/.well-known/csrf", false) {
		t.Fatal("exclude rule should prevent proxying")
	}
	if !m.ShouldProxy("GET", "/.well-known/csrf", false) {
		t.Fatal("method-scoped exclude should not block GET")
	}
}

func TestMatcherDoubleStarRespectsPathBoundary(t *testing.T) {
	m := NewMatcher(nil, []string{"GET /api/**"})

	if !m.ShouldProxy("GET", "/apix", false) {
		t.Fatal("exclude /api/** must not match /apix")
	}
	if m.ShouldProxy("GET", "/api", false) {
		t.Fatal("exclude /api/** should match /api")
	}
	if m.ShouldProxy("GET", "/api/v1/items", false) {
		t.Fatal("exclude /api/** should match nested paths")
	}
}
