package latexml

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"MetaBlog/internal/latex/blocks"
)

type Runner struct {
	Bin       string
	CacheDir  string
	KeepTemp  bool
	Identity  *CacheIdentity
	Warnings  *[]string
	WarningMu *sync.Mutex
	Log       io.Writer
	LogMu     *sync.Mutex
	Stats     *CacheStats
}

type CacheIdentity struct {
	once    sync.Once
	bin     string
	version string
	ok      bool
}

type CacheStats struct {
	mu            sync.Mutex
	Hits          int
	Misses        int
	Bypassed      int
	BypassReasons map[string]int
}

type CacheStatsSnapshot struct {
	Hits          int
	Misses        int
	Bypassed      int
	BypassReasons map[string]int
}

const cacheSchema = 1

const lateXMLWrapperPrefix = `\documentclass{article}
\usepackage{amsmath,amsfonts}
\usepackage{algorithmic}
\usepackage{array}
\usepackage[caption=false,font=normalsize,labelfont=sf,textfont=sf]{subfig}
\usepackage{textcomp}
\usepackage{stfloats}
\usepackage{url}
\usepackage{hyperref}
\usepackage{verbatim}
\usepackage{graphicx}
\usepackage{cite}
\usepackage{multirow}
\usepackage{makecell}
\usepackage{booktabs}
\usepackage{tabularx}
\usepackage{amssymb}
\usepackage{ulem}
\usepackage[table]{xcolor}
\usepackage[ruled,linesnumbered]{algorithm2e}
\begin{document}
`

const lateXMLWrapperSuffix = `
\end{document}
`

var lateXMLArgs = []string{
	"--format=html5",
	"--whatsout=fragment",
	"--nodefaultresources",
	"--destination=fragment.html",
	"fragment.tex",
}

type cacheEntry struct {
	Schema         int    `json:"schema"`
	Key            string `json:"key"`
	RawTeX         string `json:"raw_tex"`
	RawTeXSHA256   string `json:"raw_tex_sha256"`
	WrapperSHA256  string `json:"wrapper_sha256"`
	ArgsSHA256     string `json:"args_sha256"`
	LaTeXMLBin     string `json:"latexml_bin"`
	LaTeXMLVersion string `json:"latexml_version"`
	RawHTML        string `json:"raw_html"`
}

func (r Runner) Convert(block *blocks.ComplexBlock) {
	rawHTML, err := r.convertRawHTML(block)
	if err != nil {
		r.warn(fmt.Sprintf("latexml fallback for %s: %v", block.ID, err))
		block.HTML = fallback(block)
		return
	}
	htmlText := extractBody(rawHTML)
	htmlText = repairAlignedMathFromRaw(htmlText, block.RawTeX)
	block.HTML = wrapFragment(htmlText)
}

func (r Runner) convertRawHTML(block *blocks.ComplexBlock) (string, error) {
	cacheOK, reason := r.cacheStatus(block.RawTeX)
	if cacheOK {
		if htmlText, ok := r.readCache(block.RawTeX); ok {
			r.recordCacheHit()
			return htmlText, nil
		}
		r.recordCacheMiss()
		rawHTML, err := r.convertWithLateXML(block.RawTeX)
		if err != nil {
			return "", err
		}
		r.writeCache(block.RawTeX, rawHTML)
		return rawHTML, nil
	}
	r.recordCacheBypass(reason)
	return r.convertWithLateXML(block.RawTeX)
}

func (r Runner) recordCacheHit() {
	if r.Stats != nil {
		r.Stats.RecordHit()
	}
}

func (r Runner) recordCacheMiss() {
	if r.Stats != nil {
		r.Stats.RecordMiss()
	}
}

func (r Runner) recordCacheBypass(reason string) {
	if r.Stats != nil {
		r.Stats.RecordBypass(reason)
	}
}

func (s *CacheStats) RecordHit() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Hits++
}

func (s *CacheStats) RecordMiss() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Misses++
}

func (s *CacheStats) RecordBypass(reason string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Bypassed++
	if reason == "" {
		reason = "unknown"
	}
	if s.BypassReasons == nil {
		s.BypassReasons = map[string]int{}
	}
	s.BypassReasons[reason]++
}

func (s *CacheStats) Snapshot() CacheStatsSnapshot {
	if s == nil {
		return CacheStatsSnapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	reasons := make(map[string]int, len(s.BypassReasons))
	for reason, count := range s.BypassReasons {
		reasons[reason] = count
	}
	return CacheStatsSnapshot{
		Hits:          s.Hits,
		Misses:        s.Misses,
		Bypassed:      s.Bypassed,
		BypassReasons: reasons,
	}
}

func (r Runner) convertWithLateXML(raw string) (string, error) {
	bin, err := r.resolveBin()
	if err != nil {
		return "", err
	}
	tempDir, err := os.MkdirTemp("", "metablog-latexml-*")
	if err != nil {
		return "", err
	}
	if !r.KeepTemp {
		defer os.RemoveAll(tempDir)
	}
	texPath := filepath.Join(tempDir, "fragment.tex")
	htmlPath := filepath.Join(tempDir, "fragment.html")
	wrapped := lateXMLWrapperPrefix + raw + lateXMLWrapperSuffix
	if err := os.WriteFile(texPath, []byte(wrapped), 0644); err != nil {
		return "", err
	}
	cmd := command(bin, lateXMLArgs...)
	cmd.Dir = tempDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	b, err := os.ReadFile(htmlPath)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (r Runner) cacheEnabled(raw string) bool {
	ok, _ := r.cacheStatus(raw)
	return ok
}

func (r Runner) cacheStatus(raw string) (bool, string) {
	if r.CacheDir == "" {
		return false, "cache disabled"
	}
	if r.KeepTemp {
		return false, "-keep-temp enabled"
	}
	if cmd, ok := externalDependencyCommand(raw); ok {
		return false, "external dependency command " + cmd
	}
	return true, ""
}

func (r Runner) readCache(raw string) (string, bool) {
	meta, ok := r.cacheMeta(raw)
	if !ok {
		return "", false
	}
	path := filepath.Join(r.CacheDir, meta.Key+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var entry cacheEntry
	if err := json.Unmarshal(b, &entry); err != nil {
		return "", false
	}
	if !cacheEntryMatches(entry, meta, raw) {
		return "", false
	}
	return entry.RawHTML, true
}

func (r Runner) writeCache(raw, rawHTML string) {
	if strings.TrimSpace(rawHTML) == "" {
		return
	}
	meta, ok := r.cacheMeta(raw)
	if !ok {
		return
	}
	if err := os.MkdirAll(r.CacheDir, 0755); err != nil {
		return
	}
	entry := cacheEntry{
		Schema:         cacheSchema,
		Key:            meta.Key,
		RawTeX:         raw,
		RawTeXSHA256:   meta.RawTeXSHA256,
		WrapperSHA256:  meta.WrapperSHA256,
		ArgsSHA256:     meta.ArgsSHA256,
		LaTeXMLBin:     meta.LaTeXMLBin,
		LaTeXMLVersion: meta.LaTeXMLVersion,
		RawHTML:        rawHTML,
	}
	b, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return
	}
	target := filepath.Join(r.CacheDir, meta.Key+".json")
	tmp, err := os.CreateTemp(r.CacheDir, meta.Key+".*.tmp")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()
	_, writeErr := tmp.Write(b)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(tmpPath)
		return
	}
	if err := os.Rename(tmpPath, target); err != nil {
		_ = os.Remove(target)
		if err := os.Rename(tmpPath, target); err != nil {
			os.Remove(tmpPath)
		}
	}
}

type cacheMeta struct {
	Key            string
	RawTeXSHA256   string
	WrapperSHA256  string
	ArgsSHA256     string
	LaTeXMLBin     string
	LaTeXMLVersion string
}

func (r Runner) cacheMeta(raw string) (cacheMeta, bool) {
	bin, version, ok := r.cacheIdentity()
	if !ok {
		return cacheMeta{}, false
	}
	meta := cacheMeta{
		RawTeXSHA256:   sha256String(raw),
		WrapperSHA256:  sha256String(lateXMLWrapperPrefix + lateXMLWrapperSuffix),
		ArgsSHA256:     sha256String(strings.Join(lateXMLArgs, "\x00")),
		LaTeXMLBin:     filepath.Clean(bin),
		LaTeXMLVersion: version,
	}
	keyMaterial := strings.Join([]string{
		fmt.Sprint(cacheSchema),
		meta.RawTeXSHA256,
		meta.WrapperSHA256,
		meta.ArgsSHA256,
		meta.LaTeXMLBin,
		meta.LaTeXMLVersion,
	}, "\x00")
	meta.Key = sha256String(keyMaterial)
	return meta, true
}

func (r Runner) cacheIdentity() (string, string, bool) {
	if r.Identity != nil {
		return r.Identity.resolve(r)
	}
	bin, err := r.resolveBin()
	if err != nil {
		return "", "", false
	}
	version, ok := lateXMLVersion(bin)
	if !ok {
		return "", "", false
	}
	return filepath.Clean(bin), version, true
}

func (r Runner) PrepareCacheIdentity() (string, string, bool) {
	return r.cacheIdentity()
}

func (ci *CacheIdentity) resolve(r Runner) (string, string, bool) {
	ci.once.Do(func() {
		bin, err := r.resolveBin()
		if err != nil {
			return
		}
		version, ok := lateXMLVersion(bin)
		if !ok {
			return
		}
		ci.bin = filepath.Clean(bin)
		ci.version = version
		ci.ok = true
	})
	return ci.bin, ci.version, ci.ok
}

func cacheEntryMatches(entry cacheEntry, meta cacheMeta, raw string) bool {
	return entry.Schema == cacheSchema &&
		entry.Key == meta.Key &&
		entry.RawTeX == raw &&
		entry.RawTeXSHA256 == meta.RawTeXSHA256 &&
		entry.WrapperSHA256 == meta.WrapperSHA256 &&
		entry.ArgsSHA256 == meta.ArgsSHA256 &&
		entry.LaTeXMLBin == meta.LaTeXMLBin &&
		entry.LaTeXMLVersion == meta.LaTeXMLVersion &&
		strings.TrimSpace(entry.RawHTML) != ""
}

func lateXMLVersion(bin string) (string, bool) {
	for _, arg := range []string{"--VERSION", "--version"} {
		cmd := command(bin, arg)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			continue
		}
		version := strings.TrimSpace(out.String())
		if version != "" {
			return version, true
		}
	}
	return "", false
}

func sha256String(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func hasExternalDependencyCommand(raw string) bool {
	return externalDependencyCommandRE.MatchString(raw)
}

func externalDependencyCommand(raw string) (string, bool) {
	match := externalDependencyCommandRE.FindStringSubmatch(raw)
	if len(match) < 2 {
		return "", false
	}
	return `\` + match[1], true
}

func (r Runner) logf(format string, args ...any) {
	if r.Log == nil {
		return
	}
	if r.LogMu != nil {
		r.LogMu.Lock()
		defer r.LogMu.Unlock()
	}
	fmt.Fprintf(r.Log, format, args...)
}

func (r Runner) resolveBin() (string, error) {
	if r.Bin != "" {
		if filepath.IsAbs(r.Bin) {
			return filepath.Clean(r.Bin), nil
		}
		if strings.ContainsAny(r.Bin, `/\`) {
			return filepath.Abs(r.Bin)
		}
		if bin, err := exec.LookPath(r.Bin); err == nil {
			return bin, nil
		}
		return "", exec.ErrNotFound
	}
	if bin, err := exec.LookPath("latexmlc"); err == nil {
		return bin, nil
	}
	return "", exec.ErrNotFound
}

func command(bin string, args ...string) *exec.Cmd {
	ext := strings.ToLower(filepath.Ext(bin))
	if runtime.GOOS == "windows" && (ext == ".bat" || ext == ".cmd") {
		cmdArgs := append([]string{"/C", bin}, args...)
		return exec.Command("cmd", cmdArgs...)
	}
	return exec.Command(bin, args...)
}

func (r Runner) warn(msg string) {
	if r.Warnings != nil {
		if r.WarningMu != nil {
			r.WarningMu.Lock()
			defer r.WarningMu.Unlock()
		}
		*r.Warnings = append(*r.Warnings, msg)
	}
}

func extractBody(s string) string {
	s = stripTagBlocks(s, "style")
	s = stripTagBlocks(s, "script")
	lower := strings.ToLower(s)
	start := strings.Index(lower, "<body")
	if start < 0 {
		return sanitizeFragment(s)
	}
	closeStart := strings.Index(lower[start:], ">")
	if closeStart < 0 {
		return s
	}
	start += closeStart + 1
	end := strings.LastIndex(lower, "</body>")
	if end < start {
		return sanitizeFragment(s[start:])
	}
	return sanitizeFragment(s[start:end])
}

func stripTagBlocks(s, tag string) string {
	lower := strings.ToLower(s)
	open := "<" + tag
	close := "</" + tag + ">"
	for {
		start := strings.Index(lower, open)
		if start < 0 {
			return s
		}
		openEnd := strings.Index(lower[start:], ">")
		if openEnd < 0 {
			return s
		}
		end := strings.Index(lower[start+openEnd+1:], close)
		if end < 0 {
			return s
		}
		endAbs := start + openEnd + 1 + end + len(close)
		s = s[:start] + s[endAbs:]
		lower = strings.ToLower(s)
	}
}

func wrapFragment(fragment string) string {
	fragment = strings.TrimSpace(fragment)
	if fragment == "" {
		return ""
	}
	return `<div class="metablog-latexml-fragment">` + fragment + `</div>`
}

var generatedIDAttrRE = regexp.MustCompile(`(?i)\sid=("[^"]*"|'[^']*')`)
var styleAttrRE = regexp.MustCompile(`(?is)\sstyle=("[^"]*"|'[^']*')`)
var mathTagRE = regexp.MustCompile(`(?is)<math\b([^>]*)>.*?</math>`)
var htmlAttrRE = regexp.MustCompile(`(?i)([a-z_:][-a-z0-9_:.]*)=("[^"]*"|'[^']*')`)
var externalDependencyCommandRE = regexp.MustCompile(`\\(input|include|includegraphics|bibliography|addbibresource)\b`)
var cssHexColorRE = regexp.MustCompile(`(?i)^#[0-9a-f]{3}([0-9a-f]{3})?([0-9a-f]{2})?$`)
var cssNamedColorRE = regexp.MustCompile(`(?i)^[a-z]+$`)
var cssRGBColorRE = regexp.MustCompile(`(?i)^rgba?\(\s*(\d{1,3}%?\s*,\s*){2}\d{1,3}%?(\s*,\s*(0|1|0?\.\d+|\d{1,3}%))?\s*\)$`)
var rawInlineAlignedMathRE = regexp.MustCompile(`(?s)\\\((\\begin\{aligned\}.*?\\end\{aligned\})\\\)`)
var renderedInlineAlignedMathRE = regexp.MustCompile(`(?s)<span class="math inline">\\\((\\begin\{aligned\}.*?\\end\{aligned\})\\\)</span>`)

func sanitizeFragment(s string) string {
	s = mathTagRE.ReplaceAllStringFunc(s, replaceMathTag)
	s = generatedIDAttrRE.ReplaceAllString(s, "")
	s = sanitizeStyleAttrs(s)
	s = annotateAlgorithmLines(s)
	return strings.TrimSpace(s)
}

func sanitizeStyleAttrs(s string) string {
	return styleAttrRE.ReplaceAllStringFunc(s, func(attr string) string {
		eq := strings.Index(attr, "=")
		if eq < 0 {
			return ""
		}
		raw := strings.TrimSpace(attr[eq+1:])
		if len(raw) < 2 {
			return ""
		}
		quote := raw[0]
		if (quote != '"' && quote != '\'') || raw[len(raw)-1] != quote {
			return ""
		}
		style := sanitizeStyleValue(html.UnescapeString(raw[1 : len(raw)-1]))
		if style == "" {
			return ""
		}
		return ` style="` + html.EscapeString(style) + `"`
	})
}

func sanitizeStyleValue(style string) string {
	var kept []string
	for _, decl := range strings.Split(style, ";") {
		prop, val, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		prop = strings.ToLower(strings.TrimSpace(prop))
		val = strings.TrimSpace(val)
		if !isAllowedStyleProp(prop) || !isSafeCSSColor(val) {
			continue
		}
		kept = append(kept, prop+":"+val)
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, ";")
}

func isAllowedStyleProp(prop string) bool {
	return prop == "color" || prop == "background-color"
}

func isSafeCSSColor(val string) bool {
	val = strings.TrimSpace(val)
	if val == "" || strings.ContainsAny(val, `;"'<>\`) {
		return false
	}
	if cssHexColorRE.MatchString(val) || cssNamedColorRE.MatchString(val) || cssRGBColorRE.MatchString(val) {
		return true
	}
	return false
}

func repairAlignedMathFromRaw(htmlText, rawTeX string) string {
	rawMatches := rawInlineAlignedMathRE.FindAllStringSubmatch(rawTeX, -1)
	if len(rawMatches) == 0 {
		return htmlText
	}
	i := 0
	return renderedInlineAlignedMathRE.ReplaceAllStringFunc(htmlText, func(rendered string) string {
		if i >= len(rawMatches) {
			return rendered
		}
		raw := normalizeAltTeX(rawMatches[i][1])
		i++
		if raw == "" {
			return rendered
		}
		return `<span class="math inline">\(` + html.EscapeString(raw) + `\)</span>`
	})
}

var (
	algorithmFigureRE      = regexp.MustCompile(`(?is)<figure\s+class="[^"]*\bltx_algorithm\b[^"]*"[^>]*>.*?</figure>`)
	listingLineRE          = regexp.MustCompile(`(?is)<div\s+class="([^"]*\bltx_listingline\b[^"]*)"([^>]*)>(.*?)</div>`)
	listingLineAdornmentRE = regexp.MustCompile(`(?is)<span\s+class="[^"]*\b(?:ltx_tag_listingline|ltx_rule)\b[^"]*"[^>]*>.*?</span>`)
	listingRuleRE          = regexp.MustCompile(`(?is)<span\s+class="[^"]*\bltx_rule\b[^"]*"[^>]*>.*?</span>`)
	htmlTagRE              = regexp.MustCompile(`(?is)<[^>]+>`)
)

func annotateAlgorithmLines(s string) string {
	return algorithmFigureRE.ReplaceAllStringFunc(s, func(fig string) string {
		return listingLineRE.ReplaceAllStringFunc(fig, func(line string) string {
			m := listingLineRE.FindStringSubmatch(line)
			if len(m) != 4 {
				return line
			}
			className := m[1]
			attrs := m[2]
			body := m[3]
			lineText := algorithmLineText(body)
			if lineText == "" {
				return ""
			}
			if strings.Contains(body, "ltx_tag_listingline") {
				className += " metablog-algorithm-numbered"
			}
			ruleDepth := len(listingRuleRE.FindAllString(body, -1))
			if ruleDepth > 0 {
				if ruleDepth > 6 {
					ruleDepth = 6
				}
				className += fmt.Sprintf(" metablog-algorithm-depth-%d", ruleDepth)
			}
			if strings.HasPrefix(lineText, "input:") || strings.HasPrefix(lineText, "output:") {
				className += " metablog-algorithm-io"
			}
			return `<div class="` + className + `"` + attrs + `>` + body + `</div>`
		})
	})
}

func algorithmLineText(body string) string {
	body = listingLineAdornmentRE.ReplaceAllString(body, "")
	text := html.UnescapeString(htmlTagRE.ReplaceAllString(body, ""))
	text = strings.ReplaceAll(text, "\u00a0", " ")
	return strings.ToLower(strings.TrimSpace(text))
}

func replaceMathTag(tag string) string {
	attrs := parseAttrs(tag)
	tex := strings.TrimSpace(attrs["alttext"])
	if tex == "" {
		return tag
	}
	tex = normalizeAltTeX(tex)
	if attrs["display"] == "inline" || attrs["display"] == "" {
		return `<span class="math inline">\(` + html.EscapeString(tex) + `\)</span>`
	}
	return `<div class="math display">\[` + html.EscapeString(tex) + `\]</div>`
}

func parseAttrs(tag string) map[string]string {
	out := map[string]string{}
	for _, match := range htmlAttrRE.FindAllStringSubmatch(tag, -1) {
		if len(match) != 3 {
			continue
		}
		key := strings.ToLower(match[1])
		val := match[2]
		if len(val) >= 2 {
			val = val[1 : len(val)-1]
		}
		out[key] = html.UnescapeString(val)
	}
	return out
}

func normalizeAltTeX(tex string) string {
	tex = strings.ReplaceAll(tex, "\r\n", "\n")
	tex = strings.ReplaceAll(tex, "\r", "\n")
	tex = strings.ReplaceAll(tex, "%\n", "")
	tex = strings.ReplaceAll(tex, "\n", " ")
	return strings.Join(strings.Fields(tex), " ")
}

func fallback(block *blocks.ComplexBlock) string {
	var b strings.Builder
	kind := html.EscapeString(block.EnvName)
	b.WriteString(`<figure class="complex-block complex-`)
	b.WriteString(kind)
	b.WriteString(`">`)
	if block.Caption != "" {
		b.WriteString(`<figcaption>`)
		b.WriteString(html.EscapeString(block.Caption))
		b.WriteString(`</figcaption>`)
	}
	b.WriteString(`<pre><code>`)
	b.WriteString(html.EscapeString(block.RawTeX))
	b.WriteString(`</code></pre></figure>`)
	return b.String()
}
