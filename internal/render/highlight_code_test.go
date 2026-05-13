package render

import (
	"strings"
	"testing"

	"MetaBlog/internal/latex/ast"
)

func TestRenderCodeBlockMultiLine(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.CodeBlock{
				EnvName:  "lstlisting",
				Language: "python",
				Text:     "def hello():\n    print('world')\n    return True\n",
			},
		},
	}
	got := Render(doc)
	// Must contain the code block structure.
	if !strings.Contains(got, `<div class="code-block chchroma" data-wrap="false">`) {
		t.Fatalf("missing code-block div: %s", got)
	}
	if !strings.Contains(got, `<span class="code-block-lang">python</span>`) {
		t.Fatalf("missing language label: %s", got)
	}
	// Must have 3 line numbers (1 per source line).
	if strings.Count(got, `<td class="code-block-line-no">`) != 3 {
		t.Fatalf("expected 3 line numbers, got %d:\n%s",
			strings.Count(got, `<td class="code-block-line-no">`), got)
	}
	// Must contain the original code text.
	if !strings.Contains(got, "hello") {
		t.Fatalf("missing code text: %s", got)
	}
	if !strings.Contains(got, `<textarea class="code-block-source" hidden readonly>`) {
		t.Fatalf("missing raw code source for copy: %s", got)
	}
	// Must NOT contain chroma pre wrapper.
	if strings.Contains(got, `<pre class="chchroma">`) {
		t.Fatalf("output contains chroma pre wrapper: %s", got)
	}
}

func TestRenderCodeBlockEmptyCode(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.CodeBlock{
				EnvName:  "verbatim",
				Language: "",
				Text:     "",
			},
		},
	}
	got := Render(doc)
	// Should produce a code block with at least line number 1.
	if !strings.Contains(got, `<td class="code-block-line-no">1</td>`) {
		t.Fatalf("empty code should produce at least one line number: %s", got)
	}
	if !strings.Contains(got, `<span class="code-block-lang">text</span>`) {
		t.Fatalf("empty language should default to 'text': %s", got)
	}
}

func TestRenderCodeBlockCopySourcePreservesRawText(t *testing.T) {
	raw := "first\n\n    indented <tag>\n"
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.CodeBlock{EnvName: "lstlisting", Language: "text", Text: raw},
		},
	}
	got := Render(doc)
	want := `<textarea class="code-block-source" hidden readonly>first

    indented &lt;tag&gt;
</textarea>`
	if !strings.Contains(got, want) {
		t.Fatalf("raw copy textarea did not preserve source text; want %q in:\n%s", want, got)
	}
}

func TestRenderCodeBlockLanguageDisplayName(t *testing.T) {
	tests := []struct{ lang, want string }{
		{"C++", "C++"},
		{"C#", "C#"},
		{"go", "go"},
		{"python", "python"},
		{"Rust", "Rust"},
	}
	for _, tt := range tests {
		doc := &ast.Document{
			Title: []ast.Inline{&ast.Text{Value: "Test"}},
			Children: []ast.Block{
				&ast.CodeBlock{
					EnvName:  "lstlisting",
					Language: tt.lang,
					Text:     "code",
				},
			},
		}
		got := Render(doc)
		wantSpan := `<span class="code-block-lang">` + tt.want + `</span>`
		if !strings.Contains(got, wantSpan) {
			t.Errorf("language %q: expected display name %q in output", tt.lang, tt.want)
		}
	}
}

func TestRenderCodeBlockNoStrayHTMLCharacters(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.CodeBlock{EnvName: "lstlisting", Language: "go", Text: "x := 1"},
		},
	}
	got := Render(doc)
	// Verify the HTML structure around the CSS links is clean.
	// The <head> should not contain stray "> characters between CSS links.
	if strings.Contains(got, `">\n">`) {
		t.Fatalf("HTML head contains stray close quote: %s", got)
	}
	// Verify both CSS links exist.
	if !strings.Contains(got, `static/style.css`) {
		t.Fatal("missing style.css link")
	}
	if !strings.Contains(got, `static/chroma-theme.css`) {
		t.Fatal("missing chroma-theme.css link")
	}
}

func TestRenderCodeBlockChromaCSSClassPrefix(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.CodeBlock{EnvName: "lstlisting", Language: "go", Text: "package main"},
		},
	}
	got := Render(doc)
	// Chroma CSS classes should use the "ch" prefix.
	if !strings.Contains(got, `class="ch`) {
		t.Fatalf("chroma CSS class prefix 'ch' not found: %s", got)
	}
}

func TestRenderCodeBlockJSIncluded(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.CodeBlock{EnvName: "minted", Language: "python", Text: "x = 1"},
		},
	}
	got := Render(doc)
	// The code block JS must be present.
	if !strings.Contains(got, "code-block-wrap-btn") {
		t.Fatal("missing code block JS (wrap button reference)")
	}
	if !strings.Contains(got, "code-block-copy-btn") {
		t.Fatal("missing code block JS (copy button reference)")
	}
	if !strings.Contains(got, "code-block-collapse-btn") {
		t.Fatal("missing code block JS (collapse button reference)")
	}
	if !strings.Contains(got, "navigator.clipboard") {
		t.Fatal("missing clipboard API usage in JS")
	}
	if !strings.Contains(got, "code-block-source") {
		t.Fatal("copy script should use raw source textarea")
	}
}
