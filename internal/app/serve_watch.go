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
	components      customComponents
	lastMod         map[string]time.Time
	mu              sync.Mutex
	outDir          string
	store           *memStore
	sitePageKeys    map[string]struct{}
	articlePageKeys map[string]struct{}
	changes         chan string
	rebuildRequest  chan struct{}
	rebuildDebounce time.Duration
	liveReload      *liveReloadState
	generation      uint64
	dirtyReasons    map[string]struct{}
}

type watchSnapshot struct {
	cfg        Config
	siteData   *blog.Site
	components customComponents
	lastMod    map[string]time.Time
}

type watchChange struct {
	key     string
	path    string
	what    string
	latest  time.Time
	changed bool
}

func startWatcher(cfg Config, siteData *blog.Site, components customComponents, outDir string, stop <-chan struct{}, store *memStore, liveReload *liveReloadState) {
	state := &watchState{
		cfg:             cfg,
		siteData:        siteData,
		components:      components,
		lastMod:         make(map[string]time.Time),
		outDir:          outDir,
		store:           store,
		sitePageKeys:    sitePageKeys(siteData),
		articlePageKeys: articlePageKeys(siteData),
		changes:         make(chan string, 16),
		rebuildRequest:  make(chan struct{}, 2),
		rebuildDebounce: 300 * time.Millisecond,
		liveReload:      liveReload,
		dirtyReasons:    make(map[string]struct{}),
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
	snap := s.snapshot()

	var changes []watchChange
	siteDataChanged := false
	if strings.TrimSpace(snap.cfg.ArticlesFile) != "" {
		articlesPath := resolveWatchPath(snap.cfg.RootDir, snap.cfg.ArticlesFile)
		change := s.detectChange(snap, "articles_config", articlesPath, "articles config changed")
		siteDataChanged = siteDataChanged || change.changed
		changes = append(changes, change)
	}

	if strings.TrimSpace(snap.cfg.SiteConfig) != "" {
		configPath := resolveWatchPath(snap.cfg.RootDir, snap.cfg.SiteConfig)
		change := s.detectChange(snap, "site_config", configPath, "site config changed")
		siteDataChanged = siteDataChanged || change.changed
		changes = append(changes, change)
	}

	componentsDir := filepath.Join(snap.cfg.RootDir, "data", "custom_components")
	componentsChange := s.detectChange(snap, "custom_components_dir", componentsDir, "custom components changed")
	changes = append(changes, componentsChange)

	aboutDir := filepath.Join(snap.cfg.RootDir, "data", "about_page")
	changes = append(changes, s.detectChange(snap, "about_page_dir", aboutDir, "about page"))

	if snap.siteData != nil {
		for _, article := range snap.siteData.Articles {
			sourceDir := filepath.Join(snap.cfg.RootDir, filepath.FromSlash(article.Folder))
			if sourceDir == "" {
				continue
			}
			slug := blog.Slugify(article.Slug)
			key := "article_" + slug
			changes = append(changes, s.detectChange(snap, key, sourceDir, "article:"+slug))
		}
	}

	var newData *blog.Site
	var newComponents customComponents
	reloadPrepared := siteDataChanged || componentsChange.changed
	if reloadPrepared {
		data, components, err := loadPreparedSiteForWatch(snap.cfg.RootDir, s.outDir, snap.cfg.SiteConfig, snap.cfg.ArticlesFile, s.store)
		if err != nil {
			snap.cfg.logf("Watch: reload site data failed: %v\n", err)
			return
		}
		newData = data
		newComponents = components
	}
	reasons := s.applyChanges(changes, newData, newComponents, reloadPrepared)
	for _, reason := range reasons {
		s.requestRebuild(reason)
	}
}

func (s *watchState) snapshot() watchSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	lastMod := make(map[string]time.Time, len(s.lastMod))
	for k, v := range s.lastMod {
		lastMod[k] = v
	}
	return watchSnapshot{cfg: s.cfg, siteData: s.siteData, components: s.components, lastMod: lastMod}
}

func resolveWatchPath(rootDir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(rootDir, filepath.FromSlash(path))
}

func (s *watchState) detectChange(snap watchSnapshot, key, path, what string) watchChange {
	latest, hasFiles, err := latestFileModTime(path)
	if err != nil || !hasFiles {
		return watchChange{key: key, path: path, what: what}
	}
	prev, exists := snap.lastMod[key]
	return watchChange{
		key:     key,
		path:    path,
		what:    what,
		latest:  latest,
		changed: exists && latest.After(prev),
	}
}

func (s *watchState) applyChanges(changes []watchChange, newData *blog.Site, newComponents customComponents, componentsChanged bool) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if newData != nil {
		s.siteData = newData
	}
	if componentsChanged {
		s.components = newComponents
	}
	var reasons []string
	for _, change := range changes {
		if change.latest.IsZero() {
			continue
		}
		prev, exists := s.lastMod[change.key]
		s.lastMod[change.key] = change.latest
		if !change.changed || !exists || !change.latest.After(prev) {
			continue
		}
		if _, ok := s.dirtyReasons[change.what]; !ok {
			reasons = append(reasons, change.what)
		}
		s.dirtyReasons[change.what] = struct{}{}
	}
	if len(reasons) > 0 {
		s.generation++
	}
	return reasons
}

func (s *watchState) beginRebuild(pending []string) (*blog.Site, customComponents, []string, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]struct{}, len(pending)+len(s.dirtyReasons))
	merged := make([]string, 0, len(pending)+len(s.dirtyReasons))
	for _, reason := range pending {
		if _, ok := seen[reason]; ok {
			continue
		}
		seen[reason] = struct{}{}
		merged = append(merged, reason)
	}
	for reason := range s.dirtyReasons {
		if _, ok := seen[reason]; ok {
			continue
		}
		seen[reason] = struct{}{}
		merged = append(merged, reason)
	}
	return s.siteData, s.components, merged, s.generation
}

func (s *watchState) finishRebuild(startGeneration uint64, processed []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation == startGeneration {
		for _, reason := range processed {
			delete(s.dirtyReasons, reason)
		}
		return
	}
	for reason := range s.dirtyReasons {
		s.requestRebuild(reason)
	}
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
					select {
					case s.rebuildRequest <- struct{}{}:
					default:
					}
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
	siteData, components, pending, startGeneration := s.beginRebuild(pending)
	defer s.finishRebuild(startGeneration, pending)

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
		} else if what == "custom components changed" {
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
			s.markReloadPagesFromMap(pages)
		}
	}

	if s.store != nil && (rebuildStaleArticles || rebuildAllArticles) {
		nextArticleKeys := articlePageKeys(siteData)
		removed := diffKeys(s.articlePageKeys, nextArticleKeys)
		s.store.deleteFiles(removed)
		s.articlePageKeys = nextArticleKeys
	}

	if rebuildAbout {
		s.rebuildAboutPage(siteData, components)
	}

	if rebuildAllArticles {
		s.rebuildSiteArticles(siteData, components, true)
		return
	}
	if rebuildStaleArticles {
		s.rebuildSiteArticles(siteData, components, false)
		return
	}
	for _, article := range rebuildArticles {
		s.rebuildOneArticle(article, siteData, components)
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

func (s *watchState) rebuildSiteArticles(siteData *blog.Site, components customComponents, force bool) {
	cfg := s.cfg
	cfg.OutDir = s.outDir
	cfg.Force = force
	cfg.MemStore = s.store
	if force {
		cfg.logf("Watch: rebuilding all article pages\n")
	} else {
		cfg.logf("Watch: checking article pages after metadata change\n")
	}
	if _, err := buildSiteArticles(cfg, siteData, components); err != nil {
		cfg.logf("Watch: rebuild article pages failed: %v\n", err)
		return
	}
	if force {
		for _, article := range siteData.Articles {
			slug := blog.Slugify(article.Slug)
			if slug != "" {
				s.markReloadPages("articles/" + slug + "/index.html")
			}
		}
	}
}

func (s *watchState) rebuildAboutPage(siteData *blog.Site, components customComponents) {
	aboutDir := filepath.Join(s.cfg.RootDir, "data", "about_page")
	outPath := filepath.Join(s.outDir, "about", "index.html")

	docCfg, docLog := s.cfg.withDocumentLog()
	opts := render.Options{
		AssetPrefix: "..",
		HeaderHTML:  blog.Header(siteData.Config, ".."),
		FooterHTML:  components.PageFooterHTML,
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
	s.markReloadPages("about/index.html")
	s.cfg.flushDocumentLogString("about page", docLog.String())
}

func (s *watchState) rebuildOneArticle(article blog.Article, siteData *blog.Site, components customComponents) {
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
		AssetPrefix:     "../..",
		HeaderHTML:      blog.Header(siteData.Config, "../.."),
		FooterHTML:      components.PageFooterHTML,
		ArticleStatHTML: components.ArticleStatHTML,
		BodyClass:       "site-layout",
		IconHref:        siteData.Config.Icon,
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
	s.markReloadPages("articles/" + slug + "/index.html")
	s.cfg.flushDocumentLogString(article.Title, docLog.String())
}

func (s *watchState) markReloadPages(paths ...string) {
	if s.liveReload != nil {
		s.liveReload.MarkUpdated(paths...)
	}
}

func (s *watchState) markReloadPagesFromMap(pages map[string]string) {
	if s.liveReload == nil || len(pages) == 0 {
		return
	}
	paths := make([]string, 0, len(pages))
	for path := range pages {
		paths = append(paths, path)
	}
	s.liveReload.MarkUpdated(paths...)
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
