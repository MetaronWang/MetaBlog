package blog

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"MetaBlog/internal/pathutil"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Title               string `json:"title" yaml:"title" toml:"title"`
	Logo                string `json:"logo" yaml:"logo" toml:"logo"`
	Icon                string `json:"icon" yaml:"icon" toml:"icon"`
	HomePageSize        int    `json:"home_page_size" yaml:"home_page_size" toml:"home_page_size"`
	ArticleListPageSize int    `json:"article_list_page_size" yaml:"article_list_page_size" toml:"article_list_page_size"`
}

type Article struct {
	Title       string   `json:"title" yaml:"title" toml:"title"`
	Description string   `json:"description" yaml:"description" toml:"description"`
	Author      string   `json:"author" yaml:"author" toml:"author"`
	Date        string   `json:"date" yaml:"date" toml:"date"`
	Category    []string `json:"category" yaml:"category" toml:"category"`
	Tags        []string `json:"tags" yaml:"tags" toml:"tags"`
	Folder      string   `json:"folder" yaml:"folder" toml:"folder"`
	MainFig     string   `json:"main_fig" yaml:"main_fig" toml:"main_fig"`
	MainFile    string   `json:"main_file" yaml:"main_file" toml:"main_file"`
	Input       string   `json:"input,omitempty" yaml:"input,omitempty" toml:"input,omitempty"`
	Slug        string   `json:"slug,omitempty" yaml:"slug,omitempty" toml:"slug,omitempty"`
	Deleted     bool     `json:"deleted,omitempty" yaml:"deleted,omitempty" toml:"deleted,omitempty"`
}

type Site struct {
	Config   Config
	Articles []Article
	RootDir  string
}

type articleDataFile struct {
	Articles []Article `json:"articles" yaml:"articles" toml:"articles"`
}

const (
	DefaultHomePageSize        = 10
	DefaultArticleListPageSize = 20
)

var dateOnlyRE = regexp.MustCompile(`^\d{4}$`)

func Load(rootDir, configPath, articlesPath string) (*Site, error) {
	cfg := Config{Title: "MetaBlog"}
	if configPath != "" {
		if err := readData(resolveUserPath(rootDir, configPath), &cfg); err != nil {
			return nil, err
		}
	}
	cfg.normalize()
	var articles []Article
	if articlesPath != "" {
		if err := readData(resolveUserPath(rootDir, articlesPath), &articles); err != nil {
			return nil, err
		}
	}
	for i := range articles {
		articles[i].normalize()
		if err := validateArticlePaths(rootDir, articles[i]); err != nil {
			return nil, err
		}
	}
	articles = ActiveArticles(articles)
	sortArticles(articles)
	return &Site{Config: cfg, Articles: articles, RootDir: rootDir}, nil
}

func (c *Config) normalize() {
	c.Title = strings.TrimSpace(c.Title)
	c.Logo = strings.TrimSpace(filepath.ToSlash(c.Logo))
	c.Icon = strings.TrimSpace(filepath.ToSlash(c.Icon))
	if c.Title == "" {
		c.Title = "MetaBlog"
	}
	if c.HomePageSize <= 0 {
		c.HomePageSize = DefaultHomePageSize
	}
	if c.ArticleListPageSize <= 0 {
		c.ArticleListPageSize = DefaultArticleListPageSize
	}
}

func LoadArticles(path string) ([]Article, error) {
	var articles []Article
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return articles, nil
	}
	if err := readData(path, &articles); err != nil {
		return nil, err
	}
	for i := range articles {
		articles[i].normalize()
	}
	return articles, nil
}

func ActiveArticles(articles []Article) []Article {
	out := make([]Article, 0, len(articles))
	for _, article := range articles {
		if !article.Deleted {
			out = append(out, article)
		}
	}
	return out
}

func SaveArticles(path string, articles []Article) error {
	for i := range articles {
		articles[i].normalize()
	}
	b, err := marshalData(path, articles)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

func readData(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := strings.TrimPrefix(string(b), "\ufeff")
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		if err := json.Unmarshal([]byte(text), v); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
	case ".toml":
		if articles, ok := v.(*[]Article); ok {
			var data articleDataFile
			if err := toml.Unmarshal([]byte(text), &data); err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}
			*articles = data.Articles
			return nil
		}
		if err := toml.Unmarshal([]byte(text), v); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
	default:
		if err := yaml.Unmarshal([]byte(text), v); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
	}
	return nil
}

func marshalData(path string, v any) ([]byte, error) {
	var (
		b   []byte
		err error
	)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		b, err = json.MarshalIndent(v, "", "  ")
	case ".toml":
		if articles, ok := v.([]Article); ok {
			b, err = toml.Marshal(articleDataFile{Articles: articles})
		} else {
			b, err = toml.Marshal(v)
		}
	default:
		b, err = yaml.Marshal(v)
	}
	if err != nil {
		return nil, err
	}
	if len(b) == 0 || b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	return b, nil
}

func resolveUserPath(rootDir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(rootDir, filepath.FromSlash(path)))
}

func resolveArticlePath(rootDir, path string) (string, error) {
	clean, err := pathutil.CleanRelativePath(path)
	if err != nil {
		return "", err
	}
	result := filepath.Clean(filepath.Join(rootDir, clean))
	if !pathutil.IsWithinDir(rootDir, result) {
		return "", fmt.Errorf("path escapes root directory: %s", path)
	}
	return result, nil
}

func validateArticlePaths(rootDir string, a Article) error {
	folder, err := resolveArticlePath(rootDir, a.Folder)
	if err != nil {
		return fmt.Errorf("article %s folder: %w", a.Title, err)
	}
	if a.MainFile != "" {
		if _, err := resolveArticlePath(folder, a.MainFile); err != nil {
			return fmt.Errorf("article %s main_file: %w", a.Title, err)
		}
	}
	if a.MainFig != "" {
		if _, err := pathutil.CleanRelativePath(a.MainFig); err != nil {
			return fmt.Errorf("article %s main_fig: %w", a.Title, err)
		}
	}
	return nil
}

func (a *Article) normalize() {
	a.Title = strings.TrimSpace(a.Title)
	a.Description = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(a.Description, "\r\n", "\n"), "\r", "\n"))
	a.Author = strings.TrimSpace(a.Author)
	a.Date = strings.TrimSpace(a.Date)
	a.Folder = strings.TrimSpace(filepath.ToSlash(a.Folder))
	a.MainFig = strings.TrimSpace(filepath.ToSlash(a.MainFig))
	a.MainFile = strings.TrimSpace(filepath.ToSlash(a.MainFile))
	a.Input = strings.TrimSpace(filepath.ToSlash(a.Input))
	if a.MainFile == "" && a.Input != "" {
		a.MainFile = a.Input
	}
	a.Input = ""
	a.Slug = strings.Trim(strings.TrimSpace(a.Slug), "/")
	if a.Slug == "" {
		base := filepath.Base(a.Folder)
		if base == "." || base == "/" || base == "" {
			base = a.Title
		}
		a.Slug = Slugify(base)
	}
	if a.Title == "" {
		a.Title = a.Slug
	}
	a.Tags = cleanList(a.Tags)
	a.Category = cleanList(a.Category)
}

func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func sortArticles(articles []Article) {
	sort.SliceStable(articles, func(i, j int) bool {
		di, iok := parseDate(articles[i].Date)
		dj, jok := parseDate(articles[j].Date)
		if iok && jok && !di.Equal(dj) {
			return di.Before(dj)
		}
		if articles[i].Date != articles[j].Date {
			return articles[i].Date < articles[j].Date
		}
		return articles[i].Title < articles[j].Title
	})
}

func ResolveArticleInput(rootDir string, a Article) (string, error) {
	folder, err := resolveArticlePath(rootDir, a.Folder)
	if err != nil {
		return "", err
	}
	if a.MainFile != "" {
		return resolveArticlePath(folder, a.MainFile)
	}
	candidates := []string{"main.tex", "index.tex", "article.tex"}
	for _, name := range candidates {
		path := filepath.Join(folder, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	matches, err := filepath.Glob(filepath.Join(folder, "*.tex"))
	if err != nil {
		return "", err
	}
	if len(matches) > 0 {
		sort.Strings(matches)
		return matches[0], nil
	}
	return "", fmt.Errorf("no LaTeX entry found in %s", folder)
}

func ArticleURL(slug string) string {
	return "articles/" + Slugify(slug) + "/index.html"
}

func MainFigURL(a Article, basePrefix string) string {
	if a.MainFig == "" {
		return ""
	}
	rel := strings.Trim(strings.TrimPrefix(filepath.ToSlash(a.MainFig), "./"), "/")
	if strings.EqualFold(filepath.Ext(rel), ".pdf") {
		rel = strings.TrimSuffix(rel, filepath.Ext(rel)) + ".svg"
	}
	return joinURL(basePrefix, "assets/articles/"+Slugify(a.Slug)+"/"+rel)
}

func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r > 127 {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "article"
	}
	return out
}

func RenderShell(cfg Config, basePrefix, pageTitle, body string) string {
	cfg.normalize()
	title := cfg.Title
	if pageTitle != "" {
		title = pageTitle + " - " + cfg.Title
	}
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"zh-CN\">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<title>")
	b.WriteString(html.EscapeString(title))
	b.WriteString("</title>\n<link rel=\"stylesheet\" href=\"")
	b.WriteString(html.EscapeString(joinURL(basePrefix, "static/fonts.css")))
	b.WriteString("\">\n<link rel=\"stylesheet\" href=\"")
	b.WriteString(html.EscapeString(joinURL(basePrefix, "static/style.css")))
	b.WriteString("\">\n")
	if cfg.Icon != "" {
		b.WriteString(`<link rel="icon" href="`)
		b.WriteString(html.EscapeString(joinURL(basePrefix, cfg.Icon)))
		b.WriteString(`">`)
		b.WriteByte('\n')
	}
	b.WriteString("</head>\n<body class=\"site-layout\">\n")
	b.WriteString(Header(cfg, basePrefix))
	b.WriteString(`<main class="site-page">`)
	b.WriteString(body)
	b.WriteString("</main>\n</body>\n</html>\n")
	return b.String()
}

func Header(cfg Config, basePrefix string) string {
	siteTitle := cfg.Title
	if strings.TrimSpace(siteTitle) == "" {
		siteTitle = "MetaBlog"
	}
	home := joinURL(basePrefix, "index.html")
	var b strings.Builder
	b.WriteString(`<header class="site-topbar"><div class="site-topbar-inner">`)
	b.WriteString(`<a class="site-brand" href="`)
	b.WriteString(html.EscapeString(home))
	b.WriteString(`">`)
	if cfg.Logo != "" {
		b.WriteString(`<img class="site-logo" src="`)
		b.WriteString(html.EscapeString(joinURL(basePrefix, cfg.Logo)))
		b.WriteString(`" alt="">`)
	}
	b.WriteString(`<span class="site-title">`)
	b.WriteString(html.EscapeString(siteTitle))
	b.WriteString(`</span></a>`)
	b.WriteString(`<nav class="site-nav" aria-label="Site">`)
	for _, item := range []struct {
		Text string
		Href string
	}{
		{"所有文章", "articles/index.html"},
		{"标签", "tags/index.html"},
		{"分类", "categories/index.html"},
		{"关于", "about/index.html"},
	} {
		b.WriteString(`<a href="`)
		b.WriteString(html.EscapeString(joinURL(basePrefix, item.Href)))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(item.Text))
		b.WriteString(`</a>`)
	}
	b.WriteString(`</nav></div></header>`)
	return b.String()
}

func RenderHome(cfg Config, articles []Article) string {
	return RenderHomePage(cfg, articles, 1, "")
}

func RenderHomePage(cfg Config, articles []Article, page int, basePrefix string) string {
	cfg.normalize()
	var b strings.Builder
	if len(articles) == 0 {
		b.WriteString(`<p class="home-empty">暂无文章。</p>`)
		return RenderShell(cfg, basePrefix, "", b.String())
	}
	paged, totalPages := paginateArticles(articlesByNewest(articles), page, cfg.HomePageSize)
	b.WriteString(`<section class="home-article-feed" aria-label="Articles">`)
	for _, a := range paged {
		articleHref := joinURL(basePrefix, ArticleURL(a.Slug))
		b.WriteString(`<article class="home-article-card">`)
		if fig := MainFigURL(a, basePrefix); fig != "" {
			b.WriteString(`<a class="home-article-figure" href="`)
			b.WriteString(html.EscapeString(articleHref))
			b.WriteString(`"><img src="`)
			b.WriteString(html.EscapeString(fig))
			b.WriteString(`" alt=""></a>`)
		}
		b.WriteString(`<div class="home-article-body"><h2 class="home-article-title"><a href="`)
		b.WriteString(html.EscapeString(articleHref))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(a.Title))
		b.WriteString(`</a></h2><div class="home-article-meta"><time>`)
		b.WriteString(html.EscapeString(a.Date))
		b.WriteString(`</time>`)
		if len(a.Category) > 0 {
			b.WriteString(`<span class="home-article-categories">`)
			for i := range a.Category {
				if i > 0 {
					b.WriteString(`<span class="category-separator">/</span>`)
				}
				b.WriteString(`<a href="`)
				b.WriteString(html.EscapeString(joinURL(basePrefix, CategoryPageURL(a.Category[:i+1], 1))))
				b.WriteString(`">`)
				b.WriteString(html.EscapeString(a.Category[i]))
				b.WriteString(`</a>`)
			}
			b.WriteString(`</span>`)
		}
		b.WriteString(`</div>`)
		if desc := truncateDescription(a.Description, 1000); desc != "" {
			b.WriteString(`<p class="home-article-description">`)
			b.WriteString(html.EscapeString(desc))
			b.WriteString(`</p>`)
		}
		b.WriteString(`<div class="home-article-tags"><span class="tag-icon" aria-hidden="true">`)
		b.WriteString(tagIcon())
		b.WriteString(`</span><div class="tag-links">`)
		for _, tag := range a.Tags {
			b.WriteString(`<a href="`)
			b.WriteString(html.EscapeString(joinURL(basePrefix, TagPageURL(tag, 1))))
			b.WriteString(`">`)
			b.WriteString(html.EscapeString(tag))
			b.WriteString(`</a>`)
		}
		b.WriteString(`</div></div></div></article>`)
	}
	b.WriteString(`</section>`)
	renderPagination(&b, basePrefix, page, totalPages, HomePageURL)
	return RenderShell(cfg, basePrefix, "", b.String())
}

func RenderArticlesPage(cfg Config, articles []Article, basePrefix, title string) string {
	return renderArticleCollectionPage(cfg, articles, basePrefix, title, 1, ArticleListPageURL)
}

func RenderArticlesPagePage(cfg Config, articles []Article, basePrefix, title string, page int) string {
	return renderArticleCollectionPage(cfg, articles, basePrefix, title, page, ArticleListPageURL)
}

func renderArticleCollectionPage(cfg Config, articles []Article, basePrefix, title string, page int, pageURL func(int) string) string {
	cfg.normalize()
	var b strings.Builder
	b.WriteString("<h1>")
	b.WriteString(html.EscapeString(title))
	b.WriteString("</h1>")
	paged, totalPages := paginateArticles(articlesByNewest(articles), page, cfg.ArticleListPageSize)
	renderArticleList(&b, paged, basePrefix)
	renderPagination(&b, basePrefix, page, totalPages, pageURL)
	return RenderShell(cfg, basePrefix, title, b.String())
}

func RenderTagsIndex(cfg Config, articles []Article) string {
	counts := map[string]int{}
	for _, a := range articles {
		for _, tag := range a.Tags {
			counts[tag]++
		}
	}
	terms := sortedKeys(counts)
	var b strings.Builder
	b.WriteString("<h1>标签</h1><ul class=\"term-cloud\">")
	for _, tag := range terms {
		b.WriteString(`<li><a href="`)
		b.WriteString(html.EscapeString(Slugify(tag)))
		b.WriteString(`/index.html">`)
		b.WriteString(html.EscapeString(tag))
		b.WriteString(`<sup>`)
		b.WriteString(fmt.Sprint(counts[tag]))
		b.WriteString(`</sup></a></li>`)
	}
	b.WriteString("</ul>")
	return RenderShell(cfg, "..", "标签", b.String())
}

func RenderTagPage(cfg Config, tag string, articles []Article) string {
	return RenderTagPagePage(cfg, tag, articles, 1, "../..")
}

func RenderTagPagePage(cfg Config, tag string, articles []Article, page int, basePrefix string) string {
	return renderArticleCollectionPage(cfg, articlesWithTag(articles, tag), basePrefix, "标签："+tag, page, func(page int) string {
		return TagPageURL(tag, page)
	})
}

func RenderCategoriesIndex(cfg Config, articles []Article) string {
	tree := buildCategoryTree(articles)
	var b strings.Builder
	b.WriteString("<h1>分类</h1>")
	b.WriteString(renderCategoryNodes(tree.Children, "..", ""))
	return RenderShell(cfg, "..", "分类", b.String())
}

func RenderCategoryPage(cfg Config, categoryPath []string, articles []Article, basePrefix string) string {
	return RenderCategoryPagePage(cfg, categoryPath, articles, 1, basePrefix)
}

func RenderCategoryPagePage(cfg Config, categoryPath []string, articles []Article, page int, basePrefix string) string {
	title := "分类：" + strings.Join(categoryPath, " / ")
	return renderArticleCollectionPage(cfg, articlesInCategory(articles, categoryPath), basePrefix, title, page, func(page int) string {
		return CategoryPageURL(categoryPath, page)
	})
}

func Tags(articles []Article) []string {
	counts := map[string]int{}
	for _, a := range articles {
		for _, tag := range a.Tags {
			counts[tag]++
		}
	}
	return sortedKeys(counts)
}

func CategoryPaths(articles []Article) [][]string {
	seen := map[string][]string{}
	for _, a := range articles {
		for i := range a.Category {
			path := append([]string{}, a.Category[:i+1]...)
			seen[strings.Join(path, "\x00")] = path
		}
	}
	keys := sortedKeysPath(seen)
	out := make([][]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}

func renderArticleList(b *strings.Builder, articles []Article, basePrefix string) {
	if len(articles) == 0 {
		b.WriteString("<p>暂无文章。</p>")
		return
	}
	years := groupByYear(articles)
	for _, year := range sortedYears(years) {
		b.WriteString(`<h2 class="article-year">`)
		b.WriteString(html.EscapeString(year))
		b.WriteString(`</h2><ul class="article-list">`)
		for _, a := range years[year] {
			b.WriteString("<li>")
			if fig := MainFigURL(a, basePrefix); fig != "" {
				b.WriteString(`<a class="article-thumb" href="`)
				b.WriteString(html.EscapeString(joinURL(basePrefix, ArticleURL(a.Slug))))
				b.WriteString(`"><img src="`)
				b.WriteString(html.EscapeString(fig))
				b.WriteString(`" alt=""></a>`)
			}
			b.WriteString(`<div class="article-list-main"><a href="`)
			b.WriteString(html.EscapeString(joinURL(basePrefix, ArticleURL(a.Slug))))
			b.WriteString("\">")
			b.WriteString(html.EscapeString(a.Title))
			b.WriteString("</a></div><span class=\"article-date\">")
			b.WriteString(html.EscapeString(monthDay(a.Date)))
			b.WriteString("</span></li>")
		}
		b.WriteString("</ul>")
	}
}

func renderPagination(b *strings.Builder, basePrefix string, page, totalPages int, pageURL func(int) string) {
	if totalPages <= 1 {
		return
	}
	b.WriteString(`<nav class="pagination" aria-label="Pagination">`)
	if page > 1 {
		b.WriteString(`<a class="pagination-prev" href="`)
		b.WriteString(html.EscapeString(joinURL(basePrefix, pageURL(page-1))))
		b.WriteString(`">上一页</a>`)
	}
	b.WriteString(`<ol>`)
	for i := 1; i <= totalPages; i++ {
		b.WriteString(`<li>`)
		if i == page {
			b.WriteString(`<span aria-current="page">`)
			b.WriteString(fmt.Sprint(i))
			b.WriteString(`</span>`)
		} else {
			b.WriteString(`<a href="`)
			b.WriteString(html.EscapeString(joinURL(basePrefix, pageURL(i))))
			b.WriteString(`">`)
			b.WriteString(fmt.Sprint(i))
			b.WriteString(`</a>`)
		}
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ol>`)
	if page < totalPages {
		b.WriteString(`<a class="pagination-next" href="`)
		b.WriteString(html.EscapeString(joinURL(basePrefix, pageURL(page+1))))
		b.WriteString(`">下一页</a>`)
	}
	b.WriteString(`</nav>`)
}

func groupByYear(articles []Article) map[string][]Article {
	out := map[string][]Article{}
	for _, a := range articles {
		year := "未知"
		if t, ok := parseDate(a.Date); ok {
			year = fmt.Sprintf("%04d", t.Year())
		} else if len(a.Date) >= 4 {
			year = a.Date[:4]
		}
		out[year] = append(out[year], a)
	}
	return out
}

func sortedYears(years map[string][]Article) []string {
	out := sortedKeysMap(years)
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}

type categoryNode struct {
	Name     string
	Path     []string
	Children map[string]*categoryNode
}

func buildCategoryTree(articles []Article) *categoryNode {
	root := &categoryNode{Children: map[string]*categoryNode{}}
	for _, a := range articles {
		node := root
		var path []string
		for _, part := range a.Category {
			path = append(path, part)
			child := node.Children[part]
			if child == nil {
				child = &categoryNode{Name: part, Path: append([]string{}, path...), Children: map[string]*categoryNode{}}
				node.Children[part] = child
			}
			node = child
		}
	}
	return root
}

func renderCategoryNodes(nodes map[string]*categoryNode, basePrefix, parentURL string) string {
	if len(nodes) == 0 {
		return "<p>暂无分类。</p>"
	}
	var b strings.Builder
	b.WriteString(`<ol class="category-tree">`)
	for _, name := range sortedKeysMap(nodes) {
		node := nodes[name]
		pathURL := categoryURL(node.Path)
		b.WriteString("<li>")
		if len(node.Children) > 0 {
			b.WriteString("<details open><summary>")
		}
		b.WriteString(`<a href="`)
		b.WriteString(html.EscapeString(joinURL(basePrefix, pathURL)))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(node.Name))
		b.WriteString(`</a>`)
		if len(node.Children) > 0 {
			b.WriteString("</summary>")
			b.WriteString(renderCategoryNodes(node.Children, basePrefix, parentURL))
			b.WriteString("</details>")
		}
		b.WriteString("</li>")
	}
	b.WriteString("</ol>")
	return b.String()
}

func categoryURL(path []string) string {
	return CategoryPageURL(path, 1)
}

func HomePageCount(cfg Config, articles []Article) int {
	cfg.normalize()
	return pageCount(len(articles), cfg.HomePageSize)
}

func ArticleListPageCount(cfg Config, articles []Article) int {
	cfg.normalize()
	return pageCount(len(articles), cfg.ArticleListPageSize)
}

func HomePageURL(page int) string {
	if page <= 1 {
		return "index.html"
	}
	return fmt.Sprintf("page/%d/index.html", page)
}

func ArticleListPageURL(page int) string {
	if page <= 1 {
		return "articles/index.html"
	}
	return fmt.Sprintf("articles/page/%d/index.html", page)
}

func TagPageURL(tag string, page int) string {
	if page <= 1 {
		return "tags/" + Slugify(tag) + "/index.html"
	}
	return fmt.Sprintf("tags/%s/page/%d/index.html", Slugify(tag), page)
}

func CategoryPageURL(path []string, page int) string {
	parts := make([]string, 0, len(path)+2)
	parts = append(parts, "categories")
	for _, part := range path {
		parts = append(parts, Slugify(part))
	}
	if page > 1 {
		parts = append(parts, "page", fmt.Sprint(page))
	}
	parts = append(parts, "index.html")
	return strings.Join(parts, "/")
}

func tagURL(tag string) string {
	return TagPageURL(tag, 1)
}

func pageCount(total, pageSize int) int {
	if pageSize <= 0 || total <= 0 {
		return 1
	}
	pages := total / pageSize
	if total%pageSize != 0 {
		pages++
	}
	return pages
}

func paginateArticles(articles []Article, page, pageSize int) ([]Article, int) {
	totalPages := pageCount(len(articles), pageSize)
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		return nil, totalPages
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > len(articles) {
		end = len(articles)
	}
	return articles[start:end], totalPages
}

func articlesByNewest(articles []Article) []Article {
	out := append([]Article(nil), articles...)
	sort.SliceStable(out, func(i, j int) bool {
		di, iok := parseDate(out[i].Date)
		dj, jok := parseDate(out[j].Date)
		if iok && jok && !di.Equal(dj) {
			return di.After(dj)
		}
		if out[i].Date != out[j].Date {
			return out[i].Date > out[j].Date
		}
		return out[i].Title < out[j].Title
	})
	return out
}

func truncateDescription(s string, limit int) string {
	s = strings.TrimSpace(s)
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return strings.TrimSpace(string(runes[:limit])) + " ..."
}

func tagIcon() string {
	return `<svg viewBox="0 0 24 24" focusable="false"><path d="M20.6 13.4 13.4 20.6a2 2 0 0 1-2.8 0L3 13V3h10l7.6 7.6a2 2 0 0 1 0 2.8Z"></path><circle cx="8" cy="8" r="1.7"></circle></svg>`
}

func ArticlesWithTag(articles []Article, tag string) []Article {
	return articlesWithTag(articles, tag)
}

func ArticlesInCategory(articles []Article, categoryPath []string) []Article {
	return articlesInCategory(articles, categoryPath)
}

func articlesWithTag(articles []Article, tag string) []Article {
	var out []Article
	for _, a := range articles {
		for _, item := range a.Tags {
			if item == tag {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

func articlesInCategory(articles []Article, categoryPath []string) []Article {
	var out []Article
	for _, a := range articles {
		if len(a.Category) < len(categoryPath) {
			continue
		}
		match := true
		for i := range categoryPath {
			if a.Category[i] != categoryPath[i] {
				match = false
				break
			}
		}
		if match {
			out = append(out, a)
		}
	}
	return out
}

func parseDate(s string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02", "2006/01/02", "2006-01", "2006/01", "2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func monthDay(s string) string {
	if t, ok := parseDate(s); ok {
		if t.Month() == time.January && t.Day() == 1 && dateOnlyRE.MatchString(strings.TrimSpace(s)) {
			return ""
		}
		return t.Format("01-02")
	}
	if len(s) >= 10 {
		return s[5:10]
	}
	return s
}

func sortedKeys[V any](m map[string]V) []string {
	return sortedKeysMap(m)
}

func sortedKeysMap[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeysPath(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func joinURL(prefix, path string) string {
	prefix = strings.TrimRight(prefix, "/")
	path = strings.TrimLeft(path, "/")
	if prefix == "" {
		return path
	}
	return prefix + "/" + path
}
