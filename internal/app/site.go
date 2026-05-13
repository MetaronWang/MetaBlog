package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"MetaBlog/internal/assets"
	"MetaBlog/internal/blog"
	"MetaBlog/internal/render"
	"MetaBlog/internal/site"
)

type articleBuildResult struct {
	index    int
	title    string
	warnings []string
	err      error
	duration time.Duration
	skipped  bool
	log      string
}

type articleBuildJob struct {
	index   int
	article blog.Article
}

type siteBuildResult struct {
	name     string
	warnings []string
	err      error
	log      string
}

func RunSite(cfg Config) error {
	cfg.ensureLaTeXMLIdentity()
	rootDir := cfg.RootDir
	if rootDir == "" {
		rootDir = "."
	}
	rootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return err
	}
	cfg.RootDir = rootDir
	siteData, err := blog.Load(rootDir, cfg.SiteConfig, cfg.ArticlesFile)
	if err != nil {
		return err
	}
	logSiteBuildOverview(cfg, len(siteData.Articles))
	cfg.prepareLaTeXMLIdentity()
	if err := os.MkdirAll(cfg.OutDir, 0755); err != nil {
		return err
	}
	if err := prepareSiteAssets(rootDir, cfg.OutDir, &siteData.Config); err != nil {
		return err
	}
	cfg.logf("Site assets prepared: logo=%s icon=%s\n", valueOrDefault(siteData.Config.Logo, "(none)"), valueOrDefault(siteData.Config.Icon, "(none)"))
	if err := writeSitePages(cfg.OutDir, siteData); err != nil {
		return err
	}
	cfg.logf("Index pages written: home=%d article_list=%d tags=%d categories=%d\n",
		blog.HomePageCount(siteData.Config, siteData.Articles),
		blog.ArticleListPageCount(siteData.Config, siteData.Articles),
		len(blog.Tags(siteData.Articles)),
		len(blog.CategoryPaths(siteData.Articles)))

	results := make(chan siteBuildResult, 2)
	go func() {
		warnings, logText, err := buildSiteAboutPage(cfg, siteData)
		results <- siteBuildResult{name: "about page", warnings: warnings, err: err, log: logText}
	}()
	go func() {
		warnings, err := buildSiteArticles(cfg, siteData)
		results <- siteBuildResult{name: "articles", warnings: warnings, err: err}
	}()

	var warnings []string
	var buildErr error
	for i := 0; i < 2; i++ {
		result := <-results
		cfg.flushDocumentLogString(result.name, result.log)
		warnings = append(warnings, result.warnings...)
		if result.err != nil && buildErr == nil {
			buildErr = fmt.Errorf("build %s: %w", result.name, result.err)
		}
	}
	if buildErr != nil {
		return buildErr
	}
	if cfg.Strict && len(warnings) > 0 {
		return fmt.Errorf("strict mode failed with %d warnings", len(warnings))
	}
	staticDir := filepath.Join(rootDir, "web", "static")
	if err := site.WriteStaticWithOptions(cfg.OutDir, warnings, site.StaticOptions{
		StaticDir:     staticDir,
		SkipFontFiles: cfg.SubsetFonts,
	}); err != nil {
		return err
	}
	if cfg.SubsetFonts {
		cfg.logf("Subsetting fonts by generated HTML content...\n")
		if err := site.SubsetFontsByHTML(cfg.OutDir, staticDir); err != nil {
			return err
		}
	}
	cfg.logf("Site built: %s warning(s)=%d\n", filepath.ToSlash(cfg.OutDir), len(warnings))
	return nil
}

func prepareSiteAssets(rootDir, outDir string, cfg *blog.Config) error {
	if cfg == nil {
		return nil
	}
	var err error
	if cfg.Logo, err = copyConfiguredSiteAsset(rootDir, outDir, cfg.Logo); err != nil {
		return err
	}
	if cfg.Icon, err = copyConfiguredSiteAsset(rootDir, outDir, cfg.Icon); err != nil {
		return err
	}
	return nil
}

func copyConfiguredSiteAsset(rootDir, outDir, rel string) (string, error) {
	rel = strings.Trim(strings.TrimSpace(filepath.ToSlash(rel)), "/")
	if rel == "" {
		return "", nil
	}
	src := filepath.Join(rootDir, "asset", filepath.FromSlash(rel))
	if _, err := os.Stat(src); err != nil {
		return "", fmt.Errorf("site asset not found %s: %w", rel, err)
	}
	outRel := filepath.ToSlash(filepath.Join("assets", "site", filepath.FromSlash(rel)))
	dst := filepath.Join(outDir, filepath.FromSlash(outRel))
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return "", err
	}
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return "", err
	}
	return outRel, nil
}

func buildSitePageMap(s *blog.Site) map[string]string {
	pages := map[string]string{
		"tags/index.html":       blog.RenderTagsIndex(s.Config, s.Articles),
		"categories/index.html": blog.RenderCategoriesIndex(s.Config, s.Articles),
	}
	for page := 1; page <= blog.HomePageCount(s.Config, s.Articles); page++ {
		base := ""
		if page > 1 {
			base = upPrefix(2)
		}
		pages[blog.HomePageURL(page)] = blog.RenderHomePage(s.Config, s.Articles, page, base)
	}
	for page := 1; page <= blog.ArticleListPageCount(s.Config, s.Articles); page++ {
		base := ".."
		if page > 1 {
			base = upPrefix(3)
		}
		pages[blog.ArticleListPageURL(page)] = blog.RenderArticlesPagePage(s.Config, s.Articles, base, "所有文章", page)
	}
	for _, tag := range blog.Tags(s.Articles) {
		tagArticles := blog.ArticlesWithTag(s.Articles, tag)
		for page := 1; page <= blog.ArticleListPageCount(s.Config, tagArticles); page++ {
			base := "../.."
			if page > 1 {
				base = upPrefix(4)
			}
			pages[blog.TagPageURL(tag, page)] = blog.RenderTagPagePage(s.Config, tag, s.Articles, page, base)
		}
	}
	for _, path := range blog.CategoryPaths(s.Articles) {
		categoryArticles := blog.ArticlesInCategory(s.Articles, path)
		for page := 1; page <= blog.ArticleListPageCount(s.Config, categoryArticles); page++ {
			base := upPrefix(len(path) + 1)
			if page > 1 {
				base = upPrefix(len(path) + 3)
			}
			pages[blog.CategoryPageURL(path, page)] = blog.RenderCategoryPagePage(s.Config, path, s.Articles, page, base)
		}
	}
	return pages
}

func writeSitePages(outDir string, s *blog.Site) error {
	pages := buildSitePageMap(s)
	for rel, htmlText := range pages {
		path := filepath.Join(outDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(htmlText), 0644); err != nil {
			return err
		}
	}
	return nil
}

func buildSiteAboutPage(cfg Config, s *blog.Site) ([]string, string, error) {
	started := time.Now()
	docCfg, docLog := cfg.withDocumentLog()
	docCfg.logf("About page started\n")
	sourceDir := filepath.Join(s.RootDir, "data", "about_page")
	outPath := filepath.Join(cfg.OutDir, "about", "index.html")
	if !cfg.Force {
		fresh, err := documentOutputsFresh(sourceDir, outPath)
		if err != nil {
			docCfg.logf("Freshness check failed: %v\n", err)
			return nil, docLog.String(), err
		}
		if fresh {
			docCfg.logf("Skipped: outputs are fresh (%s)\n", time.Since(started).Round(time.Millisecond))
			return nil, docLog.String(), nil
		}
	} else {
		docCfg.logf("Freshness check skipped by -force\n")
	}
	input := filepath.Join(sourceDir, "main.tex")
	docCfg.logf("Input: %s\n", filepath.ToSlash(input))
	docCfg.logf("Output: %s\n", filepath.ToSlash(outPath))
	opts := render.Options{
		AssetPrefix: "..",
		HeaderHTML:  blog.Header(s.Config, ".."),
		BodyClass:   "site-layout",
		IconHref:    s.Config.Icon,
	}
	htmlText, warnings, _, err := buildArticle(docCfg, input, cfg.OutDir, "about", "..", opts)
	if err != nil {
		logWarnings(docCfg, warnings)
		docCfg.logf("Failed: %v\n", err)
		return warnings, docLog.String(), err
	}
	logWarnings(docCfg, warnings)
	if cfg.MemStore != nil {
		cfg.MemStore.put("about/index.html", []byte(htmlText))
	} else {
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			docCfg.logf("Create output directory failed: %v\n", err)
			return warnings, docLog.String(), err
		}
		if err := os.WriteFile(outPath, []byte(htmlText), 0644); err != nil {
			docCfg.logf("Write output failed: %v\n", err)
			return warnings, docLog.String(), err
		}
	}
	docCfg.logf("Built: %s (%s, warning(s)=%d)\n", filepath.ToSlash(outPath), time.Since(started).Round(time.Millisecond), len(warnings))
	return warnings, docLog.String(), nil
}

func buildSiteArticles(cfg Config, s *blog.Site) ([]string, error) {
	if len(s.Articles) == 0 {
		cfg.logf("No active articles to build.\n")
		return nil, nil
	}
	workers := cfg.ArticleWorkers
	if workers <= 0 {
		workers = runtime.NumCPU()
		if workers > 4 {
			workers = 4
		}
	}
	if workers > len(s.Articles) {
		workers = len(s.Articles)
	}
	if workers < 1 {
		workers = 1
	}
	cfg.logf("Article build queue: %d article(s), %d worker(s)\n", len(s.Articles), workers)

	jobs := make(chan articleBuildJob)
	results := make(chan articleBuildResult, len(s.Articles))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for job := range jobs {
				results <- buildOneSiteArticle(cfg, s, job.index, job.article, workerID)
			}
		}(i + 1)
	}
	for i, article := range s.Articles {
		jobs <- articleBuildJob{index: i, article: article}
	}
	close(jobs)
	wg.Wait()
	close(results)

	var warnings []string
	completed := 0
	for res := range results {
		completed++
		warnings = append(warnings, res.warnings...)
		cfg.flushDocumentLogString(res.title, res.log)
		if res.err != nil {
			cfg.logf("[%d/%d] failed: %s (%s): %v\n", completed, len(s.Articles), res.title, res.duration.Round(time.Millisecond), res.err)
			return warnings, res.err
		}
		if res.skipped {
			cfg.logf("[%d/%d] skipped: %s (%s)\n", completed, len(s.Articles), res.title, res.duration.Round(time.Millisecond))
			continue
		}
		if len(res.warnings) > 0 {
			cfg.logf("[%d/%d] done: %s (%s, %d warning(s))\n", completed, len(s.Articles), res.title, res.duration.Round(time.Millisecond), len(res.warnings))
		} else {
			cfg.logf("[%d/%d] done: %s (%s)\n", completed, len(s.Articles), res.title, res.duration.Round(time.Millisecond))
		}
	}
	return warnings, nil
}

func buildOneSiteArticle(cfg Config, s *blog.Site, index int, article blog.Article, workerID int) (result articleBuildResult) {
	started := time.Now()
	result = articleBuildResult{index: index, title: article.Title}
	docCfg, docLog := cfg.withDocumentLog()
	defer func() {
		result.duration = time.Since(started)
		result.log = docLog.String()
	}()
	docCfg.logf("Article started: index=%d worker=%d title=%s\n", index+1, workerID, article.Title)
	input, err := blog.ResolveArticleInput(s.RootDir, article)
	if err != nil {
		docCfg.logf("Resolve input failed: %v\n", err)
		result.err = err
		return result
	}
	slug := blog.Slugify(article.Slug)
	sourceDir := filepath.Join(s.RootDir, filepath.FromSlash(article.Folder))
	outPath := filepath.Join(cfg.OutDir, "articles", filepath.FromSlash(slug), "index.html")
	docCfg.logf("Slug: %s\n", slug)
	docCfg.logf("Input: %s\n", filepath.ToSlash(input))
	docCfg.logf("Source directory: %s\n", filepath.ToSlash(sourceDir))
	docCfg.logf("Output: %s\n", filepath.ToSlash(outPath))
	if !cfg.Force {
		fresh, err := documentOutputsFresh(sourceDir, outPath)
		if err != nil {
			docCfg.logf("Freshness check failed: %v\n", err)
			result.err = err
			return result
		}
		if fresh {
			docCfg.logf("Skipped: outputs are fresh\n")
			result.skipped = true
			return result
		}
	} else {
		docCfg.logf("Freshness check skipped by -force\n")
	}
	var warnings []string
	if article.MainFig != "" && !cfg.NoAssets {
		docCfg.logf("Processing article main figure: %s\n", article.MainFig)
		phaseStarted := time.Now()
		assetStats := &assets.Stats{}
		converter := assets.Converter{
			SourceRoot:  sourceDir,
			OutDir:      cfg.OutDir,
			AssetSubdir: "articles/" + slug,
			LinkPrefix:  "../..",
			Log:         docCfg.Log,
			LogMu:       docCfg.LogMu,
			Stats:       assetStats,
		}
		if cfg.MemStore != nil {
			converter.MemoryStore = cfg.MemStore
		}
		if _, err := converter.ConvertFile(article.MainFig); err != nil {
			docCfg.logf("Main figure failed: %v\n", err)
			warnings = append(warnings, err.Error())
		}
		logAssetStats(docCfg, "Main figure asset summary", assetStats)
		docCfg.logf("Main figure timing: %s\n", roundForLog(time.Since(phaseStarted)))
	} else if article.MainFig == "" {
		docCfg.logf("Article main figure: none\n")
	} else {
		docCfg.logf("Article main figure skipped by -no-assets\n")
	}
	opts := render.Options{
		AssetPrefix: "../..",
		HeaderHTML:  blog.Header(s.Config, "../.."),
		BodyClass:   "site-layout",
		IconHref:    s.Config.Icon,
	}
	htmlText, articleWarnings, _, err := buildArticle(docCfg, input, cfg.OutDir, "articles/"+slug, "../..", opts)
	warnings = append(warnings, articleWarnings...)
	result.warnings = warnings
	if err != nil {
		logWarnings(docCfg, warnings)
		docCfg.logf("Failed: %v\n", err)
		result.err = err
		return result
	}
	logWarnings(docCfg, warnings)
	if cfg.MemStore != nil {
		cfg.MemStore.put("articles/"+slug+"/index.html", []byte(htmlText))
	} else {
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			docCfg.logf("Create output directory failed: %v\n", err)
			result.err = err
			return result
		}
		if err := os.WriteFile(outPath, []byte(htmlText), 0644); err != nil {
			docCfg.logf("Write output failed: %v\n", err)
			result.err = err
			return result
		}
	}
	docCfg.logf("Built: %s (%s, warning(s)=%d)\n", filepath.ToSlash(outPath), time.Since(started).Round(time.Millisecond), len(warnings))
	return result
}

func upPrefix(levels int) string {
	if levels <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("../", levels), "/")
}

func documentOutputsFresh(sourceDir, requiredOutput string) (bool, error) {
	sourceLatest, hasSource, err := latestFileModTime(sourceDir)
	if err != nil {
		return false, err
	}
	if !hasSource {
		return false, nil
	}
	outputTime, hasOutput, err := documentOutputModTime(requiredOutput)
	if err != nil {
		return false, err
	}
	if !hasOutput {
		return false, nil
	}
	return !sourceLatest.After(outputTime), nil
}

func latestFileModTime(root string) (time.Time, bool, error) {
	var latest time.Time
	hasFile := false
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !hasFile || info.ModTime().After(latest) {
			latest = info.ModTime()
			hasFile = true
		}
		return nil
	})
	if err != nil {
		return time.Time{}, false, err
	}
	return latest, hasFile, nil
}

func documentOutputModTime(requiredOutput string) (time.Time, bool, error) {
	info, err := os.Stat(requiredOutput)
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	if info.IsDir() || info.Size() == 0 {
		return time.Time{}, false, nil
	}
	return info.ModTime(), true, nil
}
