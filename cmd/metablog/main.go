package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"MetaBlog/internal/app"
)

func main() {
	var err error
	if len(os.Args) >= 2 && !strings.HasPrefix(os.Args[1], "-") {
		err = runTopLevelCommand(os.Args[1], os.Args[2:])
	} else {
		err = runLegacyBuild(os.Args[1:])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "metablog: %v\n", err)
		os.Exit(1)
	}
}

func runTopLevelCommand(name string, args []string) error {
	switch name {
	case "site":
		if len(args) == 0 {
			return fmt.Errorf("missing site command; use init, build or serve")
		}
		return runSiteCommand(args[0], args[1:])
	case "article":
		if len(args) == 0 {
			return fmt.Errorf("missing article command; use build, init, edit or delete")
		}
		return runArticleCommand(args[0], args[1:])
	case "cache":
		if len(args) == 0 {
			return fmt.Errorf("missing cache command; use clean")
		}
		return runCacheCommand(args[0], args[1:])
	default:
		return fmt.Errorf("unknown command %q; use site, article or cache", name)
	}
}

func runLegacyBuild(args []string) error {
	var cfg app.Config
	fs := flag.NewFlagSet("metablog", flag.ExitOnError)
	registerBuildFlags(fs, &cfg, true)
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg.Log = os.Stderr
	return app.Run(cfg)
}

func runSiteCommand(name string, args []string) error {
	switch name {
	case "init":
		var cfg app.SiteInitConfig
		fs := flag.NewFlagSet("metablog site init", flag.ExitOnError)
		fs.StringVar(&cfg.RootDir, "root", ".", "site root directory")
		fs.StringVar(&cfg.Title, "title", "MetaBlog", "site title")
		fs.StringVar(&cfg.LaTeXMLBin, "latexml-bin", "", "latexmlc executable path for environment check")
		fs.BoolVar(&cfg.SkipFonts, "skip-fonts", false, "skip downloading web font files")
		fs.BoolVar(&cfg.SkipEnvCheck, "skip-env-check", false, "skip Python, LaTeXML and converter checks")
		if err := fs.Parse(args); err != nil {
			return err
		}
		cfg.Out = os.Stdout
		return app.RunSiteInit(cfg)
	case "build":
		var cfg app.Config
		cfg.Site = true
		fs := flag.NewFlagSet("metablog site build", flag.ExitOnError)
		registerBuildFlags(fs, &cfg, false)
		if err := fs.Parse(args); err != nil {
			return err
		}
		cfg.Site = true
		cfg.Log = os.Stderr
		return app.Run(cfg)
	case "serve":
		var cfg app.SiteServeConfig
		fs := flag.NewFlagSet("metablog site serve", flag.ExitOnError)
		fs.StringVar(&cfg.OutDir, "out", "out", "static site output directory")
		fs.StringVar(&cfg.Host, "host", "127.0.0.1", "host address to listen on")
		fs.IntVar(&cfg.Port, "port", 0, "port to listen on; 0 chooses a random free port")
		fs.BoolVar(&cfg.Watch, "watch", false, "watch source files and hot-rebuild changed articles")
		fs.StringVar(&cfg.RootDir, "root", ".", "project root directory (required for -watch)")
		fs.StringVar(&cfg.SiteConfig, "config", "data/config.toml", "site config TOML (for -watch)")
		fs.StringVar(&cfg.ArticlesFile, "articles", "data/articles.toml", "article metadata TOML (for -watch)")
		fs.StringVar(&cfg.LaTeXMLBin, "latexml-bin", "", "latexmlc executable path (for -watch)")
		fs.IntVar(&cfg.ArticleWorkers, "article-workers", 0, "parallel article workers (for -watch); 0=auto")
		fs.IntVar(&cfg.LaTeXMLWorkers, "latexml-workers", 0, "parallel LaTeXML workers (for -watch); 0=auto")
		fs.BoolVar(&cfg.NoAssets, "no-assets", false, "skip asset conversion during watch rebuild")
		if err := fs.Parse(args); err != nil {
			return err
		}
		cfg.Out = os.Stdout
		return app.RunSiteServe(cfg)
	default:
		return fmt.Errorf("unknown site command %q; use init, build or serve", name)
	}
}

func runCacheCommand(name string, args []string) error {
	fs := flag.NewFlagSet("metablog cache "+name, flag.ExitOnError)
	var cfg app.CacheCLIConfig
	fs.StringVar(&cfg.RootDir, "root", ".", "project root")
	cfg.Out = os.Stdout
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch name {
	case "clean":
		return app.RunCacheClean(cfg)
	default:
		return fmt.Errorf("unknown cache command %q; use clean", name)
	}
}

func runArticleCommand(name string, args []string) error {
	if name == "build" {
		var cfg app.Config
		fs := flag.NewFlagSet("metablog article build", flag.ExitOnError)
		registerBuildFlags(fs, &cfg, false)
		if err := fs.Parse(args); err != nil {
			return err
		}
		cfg.Site = false
		cfg.Log = os.Stderr
		return app.Run(cfg)
	}

	fs := flag.NewFlagSet("metablog article "+name, flag.ExitOnError)
	var cfg app.ArticleCLIConfig
	fs.StringVar(&cfg.RootDir, "root", ".", "project root")
	fs.StringVar(&cfg.ArticlesFile, "articles", "data/articles.toml", "article metadata TOML")
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch name {
	case "init":
		return app.RunArticleInit(cfg)
	case "edit":
		return app.RunArticleEdit(cfg)
	case "delete":
		return app.RunArticleDelete(cfg)
	default:
		return fmt.Errorf("unknown article command %q; use build, init, edit or delete", name)
	}
}

func registerBuildFlags(fs *flag.FlagSet, cfg *app.Config, includeSiteFlag bool) {
	fs.StringVar(&cfg.Input, "input", "sample_latex/DACE-with_supplementary.tex", "main LaTeX file")
	fs.StringVar(&cfg.OutDir, "out", "out", "output directory")
	if includeSiteFlag {
		fs.BoolVar(&cfg.Site, "site", false, "build the full blog site from data files")
	}
	fs.StringVar(&cfg.RootDir, "root", ".", "project root")
	fs.StringVar(&cfg.SiteConfig, "config", "data/config.toml", "site config TOML")
	fs.StringVar(&cfg.ArticlesFile, "articles", "data/articles.toml", "article metadata TOML")
	fs.StringVar(&cfg.LaTeXMLBin, "latexml-bin", "", "latexmlc executable path")
	fs.BoolVar(&cfg.DumpAST, "dump-ast", false, "write debug AST JSON")
	fs.BoolVar(&cfg.KeepTemp, "keep-temp", false, "keep temporary latexml files")
	fs.BoolVar(&cfg.Strict, "strict", false, "fail on parser warnings")
	fs.BoolVar(&cfg.NoAssets, "no-assets", false, "skip asset conversion")
	fs.BoolVar(&cfg.Force, "force", false, "force rebuild site documents even when outputs are fresh")
	fs.BoolVar(&cfg.NoLaTeXMLCache, "no-latexml-cache", false, "ignore LaTeXML complex block cache")
	fs.BoolVar(&cfg.SubsetFonts, "subset-fonts", false, "subset web fonts by generated site HTML content")
	fs.IntVar(&cfg.ArticleWorkers, "article-workers", 0, "parallel article workers for site mode; 0 chooses a conservative default")
	fs.IntVar(&cfg.LaTeXMLWorkers, "latexml-workers", 0, "parallel LaTeXML workers; 0 chooses a conservative default")
}
