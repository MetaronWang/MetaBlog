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
	"MetaBlog/internal/latexml"
	"MetaBlog/internal/pathutil"
)

type SiteServeConfig struct {
	OutDir   string
	Host     string
	Port     int
	Out      io.Writer
	Listener net.Listener
	Stop     <-chan struct{}

	// InitialBuild runs a full site build before starting the HTTP server.
	InitialBuild bool

	// Watch mode: monitor source files and hot-rebuild changed articles.
	Watch          bool
	RootDir        string
	SiteConfig     string
	ArticlesFile   string
	LaTeXMLBin     string
	ArticleWorkers int
	LaTeXMLWorkers int
	NoAssets       bool
	Force          bool
	NoLaTeXMLCache bool

	// OnlyRAM: serve and rebuild entirely in memory (no disk writes for output).
	OnlyRAM bool
}

func RunSiteServe(cfg SiteServeConfig) error {
	cfg = normalizeSiteServeConfig(cfg)
	if cfg.InitialBuild {
		if err := buildBeforeServe(cfg); err != nil {
			return err
		}
	}
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

	var store *memStore
	if cfg.OnlyRAM {
		store = newMemStore()
		if err := store.loadDir(outDir); err != nil {
			return fmt.Errorf("only-ram: load output directory: %w", err)
		}
		cfg.logf("Only-RAM: loaded %d files from %s\n", len(store.files), filepath.ToSlash(outDir))
	}

	var handler http.Handler
	if store != nil {
		handler = store
	} else {
		handler = http.FileServer(http.Dir(outDir))
	}
	var liveReload *liveReloadState
	if cfg.Watch {
		liveReload = newLiveReloadState()
		handler = liveReloadHandler{
			outDir: outDir,
			store:  store,
			base:   handler,
			state:  liveReload,
		}
	}
	server := &http.Server{Handler: handler}

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
		siteData, buildCfg, err := loadSiteForWatch(cfg, outDir, store)
		if err != nil {
			return fmt.Errorf("watch: load site data: %w", err)
		}
		buildCfg.ensureLaTeXMLIdentity()
		buildCfg.prepareLaTeXMLIdentity()
		startWatcher(buildCfg, siteData, outDir, stopCh, store, liveReload)
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

func loadSiteForWatch(cfg SiteServeConfig, outDir string, store *memStore) (*blog.Site, Config, error) {
	rootDir := cfg.RootDir
	if rootDir == "" {
		rootDir = "."
	}
	rootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, Config{}, err
	}
	siteData, err := loadPreparedSiteForWatch(rootDir, outDir, cfg.SiteConfig, cfg.ArticlesFile, store)
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
		Force:          cfg.Force,
		NoLaTeXMLCache: cfg.NoLaTeXMLCache,
		Log:            cfg.Out,
		MemStore:       store,
	}
	if store != nil {
		buildCfg.CacheStore = latexml.NewCacheStore(256)
	}
	return siteData, buildCfg, nil
}

func loadPreparedSiteForWatch(rootDir, outDir, siteConfig, articlesFile string, store *memStore) (*blog.Site, error) {
	siteData, err := blog.Load(rootDir, siteConfig, articlesFile)
	if err != nil {
		return nil, err
	}
	if store != nil {
		if err := prepareSiteAssetsInMemory(rootDir, store, &siteData.Config); err != nil {
			return nil, err
		}
		return siteData, nil
	}
	if err := prepareSiteAssets(rootDir, outDir, &siteData.Config); err != nil {
		return nil, err
	}
	return siteData, nil
}

func prepareSiteAssetsInMemory(rootDir string, store *memStore, cfg *blog.Config) error {
	if store == nil {
		return nil
	}
	var err error
	if cfg.Logo, err = copyConfiguredSiteAssetToMemory(rootDir, store, cfg.Logo); err != nil {
		return err
	}
	if cfg.Icon, err = copyConfiguredSiteAssetToMemory(rootDir, store, cfg.Icon); err != nil {
		return err
	}
	return nil
}

func copyConfiguredSiteAssetToMemory(rootDir string, store *memStore, rel string) (string, error) {
	cleanRel, err := pathutil.CleanRelativePath(rel)
	if err != nil {
		return "", fmt.Errorf("site asset path not allowed %s: %w", rel, err)
	}
	if cleanRel == "" {
		return "", nil
	}
	src := filepath.Join(rootDir, "asset", cleanRel)
	info, err := os.Stat(src)
	if err != nil {
		return "", fmt.Errorf("site asset not found %s: %w", rel, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("site asset is a directory: %s", rel)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	outRel := filepath.ToSlash(filepath.Join("assets", "site", cleanRel))
	store.PutFile(outRel, data, info.ModTime())
	return outRel, nil
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

func buildBeforeServe(cfg SiteServeConfig) error {
	buildCfg := Config{
		Site:           true,
		RootDir:        cfg.RootDir,
		SiteConfig:     cfg.SiteConfig,
		ArticlesFile:   cfg.ArticlesFile,
		OutDir:         cfg.OutDir,
		LaTeXMLBin:     cfg.LaTeXMLBin,
		ArticleWorkers: cfg.ArticleWorkers,
		LaTeXMLWorkers: cfg.LaTeXMLWorkers,
		NoAssets:       cfg.NoAssets,
		Force:          cfg.Force,
		NoLaTeXMLCache: cfg.NoLaTeXMLCache,
		Log:            cfg.Out,
	}
	cfg.logf("Initial build before serve\n")
	return RunSite(buildCfg)
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
