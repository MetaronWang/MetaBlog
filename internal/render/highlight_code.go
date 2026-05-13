package render

import (
	"fmt"
	"html"
	"strings"

	"MetaBlog/internal/highlight"
	"MetaBlog/internal/latex/ast"
)

func (r *Renderer) renderCodeBlock(b *strings.Builder, n *ast.CodeBlock) {
	lang := n.Language
	if lang == "" {
		lang = "text"
	}
	highlighted := highlight.Highlight(n.Text, lang)
	if highlighted == "" {
		highlighted = html.EscapeString(n.Text)
	}
	lines := splitChromaLines(highlighted)

	b.WriteString(`<div class="code-block chchroma" data-wrap="false">`)
	b.WriteString(`<textarea class="code-block-source" hidden readonly>`)
	b.WriteString(html.EscapeString(n.Text))
	b.WriteString(`</textarea>`)
	b.WriteString(`<div class="code-block-header">`)
	b.WriteString(`<span class="code-block-lang">`)
	b.WriteString(html.EscapeString(lang))
	b.WriteString(`</span>`)
	b.WriteString(`<div class="code-block-actions">`)
	// Wrap button
	b.WriteString(`<button class="code-block-btn code-block-wrap-btn" title="Toggle word wrap" aria-label="Toggle word wrap"><svg width="20" height="20" viewBox="0 0 24 24" focusable="false"><path d="M4 19h6v-2H4v2zM20 5H4v2h16V5zm-3 6H4v2h13.25c1.1 0 2 .9 2 2s-.9 2-2 2H15v-2l-3 3 3 3v-2h2c2.21 0 4-1.79 4-4s-1.79-4-4-4z" fill="currentColor"/></svg></button>`)
	// Copy button
	b.WriteString(`<button class="code-block-btn code-block-copy-btn" title="Copy code" aria-label="Copy code"><svg width="20" height="20" viewBox="0 0 24 24" focusable="false"><path d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm3 4H8c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h11c1.1 0 2-.9 2-2V7c0-1.1-.9-2-2-2zm0 16H8V7h11v14z" fill="currentColor"/></svg></button>`)
	// Collapse button
	b.WriteString(`<button class="code-block-btn code-block-collapse-btn" title="Collapse code" aria-label="Collapse code"><svg width="20" height="20" viewBox="0 0 24 24" focusable="false"><path d="M12 8l-6 6 1.41 1.41L12 10.83l4.59 4.58L18 14z" fill="currentColor"/></svg></button>`)
	b.WriteString(`</div></div>`)
	b.WriteString(`<div class="code-block-body"><table class="code-block-table"><tbody>`)
	for i, line := range lines {
		b.WriteString(`<tr class="code-block-row">`)
		b.WriteString(`<td class="code-block-line-no">`)
		b.WriteString(html.EscapeString(fmt.Sprint(i + 1)))
		b.WriteString(`</td>`)
		b.WriteString(`<td class="code-block-line-code">`)
		cleaned := strings.ReplaceAll(line, "\n", "")
		if strings.TrimSpace(cleaned) == "" || strings.TrimSpace(stripHTMLTags(cleaned)) == "" {
			b.WriteString(" ")
		} else {
			b.WriteString(cleaned)
		}
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</tbody></table></div></div>`)
}

// splitChromaLines splits chroma HTML output into individual lines, reassembling
// correctly balanced span tags. chroma outputs lines as:
//
//	<span class="chline"><span class="chcl">LINE1\n</span></span><span class="chline"><span class="chcl">LINE2\n</span></span>
//
// Each \n is inside the innermost span. We split on </span></span><span class="chline">
// and re-wrap each fragment with the correct opening/closing span tags.
func splitChromaLines(html string) []string {
	delim := `</span></span><span class="chline">`
	parts := strings.Split(html, delim)
	if len(parts) <= 1 {
		// No chroma line spans; just split by newlines as fallback.
		if strings.TrimSpace(html) == "" {
			return []string{""}
		}
		return strings.Split(html, "\n")
	}
	lines := make([]string, 0, len(parts))
	openTag := `<span class="chline">`
	closeTag := `</span></span>`
	for i, part := range parts {
		var line string
		if i == 0 {
			// First part starts with <span class="chline">, needs </span></span>.
			line = part + closeTag
		} else if i == len(parts)-1 {
			// Last part ends with </span></span>, needs <span class="chline">.
			line = openTag + part
		} else {
			// Middle parts need both.
			line = openTag + part + closeTag
		}
		lines = append(lines, line)
	}
	return lines
}

// stripHTMLTags removes all HTML tags from s, returning only text content.
func stripHTMLTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return b.String()
}
