package highlight

import (
	"strings"
	"testing"
)

func TestHighlightPython(t *testing.T) {
	code := "def hello():\n    print('world')\n"
	got := Highlight(code, "python")
	if !strings.Contains(got, "chline") {
		t.Fatalf("expected chroma line class in output: %s", got)
	}
	if !strings.Contains(got, `class="chk"`) {
		t.Fatalf("expected keyword token class in output: %s", got)
	}
	if !strings.Contains(got, "hello") {
		t.Fatalf("missing original code: %s", got)
	}
	// The pre/code wrapper must NOT be present.
	if strings.Contains(got, "<pre") || strings.Contains(got, "</pre>") {
		t.Fatalf("output should not contain chroma pre wrapper: %s", got)
	}
}

func TestHighlightGoUsesLanguageNameLexer(t *testing.T) {
	code := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	got := Highlight(code, "Go")
	if !strings.Contains(got, `class="chkn"`) || !strings.Contains(got, `class="chkd"`) {
		t.Fatalf("expected Go keyword token class in output: %s", got)
	}
	if strings.Contains(got, "<pre") || strings.Contains(got, "</pre>") {
		t.Fatalf("output should not contain chroma pre wrapper: %s", got)
	}
}

func TestHighlightUnknownLanguageFallsBack(t *testing.T) {
	code := "some unknown code"
	got := Highlight(code, "nonexistent-language-xyz")
	if !strings.Contains(got, "some unknown code") {
		t.Fatalf("fallback should output plain text: %s", got)
	}
}

func TestHighlightEmptyCode(t *testing.T) {
	got := Highlight("", "python")
	// Empty input is valid — chroma produces no content, wrapper stripped to "".
	// The renderer callers handle empty output gracefully.
	if strings.Contains(got, "<pre") {
		t.Errorf("empty code output should not contain pre wrapper: %s", got)
	}
}

func TestNormalizeLanguage(t *testing.T) {
	tests := []struct{ in, want string }{
		{"C++", "C++"},
		{"cpp", "C++"},
		{"cxx", "C++"},
		{"python", "python"},
		{"Python", "Python"},
		{"Rust", "Rust"},
		{"C#", "C#"},
		{"csharp", "C#"},
		{"js", "JavaScript"},
		{"javascript", "JavaScript"},
		{"bash", "Bash"},
		{"sh", "Bash"},
		{"  Go  ", "Go"},
	}
	for _, tt := range tests {
		if got := normalizeLanguage(tt.in); got != tt.want {
			t.Errorf("normalizeLanguage(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestThemeCSSContainsMonokaiRules(t *testing.T) {
	css := ThemeCSS()
	if !strings.Contains(css, "ch") {
		t.Fatalf("CSS should contain chroma class prefix: %s", css)
	}
	if !strings.Contains(css, "{") {
		t.Fatal("CSS should contain style rules")
	}
}
