package highlight

import (
	"bytes"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

const (
	// CSSPrefix is the class prefix used for chroma-generated CSS rules and HTML classes.
	CSSPrefix = "ch"
	// DefaultTheme is the chroma style name used when none is specified.
	DefaultTheme = "monokai"
)

// Highlight returns syntax-highlighted HTML for the given code and language.
// The language name is case-insensitive. Unrecognized languages fall back to plain text.
// Returns only the inner highlighted content (no wrapping pre/code tags).
func Highlight(code, language string) string {
	lexer := lexers.Match(normalizeLanguage(language))
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	style := styles.Get(DefaultTheme)
	if style == nil {
		style = styles.Fallback
	}

	formatter := html.New(
		html.WithClasses(true),
		html.WithLineNumbers(false),
		html.TabWidth(4),
		html.ClassPrefix(CSSPrefix),
	)

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return escapeHTML(code)
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, iterator); err != nil {
		return escapeHTML(code)
	}
	raw := buf.String()
	// Strip chroma's default <pre class="chchroma"><code> wrapper.
	raw = strings.TrimPrefix(raw, `<pre class="`+CSSPrefix+`chroma"><code>`)
	raw = strings.TrimSuffix(raw, `</code></pre>`)
	return raw
}

// ThemeCSS returns the CSS rules for the current chroma theme, scoped with CSSPrefix.
func ThemeCSS() string {
	style := styles.Get(DefaultTheme)
	if style == nil {
		style = styles.Fallback
	}

	formatter := html.New(
		html.WithClasses(true),
		html.ClassPrefix(CSSPrefix),
	)

	var buf bytes.Buffer
	if err := formatter.WriteCSS(&buf, style); err != nil {
		return ""
	}
	return buf.String()
}

// normalizeLanguage maps common LaTeX-style language names to chroma-recognized names.
func normalizeLanguage(lang string) string {
	lang = strings.TrimSpace(lang)
	lower := strings.ToLower(lang)
	switch lower {
	case "c++", "cpp", "cxx":
		return "C++"
	case "c#", "csharp":
		return "C#"
	case "f#", "fsharp":
		return "F#"
	case "objective-c":
		return "Objective-C"
	case "vb.net", "vbnet":
		return "vb.net"
	case "js", "javascript":
		return "JavaScript"
	case "ts", "typescript":
		return "TypeScript"
	case "bash", "sh":
		return "Bash"
	case "powershell", "ps1":
		return "PowerShell"
	case "html":
		return "HTML"
	case "css":
		return "CSS"
	case "sql":
		return "SQL"
	case "yaml":
		return "YAML"
	case "json":
		return "JSON"
	case "xml":
		return "XML"
	case "makefile", "make":
		return "Makefile"
	case "dockerfile", "docker":
		return "Dockerfile"
	default:
		// chroma's Match is case-insensitive, so pass as-is
		return lang
	}
}

func escapeHTML(s string) string {
	var buf bytes.Buffer
	for _, r := range s {
		switch r {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		case '"':
			buf.WriteString("&#34;")
		case '\'':
			buf.WriteString("&#39;")
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
