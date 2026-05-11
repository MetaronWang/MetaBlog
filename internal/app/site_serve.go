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
)

type SiteServeConfig struct {
	OutDir   string
	Host     string
	Port     int
	Out      io.Writer
	Listener net.Listener
	Stop     <-chan struct{}
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
	if cfg.Stop != nil {
		go func() {
			<-cfg.Stop
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = server.Shutdown(ctx)
		}()
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
