package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunSiteBuildsAboutPageFromLatex(t *testing.T) {
	dir := t.TempDir()
	aboutDir := filepath.Join(dir, "data", "about_page")
	if err := os.MkdirAll(aboutDir, 0755); err != nil {
		t.Fatal(err)
	}
	aboutTex := `\begin{document}
\section{Biography}
About body from LaTeX.
\end{document}
`
	if err := os.WriteFile(filepath.Join(aboutDir, "main.tex"), []byte(aboutTex), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(dir, "out")
	err := RunSite(Config{
		RootDir:      dir,
		OutDir:       outDir,
		SiteConfig:   "",
		ArticlesFile: "",
		NoAssets:     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(filepath.Join(outDir, "about", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "Biography") || !strings.Contains(got, "About body from LaTeX.") {
		t.Fatalf("about page was not rendered from LaTeX source:\n%s", got)
	}
	if strings.Contains(got, "<h1>关于</h1>") {
		t.Fatalf("about page still uses placeholder content:\n%s", got)
	}
}

func TestRunSiteSkipsFreshAboutPage(t *testing.T) {
	dir := t.TempDir()
	aboutDir := filepath.Join(dir, "data", "about_page")
	if err := os.MkdirAll(aboutDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aboutDir, "main.tex"), []byte(`\begin{document}
\section{Biography}
Source body.
\end{document}
`), 0644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	outPath := filepath.Join(outDir, "about", "index.html")
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		t.Fatal(err)
	}
	cached := []byte("<html>cached about page</html>")
	if err := os.WriteFile(outPath, cached, 0644); err != nil {
		t.Fatal(err)
	}

	sourceTime := time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local)
	outputTime := sourceTime.Add(time.Hour)
	if err := os.Chtimes(filepath.Join(aboutDir, "main.tex"), sourceTime, sourceTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(outPath, outputTime, outputTime); err != nil {
		t.Fatal(err)
	}

	err := RunSite(Config{
		RootDir:      dir,
		OutDir:       outDir,
		SiteConfig:   "",
		ArticlesFile: "",
		NoAssets:     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != string(cached) {
		t.Fatalf("fresh about page was rebuilt; got:\n%s", string(b))
	}
}

func TestRunSiteForceRebuildsFreshAboutPage(t *testing.T) {
	dir := t.TempDir()
	aboutDir := filepath.Join(dir, "data", "about_page")
	if err := os.MkdirAll(aboutDir, 0755); err != nil {
		t.Fatal(err)
	}
	source := []byte(`\begin{document}
\section{Biography}
Forced source body.
\end{document}
`)
	if err := os.WriteFile(filepath.Join(aboutDir, "main.tex"), source, 0644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	outPath := filepath.Join(outDir, "about", "index.html")
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outPath, []byte("<html>stale but newer</html>"), 0644); err != nil {
		t.Fatal(err)
	}

	sourceTime := time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local)
	outputTime := sourceTime.Add(time.Hour)
	if err := os.Chtimes(filepath.Join(aboutDir, "main.tex"), sourceTime, sourceTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(outPath, outputTime, outputTime); err != nil {
		t.Fatal(err)
	}

	err := RunSite(Config{
		RootDir:      dir,
		OutDir:       outDir,
		SiteConfig:   "",
		ArticlesFile: "",
		NoAssets:     true,
		Force:        true,
	})
	if err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Forced source body.") {
		t.Fatalf("force build did not rebuild fresh about page:\n%s", string(b))
	}
}

func TestDocumentOutputsFreshUsesHTMLOutputTime(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "article")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(sourceDir, "main.tex")
	if err := os.WriteFile(sourcePath, []byte(`\begin{document}body\end{document}`), 0644); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, "out", "articles", "example", "index.html")
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outPath, []byte("<html>cached</html>"), 0644); err != nil {
		t.Fatal(err)
	}
	oldAssetPath := filepath.Join(dir, "out", "assets", "articles", "example", "fig", "old.svg")
	if err := os.MkdirAll(filepath.Dir(oldAssetPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldAssetPath, []byte("<svg/>"), 0644); err != nil {
		t.Fatal(err)
	}

	sourceTime := time.Date(2026, 5, 11, 2, 16, 0, 0, time.Local)
	oldAssetTime := sourceTime.Add(-time.Hour)
	htmlTime := sourceTime.Add(time.Hour)
	if err := os.Chtimes(sourcePath, sourceTime, sourceTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(oldAssetPath, oldAssetTime, oldAssetTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(outPath, htmlTime, htmlTime); err != nil {
		t.Fatal(err)
	}

	fresh, err := documentOutputsFresh(sourceDir, outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !fresh {
		t.Fatal("expected fresh document when HTML is newer than sources, even if an unchanged asset output is older")
	}
}

func TestLoadCustomComponentsWrapsRenderedFragments(t *testing.T) {
	dir := t.TempDir()
	componentsDir := filepath.Join(dir, "data", "custom_components")
	if err := os.MkdirAll(componentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(componentsDir, "page_footing.tex"), []byte(`\begin{html}<span>site stats</span>\end{html}

Footer paragraph.`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(componentsDir, "article_stat.tex"), []byte(`\begin{html}<span>article stats</span>\end{html}`), 0644); err != nil {
		t.Fatal(err)
	}

	components, warnings, err := loadCustomComponents(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
	for _, want := range []string{
		`<footer class="custom-page-footing">`,
		`<span>site stats</span>`,
		`<p>Footer paragraph.</p>`,
		`</footer>`,
	} {
		if !strings.Contains(components.PageFooterHTML, want) {
			t.Fatalf("page footer component missing %q:\n%s", want, components.PageFooterHTML)
		}
	}
	for _, want := range []string{
		`<div class="custom-article-stat">`,
		`<span>article stats</span>`,
		`</div>`,
	} {
		if !strings.Contains(components.ArticleStatHTML, want) {
			t.Fatalf("article stat component missing %q:\n%s", want, components.ArticleStatHTML)
		}
	}
}

func TestCopyConfiguredSiteAssetRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(filepath.Join(dir, "asset"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := copyConfiguredSiteAsset(dir, outDir, "../secret.txt"); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
	store := newMemStore()
	if _, err := copyConfiguredSiteAssetToMemory(dir, store, "../secret.txt"); err == nil {
		t.Fatal("expected in-memory path traversal to be rejected")
	}
}

func TestCopyConfiguredSiteAssetUsesCleanRelativePath(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(filepath.Join(dir, "asset", "figs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "asset", "logo.svg"), []byte("<svg/>"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := copyConfiguredSiteAsset(dir, outDir, "figs/../logo.svg")
	if err != nil {
		t.Fatal(err)
	}
	if got != "assets/site/logo.svg" {
		t.Fatalf("unexpected output path: %q", got)
	}
	if _, err := os.Stat(filepath.Join(outDir, "assets", "site", "logo.svg")); err != nil {
		t.Fatal(err)
	}
}
