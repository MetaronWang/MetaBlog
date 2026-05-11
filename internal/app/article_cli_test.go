package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"MetaBlog/internal/blog"
)

func TestRunArticleInitCreatesArticle(t *testing.T) {
	dir := t.TempDir()
	input := strings.NewReader(strings.Join([]string{
		"My New Article",
		"",
		"paper.tex",
		"Metaron",
		"2026-05-05",
		"Research, Optimization",
		"LaTeX, Blog",
		"fig/main.png",
		"First description line.",
		"Second description line.",
		"",
		"",
	}, "\n"))
	var out strings.Builder

	err := RunArticleInit(ArticleCLIConfig{
		RootDir:      dir,
		ArticlesFile: "data/articles.toml",
		In:           input,
		Out:          &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "articles", "my-new-article", "paper.tex")
	if _, err := os.Stat(mainPath); err != nil {
		t.Fatalf("main file was not created: %v", err)
	}
	articles, err := blog.LoadArticles(filepath.Join(dir, "data", "articles.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 1 {
		t.Fatalf("expected one article, got %d", len(articles))
	}
	a := articles[0]
	if a.Title != "My New Article" || a.MainFile != "paper.tex" || a.Folder != "articles/my-new-article" {
		t.Fatalf("unexpected article metadata: %#v", a)
	}
	if a.Description != "First description line.\nSecond description line." {
		t.Fatalf("unexpected description: %q", a.Description)
	}
	if strings.Join(a.Category, "/") != "Research/Optimization" {
		t.Fatalf("unexpected category: %#v", a.Category)
	}
}

func TestRunArticleInitPrintsExistingCategoryAndTagReferences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data", "articles.toml")
	if err := blog.SaveArticles(path, []blog.Article{
		{
			Title:    "Visible",
			Category: []string{"Research", "Optimization"},
			Tags:     []string{"LaTeX", "Blog"},
			Folder:   "articles/visible",
			MainFile: "main.tex",
		},
		{
			Title:    "Deleted",
			Category: []string{"Hidden"},
			Tags:     []string{"HiddenTag"},
			Folder:   "articles/deleted",
			MainFile: "main.tex",
			Deleted:  true,
		},
	}); err != nil {
		t.Fatal(err)
	}
	input := strings.NewReader(strings.Join([]string{
		"My New Article",
		"",
		"",
		"",
		"2026-05-05",
		"Research, Optimization",
		"Blog",
		"",
	}, "\n"))
	var out strings.Builder

	err := RunArticleInit(ArticleCLIConfig{
		RootDir:      dir,
		ArticlesFile: "data/articles.toml",
		In:           input,
		Out:          &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "Existing categories: Research / Optimization") {
		t.Fatalf("category reference missing from output:\n%s", got)
	}
	if !strings.Contains(got, "Existing tags: Blog, LaTeX") {
		t.Fatalf("tag reference missing from output:\n%s", got)
	}
	if strings.Contains(got, "Hidden") || strings.Contains(got, "HiddenTag") {
		t.Fatalf("deleted article references leaked into output:\n%s", got)
	}
}

func TestRunArticleEditUpdatesArticle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data", "articles.toml")
	if err := blog.SaveArticles(path, []blog.Article{{
		Title:       "Old Title",
		Author:      "Old Author",
		Date:        "2026-01-01",
		Category:    []string{"Old"},
		Tags:        []string{"Tag"},
		Folder:      "articles/old-title",
		MainFile:    "main.tex",
		Description: "Old description",
	}}); err != nil {
		t.Fatal(err)
	}
	input := strings.NewReader(strings.Join([]string{
		"1",
		"New Title",
		"New Author",
		"2026-05-05",
		"",
		"article.tex",
		"",
		"Research, Blog",
		"Go, LaTeX",
		"New description line 1.",
		"New description line 2.",
		"",
		"",
	}, "\n"))
	var out strings.Builder

	err := RunArticleEdit(ArticleCLIConfig{
		RootDir:      dir,
		ArticlesFile: "data/articles.toml",
		In:           input,
		Out:          &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	articles, err := blog.LoadArticles(path)
	if err != nil {
		t.Fatal(err)
	}
	a := articles[0]
	if a.Title != "New Title" || a.Author != "New Author" || a.MainFile != "article.tex" {
		t.Fatalf("unexpected edited metadata: %#v", a)
	}
	if a.Description != "New description line 1.\nNew description line 2." {
		t.Fatalf("unexpected edited description: %q", a.Description)
	}
	if strings.Join(a.Tags, "/") != "Go/LaTeX" {
		t.Fatalf("unexpected tags: %#v", a.Tags)
	}
}

func TestRunArticleDeleteMarksArticleDeleted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data", "articles.toml")
	if err := blog.SaveArticles(path, []blog.Article{
		{
			Title:    "Keep",
			Folder:   "articles/keep",
			MainFile: "main.tex",
		},
		{
			Title:    "Delete Me",
			Folder:   "articles/delete-me",
			MainFile: "main.tex",
		},
	}); err != nil {
		t.Fatal(err)
	}
	input := strings.NewReader("2\n")
	var out strings.Builder

	err := RunArticleDelete(ArticleCLIConfig{
		RootDir:      dir,
		ArticlesFile: "data/articles.toml",
		In:           input,
		Out:          &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	articles, err := blog.LoadArticles(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 2 {
		t.Fatalf("delete should not remove metadata entries, got %d", len(articles))
	}
	if !articles[1].Deleted {
		t.Fatalf("article was not marked deleted: %#v", articles[1])
	}
	if articles[0].Deleted {
		t.Fatalf("wrong article marked deleted: %#v", articles[0])
	}
}
