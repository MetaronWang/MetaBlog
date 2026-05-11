package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"MetaBlog/internal/assets"
	"MetaBlog/internal/bib"
	"MetaBlog/internal/latex/blocks"
	"MetaBlog/internal/latex/parser"
	"MetaBlog/internal/latex/source"
	"MetaBlog/internal/latexml"
	"MetaBlog/internal/render"
	"MetaBlog/internal/site"
)

type Config struct {
	Input           string
	OutDir          string
	Site            bool
	RootDir         string
	SiteConfig      string
	ArticlesFile    string
	ArticleWorkers  int
	LaTeXMLBin      string
	DumpAST         bool
	KeepTemp        bool
	Strict          bool
	NoAssets        bool
	Force           bool
	NoLaTeXMLCache  bool
	SubsetFonts     bool
	LaTeXMLWorkers  int
	Log             io.Writer
	LogMu           *sync.Mutex
	LaTeXMLIdentity *latexml.CacheIdentity
}

func Run(cfg Config) error {
	cfg.ensureLaTeXMLIdentity()
	if cfg.Site {
		return RunSite(cfg)
	}
	logSingleBuildOverview(cfg)
	cfg.prepareLaTeXMLIdentity()
	docCfg, docLog := cfg.withDocumentLog()
	docCfg.logf("Document started: %s\n", filepath.ToSlash(cfg.Input))
	htmlText, warnings, doc, err := buildArticle(docCfg, cfg.Input, cfg.OutDir, "", "", render.Options{})
	if err != nil {
		logWarnings(docCfg, warnings)
		docCfg.logf("Document failed: %v\n", err)
		cfg.flushDocumentLog("single article", docLog)
		return err
	}
	logWarnings(docCfg, warnings)
	if cfg.Strict && len(warnings) > 0 {
		docCfg.logf("Strict mode failed: %d warning(s)\n", len(warnings))
		cfg.flushDocumentLog("single article", docLog)
		return fmt.Errorf("strict mode failed with %d warnings", len(warnings))
	}
	if cfg.DumpAST {
		if err := writeAST(cfg.OutDir, doc); err != nil {
			docCfg.logf("Write AST failed: %v\n", err)
			cfg.flushDocumentLog("single article", docLog)
			return err
		}
		docCfg.logf("AST written: %s\n", filepath.ToSlash(filepath.Join(cfg.OutDir, "debug", "ast.json")))
	}
	if err := site.WriteStaticWithOptions(cfg.OutDir, warnings, site.StaticOptions{
		SkipFontFiles: cfg.SubsetFonts,
	}); err != nil {
		docCfg.logf("Static assets write failed: %v\n", err)
		cfg.flushDocumentLog("single article", docLog)
		return err
	}
	docCfg.logf("Static assets written\n")
	if err := os.WriteFile(filepath.Join(cfg.OutDir, "index.html"), []byte(htmlText), 0644); err != nil {
		docCfg.logf("Write output failed: %v\n", err)
		cfg.flushDocumentLog("single article", docLog)
		return err
	}
	if cfg.SubsetFonts {
		staticDir := filepath.Join("web", "static")
		docCfg.logf("Subsetting fonts by generated HTML content...\n")
		if err := site.SubsetFontsByHTML(cfg.OutDir, staticDir); err != nil {
			docCfg.logf("Font subsetting failed: %v\n", err)
			cfg.flushDocumentLog("single article", docLog)
			return err
		}
	}
	docCfg.logf("Document built: %s warning(s)=%d\n", filepath.ToSlash(filepath.Join(cfg.OutDir, "index.html")), len(warnings))
	cfg.flushDocumentLog("single article", docLog)
	return nil
}

func buildArticle(cfg Config, input, outDir, assetSubdir, linkPrefix string, opts render.Options) (string, []string, any, error) {
	phaseStarted := time.Now()
	loaded, err := source.Load(input)
	sourceDuration := time.Since(phaseStarted)
	if err != nil {
		return "", nil, nil, err
	}
	warnings := append([]string{}, loaded.Warnings...)
	cfg.logf("Document source loaded: input=%s root=%s warning(s)=%d\n", filepath.ToSlash(loaded.InputFile), filepath.ToSlash(loaded.RootDir), len(warnings))

	phaseStarted = time.Now()
	lifted := blocks.Lift(loaded.Document)
	liftDuration := time.Since(phaseStarted)
	latexmlWorkers := lateXMLWorkerCount(len(lifted.Blocks), cfg.LaTeXMLWorkers)
	if len(lifted.Blocks) > 0 {
		cfg.logf("LaTeXML complex blocks: %d block(s), %d worker(s)\n", len(lifted.Blocks), latexmlWorkers)
	} else {
		cfg.logf("LaTeXML complex blocks: none\n")
	}
	logWriter, logMu := cfg.logging()
	lateXMLStats := &latexml.CacheStats{}
	runner := latexml.Runner{
		Bin:      cfg.LaTeXMLBin,
		CacheDir: lateXMLCacheDir(cfg),
		KeepTemp: cfg.KeepTemp,
		Identity: cfg.LaTeXMLIdentity,
		Warnings: &warnings,
		Log:      logWriter,
		LogMu:    logMu,
		Stats:    lateXMLStats,
	}
	phaseStarted = time.Now()
	if err := convertComplexBlocks(runner, lifted.Blocks, cfg.LaTeXMLWorkers); err != nil {
		if len(lifted.Blocks) > 0 {
			logLaTeXMLCacheStats(cfg, lateXMLStats)
		}
		return "", warnings, nil, err
	}
	latexmlDuration := time.Since(phaseStarted)
	if len(lifted.Blocks) > 0 {
		logLaTeXMLCacheStats(cfg, lateXMLStats)
	}

	phaseStarted = time.Now()
	doc, err := parser.Parse(lifted.Text, lifted.Blocks, loaded.InputFile, loaded.RootDir)
	parseDuration := time.Since(phaseStarted)
	if err != nil {
		return "", warnings, nil, err
	}
	doc.Warnings = append(warnings, doc.Warnings...)

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", doc.Warnings, doc, err
	}
	assetStats := &assets.Stats{}
	converter := assets.Converter{
		SourceRoot:  loaded.RootDir,
		OutDir:      outDir,
		AssetSubdir: assetSubdir,
		LinkPrefix:  linkPrefix,
		Warnings:    &doc.Warnings,
		Skip:        cfg.NoAssets,
		Log:         logWriter,
		LogMu:       logMu,
		Stats:       assetStats,
	}
	if cfg.NoAssets {
		cfg.logf("Assets skipped by -no-assets\n")
	} else {
		cfg.logf("Processing document assets: source=%s subdir=%s\n", filepath.ToSlash(loaded.RootDir), assetSubdirForLog(assetSubdir))
	}
	phaseStarted = time.Now()
	if err := converter.Process(doc); err != nil {
		if !cfg.NoAssets {
			logAssetStats(cfg, "Asset summary", assetStats)
		}
		return "", doc.Warnings, doc, err
	}
	assetDuration := time.Since(phaseStarted)
	if !cfg.NoAssets {
		logAssetStats(cfg, "Asset summary", assetStats)
	}
	phaseStarted = time.Now()
	doc.References = bib.Load(loaded.RootDir, doc.BibliographyFiles, &doc.Warnings)
	bibDuration := time.Since(phaseStarted)
	if len(doc.BibliographyFiles) > 0 {
		cfg.logf("Bibliography files: %s\n", strings.Join(doc.BibliographyFiles, ", "))
	}

	phaseStarted = time.Now()
	htmlText := render.RenderWithOptions(doc, opts)
	renderDuration := time.Since(phaseStarted)
	cfg.logf("Document timing: source=%s lift=%s latexml=%s parse=%s assets=%s bibliography=%s render=%s\n",
		roundForLog(sourceDuration),
		roundForLog(liftDuration),
		roundForLog(latexmlDuration),
		roundForLog(parseDuration),
		roundForLog(assetDuration),
		roundForLog(bibDuration),
		roundForLog(renderDuration))
	return htmlText, doc.Warnings, doc, nil
}

func roundForLog(d time.Duration) time.Duration {
	return d.Round(time.Millisecond)
}

func (cfg *Config) ensureLaTeXMLIdentity() {
	if cfg.LaTeXMLIdentity == nil {
		cfg.LaTeXMLIdentity = &latexml.CacheIdentity{}
	}
}

func (cfg Config) prepareLaTeXMLIdentity() {
	if cfg.NoLaTeXMLCache || cfg.KeepTemp || cfg.LaTeXMLIdentity == nil {
		return
	}
	started := time.Now()
	runner := latexml.Runner{
		Bin:      cfg.LaTeXMLBin,
		CacheDir: lateXMLCacheDir(cfg),
		KeepTemp: cfg.KeepTemp,
		Identity: cfg.LaTeXMLIdentity,
	}
	if bin, version, ok := runner.PrepareCacheIdentity(); ok {
		cfg.logf("LaTeXML identity: bin=%s version=%s (%s)\n", filepath.ToSlash(bin), version, roundForLog(time.Since(started)))
	} else {
		cfg.logf("LaTeXML identity: unavailable; cache will miss until latexmlc is resolvable (%s)\n", roundForLog(time.Since(started)))
	}
}

func lateXMLCacheDir(cfg Config) string {
	if cfg.NoLaTeXMLCache {
		return ""
	}
	return filepath.Join(cacheRootDir(cfg.RootDir), "latexml")
}

func cacheRootDir(root string) string {
	if root == "" {
		root = "."
	}
	if !filepath.IsAbs(root) {
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
	}
	return filepath.Join(root, ".metablog-cache")
}

func lateXMLWorkerCount(blockCount, configured int) int {
	if blockCount == 0 {
		return 0
	}
	workers := configured
	if workers <= 0 {
		workers = runtime.NumCPU() * 2
		if workers > 4 {
			workers = 4
		}
	}
	if workers < 1 {
		workers = 1
	}
	if workers > blockCount {
		workers = blockCount
	}
	return workers
}

func convertComplexBlocks(runner latexml.Runner, blockMap map[string]*blocks.ComplexBlock, workers int) error {
	if len(blockMap) == 0 {
		return nil
	}
	workers = lateXMLWorkerCount(len(blockMap), workers)
	var warningMu sync.Mutex
	runner.WarningMu = &warningMu

	jobs := make(chan *blocks.ComplexBlock)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for block := range jobs {
				runner.Convert(block)
			}
		}()
	}
	for _, block := range blockMap {
		jobs <- block
	}
	close(jobs)
	wg.Wait()
	return nil
}

var defaultLogMu sync.Mutex

func (cfg Config) logging() (io.Writer, *sync.Mutex) {
	if cfg.Log == nil {
		return nil, nil
	}
	if cfg.LogMu != nil {
		return cfg.Log, cfg.LogMu
	}
	return cfg.Log, &defaultLogMu
}

func (cfg Config) logf(format string, args ...any) {
	w, mu := cfg.logging()
	logf(w, mu, format, args...)
}

func logf(w io.Writer, mu *sync.Mutex, format string, args ...any) {
	if w == nil {
		return
	}
	if mu != nil {
		mu.Lock()
		defer mu.Unlock()
	}
	fmt.Fprintf(w, format, args...)
}

func (cfg Config) withDocumentLog() (Config, *bytes.Buffer) {
	var buf bytes.Buffer
	docCfg := cfg
	docCfg.Log = &buf
	docCfg.LogMu = &sync.Mutex{}
	return docCfg, &buf
}

func (cfg Config) flushDocumentLog(name string, buf *bytes.Buffer) {
	if buf == nil || buf.Len() == 0 {
		return
	}
	cfg.flushDocumentLogString(name, buf.String())
}

func (cfg Config) flushDocumentLogString(name, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	w, mu := cfg.logging()
	if w == nil {
		return
	}
	if mu != nil {
		mu.Lock()
		defer mu.Unlock()
	}
	fmt.Fprintf(w, "\n[%s]\n%s", name, text)
	if !strings.HasSuffix(text, "\n") {
		fmt.Fprintln(w)
	}
}

func logSingleBuildOverview(cfg Config) {
	cfg.logf("MetaBlog build starting\n")
	cfg.logf("Mode: single article\n")
	cfg.logf("Input: %s\n", filepath.ToSlash(cfg.Input))
	cfg.logf("Output: %s\n", filepath.ToSlash(cfg.OutDir))
	logBuildOptions(cfg)
}

func logSiteBuildOverview(cfg Config, activeArticles int) {
	cfg.logf("MetaBlog build starting\n")
	cfg.logf("Mode: site\n")
	cfg.logf("Root: %s\n", filepath.ToSlash(cfg.RootDir))
	cfg.logf("Output: %s\n", filepath.ToSlash(cfg.OutDir))
	cfg.logf("Config: %s\n", filepath.ToSlash(cfg.SiteConfig))
	cfg.logf("Articles: %s\n", filepath.ToSlash(cfg.ArticlesFile))
	cfg.logf("Active articles: %d\n", activeArticles)
	logBuildOptions(cfg)
}

func logBuildOptions(cfg Config) {
	cacheStatus := filepath.ToSlash(lateXMLCacheDir(cfg))
	if cfg.NoLaTeXMLCache {
		cacheStatus = "disabled by -no-latexml-cache"
	} else if cfg.KeepTemp {
		cacheStatus = "disabled while -keep-temp is enabled"
	}
	cfg.logf("Options: strict=%t dump_ast=%t no_assets=%t subset_fonts=%t keep_temp=%t force=%t\n",
		cfg.Strict, cfg.DumpAST, cfg.NoAssets, cfg.SubsetFonts, cfg.KeepTemp, cfg.Force)
	cfg.logf("LaTeXML: bin=%s workers=%s cache=%s\n", valueOrDefault(cfg.LaTeXMLBin, "latexmlc"), workerOption(cfg.LaTeXMLWorkers), cacheStatus)
}

func logLaTeXMLCacheStats(cfg Config, stats *latexml.CacheStats) {
	s := stats.Snapshot()
	cfg.logf("LaTeXML cache summary: hit=%d miss=%d bypass=%d", s.Hits, s.Misses, s.Bypassed)
	if len(s.BypassReasons) > 0 {
		var parts []string
		for reason, count := range s.BypassReasons {
			parts = append(parts, fmt.Sprintf("%s=%d", reason, count))
		}
		sort.Strings(parts)
		cfg.logf(" (%s)", strings.Join(parts, ", "))
	}
	cfg.logf("\n")
}

func logAssetStats(cfg Config, label string, stats *assets.Stats) {
	s := stats.Snapshot()
	cfg.logf("%s: fresh=%d copied=%d pdf_converted=%d skipped=%d\n", label, s.Fresh, s.Copied, s.PDFConverted, s.Skipped)
}

func logWarnings(cfg Config, warnings []string) {
	if len(warnings) == 0 {
		return
	}
	cfg.logf("Warnings (%d):\n", len(warnings))
	for _, warning := range warnings {
		cfg.logf("  - %s\n", warning)
	}
}

func workerOption(workers int) string {
	if workers <= 0 {
		return "auto"
	}
	return fmt.Sprint(workers)
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func assetSubdirForLog(subdir string) string {
	if strings.TrimSpace(subdir) == "" {
		return "."
	}
	return filepath.ToSlash(subdir)
}

func writeAST(outDir string, v any) error {
	if err := os.MkdirAll(filepath.Join(outDir, "debug"), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "debug", "ast.json"), b, 0644)
}
