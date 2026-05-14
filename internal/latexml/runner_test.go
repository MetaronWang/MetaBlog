package latexml

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRepairAlignedMathFromRawRestoresFirstRows(t *testing.T) {
	raw := `\begin{tabular}{cc}
CIMP & \(\begin{aligned} [1, 156, 22, 0.42, T]\\ [29, 35, 13, 0.39, T]\\ [29, 29, 2, 0.27, T]\\ [21, 27, 4, 0.56, F]\end{aligned}\) \\
CAOP & \(\begin{aligned} [7, 33, 14, 0.44, T]\\ [1, 54, 24, 0.57, T]\\ [2, 31, 40, 0.79, F]\\ [5, 159, 27, 0.79, F]\end{aligned}\)
\end{tabular}`
	htmlText := `<td><span class="math inline">\(\begin{aligned} \\ [29,35,13,0.39,T]\\ [29,29,2,0.27,T]\\ [21,27,4,0.56,F]\end{aligned}\)</span></td>` +
		`<td><span class="math inline">\(\begin{aligned} \\ [1,54,24,0.57,T]\\ [2,31,40,0.79,F]\\ [5,159,27,0.79,F]\end{aligned}\)</span></td>`

	got := repairAlignedMathFromRaw(htmlText, raw)
	if !strings.Contains(got, `[1, 156, 22, 0.42, T]`) {
		t.Fatalf("first aligned row was not restored: %s", got)
	}
	if !strings.Contains(got, `[7, 33, 14, 0.44, T]`) {
		t.Fatalf("second aligned block first row was not restored: %s", got)
	}
	if strings.Contains(got, `\begin{aligned} \\`) {
		t.Fatalf("leading empty aligned row was preserved: %s", got)
	}
}

func TestSanitizeFragmentPreservesSafeColorStyles(t *testing.T) {
	in := `<article id="latexml"><figcaption><span class="ltx_text" style="background-color:#A6A6A6;">Gray</span></figcaption>` +
		`<td class="ltx_td" style="background-color:#A6A6A6; position:absolute; left:0;">B</td>` +
		`<span style="color:blue;">text</span></article>`

	got := sanitizeFragment(in)
	if strings.Contains(got, `id="latexml"`) {
		t.Fatalf("generated id was preserved: %s", got)
	}
	if strings.Count(got, `background-color:#A6A6A6`) != 2 {
		t.Fatalf("background colors were not preserved: %s", got)
	}
	if !strings.Contains(got, `style="color:blue"`) {
		t.Fatalf("text color was not preserved: %s", got)
	}
	if strings.Contains(got, "position") || strings.Contains(got, "left:0") {
		t.Fatalf("unsafe layout style was preserved: %s", got)
	}
}

func TestCacheReadRequiresExactRawTeXMatch(t *testing.T) {
	bin := fakeLateXMLBin(t)
	runner := Runner{Bin: bin, CacheDir: t.TempDir()}
	raw := `\begin{tabular}{c}A\end{tabular}`
	rawHTML := `<html><body><table><tr><td>A</td></tr></table></body></html>`

	runner.writeCache(raw, rawHTML)
	got, ok := runner.readCache(raw)
	if !ok || got != rawHTML {
		t.Fatalf("cache read failed: ok=%v got=%q", ok, got)
	}

	meta, ok := runner.cacheMeta(raw)
	if !ok {
		t.Fatal("cache meta failed")
	}
	path := filepath.Join(runner.CacheDir, meta.Key+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var entry cacheEntry
	if err := json.Unmarshal(b, &entry); err != nil {
		t.Fatal(err)
	}
	entry.RawTeX = raw + "% changed"
	b, err = json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatal(err)
	}
	if _, ok := runner.readCache(raw); ok {
		t.Fatal("cache hit with mismatched raw TeX")
	}
}

func TestCacheDisabledForExternalDependencyCommands(t *testing.T) {
	for _, raw := range []string{
		`\begin{tabular}{c}\input{table-row}\end{tabular}`,
		`\begin{tabular}{c}\include{table-row}\end{tabular}`,
		`\begin{tabular}{c}\includegraphics{fig/a.pdf}\end{tabular}`,
		`\begin{tabular}{c}\bibliography{refs}\end{tabular}`,
		`\begin{tabular}{c}\addbibresource{refs.bib}\end{tabular}`,
	} {
		if !hasExternalDependencyCommand(raw) {
			t.Fatalf("external dependency command not detected: %s", raw)
		}
	}
	runner := Runner{Bin: fakeLateXMLBin(t), CacheDir: t.TempDir()}
	if runner.cacheEnabled(`\begin{tabular}{c}\includegraphics{fig/a.pdf}\end{tabular}`) {
		t.Fatal("cache enabled for includegraphics dependency")
	}
}

func TestMemoryCacheRequiresExactRawTeXMatch(t *testing.T) {
	bin := fakeLateXMLBin(t)
	runner := Runner{Bin: bin, CacheDir: t.TempDir(), CacheStore: NewCacheStore(8)}
	raw := `\begin{tabular}{c}A\end{tabular}`
	rawHTML := `<html><body><table><tr><td>A</td></tr></table></body></html>`
	meta, ok := runner.cacheMeta(raw)
	if !ok {
		t.Fatal("cache meta failed")
	}
	runner.CacheStore.set(cacheEntry{
		Schema:         cacheSchema,
		Key:            meta.Key,
		RawTeX:         raw + "% changed",
		RawTeXSHA256:   meta.RawTeXSHA256,
		WrapperSHA256:  meta.WrapperSHA256,
		ArgsSHA256:     meta.ArgsSHA256,
		LaTeXMLBin:     meta.LaTeXMLBin,
		LaTeXMLVersion: meta.LaTeXMLVersion,
		RawHTML:        rawHTML,
	})
	if _, ok := runner.readCache(raw); ok {
		t.Fatal("memory cache hit with mismatched raw TeX")
	}
}

func TestMemoryCacheEvictsOldEntries(t *testing.T) {
	store := NewCacheStore(2)
	for _, key := range []string{"a", "b", "c"} {
		store.set(cacheEntry{Schema: cacheSchema, Key: key, RawTeX: key, RawHTML: key})
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if len(store.data) != 2 {
		t.Fatalf("expected bounded memory cache size 2, got %d", len(store.data))
	}
	if _, ok := store.data["a"]; ok {
		t.Fatal("oldest memory cache entry was not evicted")
	}
}

func TestMemoryCachePromotesHitsForLRU(t *testing.T) {
	runner := Runner{Bin: fakeLateXMLBin(t), CacheStore: NewCacheStore(2)}
	rawA := `\begin{tabular}{c}A\end{tabular}`
	rawB := `\begin{tabular}{c}B\end{tabular}`
	rawC := `\begin{tabular}{c}C\end{tabular}`
	for _, raw := range []string{rawA, rawB} {
		meta, ok := runner.cacheMeta(raw)
		if !ok {
			t.Fatal("cache meta failed")
		}
		runner.CacheStore.set(cacheEntry{
			Schema:         cacheSchema,
			Key:            meta.Key,
			RawTeX:         raw,
			RawTeXSHA256:   meta.RawTeXSHA256,
			WrapperSHA256:  meta.WrapperSHA256,
			ArgsSHA256:     meta.ArgsSHA256,
			LaTeXMLBin:     meta.LaTeXMLBin,
			LaTeXMLVersion: meta.LaTeXMLVersion,
			RawHTML:        raw,
		})
	}
	metaA, _ := runner.cacheMeta(rawA)
	if _, ok := runner.CacheStore.get(metaA, rawA); !ok {
		t.Fatal("expected cache hit for A")
	}
	metaC, _ := runner.cacheMeta(rawC)
	runner.CacheStore.set(cacheEntry{
		Schema:         cacheSchema,
		Key:            metaC.Key,
		RawTeX:         rawC,
		RawTeXSHA256:   metaC.RawTeXSHA256,
		WrapperSHA256:  metaC.WrapperSHA256,
		ArgsSHA256:     metaC.ArgsSHA256,
		LaTeXMLBin:     metaC.LaTeXMLBin,
		LaTeXMLVersion: metaC.LaTeXMLVersion,
		RawHTML:        rawC,
	})
	metaB, _ := runner.cacheMeta(rawB)
	if _, ok := runner.CacheStore.get(metaB, rawB); ok {
		t.Fatal("B should have been evicted after A was promoted")
	}
	if _, ok := runner.CacheStore.get(metaA, rawA); !ok {
		t.Fatal("A should remain after LRU promotion")
	}
}

func TestWriteCacheDoesNotOverwriteValidExistingCache(t *testing.T) {
	runner := Runner{Bin: fakeLateXMLBin(t), CacheDir: t.TempDir(), CacheStore: NewCacheStore(8)}
	raw := `\begin{tabular}{c}A\end{tabular}`
	first := `<html><body>first</body></html>`
	second := `<html><body>second</body></html>`
	runner.writeCache(raw, first)
	runner.writeCache(raw, second)
	got, ok := runner.readCache(raw)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got != first {
		t.Fatalf("valid existing cache was overwritten: %q", got)
	}
}

func TestLateXMLVersionFallsBackToUppercaseOption(t *testing.T) {
	bin := fakeLateXMLBinWithVersionOption(t, "--VERSION")
	version, ok := lateXMLVersion(bin)
	if !ok {
		t.Fatal("version detection failed")
	}
	if version != "fake latexmlc 1.0" {
		t.Fatalf("unexpected version: %q", version)
	}
}

func fakeLateXMLBin(t *testing.T) string {
	return fakeLateXMLBinWithVersionOption(t, "--version")
}

func fakeLateXMLBinWithVersionOption(t *testing.T, versionOption string) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "latexmlc.bat")
		script := "@echo off\r\nif \"%1\"==\"" + versionOption + "\" (\r\necho fake latexmlc 1.0\r\nexit /b 0\r\n)\r\nexit /b 1\r\n"
		if err := os.WriteFile(path, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	path := filepath.Join(dir, "latexmlc")
	script := "#!/bin/sh\nif [ \"$1\" = \"" + versionOption + "\" ]; then\n  echo 'fake latexmlc 1.0'\n  exit 0\nfi\nexit 1\n"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}
