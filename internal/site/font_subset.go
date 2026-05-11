package site

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type subsetFont struct {
	Family string
	Weight string
	Style  string
	Source string
	Output string
}

type subsetManifest struct {
	Hash    string   `json:"hash"`
	Fonts   []string `json:"fonts"`
	Charset string   `json:"charset"`
}

var subsetFonts = []subsetFont{
	{Family: "TeX Gyre Pagella", Weight: "400", Style: "normal", Source: "texgyrepagella-regular.otf", Output: "texgyrepagella-regular.subset.woff2"},
	{Family: "TeX Gyre Pagella", Weight: "700", Style: "normal", Source: "texgyrepagella-bold.otf", Output: "texgyrepagella-bold.subset.woff2"},
	{Family: "TeX Gyre Pagella", Weight: "400", Style: "italic", Source: "texgyrepagella-italic.otf", Output: "texgyrepagella-italic.subset.woff2"},
	{Family: "TeX Gyre Pagella", Weight: "700", Style: "italic", Source: "texgyrepagella-bolditalic.otf", Output: "texgyrepagella-bolditalic.subset.woff2"},
	{Family: "HarmonyOS Sans", Weight: "400", Style: "normal", Source: "HarmonyOS_Sans_SC_Regular.ttf", Output: "HarmonyOS_Sans_SC_Regular.subset.woff2"},
	{Family: "HarmonyOS Sans", Weight: "700", Style: "normal", Source: "HarmonyOS_Sans_SC_Bold.ttf", Output: "HarmonyOS_Sans_SC_Bold.subset.woff2"},
	{Family: "Source Han Serif SC", Weight: "400", Style: "normal", Source: "SourceHanSerifSC-Regular.otf", Output: "SourceHanSerifSC-Regular.subset.woff2"},
	{Family: "Source Han Sans SC", Weight: "400", Style: "normal", Source: "SourceHanSansSC-Regular.otf", Output: "SourceHanSansSC-Regular.subset.woff2"},
	{Family: "Source Han Sans SC", Weight: "700", Style: "normal", Source: "SourceHanSansSC-Bold.otf", Output: "SourceHanSansSC-Bold.subset.woff2"},
}

var htmlTagPattern = regexp.MustCompile(`(?s)<[^>]+>`)

func SubsetFontsByHTML(outDir, staticDir string) error {
	chars, err := collectHTMLCharset(outDir)
	if err != nil {
		return err
	}
	fontSrcDir := filepath.Join(staticDir, "fonts")
	fontOutDir := filepath.Join(outDir, "static", "fonts")
	if err := os.MkdirAll(fontOutDir, 0755); err != nil {
		return err
	}
	hash, err := subsetHash(chars, fontSrcDir)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(fontOutDir, "subset-manifest.json")
	manifest := subsetManifest{
		Hash:    hash,
		Fonts:   subsetOutputNames(),
		Charset: filepath.ToSlash(filepath.Join("static", "fonts", "subset-chars.txt")),
	}
	charsPath := filepath.Join(fontOutDir, "subset-chars.txt")
	if err := os.WriteFile(charsPath, []byte(chars), 0644); err != nil {
		return err
	}
	if !subsetCacheValid(manifestPath, manifest, fontOutDir) {
		if err := runFontSubset(fontSrcDir, fontOutDir, charsPath); err != nil {
			return err
		}
		if err := writeSubsetManifest(manifestPath, manifest); err != nil {
			return err
		}
	}
	if err := removeFullFontOutputs(fontOutDir); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "static", "fonts.css"), []byte(subsetFontsCSS()), 0644)
}

func collectHTMLCharset(outDir string) (string, error) {
	seen := map[rune]bool{}
	addDefaultSubsetRunes(seen)
	err := filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || strings.ToLower(filepath.Ext(path)) != ".html" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := htmlTagPattern.ReplaceAllString(string(b), " ")
		text = html.UnescapeString(text)
		for _, r := range text {
			if r != '\uFFFD' {
				seen[r] = true
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	runes := make([]rune, 0, len(seen))
	for r := range seen {
		runes = append(runes, r)
	}
	sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })
	return string(runes), nil
}

func addDefaultSubsetRunes(seen map[rune]bool) {
	for r := rune(0x20); r <= 0x7e; r++ {
		seen[r] = true
	}
	for _, r := range "\u00a0，。！？；：《》、“”‘’（）【】—…" {
		seen[r] = true
	}
}

func subsetHash(chars, fontSrcDir string) (string, error) {
	h := sha256.New()
	h.Write([]byte(chars))
	for _, font := range subsetFonts {
		src := filepath.Join(fontSrcDir, font.Source)
		info, err := os.Stat(src)
		if err != nil {
			return "", fmt.Errorf("font source not found %s: %w", src, err)
		}
		h.Write([]byte(font.Source))
		h.Write([]byte(fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano())))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func subsetOutputNames() []string {
	out := make([]string, 0, len(subsetFonts))
	for _, font := range subsetFonts {
		out = append(out, font.Output)
	}
	return out
}

func subsetCacheValid(path string, want subsetManifest, fontOutDir string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var got subsetManifest
	if err := json.Unmarshal(b, &got); err != nil {
		return false
	}
	if got.Hash != want.Hash || !sameStringSet(got.Fonts, want.Fonts) {
		return false
	}
	for _, name := range want.Fonts {
		if _, err := os.Stat(filepath.Join(fontOutDir, name)); err != nil {
			return false
		}
	}
	return true
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]string{}, a...)
	bb := append([]string{}, b...)
	sort.Strings(aa)
	sort.Strings(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}

func runFontSubset(fontSrcDir, fontOutDir, charsPath string) error {
	bin, err := exec.LookPath("pyftsubset")
	if err != nil {
		return fmt.Errorf("pyftsubset not found in PATH; install fonttools and brotli or activate the conda environment: %w", err)
	}
	for _, font := range subsetFonts {
		src := filepath.Join(fontSrcDir, font.Source)
		dst := filepath.Join(fontOutDir, font.Output)
		args := []string{
			src,
			"--text-file=" + charsPath,
			"--flavor=woff2",
			"--output-file=" + dst,
			"--layout-features=*",
			"--ignore-missing-glyphs",
			"--symbol-cmap",
			"--legacy-cmap",
			"--notdef-glyph",
			"--notdef-outline",
			"--recommended-glyphs",
		}
		cmd := exec.Command(bin, args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("pyftsubset %s: %w: %s", font.Source, err, strings.TrimSpace(stderr.String()))
		}
	}
	return nil
}

func writeSubsetManifest(path string, manifest subsetManifest) error {
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0644)
}

func removeFullFontOutputs(fontOutDir string) error {
	for _, font := range subsetFonts {
		if err := os.Remove(filepath.Join(fontOutDir, font.Source)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func subsetFontsCSS() string {
	var b strings.Builder
	for _, font := range subsetFonts {
		b.WriteString("@font-face {\n")
		b.WriteString(`  font-family: "`)
		b.WriteString(font.Family)
		b.WriteString("\";\n")
		b.WriteString(`  src: url("fonts/`)
		b.WriteString(font.Output)
		b.WriteString(`") format("woff2");`)
		b.WriteByte('\n')
		b.WriteString("  font-weight: ")
		b.WriteString(font.Weight)
		b.WriteString(";\n")
		b.WriteString("  font-style: ")
		b.WriteString(font.Style)
		b.WriteString(";\n")
		b.WriteString("  font-display: swap;\n")
		b.WriteString("}\n\n")
	}
	return b.String()
}
