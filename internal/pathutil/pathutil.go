package pathutil

import (
	"fmt"
	"path/filepath"
	"strings"
)

// CleanRelativePath returns a clean native relative path and rejects absolute,
// drive-relative, or parent-escaping paths.
func CleanRelativePath(rel string) (string, error) {
	original := rel
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", nil
	}
	slashRaw := strings.ReplaceAll(filepath.ToSlash(rel), "\\", "/")
	nativeRaw := filepath.FromSlash(slashRaw)
	if filepath.IsAbs(nativeRaw) || filepath.VolumeName(nativeRaw) != "" || hasWindowsDrivePrefix(slashRaw) || strings.HasPrefix(slashRaw, "/") {
		return "", fmt.Errorf("absolute path not allowed: %s", original)
	}
	rel = strings.TrimPrefix(slashRaw, "./")
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." {
		return "", nil
	}
	if filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes allowed directory: %s", original)
	}
	return clean, nil
}

func hasWindowsDrivePrefix(path string) bool {
	if len(path) < 2 || path[1] != ':' {
		return false
	}
	c := path[0]
	if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
		return false
	}
	// On Unix, "a:b" is a valid relative path (colons are legal filename characters).
	// A real Windows drive prefix always has '/' or '\' after the colon (e.g. "C:/" or "C:\").
	return len(path) == 2 || path[2] == '/' || path[2] == '\\'
}

// IsWithinDir reports whether path resolves inside root after cleaning both.
func IsWithinDir(root, path string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rootAbs = filepath.Clean(rootAbs)
	pathAbs = filepath.Clean(pathAbs)
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel == "." || (!filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
