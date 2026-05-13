package app

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"MetaBlog/internal/blog"
	"MetaBlog/internal/render"
)

type watchState struct {
	cfg             Config
	siteData        *blog.Site
	lastMod         map[string]time.Time
	mu              sync.Mutex
	outDir          string
	store           *memStore
	sitePageKeys    map[string]struct{}
	articlePageKeys map[string]struct{}
	changes         chan string
	rebuildRequest  chan struct{}
	rebuildDebounce time.Duration
}

func startWatcher(cfg Config, siteData *blog.Site, outDir string, stop <-chan struct{}, store *memStore) {
	state := &watchState{
		cfg:             cfg,
		siteData:        siteData,
		lastMod:         make(map[string]time.Time),
		outDir:          outDir,
		store:           store,
		sitePageKeys:    sitePageKeys(siteData),
		articlePageKeys: articlePageKeys(siteData),
		changes:         make(chan string, 16),
		rebuildRequest:  make(chan struct{}, 1),
		rebuildDebounce: 300 * time.Millisecond,
	}

	cfg.logf("Watch: monitoring %d article(s) and about page for changes\n", len(siteData.Articles))

	state.scan()
	go state.poll(stop)
	go state.debouncedRebuild(stop)
}

func (s *watchState) poll(stop <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			s.scan()
		}
	}
}

func (s *watchState) scan() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(s.cfg.ArticlesFile) != "" {
		articlesPath := filepath.Join(s.cfg.RootDir, s.cfg.ArticlesFile)
		if s.changed("articles_config", articlesPath) {
			newData, err := loadPreparedSiteForWatch(s.cfg.RootDir, s.outDir, s.cfg.SiteConfig, s.cfg.ArticlesFile, s.store)
			if err != nil {
				s.cfg.logf("Watch: reload site data failed: %v\n", err)
				return
			}
			s.siteData = newData
			s.requestRebuild("articles config changed")
		}
	}

	if strings.TrimSpace(s.cfg.SiteConfig) != "" {
		configPath := filepath.Join(s.cfg.RootDir, s.cfg.SiteConfig)
		if s.changed("site_config", configPath) {
			newData, err := loadPreparedSiteForWatch(s.cfg.RootDir, s.outDir, s.cfg.SiteConfig, s.cfg.ArticlesFile, s.store)
			if err != nil {
				s.cfg.logf("Watch: reload site data failed: %v\n", err)
				return
			}
			s.siteData = newData
			s.requestRebuild("site config changed")
		}
	}

	aboutDir := filepath.Join(s.cfg.RootDir, "data", "about_page")
	if s.changed("about_page_dir", aboutDir) {
		s.requestRebuild("about page")
	}

	for _, article := range s.siteData.Articles {
		sourceDir := filepath.Join(s.cfg.RootDir, filepath.FromSlash(article.Folder))
		if sourceDir == "" {
			continue
		}
		slug := blog.Slugify(article.Slug)
		key := "article_" + slug
		if s.changed(key, sourceDir) {
			s.requestRebuild("article:" + slug)
		}
	}
}

func (s *watchState) changed(key, path string) bool {
	latest, hasFiles, err := latestFileModTime(path)
	if err != nil || !hasFiles {
		return false
	}
	prev, exists := s.lastMod[key]
	s.lastMod[key] = latest
	if !exists {
		return false
	}
	return latest.After(prev)
}

func (s *watchState) requestRebuild(what string) {
	select {
	case s.changes <- what:
	default:
	}
}

func (s *watchState) debouncedRebuild(stop <-chan struct{}) {
	var timer *time.Timer
	var pending []string

	for {
		select {
		case <-stop:
			if timer != nil {
				timer.Stop()
			}
			return
		case what := <-s.changes:
			pending = append(pending, what)
			if timer == nil {
				timer = time.AfterFunc(s.rebuildDebounce, func() {
					s.rebuildRequest <- struct{}{}
				})
			} else {
				timer.Reset(s.rebuildDebounce)
			}
		case <-s.rebuildRequest:
			if timer != nil {
				timer.Stop()
				timer = nil
			}
			if len(pending) == 0 {
				continue
			}
			s.executeRebuild(pending)
			pending = pending[:0]
		}
	}
}

func (s *watchState) executeRebuild(pending []string) {
	s.mu.Lock()
	siteData := s.siteData
	s.mu.Unlock()

	rebuildIndex := false
	rebuildAbout := false
	rebuildStaleArticles := false
	rebuildAllArticles := false
	rebuildArticles := make(map[string]blog.Article)

	for _, what := range pending {
		if strings.HasPrefix(what, "article:") {
			if rebuildAllArticles || rebuildStaleArticles {
				continue
			}
			slug := strings.TrimPrefix(what, "article:")
			for _, a := range siteData.Articles {
				if blog.Slugify(a.Slug) == slug {
					rebuildArticles[blog.Slugify(a.Slug)] = a
					break
				}
			}
		} else if what == "about page" {
			rebuildAbout = true
		} else if what == "articles config changed" {
			rebuildIndex = true
			rebuildStaleArticles = true
		} else if what == "site config changed" {
			rebuildIndex = true
			rebuildAbout = true
			rebuildAllArticles = true
		}
	}

	if rebuildIndex {
		pages := buildSitePageMap(siteData)
		indexOK := true
		if s.store != nil {
			s.sitePageKeys = s.store.replaceHTMLPages(s.sitePageKeys, pages)
		} else {
			if err := writePageMapToDisk(s.outDir, pages); err != nil {
				s.cfg.logf("Watch: write index pages failed: %v\n", err)
				indexOK = false
			}
		}
		if indexOK {
			s.cfg.logf("Watch: index pages regenerated\n")
		}
	}

	if s.store != nil && (rebuildStaleArticles || rebuildAllArticles) {
		nextArticleKeys := articlePageKeys(siteData)
		removed := diffKeys(s.articlePageKeys, nextArticleKeys)
		s.store.deleteFiles(removed)
		s.articlePageKeys = nextArticleKeys
	}

	if rebuildAbout {
		s.rebuildAboutPage(siteData)
	}

	if rebuildAllArticles {
		s.rebuildSiteArticles(siteData, true)
		return
	}
	if rebuildStaleArticles {
		s.rebuildSiteArticles(siteData, false)
		return
	}
	for _, article := range rebuildArticles {
		s.rebuildOneArticle(article, siteData)
	}
}

func writePageMapToDisk(outDir string, pages map[string]string) error {
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

func (s *watchState) rebuildSiteArticles(siteData *blog.Site, force bool) {
	cfg := s.cfg
	cfg.OutDir = s.outDir
	cfg.Force = force
	cfg.MemStore = s.store
	if force {
		cfg.logf("Watch: rebuilding all article pages\n")
	} else {
		cfg.logf("Watch: checking article pages after metadata change\n")
	}
	if _, err := buildSiteArticles(cfg, siteData); err != nil {
		cfg.logf("Watch: rebuild article pages failed: %v\n", err)
	}
}

func (s *watchState) rebuildAboutPage(siteData *blog.Site) {
	aboutDir := filepath.Join(s.cfg.RootDir, "data", "about_page")
	outPath := filepath.Join(s.outDir, "about", "index.html")

	docCfg, docLog := s.cfg.withDocumentLog()
	opts := render.Options{
		AssetPrefix: "..",
		HeaderHTML:  blog.Header(siteData.Config, ".."),
		BodyClass:   "site-layout",
		IconHref:    siteData.Config.Icon,
	}
	htmlText, warnings, _, err := buildArticle(docCfg, filepath.Join(aboutDir, "main.tex"), s.outDir, "about", "..", opts)
	if err != nil {
		s.cfg.logf("Watch: about page rebuild failed: %v\n", err)
		return
	}
	if s.store != nil {
		s.store.put("about/index.html", []byte(htmlText))
	} else {
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			s.cfg.logf("Watch: create about output directory failed: %v\n", err)
			return
		}
		if err := os.WriteFile(outPath, []byte(htmlText), 0644); err != nil {
			s.cfg.logf("Watch: write about page failed: %v\n", err)
			return
		}
	}
	s.cfg.logf("Watch: about page rebuilt (%d warning(s))\n", len(warnings))
	s.cfg.flushDocumentLogString("about page", docLog.String())
}

func (s *watchState) rebuildOneArticle(article blog.Article, siteData *blog.Site) {
	slug := blog.Slugify(article.Slug)
	outPath := filepath.Join(s.outDir, "articles", filepath.FromSlash(slug), "index.html")
	sourceDir := filepath.Join(s.cfg.RootDir, filepath.FromSlash(article.Folder))

	docCfg, docLog := s.cfg.withDocumentLog()

	input, err := blog.ResolveArticleInput(s.cfg.RootDir, article)
	if err != nil {
		s.cfg.logf("Watch: resolve input for %s failed: %v\n", article.Title, err)
		return
	}

	opts := render.Options{
		AssetPrefix: "../..",
		HeaderHTML:  blog.Header(siteData.Config, "../.."),
		BodyClass:   "site-layout",
		IconHref:    siteData.Config.Icon,
	}

	htmlText, warnings, _, err := buildArticle(docCfg, input, s.outDir, "articles/"+slug, "../..", opts)
	if err != nil {
		s.cfg.logf("Watch: rebuild %s failed: %v\n", article.Title, err)
		return
	}
	if s.store != nil {
		rel := "articles/" + slug + "/index.html"
		s.store.put(rel, []byte(htmlText))
	} else {
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			s.cfg.logf("Watch: create article output directory for %s failed: %v\n", article.Title, err)
			return
		}
		if err := os.WriteFile(outPath, []byte(htmlText), 0644); err != nil {
			s.cfg.logf("Watch: write article %s failed: %v\n", article.Title, err)
			return
		}
	}

	s.cfg.logf("Watch: rebuilt %s (%d warning(s), source=%s)\n", article.Title, len(warnings), filepath.ToSlash(sourceDir))
	s.cfg.flushDocumentLogString(article.Title, docLog.String())
}

func sitePageKeys(siteData *blog.Site) map[string]struct{} {
	keys := make(map[string]struct{})
	for path := range buildSitePageMap(siteData) {
		keys[cleanMemPath(path)] = struct{}{}
	}
	return keys
}

func articlePageKeys(siteData *blog.Site) map[string]struct{} {
	keys := make(map[string]struct{}, len(siteData.Articles))
	for _, article := range siteData.Articles {
		slug := blog.Slugify(article.Slug)
		if slug == "" {
			continue
		}
		keys["articles/"+slug+"/index.html"] = struct{}{}
	}
	return keys
}

func diffKeys(oldKeys, newKeys map[string]struct{}) map[string]struct{} {
	diff := make(map[string]struct{})
	for key := range oldKeys {
		if _, ok := newKeys[key]; !ok {
			diff[key] = struct{}{}
		}
	}
	return diff
}
