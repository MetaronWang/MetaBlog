package blog

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestMainFigURLMapsPDFToSVG(t *testing.T) {
	article := Article{
		Title:   "PDF Figure",
		Folder:  "articles/pdf-figure",
		MainFig: "fig/main.pdf",
		Slug:    "pdf-figure",
	}
	article.normalize()

	got := MainFigURL(article, "..")
	want := "../assets/articles/pdf-figure/fig/main.svg"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestArticleListUsesConvertedMainFigURL(t *testing.T) {
	article := Article{
		Title:   "PDF Figure",
		Date:    "2026-05-05",
		Folder:  "articles/pdf-figure",
		MainFig: "fig/main.pdf",
		Slug:    "pdf-figure",
	}
	article.normalize()

	got := RenderArticlesPage(Config{Title: "MetaBlog"}, []Article{article}, "..", "All")
	if !strings.Contains(got, `../assets/articles/pdf-figure/fig/main.svg`) {
		t.Fatalf("converted main figure URL not found: %s", got)
	}
	if strings.Contains(got, `fig/main.pdf`) {
		t.Fatalf("PDF main figure URL leaked into page: %s", got)
	}
}

func TestRenderHomeShowsNewestArticlesWithMetadata(t *testing.T) {
	longDescription := strings.Repeat("a", 1001)
	articles := []Article{
		{
			Title:       "Older",
			Date:        "2025-01-02",
			Folder:      "articles/older",
			Slug:        "older",
			Description: "Older description",
		},
		{
			Title:       "Newer",
			Date:        "2026-05-05",
			Folder:      "articles/newer",
			MainFig:     "fig/main.pdf",
			Slug:        "newer",
			Category:    []string{"Paper", "Learn to Optimize"},
			Tags:        []string{"L2O", "Go"},
			Description: longDescription,
		},
	}
	for i := range articles {
		articles[i].normalize()
	}

	got := RenderHome(Config{Title: "MetaBlog"}, articles)
	newerPos := strings.Index(got, "Newer")
	olderPos := strings.Index(got, "Older")
	if newerPos < 0 || olderPos < 0 || newerPos > olderPos {
		t.Fatalf("home articles are not ordered newest first:\n%s", got)
	}
	for _, want := range []string{
		`assets/articles/newer/fig/main.svg`,
		`href="categories/paper/index.html"`,
		`href="categories/paper/learn-to-optimize/index.html"`,
		`href="tags/l2o/index.html"`,
		`href="tags/go/index.html"`,
		`class="tag-icon"`,
		strings.Repeat("a", 1000) + " ...",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("home page missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, strings.Repeat("a", 1001)) {
		t.Fatalf("description was not truncated:\n%s", got)
	}
}

func TestRenderHomePaginatesTenArticles(t *testing.T) {
	var articles []Article
	for i := 1; i <= 11; i++ {
		articles = append(articles, Article{
			Title:  "Article " + string(rune('A'+i-1)),
			Date:   "2026-05-" + twoDigits(i),
			Folder: "articles/article-" + twoDigits(i),
			Slug:   "article-" + twoDigits(i),
		})
	}

	first := RenderHome(Config{Title: "MetaBlog", HomePageSize: 10}, articles)
	if !strings.Contains(first, `href="page/2/index.html"`) {
		t.Fatalf("home first page missing next pagination link:\n%s", first)
	}
	if strings.Contains(first, "Article A") {
		t.Fatalf("home first page should not include 11th newest article:\n%s", first)
	}
	second := RenderHomePage(Config{Title: "MetaBlog", HomePageSize: 10}, articles, 2, "../..")
	if !strings.Contains(second, "Article A") {
		t.Fatalf("home second page missing remaining article:\n%s", second)
	}
	if !strings.Contains(second, `href="../../index.html"`) {
		t.Fatalf("home second page missing previous pagination link:\n%s", second)
	}
}

func TestRenderArticleCollectionPaginatesTwentyArticles(t *testing.T) {
	var articles []Article
	for i := 1; i <= 21; i++ {
		articles = append(articles, Article{
			Title:  "Post " + twoDigits(i),
			Date:   "2026-04-" + twoDigits(i),
			Folder: "articles/post-" + twoDigits(i),
			Slug:   "post-" + twoDigits(i),
		})
	}

	cfg := Config{Title: "MetaBlog", ArticleListPageSize: 20}
	first := RenderArticlesPage(cfg, articles, "..", "All")
	if !strings.Contains(first, `href="../articles/page/2/index.html"`) {
		t.Fatalf("article list first page missing next pagination link:\n%s", first)
	}
	if strings.Contains(first, "Post 01") {
		t.Fatalf("article list first page should not include 21st newest article:\n%s", first)
	}
	second := RenderArticlesPagePage(cfg, articles, "../../..", "All", 2)
	if !strings.Contains(second, "Post 01") {
		t.Fatalf("article list second page missing remaining article:\n%s", second)
	}
	if !strings.Contains(second, `href="../../../articles/index.html"`) {
		t.Fatalf("article list second page missing previous pagination link:\n%s", second)
	}
}

func TestLoadFiltersDeletedArticles(t *testing.T) {
	dir := t.TempDir()
	if err := SaveArticles(filepath.Join(dir, "articles.toml"), []Article{
		{Title: "Visible", Folder: "articles/visible", MainFile: "main.tex"},
		{Title: "Deleted", Folder: "articles/deleted", MainFile: "main.tex", Deleted: true},
	}); err != nil {
		t.Fatal(err)
	}
	site, err := Load(dir, "", "articles.toml")
	if err != nil {
		t.Fatal(err)
	}
	if len(site.Articles) != 1 || site.Articles[0].Title != "Visible" {
		t.Fatalf("deleted articles were not filtered: %#v", site.Articles)
	}
}

func TestLoadRejectsArticlePathTraversal(t *testing.T) {
	dir := t.TempDir()
	for name, toml := range map[string]string{
		"folder": `[[articles]]
title = "Bad"
folder = "../outside"
main_file = "main.tex"
`,
		"main_file": `[[articles]]
title = "Bad"
folder = "articles/bad"
main_file = "../main.tex"
`,
		"main_fig": `[[articles]]
title = "Bad"
folder = "articles/bad"
main_file = "main.tex"
main_fig = "../fig.png"
`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".toml")
			if err := os.WriteFile(path, []byte(toml), 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(dir, "", path); err == nil {
				t.Fatal("expected path traversal to be rejected")
			}
		})
	}
}

func TestLoadAllowsAbsoluteCLIDataPath(t *testing.T) {
	dir := t.TempDir()
	articlesPath := filepath.Join(dir, "articles.toml")
	if err := SaveArticles(articlesPath, []Article{{Title: "Visible", Folder: "articles/visible", MainFile: "main.tex"}}); err != nil {
		t.Fatal(err)
	}
	site, err := Load(dir, "", articlesPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(site.Articles) != 1 {
		t.Fatalf("expected one article, got %d", len(site.Articles))
	}
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
