package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type CacheCLIConfig struct {
	RootDir string
	Out     io.Writer
}

func RunCacheClean(cfg CacheCLIConfig) error {
	root := cfg.RootDir
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	cacheDir := cacheRootDir(absRoot)
	if err := validateCacheDir(absRoot, cacheDir); err != nil {
		return err
	}
	if _, err := os.Stat(cacheDir); err != nil {
		if os.IsNotExist(err) {
			if cfg.Out != nil {
				fmt.Fprintf(cfg.Out, "Cache directory does not exist: %s\n", filepath.ToSlash(cacheDir))
			}
			return nil
		}
		return err
	}
	if err := os.RemoveAll(cacheDir); err != nil {
		return err
	}
	if cfg.Out != nil {
		fmt.Fprintf(cfg.Out, "Removed cache directory: %s\n", filepath.ToSlash(cacheDir))
	}
	return nil
}

func validateCacheDir(root, cacheDir string) error {
	root = filepath.Clean(root)
	cacheDir = filepath.Clean(cacheDir)
	if filepath.Base(cacheDir) != ".metablog-cache" {
		return fmt.Errorf("refusing to remove non-cache directory: %s", cacheDir)
	}
	rel, err := filepath.Rel(root, cacheDir)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("refusing to remove cache outside root: %s", cacheDir)
	}
	return nil
}
