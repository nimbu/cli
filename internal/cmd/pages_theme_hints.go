package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/nimbu/cli/internal/api"
	"github.com/nimbu/cli/internal/config"
)

type pageThemeHint struct {
	Kind   string
	Name   string
	Canvas string
}

type hintOverlayError struct {
	err  error
	hint string
}

func (e *hintOverlayError) Error() string { return e.err.Error() }
func (e *hintOverlayError) Unwrap() error { return e.err }

var (
	invalidEditableRE = regexp.MustCompile(`(?i)^invalid editable\s+(.+)$`)
	invalidSlugRE     = regexp.MustCompile(`(?i)^invalid slug\s+\(([^)]+)\)\s+for repeatable(?:\s+in canvas '([^']+)')?`)
	itemNotFoundRE    = regexp.MustCompile(`(?i)^Item '([^']+)' not found(?: in repeatable)?$`)
	liquidDefineRE    = regexp.MustCompile(`(?is){%-?\s*(?:editable_[A-Za-z0-9_]+|repeatable|canvas)\s+['"]([^'"]+)['"]`)
)

func withPageThemeHint(err error, pageTemplate string) error {
	if err == nil {
		return nil
	}
	hint := pageThemePushHint(err, pageTemplate)
	if hint == "" {
		return err
	}
	return &hintOverlayError{err: err, hint: hint}
}

func overlayHintFrom(err error) string {
	var overlay *hintOverlayError
	if errors.As(err, &overlay) {
		return overlay.hint
	}
	return ""
}

func pageThemePushHint(err error, pageTemplate string) string {
	match, ok := parsePageThemeHint(err)
	if !ok {
		return ""
	}
	subject := formatPageThemeHintSubject(match)
	only, inProject := themePushOnlySelectors(match, pageTemplate)
	phrase := "Push the theme that defines it first"
	if inProject {
		phrase = "Push the template first"
	}
	return fmt.Sprintf("%s is not in the pushed theme. %s: nimbu themes push --only %s --dry-run", subject, phrase, strings.Join(only, ","))
}

func formatPageThemeHintSubject(match pageThemeHint) string {
	if match.Kind == "repeatable" && match.Canvas != "" {
		return fmt.Sprintf("repeatable '%s' (canvas '%s')", match.Name, match.Canvas)
	}
	return fmt.Sprintf("%s '%s'", match.Kind, match.Name)
}

func parsePageThemeHint(err error) (pageThemeHint, bool) {
	if err == nil {
		return pageThemeHint{}, false
	}
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		if got, ok := hintFromStructured(apiErr.Code, apiErr.Details); ok {
			return got, true
		}
		for _, result := range apiErr.BatchResults() {
			if result.Error == nil {
				continue
			}
			if got, ok := hintFromStructured(result.Error.Code, nil); ok {
				return got, true
			}
			if got, ok := hintFromMessage(result.Error.Message); ok {
				return got, true
			}
		}
		if got, ok := hintFromMessage(apiErr.Message); ok {
			return got, true
		}
	}
	return hintFromMessage(err.Error())
}

func hintFromStructured(code string, details map[string]any) (pageThemeHint, bool) {
	switch strings.TrimSpace(code) {
	case "invalid_editable":
		return pageThemeHint{Kind: "editable", Name: structuredHintName(details), Canvas: structuredHintCanvas(details)}, structuredHintName(details) != ""
	case "invalid_slug":
		return pageThemeHint{Kind: "repeatable", Name: structuredHintName(details), Canvas: structuredHintCanvas(details)}, structuredHintName(details) != ""
	default:
		return pageThemeHint{}, false
	}
}

func structuredHintName(details map[string]any) string {
	data := structuredHintData(details)
	for _, key := range []string{"name", "slug"} {
		if value := strings.TrimSpace(stringAny(data[key])); value != "" {
			return value
		}
	}
	return ""
}

func structuredHintCanvas(details map[string]any) string {
	return strings.TrimSpace(stringAny(structuredHintData(details)["canvas"]))
}

func structuredHintData(details map[string]any) map[string]any {
	if details == nil {
		return nil
	}
	if data, ok := details["data"].(map[string]any); ok {
		return data
	}
	return details
}

func hintFromMessage(message string) (pageThemeHint, bool) {
	message = strings.TrimSpace(message)
	if match := invalidSlugRE.FindStringSubmatch(message); len(match) >= 2 {
		return pageThemeHint{Kind: "repeatable", Name: strings.TrimSpace(match[1]), Canvas: strings.TrimSpace(match[2])}, true
	}
	if match := invalidEditableRE.FindStringSubmatch(message); len(match) == 2 {
		return pageThemeHint{Kind: "editable", Name: strings.TrimSpace(match[1])}, true
	}
	if match := itemNotFoundRE.FindStringSubmatch(message); len(match) == 2 {
		return pageThemeHint{Kind: "editable", Name: strings.TrimSpace(match[1])}, true
	}
	return pageThemeHint{}, false
}

func themePushOnlySelectors(match pageThemeHint, pageTemplate string) ([]string, bool) {
	only := map[string]struct{}{}
	if path := templateOnlyPath(pageTemplate); pageTemplate != "" {
		only[path] = struct{}{}
	}
	root, inProject := themeProjectRoot()
	if inProject {
		for _, path := range localThemeFilesDefining(root, match) {
			only[path] = struct{}{}
		}
	}
	if len(only) == 0 {
		return []string{templateOnlyPath(pageTemplate)}, inProject
	}
	paths := make([]string, 0, len(only))
	for path := range only {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, inProject
}

func templateOnlyPath(pageTemplate string) string {
	name := strings.TrimSpace(pageTemplate)
	if name == "" {
		return "templates/<template>.liquid"
	}
	name = strings.TrimPrefix(name, "templates/")
	if !strings.HasSuffix(name, ".liquid") {
		name += ".liquid"
	}
	return "templates/" + name
}

func themeProjectRoot() (string, bool) {
	path, err := config.FindProjectFile()
	if err != nil {
		return "", false
	}
	return filepath.Dir(path), true
}

func localThemeFilesDefining(root string, match pageThemeHint) []string {
	names := []string{match.Name}
	if match.Canvas != "" {
		names = append(names, match.Canvas)
	}
	var found []string
	for _, dir := range []string{"templates", "snippets", "layouts"} {
		base := filepath.Join(root, dir)
		_ = filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".liquid") {
				return nil
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			if !liquidDefinesAny(content, names) {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return nil
			}
			found = append(found, filepath.ToSlash(rel))
			return nil
		})
	}
	return found
}

func liquidDefinesAny(content []byte, names []string) bool {
	wanted := map[string]struct{}{}
	for _, name := range names {
		if name != "" {
			wanted[name] = struct{}{}
		}
	}
	for _, match := range liquidDefineRE.FindAllSubmatch(content, -1) {
		if _, ok := wanted[string(match[1])]; ok {
			return true
		}
	}
	return false
}
