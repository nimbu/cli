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
	"path/filepath"
	"sort"
	"strings"
)

func buildCompressedTemplates(root string) (compressed string, fingerprint string, err error) {
	entries, err := collectTemplateEntries(root)
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

func computeTemplateFingerprint(root string) (string, error) {
	entries, err := collectTemplateEntries(root)
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
	return hex.EncodeToString(hash.Sum(nil)), nil
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
