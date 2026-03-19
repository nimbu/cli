package bootstrap

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

type excludeMatcher struct {
	pattern string
	re      *regexp.Regexp
}

// gitTrackedFiles returns all files tracked by git in dir, as forward-slash
// relative paths. The directory must be inside a git repository with at least
// one commit.
func gitTrackedFiles(dir string) ([]string, error) {
	cmd := exec.Command("git", "-C", dir, "ls-files", "-z")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git ls-files in %s: %s", dir, msg)
	}

	entries := strings.Split(stdout.String(), "\x00")
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry != "" {
			files = append(files, entry)
		}
	}
	return files, nil
}

// compileExcludePatterns compiles gitignore-style exclude patterns into
// matchers. Pattern semantics:
//   - Trailing "/"  (e.g. "bootstrap/")           → directory prefix match
//   - No "/" in pattern (e.g. "*.md", "README.md") → matches at any depth
//   - Contains "/" (e.g. "src/foo/Bar*")           → anchored to repo root
func compileExcludePatterns(patterns []string) ([]excludeMatcher, error) {
	matchers := make([]excludeMatcher, 0, len(patterns))
	for _, p := range patterns {
		if strings.TrimSpace(p) == "" {
			continue
		}
		re, err := compileExcludePattern(p)
		if err != nil {
			return nil, fmt.Errorf("bad exclude pattern %q: %w", p, err)
		}
		matchers = append(matchers, excludeMatcher{pattern: p, re: re})
	}
	return matchers, nil
}

func compileExcludePattern(pattern string) (*regexp.Regexp, error) {
	if dir, ok := strings.CutSuffix(pattern, "/"); ok {
		prefix := excludeGlobToRegex(dir)
		return regexp.Compile("^" + prefix + "/")
	}

	glob := excludeGlobToRegex(pattern)

	if !strings.Contains(pattern, "/") {
		return regexp.Compile("(^|/)" + glob + "$")
	}

	return regexp.Compile("^" + glob + "$")
}

// excludeGlobToRegex converts a simple glob to a regex fragment.
// Supports * (single-level wildcard) and ? (single char).
func excludeGlobToRegex(pattern string) string {
	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			b.WriteByte('\\')
			b.WriteByte(pattern[i])
		default:
			b.WriteByte(pattern[i])
		}
	}
	return b.String()
}

// excludeMatches returns true if path matches any of the compiled matchers.
func excludeMatches(matchers []excludeMatcher, path string) bool {
	for _, m := range matchers {
		if m.re.MatchString(path) {
			return true
		}
	}
	return false
}

// pathIsClaimed reports whether file is claimed by any of the declared paths.
// A declared path claims a file when it matches exactly or when the file lives
// under the declared path treated as a directory prefix.
func pathIsClaimed(file string, declaredPaths []string) bool {
	for _, dp := range declaredPaths {
		if file == dp || strings.HasPrefix(file, dp+"/") {
			return true
		}
	}
	return false
}
