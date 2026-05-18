package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteStaticWithSubsetFontsDoesNotCopyOriginalFontsCSS(t *testing.T) {
	staticDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(staticDir, "fonts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "fonts.css"), []byte(`@font-face { src: url("fonts/source.otf") format("opentype"); }`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, "fonts", "source.otf"), []byte("font"), 0644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	if err := WriteStaticWithOptions(outDir, nil, StaticOptions{
		StaticDir:     staticDir,
		SkipFontFiles: true,
	}); err != nil {
		t.Fatal(err)
	}

	css, err := os.ReadFile(filepath.Join(outDir, "static", "fonts.css"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(css)
	if strings.Contains(text, ".otf") || strings.Contains(text, ".ttf") {
		t.Fatalf("subset mode copied original fonts.css: %s", text)
	}
	if !strings.Contains(text, ".subset.woff2") {
		t.Fatalf("subset mode did not write subset fonts css: %s", text)
	}
	if _, err := os.Stat(filepath.Join(outDir, "static", "fonts", "source.otf")); !os.IsNotExist(err) {
		t.Fatalf("subset mode copied original font file, err=%v", err)
	}
}

func TestDefaultCodeBlockCSSPreservesWhitespaceAndWrapMode(t *testing.T) {
	for _, want := range []string{
		`font-family: "Source Code Pro"`,
		`.code-block-line-code`,
		`white-space: pre;`,
		`.code-block[data-wrap="true"] .code-block-line-code`,
		`white-space: pre-wrap;`,
		`overflow-wrap: anywhere;`,
		`.code-block-source`,
		`display: none;`,
	} {
		if !strings.Contains(defaultCSS, want) {
			t.Fatalf("default CSS missing %q", want)
		}
	}
}

func TestDefaultSiteLayoutSupportsStickyFooter(t *testing.T) {
	for _, want := range []string{
		`body.site-layout {`,
		`min-height: calc(100vh - 68px);`,
		`display: flex;`,
		`flex-direction: column;`,
		`body.site-layout > .site-page,`,
		`body.site-layout > .page {`,
		`flex: 1 0 auto;`,
		`.custom-page-footing {`,
		`margin: auto auto 0;`,
		`min-height: calc(100vh - 112px);`,
	} {
		if !strings.Contains(defaultCSS, want) {
			t.Fatalf("default CSS missing sticky footer rule %q", want)
		}
	}
}

func TestDefaultCSSContainsMobileOverflowIsolation(t *testing.T) {
	for _, want := range []string{
		`.complex-wrapper {`,
		`width: 100%;`,
		`overflow-x: auto;`,
		`overscroll-behavior-x: contain;`,
		`contain: layout paint;`,
		`.katex .katex-mathml {`,
		`width: 1px;`,
		`height: 1px;`,
		`overflow: hidden;`,
		`contain: strict;`,
		`clip-path: inset(50%);`,
	} {
		if !strings.Contains(defaultCSS, want) {
			t.Fatalf("default CSS missing mobile overflow isolation rule %q", want)
		}
	}
}

func TestDefaultCSSKeepsWrappedAlgorithmLinesIndented(t *testing.T) {
	for _, want := range []string{
		`--metablog-algorithm-gutter: 2.4rem;`,
		`--metablog-algorithm-indent-step: 1rem;`,
		`--metablog-algorithm-rule-gap: 0.25rem;`,
		`--metablog-algorithm-rule-offset: 0.5em;`,
		`--metablog-algorithm-rule-1: calc(var(--metablog-algorithm-gutter) - var(--metablog-algorithm-rule-gap) + var(--metablog-algorithm-rule-offset));`,
		`--metablog-algorithm-rule-6: calc(var(--metablog-algorithm-rule-5) + var(--metablog-algorithm-indent-step));`,
		`--metablog-algorithm-depth: 0;`,
		`padding-left: calc(var(--metablog-algorithm-gutter) + var(--metablog-algorithm-depth) * var(--metablog-algorithm-indent-step));`,
		`display: flex;`,
		`align-items: flex-start;`,
		`padding-left: var(--metablog-algorithm-gutter);`,
		`.metablog-latexml-fragment figure.ltx_algorithm .metablog-algorithm-io-label {`,
		`.metablog-latexml-fragment figure.ltx_algorithm .metablog-algorithm-io-content {`,
		`background-position: var(--metablog-algorithm-rule-1) -1px;`,
		`background-position: var(--metablog-algorithm-rule-1) -1px, var(--metablog-algorithm-rule-2) -1px, var(--metablog-algorithm-rule-3) -1px, var(--metablog-algorithm-rule-4) -1px, var(--metablog-algorithm-rule-5) -1px, var(--metablog-algorithm-rule-6) -1px;`,
		`.metablog-latexml-fragment figure.ltx_algorithm .metablog-algorithm-depth-1 {`,
		`--metablog-algorithm-depth: 1;`,
		`.metablog-latexml-fragment figure.ltx_algorithm .metablog-algorithm-depth-6 {`,
		`--metablog-algorithm-depth: 6;`,
		`.metablog-latexml-fragment figure.ltx_algorithm .ltx_rule {`,
		`display: none;`,
	} {
		if !strings.Contains(defaultCSS, want) {
			t.Fatalf("default CSS missing algorithm wrap indentation rule %q", want)
		}
	}
}

func TestDefaultFontsCSSIncludesSourceCodePro(t *testing.T) {
	for _, want := range []string{
		`font-family: "Source Code Pro"`,
		`source-code-pro-regular.woff2`,
		`source-code-pro-bold.woff2`,
	} {
		if !strings.Contains(defaultFontsCSS, want) {
			t.Fatalf("default fonts CSS missing %q", want)
		}
	}
}
