package app

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunSiteServeServesStaticFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("hello metablog"), 0644); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	errCh := make(chan error, 1)
	var out bytes.Buffer
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
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello metablog" {
		t.Fatalf("unexpected body: %q", string(body))
	}
	close(stop)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "URL: http://127.0.0.1:") {
		t.Fatalf("server URL was not logged:\n%s", out.String())
	}
}

func TestRunSiteServeInitialBuildBeforeServing(t *testing.T) {
	dir := t.TempDir()
	aboutDir := filepath.Join(dir, "data", "about_page")
	if err := os.MkdirAll(aboutDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aboutDir, "main.tex"), []byte(`\begin{document}About\end{document}`), 0644); err != nil {
		t.Fatal(err)
	}
	articleDir := filepath.Join(dir, "articles", "new-article")
	if err := os.MkdirAll(articleDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(articleDir, "main.tex"), []byte(`\begin{document}
\section{New Article}
Built before serve.
\end{document}`), 0644); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), []byte(`title = "Initial Build"`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "articles.toml"), []byte(`[[articles]]
title = "New Article"
date = "2026-05-14"
folder = "articles/new-article"
main_file = "main.tex"
`), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(dir, "out")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	errCh := make(chan error, 1)
	var out bytes.Buffer
	go func() {
		errCh <- RunSiteServe(SiteServeConfig{
			InitialBuild: true,
			OutDir:       outDir,
			Host:         "127.0.0.1",
			Out:          &out,
			Listener:     listener,
			Stop:         stop,
			RootDir:      dir,
			SiteConfig:   "data/config.toml",
			ArticlesFile: "data/articles.toml",
			NoAssets:     true,
		})
	}()
	baseURL := "http://" + listener.Addr().String()
	waitForHTTP(t, baseURL+"/")
	resp, err := http.Get(baseURL + "/articles/new-article/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Built before serve.") {
		t.Fatalf("initial build article not served:\n%s\nlog:\n%s", string(body), out.String())
	}
	close(stop)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Initial build before serve") {
		t.Fatalf("initial build log missing:\n%s", out.String())
	}
}

func TestRunSiteServeRejectsMissingDirectory(t *testing.T) {
	err := RunSiteServe(SiteServeConfig{
		OutDir: filepath.Join(t.TempDir(), "missing"),
	})
	if err == nil || !strings.Contains(err.Error(), "serve directory") {
		t.Fatalf("expected missing directory error, got %v", err)
	}
}

func TestServeURLFormatsWildcardHostAsLocalhost(t *testing.T) {
	addr := &net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 12345}
	if got := serveURL(addr, "0.0.0.0"); got != "http://127.0.0.1:12345/" {
		t.Fatalf("unexpected URL: %s", got)
	}
}

func waitForHTTP(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server did not become ready: %s", url)
}
