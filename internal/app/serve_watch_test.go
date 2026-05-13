package app

import (
	"bytes"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunSiteServeWatchRebuildsAboutPage(t *testing.T) {
	dir := t.TempDir()
	aboutDir := filepath.Join(dir, "data", "about_page")
	if err := os.MkdirAll(aboutDir, 0755); err != nil {
		t.Fatal(err)
	}
	aboutTex := `\begin{document}
\section{Original}
Original content.
\end{document}
`
	if err := os.WriteFile(filepath.Join(aboutDir, "main.tex"), []byte(aboutTex), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(dir, "out")
	if err := RunSite(Config{
		RootDir:      dir,
		OutDir:       outDir,
		SiteConfig:   "",
		ArticlesFile: "",
		NoAssets:     true,
	}); err != nil {
		t.Fatal(err)
	}

	aboutPath := filepath.Join(outDir, "about", "index.html")
	orig, err := os.ReadFile(aboutPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(orig), "Original content.") {
		t.Fatalf("original about page missing content: %s", string(orig))
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var serveOut bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunSiteServe(SiteServeConfig{
			OutDir:   outDir,
			Host:     "127.0.0.1",
			Out:      &serveOut,
			Listener: listener,
			Stop:     stop,
			Watch:    true,
			RootDir:  dir,
			NoAssets: true,
		})
	}()

	waitForHTTP(t, "http://"+listener.Addr().String()+"/")

	time.Sleep(100 * time.Millisecond)
	modifiedTex := `\begin{document}
\section{Modified}
Modified content.
\end{document}
`
	if err := os.WriteFile(filepath.Join(aboutDir, "main.tex"), []byte(modifiedTex), 0644); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	updated := false
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(aboutPath)
		if err == nil && strings.Contains(string(b), "Modified content.") {
			updated = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !updated {
		b, _ := os.ReadFile(aboutPath)
		t.Fatalf("about page was not rebuilt after watch; got:\n%s\nserver output:\n%s", string(b), serveOut.String())
	}

	close(stop)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(serveOut.String(), "Watch: monitoring") {
		t.Fatalf("watch log not found in output:\n%s", serveOut.String())
	}
	if !strings.Contains(serveOut.String(), "about page rebuilt") {
		t.Fatalf("rebuild log not found in output:\n%s", serveOut.String())
	}
}

func TestRunSiteServeWatchDetectsArticleChange(t *testing.T) {
	dir := t.TempDir()

	articlesDir := filepath.Join(dir, "articles", "test-article")
	if err := os.MkdirAll(articlesDir, 0755); err != nil {
		t.Fatal(err)
	}
	articleTex := `\begin{document}
\section{Original Article}
This is the original article content.
\end{document}
`
	if err := os.WriteFile(filepath.Join(articlesDir, "main.tex"), []byte(articleTex), 0644); err != nil {
		t.Fatal(err)
	}

	aboutDir := filepath.Join(dir, "data", "about_page")
	if err := os.MkdirAll(aboutDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aboutDir, "main.tex"), []byte(`\begin{document}About\end{document}`), 0644); err != nil {
		t.Fatal(err)
	}

	configToml := `title = "Test Blog"
home_page_size = 10
article_list_page_size = 20
`
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data", "config.toml"), []byte(configToml), 0644); err != nil {
		t.Fatal(err)
	}

	articlesToml := `[[articles]]
title = "Test Article"
description = "Test description"
author = "Author"
date = "2026-05-01"
folder = "articles/test-article"
main_file = "main.tex"
`
	if err := os.WriteFile(filepath.Join(dir, "data", "articles.toml"), []byte(articlesToml), 0644); err != nil {
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

	slug := "test-article"
	articlePath := filepath.Join(outDir, "articles", slug, "index.html")
	orig, err := os.ReadFile(articlePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(orig), "This is the original article content.") {
		t.Fatalf("original article missing content: %s", string(orig))
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var serveOut bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunSiteServe(SiteServeConfig{
			OutDir:       outDir,
			Host:         "127.0.0.1",
			Out:          &serveOut,
			Listener:     listener,
			Stop:         stop,
			Watch:        true,
			RootDir:      dir,
			SiteConfig:   "data/config.toml",
			ArticlesFile: "data/articles.toml",
			NoAssets:     true,
		})
	}()

	waitForHTTP(t, "http://"+listener.Addr().String()+"/")

	time.Sleep(200 * time.Millisecond)
	modifiedTex := `\begin{document}
\section{Updated Article}
This article has been updated after watch rebuild.
\end{document}
`
	if err := os.WriteFile(filepath.Join(articlesDir, "main.tex"), []byte(modifiedTex), 0644); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	updated := false
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(articlePath)
		if err == nil && strings.Contains(string(b), "This article has been updated") {
			updated = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !updated {
		b, _ := os.ReadFile(articlePath)
		t.Fatalf("article was not rebuilt after watch; got:\n%s\nserver output:\n%s", string(b), serveOut.String())
	}

	close(stop)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(serveOut.String(), "Watch: monitoring") {
		t.Fatalf("watch log not found in output:\n%s", serveOut.String())
	}
	if !strings.Contains(serveOut.String(), "rebuilt Test Article") {
		t.Fatalf("article rebuild log not found in output:\n%s", serveOut.String())
	}
}

func TestRunSiteServeWatchArticleRebuildUsesPreparedSiteAssets(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "asset"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "asset", "logo.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0644); err != nil {
		t.Fatal(err)
	}

	articlesDir := filepath.Join(dir, "articles", "asset-path-article")
	if err := os.MkdirAll(articlesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(articlesDir, "main.tex"), []byte(`\begin{document}
\section{Original}
Original content.
\end{document}
`), 0644); err != nil {
		t.Fatal(err)
	}

	aboutDir := filepath.Join(dir, "data", "about_page")
	if err := os.MkdirAll(aboutDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aboutDir, "main.tex"), []byte(`\begin{document}About\end{document}`), 0644); err != nil {
		t.Fatal(err)
	}

	configToml := `title = "Asset Blog"
logo = "logo.svg"
home_page_size = 10
article_list_page_size = 20
`
	if err := os.WriteFile(filepath.Join(dir, "data", "config.toml"), []byte(configToml), 0644); err != nil {
		t.Fatal(err)
	}
	articlesToml := `[[articles]]
title = "Asset Path Article"
description = "Test description"
author = "Author"
date = "2026-05-01"
folder = "articles/asset-path-article"
main_file = "main.tex"
`
	if err := os.WriteFile(filepath.Join(dir, "data", "articles.toml"), []byte(articlesToml), 0644); err != nil {
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

	articlePath := filepath.Join(outDir, "articles", "asset-path-article", "index.html")
	if !fileContains(articlePath, "../../assets/site/logo.svg") {
		b, _ := os.ReadFile(articlePath)
		t.Fatalf("initial article page does not use prepared logo path:\n%s", string(b))
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var serveOut bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunSiteServe(SiteServeConfig{
			OutDir:       outDir,
			Host:         "127.0.0.1",
			Out:          &serveOut,
			Listener:     listener,
			Stop:         stop,
			Watch:        true,
			RootDir:      dir,
			SiteConfig:   "data/config.toml",
			ArticlesFile: "data/articles.toml",
			NoAssets:     true,
		})
	}()
	waitForHTTP(t, "http://"+listener.Addr().String()+"/")

	time.Sleep(200 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(articlesDir, "main.tex"), []byte(`\begin{document}
\section{Updated}
Updated content.
\end{document}
`), 0644); err != nil {
		t.Fatal(err)
	}

	waitForFileContains(t, articlePath, "Updated content.", 5*time.Second)
	close(stop)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(articlePath)
	if err != nil {
		t.Fatal(err)
	}
	html := string(b)
	if !strings.Contains(html, "../../assets/site/logo.svg") {
		t.Fatalf("rebuilt article page lost prepared logo path:\n%s\nserver output:\n%s", html, serveOut.String())
	}
	if strings.Contains(html, "../../logo.svg") {
		t.Fatalf("rebuilt article page used raw logo path:\n%s\nserver output:\n%s", html, serveOut.String())
	}
}

func TestRunSiteServeWatchBuildsNewArticleAfterArticlesConfigChange(t *testing.T) {
	dir := t.TempDir()

	aboutDir := filepath.Join(dir, "data", "about_page")
	if err := os.MkdirAll(aboutDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aboutDir, "main.tex"), []byte(`\begin{document}About\end{document}`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "data", "config.toml"), []byte(`title = "Blog"
home_page_size = 10
article_list_page_size = 20
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data", "articles.toml"), []byte{}, 0644); err != nil {
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

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var serveOut bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunSiteServe(SiteServeConfig{
			OutDir:       outDir,
			Host:         "127.0.0.1",
			Out:          &serveOut,
			Listener:     listener,
			Stop:         stop,
			Watch:        true,
			RootDir:      dir,
			SiteConfig:   "data/config.toml",
			ArticlesFile: "data/articles.toml",
			NoAssets:     true,
		})
	}()
	waitForHTTP(t, "http://"+listener.Addr().String()+"/")

	newArticleDir := filepath.Join(dir, "articles", "new-article")
	if err := os.MkdirAll(newArticleDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newArticleDir, "main.tex"), []byte(`\begin{document}
\section{New Article}
New article content from metadata watch.
\end{document}
`), 0644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)
	articlesToml := `[[articles]]
title = "New Article"
description = "New description"
author = "Author"
date = "2026-05-02"
folder = "articles/new-article"
main_file = "main.tex"
`
	if err := os.WriteFile(filepath.Join(dir, "data", "articles.toml"), []byte(articlesToml), 0644); err != nil {
		t.Fatal(err)
	}

	articlePath := filepath.Join(outDir, "articles", "new-article", "index.html")
	waitForFileContains(t, articlePath, "New article content from metadata watch.", 5*time.Second)

	close(stop)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(serveOut.String(), "checking article pages after metadata change") {
		t.Fatalf("article metadata rebuild log not found:\n%s", serveOut.String())
	}
}

func TestRunSiteServeWatchRegeneratesIndexOnConfigChange(t *testing.T) {
	dir := t.TempDir()

	aboutDir := filepath.Join(dir, "data", "about_page")
	if err := os.MkdirAll(aboutDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aboutDir, "main.tex"), []byte(`\begin{document}About\end{document}`), 0644); err != nil {
		t.Fatal(err)
	}

	configToml := `title = "Original Title"
home_page_size = 10
article_list_page_size = 20
`
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data", "config.toml"), []byte(configToml), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "data", "articles.toml"), []byte{}, 0644); err != nil {
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

	homePath := filepath.Join(outDir, "index.html")
	orig, err := os.ReadFile(homePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(orig), "Original Title") {
		t.Fatalf("original home page missing title: %s", string(orig))
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var serveOut bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunSiteServe(SiteServeConfig{
			OutDir:       outDir,
			Host:         "127.0.0.1",
			Out:          &serveOut,
			Listener:     listener,
			Stop:         stop,
			Watch:        true,
			RootDir:      dir,
			SiteConfig:   "data/config.toml",
			ArticlesFile: "data/articles.toml",
			NoAssets:     true,
		})
	}()

	waitForHTTP(t, "http://"+listener.Addr().String()+"/")

	time.Sleep(200 * time.Millisecond)
	updatedToml := `title = "Updated Title"
home_page_size = 10
article_list_page_size = 20
`
	if err := os.WriteFile(filepath.Join(dir, "data", "config.toml"), []byte(updatedToml), 0644); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	updated := false
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(homePath)
		if err == nil && strings.Contains(string(b), "Updated Title") {
			updated = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !updated {
		b, _ := os.ReadFile(homePath)
		t.Fatalf("home page was not regenerated after config change; got:\n%s\nserver output:\n%s", string(b), serveOut.String())
	}

	close(stop)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(serveOut.String(), "index pages regenerated") {
		t.Fatalf("index regen log not found in output:\n%s", serveOut.String())
	}
}

func TestRunSiteServeNoWatchDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("static"), 0644); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var out bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunSiteServe(SiteServeConfig{
			OutDir:   dir,
			Host:     "127.0.0.1",
			Out:      &out,
			Listener: listener,
			Stop:     stop,
		})
	}()

	waitForHTTP(t, "http://"+listener.Addr().String()+"/")
	resp, err := http.Get("http://" + listener.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	close(stop)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	if strings.Contains(out.String(), "Watch:") {
		t.Fatalf("watch should not be active without -watch flag:\n%s", out.String())
	}
}

func waitForFileContains(t *testing.T, path, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fileContains(path, want) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	b, _ := os.ReadFile(path)
	t.Fatalf("%s did not contain %q before timeout; got:\n%s", path, want, string(b))
}

func fileContains(path, want string) bool {
	b, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(b), want)
}
