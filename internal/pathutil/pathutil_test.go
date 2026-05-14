package pathutil

import (
	"path/filepath"
	"runtime"
	"testing"
)

var isUnix = runtime.GOOS != "windows"

func TestCleanRelativePathRejectsEscapes(t *testing.T) {
	tests := []string{
		"../secret",
		"a/../../secret",
		"/absolute",
		`C:/absolute`,
		`C:\absolute`,
	}
	for _, tt := range tests {
		if got, err := CleanRelativePath(tt); err == nil {
			t.Fatalf("CleanRelativePath(%q) = %q, want error", tt, got)
		}
	}
}

func TestCleanRelativePathAcceptsLocalPaths(t *testing.T) {
	got, err := CleanRelativePath("./figs/../logo.svg")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join("logo.svg") {
		t.Fatalf("unexpected clean path: %q", got)
	}
}

func TestCleanRelativePathAcceptsUnixColonPaths(t *testing.T) {
	// On Windows, "a:b" is a valid volume reference and is correctly rejected
	// by filepath.VolumeName. This test only applies to Unix, where colons are
	// legal filename characters and should not be mistaken for drive prefixes.
	if !isUnix {
		t.Skip("colon filename test only applicable on Unix")
	}
	for _, path := range []string{"a:b", "a:b/c", "data:a", "fig:a/b.svg"} {
		_, err := CleanRelativePath(path)
		if err != nil {
			t.Fatalf("CleanRelativePath(%q) unexpected error: %v", path, err)
		}
	}
}

func TestIsWithinDir(t *testing.T) {
	root := t.TempDir()
	if !IsWithinDir(root, filepath.Join(root, "a", "b.txt")) {
		t.Fatal("expected child path to be inside root")
	}
	if IsWithinDir(root, filepath.Join(root, "..", "outside.txt")) {
		t.Fatal("expected parent escape to be outside root")
	}
}
