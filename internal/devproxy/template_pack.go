package devproxy

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
)

// TemplateOverlay is an in-memory template replacement supplied by the local dev server.
type TemplateOverlay struct {
	Content string `json:"content"`
	Path    string `json:"path"`
	Type    string `json:"type"`
}

func buildCompressedTemplates(root string, overlays []TemplateOverlay) (compressed string, fingerprint string, err error) {
	entries, err := collectTemplateEntries(root)
	if err != nil {
		return "", "", err
	}
	normalized, err := normalizedOverlays(overlays)
	if err != nil {
		return "", "", err
	}

	templates := map[string]map[string]string{}
	for _, dir := range templateDirs {
		templates[dir] = map[string]string{}
	}

	hash := sha256.New()
	for _, entry := range entries {
		templates[entry.TemplateType][entry.RelativePath] = entry.Content
		_, _ = io.WriteString(hash, entry.TemplateType)
		_, _ = io.WriteString(hash, "|")
		_, _ = io.WriteString(hash, entry.RelativePath)
		_, _ = io.WriteString(hash, "|")
		_, _ = io.WriteString(hash, fmt.Sprintf("%d|%d\n", entry.ModTimeUnixNano, entry.Size))
		_, _ = io.WriteString(hash, entry.Content)
		_, _ = io.WriteString(hash, "\n")
	}
	for _, overlay := range normalized {
		templates[overlay.Type][overlay.Path] = overlay.Content
		writeOverlayHash(hash, overlay)
	}

	jsonData, err := json.Marshal(templates)
	if err != nil {
		return "", "", err
	}

	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(jsonData); err != nil {
		return "", "", err
	}
	if err := zw.Close(); err != nil {
		return "", "", err
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), hex.EncodeToString(hash.Sum(nil)), nil
}

func computeTemplateFingerprint(root string, overlays []TemplateOverlay) (string, error) {
	entries, err := collectTemplateEntries(root)
	if err != nil {
		return "", err
	}
	normalized, err := normalizedOverlays(overlays)
	if err != nil {
		return "", err
	}

	hash := sha256.New()
	for _, entry := range entries {
		_, _ = io.WriteString(hash, entry.TemplateType)
		_, _ = io.WriteString(hash, "|")
		_, _ = io.WriteString(hash, entry.RelativePath)
		_, _ = io.WriteString(hash, "|")
		_, _ = io.WriteString(hash, fmt.Sprintf("%d|%d\n", entry.ModTimeUnixNano, entry.Size))
		_, _ = io.WriteString(hash, entry.Content)
		_, _ = io.WriteString(hash, "\n")
	}
	for _, overlay := range normalized {
		writeOverlayHash(hash, overlay)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeOverlayHash(hash io.Writer, overlay TemplateOverlay) {
	_, _ = io.WriteString(hash, "overlay|")
	_, _ = io.WriteString(hash, overlay.Type)
	_, _ = io.WriteString(hash, "|")
	_, _ = io.WriteString(hash, overlay.Path)
	_, _ = io.WriteString(hash, "|")
	_, _ = io.WriteString(hash, fmt.Sprintf("%d\n", len(overlay.Content)))
	_, _ = io.WriteString(hash, overlay.Content)
	_, _ = io.WriteString(hash, "\n")
}

func templateTypeAllowed(value string) bool {
	for _, dir := range templateDirs {
		if value == dir {
			return true
		}
	}
	return false
}

func normalizeOverlayPath(value string) (string, error) {
	slashed := strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	clean := pathpkg.Clean(slashed)
	if clean == "." || clean == "" ||
		strings.HasPrefix(slashed, "/") ||
		strings.HasPrefix(clean, "../") ||
		clean == ".." ||
		filepath.IsAbs(value) ||
		isWindowsDrivePath(clean) {
		return "", fmt.Errorf("invalid overlay path %q", value)
	}
	return clean, nil
}

func isWindowsDrivePath(value string) bool {
	return len(value) >= 2 && value[1] == ':'
}

func normalizedOverlays(overlays []TemplateOverlay) ([]TemplateOverlay, error) {
	out := make([]TemplateOverlay, 0, len(overlays))
	seen := map[string]struct{}{}
	for _, overlay := range overlays {
		if !templateTypeAllowed(overlay.Type) {
			return nil, fmt.Errorf("invalid overlay template type %q", overlay.Type)
		}
		cleanPath, err := normalizeOverlayPath(overlay.Path)
		if err != nil {
			return nil, err
		}
		if !isTemplateFile(cleanPath) {
			return nil, fmt.Errorf("overlay template path must end in .liquid or .liquid.haml: %s", cleanPath)
		}
		key := overlay.Type + "/" + cleanPath
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate overlay template %s", key)
		}
		seen[key] = struct{}{}
		out = append(out, TemplateOverlay{Type: overlay.Type, Path: cleanPath, Content: overlay.Content})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type == out[j].Type {
			return out[i].Path < out[j].Path
		}
		return out[i].Type < out[j].Type
	})
	return out, nil
}

type templateEntry struct {
	Content         string
	ModTimeUnixNano int64
	RelativePath    string
	Size            int64
	TemplateType    string
}

func collectTemplateEntries(root string) ([]templateEntry, error) {
	entries := make([]templateEntry, 0, 64)

	for _, templateType := range templateDirs {
		templateRoot := filepath.Join(root, templateType)
		seen := map[string]struct{}{}

		err := walkFiles(templateRoot, true, seen, func(abs string, logical string, info os.FileInfo) error {
			if !isTemplateFile(logical) {
				return nil
			}

			data, err := os.ReadFile(abs)
			if err != nil {
				return fmt.Errorf("read template %s: %w", logical, err)
			}

			rel, err := filepath.Rel(templateRoot, logical)
			if err != nil {
				rel = filepath.Base(logical)
			}
			rel = filepath.ToSlash(rel)

			entries = append(entries, templateEntry{
				Content:         string(data),
				ModTimeUnixNano: info.ModTime().UnixNano(),
				RelativePath:    rel,
				Size:            info.Size(),
				TemplateType:    templateType,
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].TemplateType == entries[j].TemplateType {
			return entries[i].RelativePath < entries[j].RelativePath
		}
		return entries[i].TemplateType < entries[j].TemplateType
	})

	return entries, nil
}

func isTemplateFile(path string) bool {
	name := filepath.Base(path)
	return strings.HasSuffix(name, ".liquid") || strings.HasSuffix(name, ".liquid.haml")
}

func walkFiles(root string, followSymlinks bool, seen map[string]struct{}, fn func(abs string, logical string, info os.FileInfo) error) error {
	info, err := os.Lstat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}

	return walkFilesRec(root, root, followSymlinks, seen, fn)
}

func walkFilesRec(current string, logicalBase string, followSymlinks bool, seen map[string]struct{}, fn func(abs string, logical string, info os.FileInfo) error) error {
	entries, err := os.ReadDir(current)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		full := filepath.Join(current, entry.Name())
		logical := filepath.Join(logicalBase, entry.Name())

		entryInfo, err := os.Lstat(full)
		if err != nil {
			continue
		}

		if entryInfo.Mode()&os.ModeSymlink != 0 {
			if !followSymlinks {
				continue
			}
			target, err := filepath.EvalSymlinks(full)
			if err != nil {
				continue
			}
			targetInfo, err := os.Stat(target)
			if err != nil {
				continue
			}
			if targetInfo.IsDir() {
				real, err := filepath.Abs(target)
				if err != nil {
					real = target
				}
				if _, ok := seen[real]; ok {
					continue
				}
				seen[real] = struct{}{}
				if err := walkFilesRec(target, logical, followSymlinks, seen, fn); err != nil {
					return err
				}
				continue
			}
			if err := fn(target, logical, targetInfo); err != nil {
				return err
			}
			continue
		}

		if entryInfo.IsDir() {
			if err := walkFilesRec(full, logical, followSymlinks, seen, fn); err != nil {
				return err
			}
			continue
		}

		if err := fn(full, logical, entryInfo); err != nil {
			return err
		}
	}

	return nil
}

func walkDirs(root string, followSymlinks bool, seen map[string]struct{}, fn func(dir string) error) error {
	info, err := os.Lstat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if !info.IsDir() {
		return nil
	}

	return walkDirsRec(root, followSymlinks, seen, fn)
}

func walkDirsRec(current string, followSymlinks bool, seen map[string]struct{}, fn func(dir string) error) error {
	if err := fn(current); err != nil {
		return err
	}

	entries, err := os.ReadDir(current)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		full := filepath.Join(current, entry.Name())
		entryInfo, err := os.Lstat(full)
		if err != nil {
			continue
		}

		if entryInfo.Mode()&os.ModeSymlink != 0 {
			if !followSymlinks {
				continue
			}
			target, err := filepath.EvalSymlinks(full)
			if err != nil {
				continue
			}
			targetInfo, err := os.Stat(target)
			if err != nil || !targetInfo.IsDir() {
				continue
			}
			real, err := filepath.Abs(target)
			if err != nil {
				real = target
			}
			if _, ok := seen[real]; ok {
				continue
			}
			seen[real] = struct{}{}
			if err := walkDirsRec(target, followSymlinks, seen, fn); err != nil {
				return err
			}
			continue
		}

		if !entryInfo.IsDir() {
			continue
		}

		if err := walkDirsRec(full, followSymlinks, seen, fn); err != nil {
			return err
		}
	}

	return nil
}
