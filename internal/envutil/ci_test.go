package envutil

import "testing"

func TestIsCI(t *testing.T) {
	t.Parallel()

	cases := map[string]bool{
		"":        false,
		"0":       false,
		"false":   false,
		"False":   false,
		" false ": false,
		"no":      false,
		"NO":      false,
		"1":       true,
		"true":    true,
		"yes":     true,
		"github":  true,
	}
	for value, want := range cases {
		getenv := func(key string) string {
			if key == "CI" {
				return value
			}
			return ""
		}
		if got := IsCI(getenv); got != want {
			t.Errorf("IsCI(CI=%q) = %v, want %v", value, got, want)
		}
	}
}
