package site

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"MetaBlog/internal/highlight"
)

type StaticOptions struct {
	StaticDir     string
	SkipFontFiles bool
}

func Write(outDir, htmlText string, warnings []string) error {
	if err := WriteStatic(outDir, warnings); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "index.html"), []byte(htmlText), 0644)
}

func WriteStatic(outDir string, warnings []string) error {
	return WriteStaticFrom(outDir, warnings, filepath.Join("web", "static"))
}

func WriteStaticFrom(outDir string, warnings []string, staticDir string) error {
	return WriteStaticWithOptions(outDir, warnings, StaticOptions{StaticDir: staticDir})
}

func WriteStaticWithOptions(outDir string, warnings []string, opts StaticOptions) error {
	staticDir := opts.StaticDir
	if staticDir == "" {
		staticDir = filepath.Join("web", "static")
	}
	if err := os.MkdirAll(filepath.Join(outDir, "static"), 0755); err != nil {
		return err
	}
	if err := copyProjectStatic(staticDir, filepath.Join(outDir, "static"), opts.SkipFontFiles); err != nil {
		return err
	}
	fontsCSSPath := filepath.Join(outDir, "static", "fonts.css")
	if opts.SkipFontFiles {
		if err := os.WriteFile(fontsCSSPath, []byte(subsetFontsCSS()), 0644); err != nil {
			return err
		}
	} else if err := ensureFontsCSS(fontsCSSPath); err != nil {
		return err
	}
	chromaCSSPath := filepath.Join(outDir, "static", "chroma-theme.css")
	if _, err := os.Stat(chromaCSSPath); os.IsNotExist(err) {
		if err := os.WriteFile(chromaCSSPath, []byte(highlight.ThemeCSS()), 0644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(outDir, "static", "style.css"), []byte(defaultCSS), 0644); err != nil {
		return err
	}
	if len(warnings) > 0 {
		if err := os.MkdirAll(filepath.Join(outDir, "debug"), 0755); err != nil {
			return err
		}
		text := ""
		for _, w := range warnings {
			text += w + "\n"
		}
		if err := os.WriteFile(filepath.Join(outDir, "debug", "warnings.txt"), []byte(text), 0644); err != nil {
			return err
		}
	} else {
		if err := os.Remove(filepath.Join(outDir, "debug", "warnings.txt")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func copyProjectStatic(srcDir, dstDir string, skipFontFiles bool) error {
	info, err := os.Stat(srcDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return nil
	}
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if skipFontFiles && isFontAsset(rel) {
			return nil
		}
		dst := filepath.Join(dstDir, rel)
		if info.IsDir() {
			return os.MkdirAll(dst, 0755)
		}
		return copyFile(path, dst, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func isFontFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".otf", ".ttf", ".woff", ".woff2", ".eot":
		return true
	default:
		return false
	}
}

func isFontAsset(path string) bool {
	if isFontFile(path) {
		return true
	}
	return filepath.ToSlash(path) == "fonts.css"
}

func ensureFontsCSS(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(defaultFontsCSS), 0644)
}

const defaultFontsCSS = `/*
Place project-supplied web fonts and their @font-face rules here.
This file is copied from web/static/fonts.css when present.
*/

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

const defaultCSS = `:root {
  color-scheme: light;
  --text: #1f2933;
  --muted: #5f6b7a;
  --line: #d8dee8;
  --panel: #f6f8fb;
  --accent: #256d85;
}

* { box-sizing: border-box; }
html,
body {
  width: 100%;
  max-width: 100%;
  overflow-x: hidden;
}
body {
  margin: 0;
  font-family: "TeX Gyre Pagella", "Source Han Serif SC", "Noto Serif CJK SC", "Source Han Serif CN", "Noto Serif SC", "Songti SC", SimSun, Georgia, serif;
  color: var(--text);
  background: #ffffff;
  line-height: 1.68;
}
body.site-layout {
  padding-top: 68px;
}
.site-topbar {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  z-index: 1200;
  height: 68px;
  border-bottom: 1px solid var(--line);
  background: rgba(255, 255, 255, 0.97);
  backdrop-filter: blur(10px);
  font-family: "HarmonyOS Sans", "HarmonyOS Sans SC", "Source Han Sans SC", "Noto Sans CJK SC", "Microsoft YaHei", Arial, sans-serif;
}
.site-topbar-inner {
  display: flex;
  width: 100%;
  max-width: 1180px;
  height: 100%;
  margin: 0 auto;
  padding: 0 24px;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
}
.site-brand {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
  color: var(--text);
  font-weight: 700;
  text-decoration: none;
}
.site-logo {
  width: 32px;
  height: 32px;
  object-fit: contain;
}
.site-title {
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 1.2em;
  white-space: nowrap;
}
.site-nav {
  display: flex;
  align-items: center;
  gap: 18px;
  font-size: 0.95rem;
}
.site-nav a {
  color: var(--muted);
  text-decoration: none;
  white-space: nowrap;
}
.site-nav a:hover {
  color: var(--accent);
}
.site-page {
  width: 100%;
  max-width: 980px;
  margin: 0 auto;
  padding: 54px 24px 72px;
}
.site-page h1 {
  margin-bottom: 28px;
}
.home-article-feed {
  display: grid;
  gap: 22px;
}
.home-empty {
  color: var(--muted);
}
.home-article-card {
  overflow: hidden;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: #fff;
}
.home-article-figure {
  display: block;
  width: 100%;
  aspect-ratio: 16 / 7;
  border-bottom: 1px solid var(--line);
  background: transparent;
}
.home-article-figure img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.home-article-body {
  padding: 18px 20px 16px;
}
.home-article-title {
  margin: 0 0 8px;
  font-size: 1.32rem;
  line-height: 1.28;
}
.home-article-title a {
  color: var(--text);
  text-decoration: none;
}
.home-article-title a:hover {
  color: var(--accent);
}
.home-article-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 14px;
  align-items: center;
  margin-bottom: 12px;
  color: var(--muted);
  font-size: 0.92rem;
}
.home-article-categories {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 5px;
  align-items: center;
}
.home-article-categories a {
  color: var(--muted);
  text-decoration: none;
}
.home-article-categories a:hover {
  color: var(--accent);
}
.category-separator {
  color: #b7c0ca;
}
.home-article-description {
  margin: 0 0 16px;
  color: var(--text);
  line-height: 1.62;
  white-space: pre-line;
}
.home-article-tags {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  align-items: center;
}
.tag-icon {
  display: inline-flex;
  flex: 0 0 auto;
  width: 20px;
  height: 20px;
  color: var(--muted);
}
.tag-icon svg {
  display: block;
  width: 100%;
  height: 100%;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.9;
  stroke-linecap: round;
  stroke-linejoin: round;
}
.tag-links {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}
.tag-links a {
  border: 1px solid var(--line);
  border-radius: 999px;
  padding: 3px 9px;
  color: var(--muted);
  font-size: 0.86rem;
  text-decoration: none;
}
.tag-links a:hover {
  border-color: var(--accent);
  color: var(--accent);
}
.pagination {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 10px;
  align-items: center;
  margin: 28px 0 0;
  font-family: "HarmonyOS Sans", "HarmonyOS Sans SC", "Source Han Sans SC", "Noto Sans CJK SC", "Microsoft YaHei", Arial, sans-serif;
}
.pagination ol {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  margin: 0;
  padding: 0;
  list-style: none;
}
.pagination a,
.pagination span {
  display: inline-flex;
  min-width: 34px;
  height: 34px;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 0 10px;
  color: var(--muted);
  font-size: 0.9rem;
  text-decoration: none;
}
.pagination a:hover {
  border-color: var(--accent);
  color: var(--accent);
}
.pagination span[aria-current="page"] {
  border-color: var(--accent);
  background: var(--accent);
  color: #fff;
}
.article-year {
  margin: 30px 0 10px;
  border-bottom: 1px solid var(--line);
  padding-bottom: 6px;
}
.article-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.article-list li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  border-bottom: 1px solid #edf1f5;
  padding: 10px 0;
}
.article-thumb {
  flex: 0 0 92px;
  width: 92px;
  height: 58px;
  overflow: hidden;
  border: 1px solid var(--line);
  border-radius: 6px;
  background: transparent;
}
.article-thumb img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.article-list-main {
  flex: 1 1 auto;
  min-width: 0;
}
.article-list a {
  color: var(--text);
  text-decoration: none;
}
.article-list a:hover {
  color: var(--accent);
}
.article-date {
  flex: 0 0 auto;
  color: var(--muted);
  font-size: 0.92rem;
}
.term-cloud {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.term-cloud a {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
  border-bottom: 1px solid var(--line);
  color: var(--text);
  text-decoration: none;
}
.term-cloud sup {
  color: var(--muted);
  font-size: 0.74em;
}
.category-tree,
.category-tree ol {
  list-style: none;
  margin: 0;
  padding-left: 20px;
}
.category-tree > li {
  margin: 6px 0;
}
.category-tree details {
  margin: 4px 0;
}
.category-tree summary {
  cursor: pointer;
}
.category-tree a {
  color: var(--text);
  text-decoration: none;
}
.category-tree a:hover {
  color: var(--accent);
}
.page {
  width: 100%;
  max-width: 980px;
  margin: 0 auto;
  padding: 40px 24px 72px;
  min-width: 0;
}
.article {
  min-width: 0;
  max-width: 100%;
}
.article-header {
  border-bottom: 1px solid var(--line);
  margin-bottom: 28px;
  padding-bottom: 22px;
}
h1 {
  font-size: 2.1rem;
  line-height: 1.16;
  margin: 0 0 16px;
  letter-spacing: 0;
}
h2, h3, h4, h5, h6 {
  line-height: 1.28;
  margin: 32px 0 12px;
  letter-spacing: 0;
}
.section > h2,
.section > h3,
.section > h4,
.section > h5,
.section > h6 {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr);
  column-gap: 0.42em;
  align-items: baseline;
  overflow-wrap: normal;
}
strong,
b,
h1,
h2,
h3,
h4,
h5,
h6,
th,
.ltx_font_bold {
  font-family: "TeX Gyre Pagella", "Source Han Sans SC", "Noto Sans CJK SC", "Source Han Sans CN", "Microsoft YaHei", SimHei, sans-serif;
  font-weight: 700;
}
p { margin: 0 0 16px; }
.article-meta {
  margin: 0 0 22px;
  color: var(--text);
  font-size: 0.96rem;
  line-height: 1.45;
  text-align: center;
}
.author-list {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  align-items: flex-start;
  gap: 14px 28px;
  margin: 0 0 16px;
}
.author-card {
  display: flex;
  min-width: 150px;
  max-width: 240px;
  flex-direction: column;
  align-items: center;
  text-align: center;
}
.author-card .author-name {
  white-space: nowrap;
}
.institution-ref {
  margin-left: 1px;
  color: var(--muted);
  font-size: 0.72em;
}
.author-email {
  display: block;
  max-width: 100%;
  color: var(--muted);
  font-size: 0.9rem;
  overflow-wrap: anywhere;
}
.author-email:hover {
  text-decoration: underline;
}
.institution-list {
  display: inline-block;
  max-width: 860px;
  margin: 0 auto 14px;
  padding-left: 26px;
  color: var(--muted);
  font-size: 0.91rem;
  text-align: left;
}
.institution-list li {
  margin: 0 0 4px;
  padding-left: 4px;
}
.author-attributes {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 4px 10px;
  max-width: 860px;
  margin: 0 auto;
  color: var(--muted);
  font-size: 0.9rem;
  text-align: left;
}
.author-attributes dt {
  font-weight: 700;
}
.author-attributes dd {
  margin: 0;
}
.abstract {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 16px 18px;
  margin: 18px 0;
}
.abstract h2, .toc h2 {
  font-size: 1rem;
  margin: 0 0 8px;
}
.tcb {
  display: block;
  margin: 18px 0;
  border-left: 5px solid var(--tcb-border);
  background: var(--tcb-body-bg);
}
.tcb,
.tcb-title,
.tcb-body {
  border-radius: 0;
}
.tcb-title {
  display: flex;
  min-height: 2.25rem;
  align-items: center;
  gap: 10px;
  margin: 0;
  padding: 8px 12px;
  background: var(--tcb-title-bg);
  color: var(--tcb-title-color);
  cursor: pointer;
  font-family: "HarmonyOS Sans", "Source Han Sans SC", "Noto Sans CJK SC", "Microsoft YaHei", SimHei, sans-serif;
  font-weight: 700;
  line-height: 1.35;
  list-style: none;
}
.tcb-title::-webkit-details-marker {
  display: none;
}
.tcb-title-text {
  flex: 1 1 auto;
  min-width: 0;
  text-align: var(--tcb-title-align);
  overflow-wrap: anywhere;
}
.tcb-toggle {
  display: inline-flex;
  flex: 0 0 auto;
  width: 1.35rem;
  height: 1.35rem;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 0;
  background: transparent;
  color: var(--tcb-title-color);
}
.tcb-toggle::before {
  content: "";
  width: 0.46rem;
  height: 0.46rem;
  border-right: 2px solid currentColor;
  border-bottom: 2px solid currentColor;
  transform: rotate(45deg) translate(-1px, -1px);
}
.tcb:not([open]) .tcb-toggle::before {
  transform: rotate(-45deg) translate(-1px, 1px);
}
.tcb-body {
  margin: 0;
  padding: 12px 14px;
  background: var(--tcb-body-bg);
}
.tcb-body > :last-child {
  margin-bottom: 0;
}
.keywords {
  color: var(--muted);
  font-size: .95rem;
}
.toc {
  border-bottom: 1px solid var(--line);
  margin-bottom: 28px;
  padding-bottom: 18px;
  overflow-x: hidden;
  --toc-marker-width: 16px;
  --toc-marker-gap: 6px;
  --toc-tree-step: 10px;
  --toc-child-gap: 2px;
}
.toc-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.toc-toggle {
  display: none;
  align-items: center;
  justify-content: center;
  min-width: 36px;
  height: 30px;
  padding: 0 8px;
  border: 1px solid var(--line);
  border-radius: 6px;
  background: #fff;
  color: var(--muted);
  font: inherit;
  font-size: 0.82rem;
  line-height: 1;
  cursor: pointer;
}
.toc-toggle:hover,
.toc-toggle:focus-visible {
  color: var(--accent);
  border-color: var(--accent);
  outline: 0;
}
.toc-toggle-closed {
  display: none;
}
.toc-toggle-closed svg {
  display: block;
  width: 22px;
  height: 42px;
  fill: none;
  stroke: currentColor;
  stroke-width: 2.2;
  stroke-linecap: round;
  stroke-linejoin: round;
}
.toc ol,
.toc details {
  min-width: 0;
}
.toc ol {
  list-style: none;
  padding: 0;
  margin: 0;
}
.toc li {
  break-inside: avoid;
  margin: 3px 0 3px var(--toc-missing-indent, 0);
}
.toc details {
  display: block;
  position: relative;
}
.toc summary {
  position: relative;
  display: flex;
  align-items: baseline;
  gap: var(--toc-marker-gap);
  min-width: 0;
  cursor: pointer;
  list-style: none;
}
.toc summary::-webkit-details-marker {
  display: none;
}
.toc summary::before {
  content: "";
  display: flex;
  align-self: baseline;
  flex: 0 0 var(--toc-marker-width);
  width: var(--toc-marker-width);
  height: 1em;
  color: var(--muted);
  background: currentColor;
  clip-path: polygon(30% 20%, 70% 50%, 30% 80%);
  transform: rotate(0deg);
  transform-origin: center;
}
.toc details[open] > summary::before {
  transform: rotate(90deg);
}
.toc details[open]::after {
  content: "";
  position: absolute;
  left: calc(var(--toc-marker-width) / 2);
  top: 1.05em;
  bottom: 0.62em;
  border-left: 1px solid var(--line);
  pointer-events: none;
}
.toc a {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr);
  column-gap: 0.38em;
  align-items: baseline;
  min-width: 0;
  max-width: 100%;
  overflow-wrap: anywhere;
}
.toc li > a {
  margin-left: calc(var(--toc-marker-width) + var(--toc-marker-gap));
}
.toc-number {
  white-space: nowrap;
}
.toc-title {
  min-width: 0;
  overflow-wrap: anywhere;
}
.toc details > .toc-child-list {
  margin: var(--toc-child-gap) 0 6px 0;
  padding-left: var(--toc-tree-step);
}
.toc-level-2 { font-size: .95rem; }
.toc-level-3 { font-size: .9rem; }
a { color: var(--accent); text-decoration: none; }
a:hover { text-decoration: underline; }
.section-number {
  color: var(--muted);
  font-weight: 600;
  white-space: nowrap;
}
.section-title {
  min-width: 0;
  overflow-wrap: break-word;
  word-break: normal;
}
.section-title a {
  white-space: nowrap;
}
figure {
  margin: 28px 0;
  padding: 0;
}
figure img {
  display: block;
  max-width: 100%;
  height: auto;
  margin: 0 auto;
}
.figure-grid {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 0;
  align-items: center;
}
.subfigure {
  flex: 1 1 220px;
  padding: 7px;
  scroll-margin-top: 18px;
}
.subfigure img {
  width: 100%;
}
.subfigure-caption {
  color: var(--muted);
  font-size: .9rem;
  line-height: 1.45;
  margin-top: 8px;
  text-align: center;
}
.table-grid {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 0;
  align-items: flex-start;
}
.subtable {
  flex: 1 1 260px;
  padding: 7px;
  scroll-margin-top: 18px;
}
.subtable-caption {
  color: var(--muted);
  font-size: .9rem;
  line-height: 1.45;
  margin-top: 8px;
  text-align: center;
}
.subfigure-break {
  flex-basis: 100%;
  width: 0;
  height: 0;
}
figcaption {
  color: var(--muted);
  font-size: .94rem;
  margin-top: 10px;
}
.cite {
  color: var(--accent);
  text-decoration: none;
  white-space: nowrap;
}
.cite-range {
  white-space: nowrap;
}
.references {
  border-top: 1px solid var(--line);
  margin-top: 34px;
  padding-top: 18px;
}
.references h2 {
  font-size: 1.35rem;
}
.references ol {
  padding-left: 28px;
}
.references li {
  margin: 0 0 10px;
  padding-left: 4px;
  font-size: 0.93rem;
  line-height: 1.45;
}
.math.display {
  position: relative;
  overflow-x: auto;
  padding: 12px 48px 12px 12px;
  margin: 18px 0;
  background: #fbfcfe;
  border-left: 3px solid var(--accent);
}
.equation-number {
  position: absolute;
  right: 12px;
  top: 12px;
  color: var(--muted);
}
.complex-wrapper {
  overflow-x: auto;
  margin: 24px 0;
}
.metablog-latexml-fragment,
.complex-block {
  color: inherit;
  font-family: inherit;
  font-size: 0.95rem;
  line-height: 1.5;
}
.metablog-latexml-fragment .ltx_document,
.metablog-latexml-fragment .ltx_para,
.metablog-latexml-fragment .ltx_float,
.metablog-latexml-fragment .ltx_table,
.metablog-latexml-fragment .ltx_listing,
.metablog-latexml-fragment .ltx_algorithm {
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
}
.metablog-latexml-fragment p,
.metablog-latexml-fragment .ltx_para {
  margin: 0 0 12px;
}
.metablog-latexml-fragment figure.ltx_algorithm {
  display: flex;
  flex-direction: column;
  width: 100%;
  max-width: 100%;
  margin: 18px auto;
  padding: 0;
  border-top: 1.5px solid var(--text);
  border-bottom: 1.5px solid var(--text);
  background: transparent;
}
.metablog-latexml-fragment figure.ltx_algorithm .ltx_caption {
  order: -1;
  margin: 0;
  padding: 6px 8px;
  border-bottom: 1px solid var(--text);
  color: inherit;
  font-size: 0.92rem;
  font-weight: 500;
}
.metablog-latexml-fragment figure.ltx_algorithm .ltx_listing {
  width: 100%;
  min-width: 0;
  margin: 0;
  padding: 8px 10px;
  font-family: "TeX Gyre Pagella", "Source Han Serif SC", "Noto Serif CJK SC", "Source Han Serif CN", "Noto Serif SC", "Songti SC", SimSun, Georgia, serif;
  font-size: 0.9rem;
  line-height: 1.45;
  overflow-wrap: anywhere;
}
.metablog-latexml-fragment figure.ltx_algorithm .ltx_listingline {
  display: block;
  min-height: 1.45em;
  padding-left: 2.4rem;
  position: relative;
  white-space: normal;
  overflow-wrap: anywhere;
  word-break: normal;
}
.metablog-latexml-fragment figure.ltx_algorithm .ltx_listingline::before {
  content: "";
  position: absolute;
  left: 1.8rem;
  top: -1px;
  bottom: -1px;
  width: 1px;
  background: var(--line);
  pointer-events: none;
}
.metablog-latexml-fragment figure.ltx_algorithm .metablog-algorithm-depth-1,
.metablog-latexml-fragment figure.ltx_algorithm .metablog-algorithm-depth-2,
.metablog-latexml-fragment figure.ltx_algorithm .metablog-algorithm-depth-3,
.metablog-latexml-fragment figure.ltx_algorithm .metablog-algorithm-depth-4,
.metablog-latexml-fragment figure.ltx_algorithm .metablog-algorithm-depth-5,
.metablog-latexml-fragment figure.ltx_algorithm .metablog-algorithm-depth-6 {
  background-repeat: no-repeat;
  background-size: 1px calc(100% + 2px);
}
.metablog-latexml-fragment figure.ltx_algorithm .metablog-algorithm-depth-1 {
  background-image: linear-gradient(var(--muted), var(--muted));
  background-position: 3.38rem -1px;
}
.metablog-latexml-fragment figure.ltx_algorithm .metablog-algorithm-depth-2 {
  background-image: linear-gradient(var(--muted), var(--muted)), linear-gradient(var(--muted), var(--muted));
  background-position: 3.38rem -1px, 4.88rem -1px;
}
.metablog-latexml-fragment figure.ltx_algorithm .metablog-algorithm-depth-3 {
  background-image: linear-gradient(var(--muted), var(--muted)), linear-gradient(var(--muted), var(--muted)), linear-gradient(var(--muted), var(--muted));
  background-position: 3.38rem -1px, 4.88rem -1px, 6.38rem -1px;
}
.metablog-latexml-fragment figure.ltx_algorithm .metablog-algorithm-depth-4 {
  background-image: linear-gradient(var(--muted), var(--muted)), linear-gradient(var(--muted), var(--muted)), linear-gradient(var(--muted), var(--muted)), linear-gradient(var(--muted), var(--muted));
  background-position: 3.38rem -1px, 4.88rem -1px, 6.38rem -1px, 7.88rem -1px;
}
.metablog-latexml-fragment figure.ltx_algorithm .metablog-algorithm-depth-5 {
  background-image: linear-gradient(var(--muted), var(--muted)), linear-gradient(var(--muted), var(--muted)), linear-gradient(var(--muted), var(--muted)), linear-gradient(var(--muted), var(--muted)), linear-gradient(var(--muted), var(--muted));
  background-position: 3.38rem -1px, 4.88rem -1px, 6.38rem -1px, 7.88rem -1px, 9.38rem -1px;
}
.metablog-latexml-fragment figure.ltx_algorithm .metablog-algorithm-depth-6 {
  background-image: linear-gradient(var(--muted), var(--muted)), linear-gradient(var(--muted), var(--muted)), linear-gradient(var(--muted), var(--muted)), linear-gradient(var(--muted), var(--muted)), linear-gradient(var(--muted), var(--muted)), linear-gradient(var(--muted), var(--muted));
  background-position: 3.38rem -1px, 4.88rem -1px, 6.38rem -1px, 7.88rem -1px, 9.38rem -1px, 10.88rem -1px;
}
.metablog-latexml-fragment figure.ltx_algorithm .metablog-algorithm-io {
  padding-left: 6.2rem;
  text-indent: -3.8rem;
}
.metablog-latexml-fragment figure.ltx_algorithm .ltx_tag_listingline {
  position: absolute;
  left: 0;
  width: 1.8rem;
  padding-right: 0.45rem;
  color: var(--muted);
  font-size: 0.78rem;
  line-height: 1.7;
  text-align: right;
}
.metablog-latexml-fragment figure.ltx_algorithm .ltx_rule {
  display: inline-block;
  width: 0.75rem;
  margin: 0 0.25rem 0 0.1rem;
  border-left: 1px solid transparent;
  color: transparent;
  vertical-align: stretch;
}
.metablog-latexml-fragment figure.ltx_algorithm .math.inline,
.metablog-latexml-fragment figure.ltx_algorithm .katex {
  white-space: normal;
}
.metablog-latexml-fragment table,
.metablog-latexml-fragment .ltx_tabular,
.complex-wrapper table {
  width: max-content;
  max-width: 100%;
  border-collapse: collapse;
  margin: 10px auto;
  font-size: 0.9rem;
  line-height: 1.35;
}
.metablog-latexml-fragment th,
.metablog-latexml-fragment td,
.complex-wrapper th,
.complex-wrapper td {
  border: 0;
  padding: 6px 8px;
  vertical-align: middle;
}
.metablog-latexml-fragment .ltx_border_t {
  border-top: 1px solid var(--line);
}
.metablog-latexml-fragment .ltx_border_tt {
  border-top: 1.8px solid var(--text);
}
.metablog-latexml-fragment .ltx_border_b {
  border-bottom: 1px solid var(--line);
}
.metablog-latexml-fragment .ltx_border_bb {
  border-bottom: 1.8px solid var(--text);
}
.metablog-latexml-fragment .ltx_border_l {
  border-left: 1px solid var(--line);
}
.metablog-latexml-fragment .ltx_border_ll {
  border-left: 1.8px solid var(--text);
}
.metablog-latexml-fragment .ltx_border_r {
  border-right: 1px solid var(--line);
}
.metablog-latexml-fragment .ltx_border_rr {
  border-right: 1.8px solid var(--text);
}
.metablog-latexml-fragment .ltx_align_left {
  text-align: left;
}
.metablog-latexml-fragment .ltx_align_center {
  text-align: center;
}
.metablog-latexml-fragment .ltx_align_right {
  text-align: right;
}
.metablog-latexml-fragment .ltx_align_justify {
  text-align: justify;
}
.metablog-latexml-fragment .ltx_align_top {
  vertical-align: top;
}
.metablog-latexml-fragment .ltx_align_middle {
  vertical-align: middle;
}
.metablog-latexml-fragment .ltx_align_bottom {
  vertical-align: bottom;
}
.metablog-latexml-fragment .ltx_nopad_l {
  padding-left: 0;
}
.metablog-latexml-fragment .ltx_nopad_r {
  padding-right: 0;
}
.metablog-latexml-fragment .ltx_centering {
  text-align: center;
}
.metablog-latexml-fragment .ltx_inline-block.ltx_align_center,
.metablog-latexml-fragment .ltx_transformed_outer.ltx_align_center {
  display: block;
  max-width: 100%;
  text-align: center;
}
.metablog-latexml-fragment .ltx_transformed_inner {
  display: inline-block;
  max-width: 100%;
}
.metablog-latexml-fragment .ltx_transformed_inner table {
  margin-left: auto;
  margin-right: auto;
}
.metablog-latexml-fragment .ltx_emph,
.metablog-latexml-fragment .ltx_font_italic {
  font-style: italic;
}
.metablog-latexml-fragment .ltx_font_upright {
  font-style: normal;
}
.metablog-latexml-fragment .ltx_font_typewriter {
  font-family: Consolas, "Liberation Mono", "Courier New", monospace;
  font-size: 0.94em;
}
.metablog-latexml-fragment .ltx_framed {
  border: 1px solid var(--line);
  padding: 2px 4px;
}
.metablog-latexml-fragment .ltx_framed_underline {
  border: 0;
  border-bottom: 1px solid currentColor;
  padding: 0;
}
.metablog-latexml-fragment .ltx_inline-block,
.metablog-latexml-fragment .ltx_inline-logical-block {
  display: inline-block;
  max-width: 100%;
}
.metablog-latexml-fragment .ltx_lst_numbers_left {
  text-align: left;
}
.metablog-latexml-fragment .ltx_noindent {
  text-indent: 0;
}
.metablog-latexml-fragment .ltx_p {
  margin: 0;
}
.metablog-latexml-fragment .ltx_tag_float,
.metablog-latexml-fragment .ltx_tag_table {
  font-weight: 700;
}
.metablog-latexml-fragment .ltx_td,
.metablog-latexml-fragment .ltx_text,
.metablog-latexml-fragment .ltx_tr {
  font: inherit;
}
.metablog-latexml-fragment .ltx_caption,
.metablog-latexml-fragment caption {
  color: inherit;
  font-size: .94rem;
  margin: 8px 0 10px;
  text-align: left;
}
.complex-block figcaption {
  color: var(--muted);
  font-size: .94rem;
  margin: 8px 0 10px;
  text-align: left;
}
.metablog-latexml-fragment .ltx_title {
  font-size: 1rem;
  font-weight: 650;
  margin: 0 0 8px;
  line-height: 1.35;
}
.metablog-latexml-fragment .ltx_equation,
.metablog-latexml-fragment .ltx_math {
  font-family: inherit;
}
.katex,
.katex-display {
  font-size: 1.02em;
}
.katex-display {
  overflow-x: auto;
  overflow-y: hidden;
  padding: 2px 0;
}
.metablog-latexml-fragment .ltx_tag {
  color: var(--muted);
}
.metablog-latexml-fragment img {
  max-width: 100%;
  height: auto;
}
pre {
  overflow-x: auto;
  background: #f3f5f8;
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 14px;
}
.code-block {
  margin: 18px 0;
  border-left: 5px solid var(--tcb-border, #75715e);
  background: #272822;
  border-radius: 0;
}
.code-block-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 10px;
  background: rgba(0,0,0,0.18);
}
.code-block-lang {
  font-weight: 700;
  color: #a6e22e;
  font-family: "HarmonyOS Sans", "Source Han Sans SC", "Noto Sans CJK SC", "Microsoft YaHei", Arial, sans-serif;
  font-size: 0.85rem;
}
.code-block-actions {
  display: flex;
  gap: 2px;
}
.code-block-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border: 0;
  border-radius: 3px;
  background: transparent;
  color: #75715e;
  cursor: pointer;
}
.code-block-btn:hover {
  color: #f8f8f2;
  background: rgba(255,255,255,0.08);
}
.code-block-body {
  overflow-x: auto;
}
.code-block[data-wrap="true"] .code-block-body {
  overflow-x: hidden;
}
.code-block-table {
  width: 100%;
  border-collapse: collapse;
  font-family: "Source Code Pro", "Cascadia Code", "Fira Code", Consolas, "Liberation Mono", "Courier New", monospace;
  font-size: 0.88rem;
  line-height: 1.55;
  color: #f8f8f2;
}
.code-block-row {
  vertical-align: top;
}
.code-block-line-no {
  width: 48px;
  min-width: 48px;
  text-align: right;
  padding: 0 12px 0 8px;
  color: #75715e;
  user-select: none;
  -webkit-user-select: none;
}
.code-block-line-code {
  padding: 0 12px 0 0;
  white-space: pre;
}
.code-block-source {
  display: none;
}
.code-block[data-wrap="true"] .code-block-line-code {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  padding-left: 24px;
  text-indent: -20px;
}
.code-block-collapsed .code-block-body {
  display: none;
}
.code-block-collapsed .code-block-collapse-btn svg {
  transform: rotate(180deg);
}
.footnote {
  position: relative;
  display: inline-block;
  margin-left: 2px;
  line-height: 1;
}
.footnote::before {
  content: "";
  position: absolute;
  left: -14px;
  right: -14px;
  bottom: 0.75em;
  height: 1.1em;
}
.footnote-marker {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.05em;
  height: 1.05em;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--accent);
  cursor: help;
  vertical-align: super;
}
.footnote-marker svg {
  width: 0.95em;
  height: 0.95em;
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}
.footnote-popover {
  position: absolute;
  left: 50%;
  bottom: 1.45em;
  z-index: 1000;
  width: max-content;
  max-width: min(340px, 82vw);
  transform: translateX(-50%) translateY(4px);
  padding: 10px 12px;
  border: 1px solid var(--line);
  border-radius: 6px;
  background: #fff;
  box-shadow: 0 10px 28px rgba(31, 41, 51, 0.16);
  color: var(--text);
  font-size: 0.88rem;
  font-weight: 400;
  line-height: 1.45;
  text-align: left;
  white-space: normal;
  visibility: hidden;
  opacity: 0;
  pointer-events: none;
  transition: opacity .12s ease, transform .12s ease, visibility .12s ease;
}
.footnote-popover::after {
  content: "";
  position: absolute;
  left: 50%;
  bottom: -6px;
  width: 10px;
  height: 10px;
  transform: translateX(-50%) rotate(45deg);
  border-right: 1px solid var(--line);
  border-bottom: 1px solid var(--line);
  background: #fff;
}
.footnote:hover .footnote-popover,
.footnote:focus-within .footnote-popover {
  visibility: visible;
  opacity: 1;
  pointer-events: auto;
  transform: translateX(-50%) translateY(0);
}
@media (max-width: 1499px) {
  .page {
    max-width: none;
  }
}
@media (min-width: 1500px) {
  .page {
    width: calc(100% - 468px);
    max-width: 980px;
    margin-left: 432px;
    margin-right: auto;
  }
  .toc {
    position: fixed;
    left: 32px;
    top: 32px;
    bottom: 32px;
    z-index: 900;
    width: 352px;
    margin: 0;
    padding: 20px 18px 20px 20px;
    overflow-x: hidden;
    overflow-y: auto;
    overscroll-behavior: contain;
    scrollbar-gutter: stable;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.96);
    box-shadow: 0 12px 34px rgba(31, 41, 51, 0.12);
  }
  body.site-layout .toc {
    top: 100px;
  }
  .toc-header {
    position: sticky;
    top: -20px;
    z-index: 2;
    margin: -20px -18px 14px -20px;
    padding: 20px 18px 12px 20px;
    border-bottom: 1px solid var(--line);
    background: rgba(255, 255, 255, 0.98);
  }
  .toc h2 {
    margin: 0;
    font-size: 1.2rem;
  }
  .toc-toggle {
    display: inline-flex;
  }
  .toc > .toc-list {
    columns: 1;
    margin-top: 0;
    padding-right: 10px;
    overflow: visible;
  }
  .toc ol ol {
    max-height: none;
    margin-top: 4px;
    overflow: visible;
  }
  body.toc-collapsed .page {
    width: auto;
    max-width: 980px;
    margin-left: auto;
    margin-right: auto;
  }
  body.toc-collapsed .toc {
    left: 18px;
    top: 50%;
    bottom: auto;
    width: 38px;
    height: 92px;
    transform: translateY(-50%);
    padding: 0;
    overflow: visible;
    border: 0;
    background: transparent;
    box-shadow: none;
  }
  body.toc-collapsed .toc h2,
  body.toc-collapsed .toc ol {
    display: none;
  }
  body.toc-collapsed .toc-header {
    justify-content: center;
    position: static;
    margin: 0;
    padding: 0;
    border: 0;
    background: transparent;
  }
  body.toc-collapsed .toc-toggle {
    width: 38px;
    min-width: 0;
    height: 92px;
    padding: 0;
    border: 0;
    background: transparent;
    color: var(--accent);
    font-weight: 700;
  }
  body.toc-collapsed .toc-toggle-open {
    display: none;
  }
  body.toc-collapsed .toc-toggle-closed {
    display: block;
  }
}
@media (min-width: 1844px) {
  .page {
    width: 100%;
    max-width: 980px;
    margin-left: auto;
    margin-right: auto;
  }
}
@media (max-width: 720px) {
  .page { padding: 24px 16px 56px; }
  body.site-layout { padding-top: 112px; }
  .site-topbar { height: 112px; }
  .site-topbar-inner {
    flex-direction: column;
    justify-content: center;
    gap: 8px;
    padding: 10px 16px;
  }
  .site-nav {
    width: 100%;
    justify-content: center;
    gap: 12px;
    overflow-x: auto;
  }
  .site-page { padding: 32px 16px 56px; }
  .article-list li {
    align-items: flex-start;
    gap: 10px;
  }
  .article-thumb {
    flex-basis: 76px;
    width: 76px;
    height: 48px;
  }
  .article-date {
    display: block;
    margin-top: 2px;
  }
  h1 { font-size: 1.55rem; }
}
@media (max-width: 600px) {
  .page {
    padding-left: 8px;
    padding-right: 8px;
  }
  .abstract {
    padding-left: 10px;
    padding-right: 10px;
  }
  .math.display {
    padding-left: 8px;
    padding-right: 36px;
  }
  .equation-number {
    right: 8px;
  }
  .complex-wrapper {
    margin-left: 0;
    margin-right: 0;
  }
}
`
