package assets

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"MetaBlog/internal/latex/ast"
	"MetaBlog/internal/pathutil"
)

type MemoryStore interface {
	PutFile(path string, data []byte, modTime time.Time)
	FileFresh(path string, sourceInfo os.FileInfo) bool
}

type Converter struct {
	SourceRoot  string
	OutDir      string
	AssetSubdir string
	LinkPrefix  string
	Warnings    *[]string
	Skip        bool
	Log         io.Writer
	LogMu       *sync.Mutex
	Stats       *Stats
	MemoryStore MemoryStore
}

type Stats struct {
	mu           sync.Mutex
	Fresh        int
	Copied       int
	PDFConverted int
	Skipped      int
}

type StatsSnapshot struct {
	Fresh        int
	Copied       int
	PDFConverted int
	Skipped      int
}

var inkscapeVersionCache sync.Map

func (c Converter) Process(doc *ast.Document) error {
	if c.Skip {
		return nil
	}
	return c.walkBlocks(doc.Children)
}

func (c Converter) ConvertFile(src string) (string, error) {
	if c.Skip {
		c.recordSkipped()
		c.logf("Asset skipped by configuration: %s\n", src)
		return "", nil
	}
	return c.convert(src)
}

func (c Converter) walkBlocks(blocks []ast.Block) error {
	for _, block := range blocks {
		switch n := block.(type) {
		case *ast.Section:
			if err := c.walkBlocks(n.Children); err != nil {
				return err
			}
		case *ast.Figure:
			for _, image := range figureImages(n) {
				if image == nil || image.SourcePath == "" {
					continue
				}
				out, err := c.convert(image.SourcePath)
				if err != nil {
					c.warn(err.Error())
				} else {
					image.OutputPath = filepath.ToSlash(out)
				}
			}
		case *ast.List:
			for _, item := range n.Items {
				if err := c.walkBlocks(item.Blocks); err != nil {
					return err
				}
			}
		case *ast.TCB:
			if err := c.walkBlocks(n.Children); err != nil {
				return err
			}
		}
	}
	return nil
}

func figureImages(fig *ast.Figure) []*ast.Image {
	if len(fig.Images) > 0 {
		return fig.Images
	}
	if fig.Image != nil {
		return []*ast.Image{fig.Image}
	}
	return nil
}

func (c Converter) convert(src string) (string, error) {
	cleanNative, err := pathutil.CleanRelativePath(src)
	if err != nil {
		return "", fmt.Errorf("asset path not allowed %s: %w", src, err)
	}
	clean := filepath.ToSlash(cleanNative)
	sourcePath := filepath.Clean(filepath.Join(c.SourceRoot, cleanNative))
	if !pathutil.IsWithinDir(c.SourceRoot, sourcePath) {
		return "", fmt.Errorf("asset path escapes source root: %s", src)
	}
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return "", fmt.Errorf("asset not found %s: %w", src, err)
	}
	relNoExt := strings.TrimSuffix(clean, filepath.Ext(clean))
	ext := strings.ToLower(filepath.Ext(clean))
	if ext == ".pdf" {
		relOut := filepath.ToSlash(relNoExt + ".svg")
		assetRel := c.assetRelPath(relOut)
		memRel := filepath.ToSlash(filepath.Join("assets", filepath.FromSlash(assetRel)))
		if c.MemoryStore != nil {
			if c.MemoryStore.FileFresh(memRel, sourceInfo) {
				c.recordFresh()
				return c.linkPath("assets", assetRel), nil
			}
			c.recordPDFConverted()
			c.logf("Asset convert PDF to SVG in memory: %s -> %s\n", filepath.ToSlash(sourcePath), memRel)
			data, err := convertPDFToBytes(sourcePath)
			if err != nil {
				return "", fmt.Errorf("convert pdf %s: %w", src, err)
			}
			c.MemoryStore.PutFile(memRel, data, sourceInfo.ModTime())
			return c.linkPath("assets", assetRel), nil
		}
		outPath := filepath.Join(c.OutDir, "assets", filepath.FromSlash(assetRel))
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return "", err
		}
		if assetOutputFresh(sourceInfo, outPath) {
			c.recordFresh()
			return c.linkPath("assets", assetRel), nil
		}
		c.recordPDFConverted()
		c.logf("Asset convert PDF to SVG: %s -> %s\n", filepath.ToSlash(sourcePath), filepath.ToSlash(outPath))
		if err := convertPDF(sourcePath, outPath); err != nil {
			return "", fmt.Errorf("convert pdf %s: %w", src, err)
		}
		return c.linkPath("assets", assetRel), nil
	}
	relOut := filepath.ToSlash(clean)
	assetRel := c.assetRelPath(relOut)
	memRel := filepath.ToSlash(filepath.Join("assets", filepath.FromSlash(assetRel)))
	if c.MemoryStore != nil {
		if c.MemoryStore.FileFresh(memRel, sourceInfo) {
			c.recordFresh()
			return c.linkPath("assets", assetRel), nil
		}
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return "", err
		}
		c.recordCopied()
		c.logf("Asset copy to memory: %s -> %s\n", filepath.ToSlash(sourcePath), memRel)
		c.MemoryStore.PutFile(memRel, data, sourceInfo.ModTime())
		return c.linkPath("assets", assetRel), nil
	}
	outPath := filepath.Join(c.OutDir, "assets", filepath.FromSlash(assetRel))
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return "", err
	}
	if assetOutputFresh(sourceInfo, outPath) {
		c.recordFresh()
		return c.linkPath("assets", assetRel), nil
	}
	c.recordCopied()
	c.logf("Asset copy: %s -> %s\n", filepath.ToSlash(sourcePath), filepath.ToSlash(outPath))
	if err := copyFile(sourcePath, outPath); err != nil {
		return "", err
	}
	return c.linkPath("assets", assetRel), nil
}

func assetOutputFresh(sourceInfo os.FileInfo, outPath string) bool {
	outInfo, err := os.Stat(outPath)
	if err != nil || outInfo.IsDir() || outInfo.Size() == 0 {
		return false
	}
	return !sourceInfo.ModTime().After(outInfo.ModTime())
}

func (c Converter) assetRelPath(rel string) string {
	subdir := strings.Trim(strings.TrimPrefix(filepath.ToSlash(c.AssetSubdir), "./"), "/")
	rel = strings.Trim(strings.TrimPrefix(filepath.ToSlash(rel), "./"), "/")
	if subdir == "" {
		return rel
	}
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(subdir), filepath.FromSlash(rel)))
}

func (c Converter) linkPath(parts ...string) string {
	var clean []string
	prefix := strings.TrimRight(filepath.ToSlash(c.LinkPrefix), "/")
	if prefix != "" {
		clean = append(clean, prefix)
	}
	for _, part := range parts {
		part = strings.Trim(filepath.ToSlash(part), "/")
		if part != "" {
			clean = append(clean, part)
		}
	}
	return strings.Join(clean, "/")
}

func convertPDF(src, dst string) error {
	_ = os.Remove(dst)
	if bin, err := exec.LookPath("pdftocairo"); err == nil {
		cmd := exec.Command(bin, "-svg", src, dst)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("pdftocairo: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return validateOutput(dst)
	}
	if bin, err := exec.LookPath("mutool"); err == nil {
		cmd := exec.Command(bin, "convert", "-o", dst, src)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("mutool: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return validateOutput(dst)
	}
	if bin, err := exec.LookPath("inkscape"); err == nil {
		cmd := exec.Command(bin, inkscapeArgs(bin, src, dst)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("inkscape: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return validateOutput(dst)
	}
	return fmt.Errorf("no PDF to SVG converter found")
}

func inkscapeArgs(bin, src, dst string) []string {
	version := inkscapeMajorVersion(bin)
	if version >= 1 {
		return []string{src, "--export-type=svg", "--export-filename=" + dst}
	}
	return []string{src, "--export-type=svg", "--export-file=" + dst}
}

func inkscapeMajorVersion(bin string) int {
	if cached, ok := inkscapeVersionCache.Load(bin); ok {
		return cached.(int)
	}
	version := detectInkscapeMajorVersion(bin)
	inkscapeVersionCache.Store(bin, version)
	return version
}

func detectInkscapeMajorVersion(bin string) int {
	cmd := exec.Command(bin, "--version")
	out, err := cmd.Output()
	if err != nil {
		return -1
	}
	match := regexp.MustCompile(`Inkscape\s+(\d+)`).FindStringSubmatch(string(out))
	if len(match) < 2 {
		return -1
	}
	version, err := strconv.Atoi(match[1])
	if err != nil {
		return -1
	}
	return version
}

func convertPDFToBytes(src string) ([]byte, error) {
	tempDir, err := os.MkdirTemp("", "metablog-asset-pdf-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)
	dst := filepath.Join(tempDir, "asset.svg")
	if err := convertPDF(src, dst); err != nil {
		return nil, err
	}
	return os.ReadFile(dst)
}

func validateOutput(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return fmt.Errorf("converter wrote empty file %s", path)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func (c Converter) warn(msg string) {
	if c.Warnings != nil {
		*c.Warnings = append(*c.Warnings, msg)
	}
}

func (c Converter) recordFresh() {
	if c.Stats != nil {
		c.Stats.RecordFresh()
	}
}

func (c Converter) recordCopied() {
	if c.Stats != nil {
		c.Stats.RecordCopied()
	}
}

func (c Converter) recordPDFConverted() {
	if c.Stats != nil {
		c.Stats.RecordPDFConverted()
	}
}

func (c Converter) recordSkipped() {
	if c.Stats != nil {
		c.Stats.RecordSkipped()
	}
}

func (s *Stats) RecordFresh() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Fresh++
}

func (s *Stats) RecordCopied() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Copied++
}

func (s *Stats) RecordPDFConverted() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PDFConverted++
}

func (s *Stats) RecordSkipped() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Skipped++
}

func (s *Stats) Snapshot() StatsSnapshot {
	if s == nil {
		return StatsSnapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return StatsSnapshot{
		Fresh:        s.Fresh,
		Copied:       s.Copied,
		PDFConverted: s.PDFConverted,
		Skipped:      s.Skipped,
	}
}

func (c Converter) logf(format string, args ...any) {
	if c.Log == nil {
		return
	}
	if c.LogMu != nil {
		c.LogMu.Lock()
		defer c.LogMu.Unlock()
	}
	fmt.Fprintf(c.Log, format, args...)
}
