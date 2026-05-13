package app

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"MetaBlog/internal/blog"
)

type SiteServeConfig struct {
	OutDir   string
	Host     string
	Port     int
	Out      io.Writer
	Listener net.Listener
	Stop     <-chan struct{}

	// Watch mode: monitor source files and hot-rebuild changed articles.
	Watch          bool
	RootDir        string
	SiteConfig     string
	ArticlesFile   string
	LaTeXMLBin     string
	ArticleWorkers int
	LaTeXMLWorkers int
	NoAssets       bool
}

func RunSiteServe(cfg SiteServeConfig) error {
	cfg = normalizeSiteServeConfig(cfg)
	outDir, err := filepath.Abs(cfg.OutDir)
	if err != nil {
		return err
	}
	info, err := os.Stat(outDir)
	if err != nil {
		return fmt.Errorf("serve directory %s: %w", filepath.ToSlash(outDir), err)
	}
	if !info.IsDir() {
		return fmt.Errorf("serve path is not a directory: %s", filepath.ToSlash(outDir))
	}

	listener := cfg.Listener
	if listener == nil {
		addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
		listener, err = net.Listen("tcp", addr)
		if err != nil {
			return err
		}
	}
	server := &http.Server{
		Handler: http.FileServer(http.Dir(outDir)),
	}

	stopCh := cfg.Stop
	if stopCh == nil && cfg.Watch {
		stopCh = make(chan struct{})
	}

	if stopCh != nil {
		go func() {
			<-stopCh
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = server.Shutdown(ctx)
		}()
	}

	if cfg.Watch {
		siteData, buildCfg, err := loadSiteForWatch(cfg, outDir)
		if err != nil {
			return fmt.Errorf("watch: load site data: %w", err)
		}
		buildCfg.ensureLaTeXMLIdentity()
		buildCfg.prepareLaTeXMLIdentity()
		startWatcher(buildCfg, siteData, outDir, stopCh)
	}

	cfg.logf("Serving %s\n", filepath.ToSlash(outDir))
	cfg.logf("URL: %s\n", serveURL(listener.Addr(), cfg.Host))
	cfg.logf("Press Ctrl+C to stop.\n")
	err = server.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func loadSiteForWatch(cfg SiteServeConfig, outDir string) (*blog.Site, Config, error) {
	rootDir := cfg.RootDir
	if rootDir == "" {
		rootDir = "."
	}
	rootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, Config{}, err
	}
	siteData, err := loadPreparedSiteForWatch(rootDir, outDir, cfg.SiteConfig, cfg.ArticlesFile)
	if err != nil {
		return nil, Config{}, err
	}
	buildCfg := Config{
		RootDir:        rootDir,
		SiteConfig:     cfg.SiteConfig,
		ArticlesFile:   cfg.ArticlesFile,
		OutDir:         outDir,
		LaTeXMLBin:     cfg.LaTeXMLBin,
		ArticleWorkers: cfg.ArticleWorkers,
		LaTeXMLWorkers: cfg.LaTeXMLWorkers,
		NoAssets:       cfg.NoAssets,
		Log:            cfg.Out,
	}
	return siteData, buildCfg, nil
}

func loadPreparedSiteForWatch(rootDir, outDir, siteConfig, articlesFile string) (*blog.Site, error) {
	siteData, err := blog.Load(rootDir, siteConfig, articlesFile)
	if err != nil {
		return nil, err
	}
	if err := prepareSiteAssets(rootDir, outDir, &siteData.Config); err != nil {
		return nil, err
	}
	return siteData, nil
}

func normalizeSiteServeConfig(cfg SiteServeConfig) SiteServeConfig {
	if cfg.OutDir == "" {
		cfg.OutDir = "out"
	}
	if strings.TrimSpace(cfg.Host) == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Out == nil {
		cfg.Out = io.Discard
	}
	return cfg
}

func serveURL(addr net.Addr, host string) string {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok {
		return "http://" + addr.String() + "/"
	}
	displayHost := strings.TrimSpace(host)
	if displayHost == "" || displayHost == "0.0.0.0" || displayHost == "::" || displayHost == "[::]" {
		displayHost = "127.0.0.1"
	}
	if strings.Contains(displayHost, ":") && !strings.HasPrefix(displayHost, "[") {
		displayHost = "[" + displayHost + "]"
	}
	return "http://" + net.JoinHostPort(displayHost, strconv.Itoa(tcp.Port)) + "/"
}

func (cfg SiteServeConfig) logf(format string, args ...any) {
	if cfg.Out != nil {
		fmt.Fprintf(cfg.Out, format, args...)
	}
}
