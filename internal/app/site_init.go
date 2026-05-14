package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"MetaBlog/internal/highlight"
	"MetaBlog/internal/latexml"
)

type SiteInitConfig struct {
	RootDir      string
	Title        string
	SkipFonts    bool
	SkipEnvCheck bool
	LaTeXMLBin   string
	Out          io.Writer
	HTTPClient   *http.Client
	FontFiles    []FontDownload
}

type FontDownload struct {
	Name string
	URL  string
}

var defaultSiteFontDownloads = []FontDownload{
	{Name: "texgyrepagella-regular.otf", URL: "https://mirrors.ctan.org/fonts/tex-gyre/opentype/texgyrepagella-regular.otf"},
	{Name: "texgyrepagella-bold.otf", URL: "https://mirrors.ctan.org/fonts/tex-gyre/opentype/texgyrepagella-bold.otf"},
	{Name: "texgyrepagella-italic.otf", URL: "https://mirrors.ctan.org/fonts/tex-gyre/opentype/texgyrepagella-italic.otf"},
	{Name: "texgyrepagella-bolditalic.otf", URL: "https://mirrors.ctan.org/fonts/tex-gyre/opentype/texgyrepagella-bolditalic.otf"},
	{Name: "HarmonyOS_Sans_SC_Regular.ttf", URL: "https://raw.githubusercontent.com/ajacocks/harmonyos-sans-font/main/HarmonyOS_Sans_SC/HarmonyOS_Sans_SC_Regular.ttf"},
	{Name: "HarmonyOS_Sans_SC_Bold.ttf", URL: "https://raw.githubusercontent.com/ajacocks/harmonyos-sans-font/main/HarmonyOS_Sans_SC/HarmonyOS_Sans_SC_Bold.ttf"},
	{Name: "SourceHanSerifSC-Regular.otf", URL: "https://raw.githubusercontent.com/adobe-fonts/source-han-serif/release/OTF/SimplifiedChinese/SourceHanSerifSC-Regular.otf"},
	{Name: "SourceHanSansSC-Regular.otf", URL: "https://raw.githubusercontent.com/adobe-fonts/source-han-sans/release/OTF/SimplifiedChinese/SourceHanSansSC-Regular.otf"},
	{Name: "SourceHanSansSC-Bold.otf", URL: "https://raw.githubusercontent.com/adobe-fonts/source-han-sans/release/OTF/SimplifiedChinese/SourceHanSansSC-Bold.otf"},
	{Name: "source-code-pro-regular.woff2", URL: "https://cdn.jsdelivr.net/npm/@fontsource/source-code-pro@5.0.18/files/source-code-pro-latin-400-normal.woff2"},
	{Name: "source-code-pro-bold.woff2", URL: "https://cdn.jsdelivr.net/npm/@fontsource/source-code-pro@5.0.18/files/source-code-pro-latin-700-normal.woff2"},
}

func RunSiteInit(cfg SiteInitConfig) error {
	cfg = normalizeSiteInitConfig(cfg)
	root, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return err
	}
	cfg.RootDir = root
	cfg.logf("MetaBlog site init\n")
	cfg.logf("Root: %s\n", filepath.ToSlash(root))

	for _, dir := range []string{
		"articles",
		"asset/figs",
		"data/about_page",
		"web/static/fonts",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0755); err != nil {
			return err
		}
		cfg.logf("Directory ready: %s\n", dir)
	}

	files := []initFile{
		{Path: "data/config.toml", Content: defaultConfigTOML(cfg.Title)},
		{Path: "data/articles.toml", Content: defaultArticlesTOML},
		{Path: "data/about_page/main.tex", Content: defaultAboutTex},
		{Path: "asset/figs/circle_example.svg", Content: defaultCircleExampleSVG},
		{Path: "web/static/fonts.css", Content: defaultInitFontsCSS},
		{Path: "web/static/chroma-theme.css", Content: highlight.ThemeCSS()},
	}
	for _, file := range files {
		if err := writeInitFile(root, file, cfg.Out); err != nil {
			return err
		}
	}
	if err := ensureGitignore(root, cfg.Out); err != nil {
		return err
	}

	if cfg.SkipFonts {
		cfg.logf("Fonts: skipped by -skip-fonts\n")
	} else if err := downloadSiteFonts(cfg); err != nil {
		return err
	}

	if cfg.SkipEnvCheck {
		cfg.logf("Environment check: skipped by -skip-env-check\n")
	} else {
		checkSiteEnvironment(cfg)
	}

	cfg.logf("Site initialized: %s\n", filepath.ToSlash(root))
	return nil
}

type initFile struct {
	Path    string
	Content string
}

func normalizeSiteInitConfig(cfg SiteInitConfig) SiteInitConfig {
	if cfg.RootDir == "" {
		cfg.RootDir = "."
	}
	if strings.TrimSpace(cfg.Title) == "" {
		cfg.Title = "MetaBlog"
	}
	if cfg.Out == nil {
		cfg.Out = io.Discard
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 5 * time.Minute}
	}
	if len(cfg.FontFiles) == 0 {
		cfg.FontFiles = defaultSiteFontDownloads
	}
	return cfg
}

func writeInitFile(root string, file initFile, out io.Writer) error {
	path := filepath.Join(root, filepath.FromSlash(file.Path))
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(out, "File exists, skipped: %s\n", file.Path)
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(file.Content), 0644); err != nil {
		return err
	}
	fmt.Fprintf(out, "File created: %s\n", file.Path)
	return nil
}

func ensureGitignore(root string, out io.Writer) error {
	path := filepath.Join(root, ".gitignore")
	lines := []string{"out/", ".metablog-cache/", ".gocache/", ".gomodcache/"}
	existing := ""
	if b, err := os.ReadFile(path); err == nil {
		existing = string(b)
	} else if !os.IsNotExist(err) {
		return err
	}
	var additions []string
	for _, line := range lines {
		if !gitignoreContains(existing, line) {
			additions = append(additions, line)
		}
	}
	if len(additions) == 0 {
		fmt.Fprintln(out, "File exists, skipped: .gitignore")
		return nil
	}
	var b strings.Builder
	b.WriteString(existing)
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		b.WriteString("\n")
	}
	for _, line := range additions {
		b.WriteString(line)
		b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		return err
	}
	if existing == "" {
		fmt.Fprintln(out, "File created: .gitignore")
	} else {
		fmt.Fprintln(out, "File updated: .gitignore")
	}
	return nil
}

func gitignoreContains(text, line string) bool {
	for _, existing := range strings.Split(text, "\n") {
		if strings.TrimSpace(existing) == line {
			return true
		}
	}
	return false
}

func downloadSiteFonts(cfg SiteInitConfig) error {
	fontDir := filepath.Join(cfg.RootDir, "web", "static", "fonts")
	_ = cleanupDownloadTemps(fontDir)
	for _, font := range cfg.FontFiles {
		if strings.TrimSpace(font.Name) == "" || strings.TrimSpace(font.URL) == "" {
			return errors.New("font download entry requires name and URL")
		}
		dst := filepath.Join(fontDir, filepath.Base(font.Name))
		if info, err := os.Stat(dst); err == nil && info.Size() > 0 {
			cfg.logf("Font exists, skipped: %s\n", font.Name)
			continue
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		cfg.logf("Font downloading: %s\n", font.Name)
		if err := downloadFile(cfg.HTTPClient, font.URL, dst); err != nil {
			return fmt.Errorf("download font %s: %w", font.Name, err)
		}
		cfg.logf("Font downloaded: %s\n", font.Name)
	}
	return nil
}

func downloadFile(client *http.Client, url, dst string) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*attempt) * 200 * time.Millisecond)
		}
		if err := downloadFileOnce(client, url, dst); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

func downloadFileOnce(client *http.Client, url, dst string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".*.download")
	if err != nil {
		return err
	}
	tmp := out.Name()
	_, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	info, err := os.Stat(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if info.Size() == 0 {
		_ = os.Remove(tmp)
		return errors.New("empty response body")
	}
	_ = os.Remove(dst)
	return os.Rename(tmp, dst)
}

func cleanupDownloadTemps(dir string) error {
	matches, err := filepath.Glob(filepath.Join(dir, "*.download"))
	if err != nil {
		return err
	}
	for _, match := range matches {
		_ = os.Remove(match)
	}
	return nil
}

func checkSiteEnvironment(cfg SiteInitConfig) {
	cfg.logf("Environment check:\n")
	checkCommand(cfg, "latexmlc", func() (string, bool) {
		id := &latexml.CacheIdentity{}
		runner := latexml.Runner{Bin: cfg.LaTeXMLBin, Identity: id}
		bin, version, ok := runner.PrepareCacheIdentity()
		if !ok {
			return "latexmlc not found or version check failed. Install: " + lateXMLInstallHint(), false
		}
		return filepath.ToSlash(bin) + " " + version, true
	})
	checkCommand(cfg, "pyftsubset", func() (string, bool) {
		bin, err := exec.LookPath("pyftsubset")
		if err != nil {
			return "pyftsubset not found; required for -subset-fonts. Install: " + fontToolsInstallHint(), false
		}
		return filepath.ToSlash(bin), true
	})
	checkCommand(cfg, "python packages", checkPythonPackages)
	checkCommand(cfg, "PDF converter", checkPDFConverter)
}

func checkCommand(cfg SiteInitConfig, name string, fn func() (string, bool)) {
	msg, ok := fn()
	status := "WARN"
	if ok {
		status = "OK"
	}
	cfg.logf("  [%s] %s: %s\n", status, name, msg)
}

func checkPythonPackages() (string, bool) {
	python, args, ok := pythonCommand()
	if !ok {
		return "python not found; fontTools and brotli cannot be checked. Install: " + pythonInstallHint(), false
	}
	args = append(args, "-c", "import fontTools, brotli")
	cmd := exec.Command(python, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return msg + ". Install: " + fontToolsInstallHint(), false
	}
	return python + " imports fontTools and brotli", true
}

func pythonCommand() (string, []string, bool) {
	if bin, err := exec.LookPath("python"); err == nil {
		return bin, nil, true
	}
	if bin, err := exec.LookPath("python3"); err == nil {
		return bin, nil, true
	}
	if bin, err := exec.LookPath("py"); err == nil {
		return bin, []string{"-3"}, true
	}
	return "", nil, false
}

func checkPDFConverter() (string, bool) {
	for _, name := range []string{"pdftocairo", "mutool", "inkscape"} {
		if bin, err := exec.LookPath(name); err == nil {
			return name + " at " + filepath.ToSlash(bin), true
		}
	}
	return "none found; PDF figures cannot be converted to SVG. Install: " + pdfConverterInstallHint(), false
}

func lateXMLInstallHint() string {
	switch runtime.GOOS {
	case "windows":
		return "install Strawberry Perl, then run `cpan LaTeXML` or `cpanm LaTeXML`, and ensure latexmlc.bat is in PATH"
	case "darwin":
		return "`brew install latexml`"
	default:
		return "`sudo apt install latexml` on Debian/Ubuntu, or install LaTeXML from your distribution package manager"
	}
}

func pythonInstallHint() string {
	switch runtime.GOOS {
	case "windows":
		return "install Python 3 from python.org or Conda, and ensure python is in PATH"
	case "darwin":
		return "`brew install python`"
	default:
		return "`sudo apt install python3 python3-pip` on Debian/Ubuntu"
	}
}

func fontToolsInstallHint() string {
	switch runtime.GOOS {
	case "windows":
		return "`python -m pip install fonttools brotli`; if using Conda, activate the environment first"
	default:
		return "`python3 -m pip install fonttools brotli`"
	}
}

func pdfConverterInstallHint() string {
	switch runtime.GOOS {
	case "windows":
		return "install Poppler for Windows and add its bin directory to PATH; alternatively install MuPDF or Inkscape"
	case "darwin":
		return "`brew install poppler`; alternatively `brew install mupdf` or `brew install --cask inkscape`"
	default:
		return "`sudo apt install poppler-utils`; alternatively install mupdf-tools or inkscape"
	}
}

func (cfg SiteInitConfig) logf(format string, args ...any) {
	if cfg.Out != nil {
		fmt.Fprintf(cfg.Out, format, args...)
	}
}

func defaultConfigTOML(title string) string {
	return fmt.Sprintf("title = %q\nlogo = \"figs/circle_example.svg\"\nicon = \"figs/circle_example.svg\"\nhome_page_size = 10\narticle_list_page_size = 20\n", title)
}

const defaultArticlesTOML = `# Add articles with:
# metablog article init -root .
`

const defaultAboutTex = `\begin{document}

\section{About}
Write your biography here.

\end{document}
`

const defaultInitFontsCSS = `@font-face {
  font-family: "TeX Gyre Pagella";
  src: url("fonts/texgyrepagella-regular.otf") format("opentype");
  font-weight: 400;
  font-style: normal;
  font-display: swap;
}

@font-face {
  font-family: "TeX Gyre Pagella";
  src: url("fonts/texgyrepagella-bold.otf") format("opentype");
  font-weight: 700;
  font-style: normal;
  font-display: swap;
}

@font-face {
  font-family: "TeX Gyre Pagella";
  src: url("fonts/texgyrepagella-italic.otf") format("opentype");
  font-weight: 400;
  font-style: italic;
  font-display: swap;
}

@font-face {
  font-family: "TeX Gyre Pagella";
  src: url("fonts/texgyrepagella-bolditalic.otf") format("opentype");
  font-weight: 700;
  font-style: italic;
  font-display: swap;
}

@font-face {
  font-family: "HarmonyOS Sans";
  src: url("fonts/HarmonyOS_Sans_SC_Regular.ttf") format("truetype");
  font-weight: 400;
  font-style: normal;
  font-display: swap;
}

@font-face {
  font-family: "HarmonyOS Sans";
  src: url("fonts/HarmonyOS_Sans_SC_Bold.ttf") format("truetype");
  font-weight: 700;
  font-style: normal;
  font-display: swap;
}

@font-face {
  font-family: "Source Han Serif SC";
  src: url("fonts/SourceHanSerifSC-Regular.otf") format("opentype");
  font-weight: 400;
  font-style: normal;
  font-display: swap;
}

@font-face {
  font-family: "Source Han Sans SC";
  src: url("fonts/SourceHanSansSC-Regular.otf") format("opentype");
  font-weight: 400;
  font-style: normal;
  font-display: swap;
}

@font-face {
  font-family: "Source Han Sans SC";
  src: url("fonts/SourceHanSansSC-Bold.otf") format("opentype");
  font-weight: 700;
  font-style: normal;
  font-display: swap;
}

@font-face {
  font-family: "Source Code Pro";
  src: url("fonts/source-code-pro-regular.woff2") format("woff2");
  font-weight: 400;
  font-style: normal;
  font-display: swap;
}

@font-face {
  font-family: "Source Code Pro";
  src: url("fonts/source-code-pro-bold.woff2") format("woff2");
  font-weight: 700;
  font-style: normal;
  font-display: swap;
}
`

const defaultCircleExampleSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128">
  <circle cx="64" cy="64" r="56" fill="#000"/>
</svg>
`
