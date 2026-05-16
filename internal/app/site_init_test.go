package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSiteInitCreatesMinimalSite(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	err := RunSiteInit(SiteInitConfig{
		RootDir:      dir,
		Title:        "Test Blog",
		SkipFonts:    true,
		SkipEnvCheck: true,
		Out:          &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"articles",
		"asset/figs",
		"data/about_page",
		"data/custom_components",
		"web/static/fonts",
	} {
		info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s was not created: %v", rel, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", rel)
		}
	}
	for _, rel := range []string{
		"data/config.toml",
		"data/articles.toml",
		"data/about_page/main.tex",
		"data/custom_components/page_footing.tex",
		"data/custom_components/article_stat.tex",
		"asset/figs/circle_example.svg",
		"web/static/fonts.css",
		".gitignore",
	} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s was not created: %v", rel, err)
		}
	}
	config, err := os.ReadFile(filepath.Join(dir, "data", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), `title = "Test Blog"`) {
		t.Fatalf("site title was not written:\n%s", string(config))
	}
	if !strings.Contains(string(config), `logo = "figs/circle_example.svg"`) ||
		!strings.Contains(string(config), `icon = "figs/circle_example.svg"`) {
		t.Fatalf("default logo/icon were not written:\n%s", string(config))
	}
	icon, err := os.ReadFile(filepath.Join(dir, "asset", "figs", "circle_example.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(icon), `<circle cx="64" cy="64" r="56" fill="#000"/>`) {
		t.Fatalf("default circle svg was not written:\n%s", string(icon))
	}
	footer, err := os.ReadFile(filepath.Join(dir, "data", "custom_components", "page_footing.tex"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(footer), `custom-page-footing`) || strings.Contains(string(footer), `<footer`) {
		t.Fatalf("default page footing should not contain its outer wrapper:\n%s", string(footer))
	}
	if !strings.Contains(string(footer), `busuanzi_value_site_pv`) {
		t.Fatalf("default page footing was not written with busuanzi markup:\n%s", string(footer))
	}
}

func TestRunSiteInitWorksFromEmptyWorkingDirectory(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	var out bytes.Buffer
	if err := RunSiteInit(SiteInitConfig{
		RootDir:      "site",
		SkipFonts:    true,
		SkipEnvCheck: true,
		Out:          &out,
	}); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"site/data/config.toml",
		"site/data/articles.toml",
		"site/data/about_page/main.tex",
		"site/data/custom_components/page_footing.tex",
		"site/data/custom_components/article_stat.tex",
		"site/asset/figs/circle_example.svg",
		"site/web/static/fonts.css",
	} {
		if _, err := os.Stat(filepath.Join(cwd, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s was not created from empty cwd: %v", rel, err)
		}
	}
}

func TestRunSiteInitDoesNotOverwriteExistingFiles(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "data", "config.toml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("title = \"Existing\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunSiteInit(SiteInitConfig{
		RootDir:      dir,
		Title:        "New",
		SkipFonts:    true,
		SkipEnvCheck: true,
		Out:          &out,
	}); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(config) != "title = \"Existing\"\n" {
		t.Fatalf("existing config was overwritten:\n%s", string(config))
	}
	if !strings.Contains(out.String(), "File exists, skipped: data/config.toml") {
		t.Fatalf("skip was not logged:\n%s", out.String())
	}
}

func TestRunSiteInitDownloadsMissingFonts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("font-bytes"))
	}))
	defer server.Close()

	dir := t.TempDir()
	var out bytes.Buffer
	if err := RunSiteInit(SiteInitConfig{
		RootDir:      dir,
		SkipEnvCheck: true,
		Out:          &out,
		FontFiles: []FontDownload{{
			Name: "test-font.otf",
			URL:  server.URL + "/test-font.otf",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "web", "static", "fonts", "test-font.otf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "font-bytes" {
		t.Fatalf("unexpected font content: %q", string(b))
	}
	if !strings.Contains(out.String(), "Font downloaded: test-font.otf") {
		t.Fatalf("download was not logged:\n%s", out.String())
	}
}

func TestSiteInitInstallHintsArePresent(t *testing.T) {
	for name, hint := range map[string]string{
		"latexml":      lateXMLInstallHint(),
		"python":       pythonInstallHint(),
		"fonttools":    fontToolsInstallHint(),
		"pdfConverter": pdfConverterInstallHint(),
	} {
		if strings.TrimSpace(hint) == "" {
			t.Fatalf("%s install hint is empty", name)
		}
	}
}

func TestRunSiteInitOutputCanBuildSite(t *testing.T) {
	dir := t.TempDir()
	if err := RunSiteInit(SiteInitConfig{
		RootDir:      dir,
		SkipFonts:    true,
		SkipEnvCheck: true,
	}); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	if err := RunSite(Config{
		RootDir:      dir,
		OutDir:       outDir,
		SiteConfig:   "data/config.toml",
		ArticlesFile: "data/articles.toml",
		NoAssets:     true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "about", "index.html")); err != nil {
		t.Fatalf("initialized site did not build about page: %v", err)
	}
}
