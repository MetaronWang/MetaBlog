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
