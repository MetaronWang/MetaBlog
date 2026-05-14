package app

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunSiteServeWatchDebounceCollapsesRapidChanges(t *testing.T) {
	dir := t.TempDir()
	aboutDir := filepath.Join(dir, "data", "about_page")
	if err := os.MkdirAll(aboutDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aboutDir, "main.tex"), []byte(`\begin{document}
\section{Original}
Original.
\end{document}
`), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(dir, "out")
	if err := RunSite(Config{
		RootDir: dir, OutDir: outDir, SiteConfig: "", ArticlesFile: "", NoAssets: true,
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
			OutDir: outDir, Host: "127.0.0.1", Out: &serveOut,
			Listener: listener, Stop: stop,
			Watch: true, RootDir: dir, NoAssets: true,
		})
	}()
	waitForHTTP(t, "http://"+listener.Addr().String()+"/")

	// Write three rapid changes within one debounce window.
	time.Sleep(200 * time.Millisecond)
	changes := []string{"Change-A", "Change-B", "Change-C"}
	for i, label := range changes {
		tex := `\begin{document}
\section{` + label + `}
` + label + `.
\end{document}
`
		if err := os.WriteFile(filepath.Join(aboutDir, "main.tex"), []byte(tex), 0644); err != nil {
			t.Fatal(err)
		}
		if i < len(changes)-1 {
			time.Sleep(80 * time.Millisecond)
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	var lastBody string
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + listener.Addr().String() + "/about/")
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastBody = string(b)
			if strings.Contains(lastBody, "Change-C") {
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !strings.Contains(lastBody, "Change-C") {
		t.Fatalf("final change not served:\n%s\nserver output:\n%s", lastBody, serveOut.String())
	}

	rebuildCount := strings.Count(serveOut.String(), "about page rebuilt")
	if rebuildCount > 1 {
		t.Fatalf("debounce failed: %d rebuilds for 3 rapid changes\nserver output:\n%s", rebuildCount, serveOut.String())
	}

	close(stop)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestRunSiteServeWatchInjectsLiveReloadAndBumpsChangedPage(t *testing.T) {
	dir := t.TempDir()
	aboutDir := filepath.Join(dir, "data", "about_page")
	if err := os.MkdirAll(aboutDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aboutDir, "main.tex"), []byte(`\begin{document}
\section{Original}
Original.
\end{document}
`), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(dir, "out")
	if err := RunSite(Config{RootDir: dir, OutDir: outDir, SiteConfig: "", ArticlesFile: "", NoAssets: true}); err != nil {
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
			OutDir: outDir, Host: "127.0.0.1", Out: &serveOut,
			Listener: listener, Stop: stop,
			Watch: true, RootDir: dir, NoAssets: true,
		})
	}()
	baseURL := "http://" + listener.Addr().String()
	waitForHTTP(t, baseURL+"/")

	resp, err := http.Get(baseURL + "/about/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), liveReloadEndpoint) {
		t.Fatalf("watch-served HTML did not contain live reload script:\n%s", string(body))
	}

	initialResp, err := http.Get(baseURL + liveReloadEndpoint + "?path=/about/")
	if err != nil {
		t.Fatal(err)
	}
	initialVersionBytes, _ := io.ReadAll(initialResp.Body)
	initialResp.Body.Close()
	initialVersion := strings.TrimSpace(string(initialVersionBytes))

	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(aboutDir, "main.tex"), []byte(`\begin{document}
\section{Modified}
Modified live reload content.
\end{document}
`), 0644); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	var version string
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + liveReloadEndpoint + "?path=/about/")
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			version = strings.TrimSpace(string(b))
			if version != "" && version != initialVersion {
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	if version == "" || version == initialVersion {
		t.Fatalf("live reload version did not change for about page; initial=%q final=%q\nserver output:\n%s", initialVersion, version, serveOut.String())
	}

	close(stop)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestInjectLiveReloadScriptReplacesOldInjectedScript(t *testing.T) {
	old := `<html><body><main>content</main><script>
(function() {
  var endpoint = "` + liveReloadEndpoint + `";
  var currentVersion = "1";
})();
</script></body></html>`
	got := injectLiveReloadScript(old, 7)
	if strings.Count(got, liveReloadEndpoint) != 1 {
		t.Fatalf("expected exactly one live reload script, got:\n%s", got)
	}
	if strings.Contains(got, `currentVersion = "1"`) {
		t.Fatalf("old live reload version was preserved:\n%s", got)
	}
	if !strings.Contains(got, `currentVersion = "7"`) {
		t.Fatalf("new live reload version missing:\n%s", got)
	}
	if !strings.Contains(got, `minReloadInterval = 3000`) {
		t.Fatalf("live reload minimum interval missing:\n%s", got)
	}
	if !strings.Contains(got, `currentVersion = version`) {
		t.Fatalf("live reload script does not consume version changes before reload:\n%s", got)
	}
}

func TestLiveReloadVersionNormalizesEscapedUnicodePath(t *testing.T) {
	state := newLiveReloadState()
	slug := "\u9ad8\u65af\u8fc7\u7a0b"
	state.MarkUpdated("articles/" + slug + "/index.html")

	escaped := "/articles/" + url.PathEscape(slug) + "/"
	if got, want := state.Version(escaped), state.Version("/articles/"+slug+"/"); got != want || got == 0 {
		t.Fatalf("escaped path version mismatch: got %d, want %d", got, want)
	}
}

func TestRunSiteServeOnlyRAMArticleHTMLDoesNotTouchDisk(t *testing.T) {
	dir := t.TempDir()
	articleDir := filepath.Join(dir, "articles", "ram-html-article")
	if err := os.MkdirAll(articleDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(articleDir, "main.tex"), []byte(`\begin{document}
\section{Original}
Original article body.
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
	if err := os.WriteFile(filepath.Join(dir, "data", "config.toml"), []byte("title = \"RAM HTML Test\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data", "articles.toml"), []byte(`[[articles]]
title = "RAM HTML Article"
description = "Test"
author = "Author"
date = "2026-05-07"
folder = "articles/ram-html-article"
main_file = "main.tex"
`), 0644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	if err := RunSite(Config{RootDir: dir, OutDir: outDir, SiteConfig: "data/config.toml", ArticlesFile: "data/articles.toml", NoAssets: true}); err != nil {
		t.Fatal(err)
	}
	articlePath := filepath.Join(outDir, "articles", "ram-html-article", "index.html")
	origData, err := os.ReadFile(articlePath)
	if err != nil {
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
			OutDir: outDir, Host: "127.0.0.1", Out: &serveOut,
			Listener: listener, Stop: stop,
			Watch: true, RootDir: dir,
			SiteConfig: "data/config.toml", ArticlesFile: "data/articles.toml",
			OnlyRAM: true, NoAssets: true,
		})
	}()
	baseURL := "http://" + listener.Addr().String()
	waitForHTTP(t, baseURL+"/")
	time.Sleep(200 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(articleDir, "main.tex"), []byte(`\begin{document}
\section{Updated}
Updated article body.
\end{document}
`), 0644); err != nil {
		t.Fatal(err)
	}
	waitForHTTPBodyContains(t, baseURL+"/articles/ram-html-article/", "Updated article body", 5*time.Second)
	diskData, err := os.ReadFile(articlePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(diskData, origData) {
		t.Fatalf("only-ram wrote article HTML to disk:\n%s\nserver output:\n%s", string(diskData), serveOut.String())
	}
	close(stop)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestRunSiteServeWatchEmptySiteDoesNotCrash(t *testing.T) {
	dir := t.TempDir()

	aboutDir := filepath.Join(dir, "data", "about_page")
	if err := os.MkdirAll(aboutDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aboutDir, "main.tex"), []byte(`\begin{document}About\end{document}`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "data", "config.toml"), []byte("title = \"Empty\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data", "articles.toml"), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	if err := RunSite(Config{
		RootDir: dir, OutDir: outDir, SiteConfig: "data/config.toml", ArticlesFile: "data/articles.toml", NoAssets: true,
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
			OutDir: outDir, Host: "127.0.0.1", Out: &serveOut,
			Listener: listener, Stop: stop,
			Watch: true, RootDir: dir,
			SiteConfig: "data/config.toml", ArticlesFile: "data/articles.toml",
			NoAssets: true,
		})
	}()
	waitForHTTP(t, "http://"+listener.Addr().String()+"/")

	resp, err := http.Get("http://" + listener.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if !strings.Contains(serveOut.String(), "Watch: monitoring 0 article(s)") {
		t.Fatalf("empty site watch log missing:\n%s", serveOut.String())
	}

	close(stop)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestMemStoreServesBinaryFilesWithCorrectContentType(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")

	staticDir := filepath.Join(outDir, "static")
	if err := os.MkdirAll(staticDir, 0755); err != nil {
		t.Fatal(err)
	}
	svgContent := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><circle cx="50" cy="50" r="40"/></svg>`)
	if err := os.WriteFile(filepath.Join(staticDir, "logo.svg"), svgContent, 0644); err != nil {
		t.Fatal(err)
	}
	woffContent := []byte{0x00, 0x01, 0x02, 0x03, 0x77, 0x4F, 0x46, 0x46}
	if err := os.WriteFile(filepath.Join(staticDir, "font.woff2"), woffContent, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "index.html"), []byte("<html>home</html>"), 0644); err != nil {
		t.Fatal(err)
	}

	store := newMemStore()
	if err := store.loadDir(outDir); err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	go func() {
		server := &http.Server{Handler: store}
		go func() { <-stop; server.Shutdown(context.Background()) }()
		server.Serve(listener)
	}()

	baseURL := "http://" + listener.Addr().String()

	resp, err := http.Get(baseURL + "/static/logo.svg")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Equal(body, svgContent) {
		t.Fatalf("SVG content mismatch: got %d bytes, want %d", len(body), len(svgContent))
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/svg+xml") {
		t.Fatalf("SVG content-type: got %q", ct)
	}

	resp, err = http.Get(baseURL + "/static/font.woff2")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Equal(body, woffContent) {
		t.Fatalf("WOFF2 content mismatch: got %d bytes, want %d", len(body), len(woffContent))
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "font/woff2") {
		t.Fatalf("WOFF2 content-type: got %q", ct)
	}

	close(stop)
}

func TestRunSiteServeOnlyRAMRemovesStaleArticlePagesFromMemory(t *testing.T) {
	dir := t.TempDir()

	articleDir := filepath.Join(dir, "articles", "ram-removable")
	if err := os.MkdirAll(articleDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(articleDir, "main.tex"), []byte(`\begin{document}RAM Removable content.\end{document}`), 0644); err != nil {
		t.Fatal(err)
	}
	aboutDir := filepath.Join(dir, "data", "about_page")
	if err := os.MkdirAll(aboutDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aboutDir, "main.tex"), []byte(`\begin{document}About\end{document}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data", "config.toml"), []byte("title = \"RAM Delete\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	articlesPath := filepath.Join(dir, "data", "articles.toml")
	if err := os.WriteFile(articlesPath, []byte(`[[articles]]
title = "RAM Removable"
description = "Test"
author = "Author"
date = "2026-05-01"
folder = "articles/ram-removable"
main_file = "main.tex"
`), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(dir, "out")
	if err := RunSite(Config{
		RootDir: dir, OutDir: outDir, SiteConfig: "data/config.toml", ArticlesFile: "data/articles.toml", NoAssets: true,
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
			OutDir: outDir, Host: "127.0.0.1", Out: &serveOut,
			Listener: listener, Stop: stop,
			Watch: true, RootDir: dir,
			SiteConfig: "data/config.toml", ArticlesFile: "data/articles.toml",
			OnlyRAM: true, NoAssets: true,
		})
	}()
	baseURL := "http://" + listener.Addr().String()
	waitForHTTP(t, baseURL+"/")

	waitForHTTPStatus(t, baseURL+"/articles/ram-removable/", http.StatusOK, 3*time.Second)

	time.Sleep(200 * time.Millisecond)
	if err := os.WriteFile(articlesPath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	waitForHTTPStatus(t, baseURL+"/articles/ram-removable/", http.StatusNotFound, 5*time.Second)

	close(stop)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}
