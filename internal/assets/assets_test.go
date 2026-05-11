package assets

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConvertFileSkipsFreshCopiedAsset(t *testing.T) {
	dir := t.TempDir()
	sourceRoot := filepath.Join(dir, "src")
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(sourceRoot, 0755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(sourceRoot, "fig.png")
	if err := os.WriteFile(sourcePath, []byte("source"), 0644); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(outDir, "assets", "article", "fig.png")
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outPath, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	sourceTime := time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local)
	outputTime := sourceTime.Add(time.Hour)
	if err := os.Chtimes(sourcePath, sourceTime, sourceTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(outPath, outputTime, outputTime); err != nil {
		t.Fatal(err)
	}

	got, err := Converter{
		SourceRoot:  sourceRoot,
		OutDir:      outDir,
		AssetSubdir: "article",
	}.ConvertFile("fig.png")
	if err != nil {
		t.Fatal(err)
	}
	if got != "assets/article/fig.png" {
		t.Fatalf("unexpected asset link: %s", got)
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "existing" {
		t.Fatalf("fresh asset was copied again; got %q", string(b))
	}
}
