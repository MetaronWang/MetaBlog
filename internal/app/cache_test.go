package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCacheCleanRemovesCacheDirectory(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, ".metablog-cache", "latexml")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "entry.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := RunCacheClean(CacheCLIConfig{RootDir: dir, Out: &out}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".metablog-cache")); !os.IsNotExist(err) {
		t.Fatalf("cache directory still exists or stat failed unexpectedly: %v", err)
	}
	if !strings.Contains(out.String(), "Removed cache directory") {
		t.Fatalf("missing clean output: %q", out.String())
	}
}

func TestRunCacheCleanMissingCacheIsNoop(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if err := RunCacheClean(CacheCLIConfig{RootDir: dir, Out: &out}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "does not exist") {
		t.Fatalf("missing noop output: %q", out.String())
	}
}

func TestValidateCacheDirRejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	err := validateCacheDir(root, filepath.Join(other, ".metablog-cache"))
	if err == nil {
		t.Fatal("expected outside cache directory to be rejected")
	}
}

func TestLateXMLCacheDirCanBeDisabled(t *testing.T) {
	dir := t.TempDir()
	if got := lateXMLCacheDir(Config{RootDir: dir, NoLaTeXMLCache: true}); got != "" {
		t.Fatalf("expected disabled cache dir to be empty, got %q", got)
	}
}
