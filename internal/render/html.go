package render

import (
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"MetaBlog/internal/latex/ast"
)

type Renderer struct {
	doc          *ast.Document
	labels       map[string]labelTarget
	missingRefs  map[string]bool
	citeNums     map[string]int
	citeOrder    []string
	missingCites map[string]bool
}

type labelTarget struct {
	AnchorID string
	Number   string
}

type Options struct {
	AssetPrefix string
	HeaderHTML  string
	BodyClass   string
	IconHref    string
}

func Render(doc *ast.Document) string {
	return RenderWithOptions(doc, Options{})
}

func RenderWithOptions(doc *ast.Document, opts Options) string {
	r := &Renderer{
		doc:          doc,
		labels:       map[string]labelTarget{},
		missingRefs:  map[string]bool{},
		citeNums:     map[string]int{},
		missingCites: map[string]bool{},
	}
	r.collectCitations(doc.Children)
	r.number(doc)

	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<title>")
	b.WriteString(html.EscapeString(inlineText(doc.Title)))
	b.WriteString("</title>\n<link rel=\"stylesheet\" href=\"")
	b.WriteString(html.EscapeString(joinURL(opts.AssetPrefix, "static/fonts.css")))
	b.WriteString("\">\n<link rel=\"stylesheet\" href=\"")
	b.WriteString(html.EscapeString(joinURL(opts.AssetPrefix, "static/style.css")))
	b.WriteString("\">\n")
	b.WriteString("\n<link rel=\"stylesheet\" href=\"")
	b.WriteString(html.EscapeString(joinURL(opts.AssetPrefix, "static/chroma-theme.css")))
	b.WriteString("\">\n")
	if opts.IconHref != "" {
		b.WriteString(`<link rel="icon" href="`)
		b.WriteString(html.EscapeString(joinURL(opts.AssetPrefix, opts.IconHref)))
		b.WriteString(`">`)
		b.WriteByte('\n')
	}
	b.WriteString(`<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/katex@0.16.11/dist/katex.min.css">
<script defer src="https://cdn.jsdelivr.net/npm/katex@0.16.11/dist/katex.min.js"></script>
<script defer src="https://cdn.jsdelivr.net/npm/katex@0.16.11/dist/contrib/auto-render.min.js"></script>
<script>
document.addEventListener("DOMContentLoaded", function () {
  renderMathInElement(document.body, {
    delimiters: [
      {left: "\\[", right: "\\]", display: true},
      {left: "\\(", right: "\\)", display: false},
      {left: "$", right: "$", display: false}
    ],
    throwOnError: false,
    strict: "ignore",
    trust: true
  });
});
</script>
`)
	b.WriteString("</head>\n<body")
	if opts.BodyClass != "" {
		b.WriteString(` class="`)
		b.WriteString(html.EscapeString(opts.BodyClass))
		b.WriteString(`"`)
	}
	b.WriteString(">\n")
	if opts.HeaderHTML != "" {
		b.WriteString(opts.HeaderHTML)
	}
	b.WriteString("<main class=\"page\"><article class=\"article\">\n")
	if len(doc.Title) > 0 {
		b.WriteString("<header class=\"article-header\"><h1")
		writeStyleAttr(&b, alignStyle(doc.TitleAlign))
		b.WriteString(">")
		b.WriteString(r.renderInlines(doc.Title))
		b.WriteString("</h1>")
		r.renderAuthorMetadata(&b)
		if len(doc.Abstract) > 0 {
			r.renderAbstractBlock(&b, doc.Abstract)
		}
		if len(doc.Keywords) > 0 {
			r.renderKeywordsBlock(&b, doc.Keywords)
		}
		b.WriteString("</header>")
	}
	if toc := r.renderTOC(doc.Children); toc != "" {
		b.WriteString(toc)
	}
	r.renderBlocks(&b, doc.Children)
	b.WriteString("</article></main>\n")
	b.WriteString(tocToggleScript())
	b.WriteString(codeBlockScript())
	b.WriteString("</body>\n</html>\n")
	return b.String()
}

func joinURL(prefix, path string) string {
	prefix = strings.TrimRight(prefix, "/")
	path = strings.TrimLeft(path, "/")
	if prefix == "" {
		return path
	}
	return prefix + "/" + path
}

func (r *Renderer) renderAuthorMetadata(b *strings.Builder) {
	if r == nil || r.doc == nil || (len(r.doc.Authors) == 0 && len(r.doc.Institutions) == 0) {
		return
	}
	instNums := map[string]int{}
	for _, inst := range r.doc.Institutions {
		instNums[inst.Code] = inst.Number
		for _, alias := range inst.Aliases {
			instNums[alias] = inst.Number
		}
	}
	b.WriteString(`<section class="article-meta">`)
	if len(r.doc.Authors) > 0 {
		b.WriteString(`<div class="author-list">`)
		for _, author := range r.doc.Authors {
			b.WriteString(`<div class="author-card">`)
			b.WriteString(`<div class="author-name">`)
			b.WriteString(r.renderInlines(author.Name))
			if nums := r.authorInstitutionNumbers(author, instNums); len(nums) > 0 {
				b.WriteString(`<sup class="institution-ref">`)
				for j, num := range nums {
					if j > 0 {
						b.WriteString(",")
					}
					b.WriteString(html.EscapeString(intString(num)))
				}
				b.WriteString(`</sup>`)
			}
			b.WriteString(`</div>`)
			if author.Email != "" {
				escaped := html.EscapeString(author.Email)
				b.WriteString(`<a class="author-email" href="mailto:`)
				b.WriteString(escaped)
				b.WriteString(`">`)
				b.WriteString(escaped)
				b.WriteString(`</a>`)
			}
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
	}
	if len(r.doc.Institutions) > 0 {
		b.WriteString(`<ol class="institution-list">`)
		for _, inst := range r.doc.Institutions {
			b.WriteString(`<li value="`)
			b.WriteString(html.EscapeString(intString(inst.Number)))
			b.WriteString(`">`)
			b.WriteString(r.renderInlines(inst.Info))
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ol>`)
	}
	if groups := r.authorAttributeGroups(); len(groups) > 0 {
		b.WriteString(`<dl class="author-attributes">`)
		for _, group := range groups {
			b.WriteString(`<dt>`)
			b.WriteString(html.EscapeString(group.name + ": "))
			b.WriteString(`</dt><dd>`)
			for i, name := range group.authorNames {
				if i > 0 {
					b.WriteString(`, `)
				}
				b.WriteString(name)
			}
			b.WriteString(`</dd>`)
		}
		b.WriteString(`</dl>`)
	}
	b.WriteString(`</section>`)
}

func (r *Renderer) authorInstitutionNumbers(author ast.Author, instNums map[string]int) []int {
	var nums []int
	seen := map[int]bool{}
	for _, code := range author.InstitutionCodes {
		num, ok := instNums[code]
		if !ok {
			if r.doc != nil {
				r.doc.Warnings = append(r.doc.Warnings, "undefined institution code: "+code)
			}
			continue
		}
		if !seen[num] {
			nums = append(nums, num)
			seen[num] = true
		}
	}
	return nums
}

func (r *Renderer) authorEmails() []string {
	var emails []string
	for _, author := range r.doc.Authors {
		if author.Email != "" {
			emails = append(emails, author.Email)
		}
	}
	return emails
}

type authorAttributeGroup struct {
	name        string
	authorNames []string
}

func (r *Renderer) authorAttributeGroups() []authorAttributeGroup {
	order := []string{}
	groups := map[string][]string{}
	for _, author := range r.doc.Authors {
		name := html.EscapeString(inlineText(author.Name))
		for _, attr := range author.Attributes {
			if attr == "" {
				continue
			}
			if _, ok := groups[attr]; !ok {
				order = append(order, attr)
			}
			groups[attr] = append(groups[attr], name)
		}
	}
	out := make([]authorAttributeGroup, 0, len(order))
	for _, attr := range order {
		out = append(out, authorAttributeGroup{name: attr, authorNames: groups[attr]})
	}
	return out
}

func (r *Renderer) number(doc *ast.Document) {
	var secNums []int
	var appNums []int
	figNo, eqNo, tableNo, algNo := 0, 0, 0, 0
	var walk func([]ast.Block)
	var walkTableContent func([]ast.Block)
	walk = func(blocks []ast.Block) {
		for _, block := range blocks {
			switch n := block.(type) {
			case *ast.Section:
				if n.Appendix {
					for len(appNums) < n.Level {
						appNums = append(appNums, 0)
					}
					appNums = appNums[:n.Level]
					appNums[n.Level-1]++
					n.Number = appendixNumber(appNums)
				} else {
					for len(secNums) < n.Level {
						secNums = append(secNums, 0)
					}
					secNums = secNums[:n.Level]
					secNums[n.Level-1]++
					n.Number = joinNumbers(secNums)
				}
				n.AnchorID = anchor(n.Label, "section-"+n.Number)
				if n.Label != "" {
					r.labels[n.Label] = labelTarget{AnchorID: n.AnchorID, Number: n.Number}
				}
				walk(n.Children)
			case *ast.Figure:
				figNo++
				n.Number = intString(figNo)
				n.AnchorID = anchor(n.Label, "figure-"+n.Number)
				if n.Label != "" {
					r.labels[n.Label] = labelTarget{AnchorID: n.AnchorID, Number: n.Number}
				}
				for i, sub := range n.Subfigures {
					if sub == nil {
						continue
					}
					sub.Number = n.Number + "." + subfigureLetter(i)
					sub.AnchorID = anchor(sub.Label, "figure-"+sub.Number)
					if sub.Label != "" {
						r.labels[sub.Label] = labelTarget{AnchorID: sub.AnchorID, Number: sub.Number}
					}
				}
			case *ast.Table:
				tableNo++
				n.Number = intString(tableNo)
				n.AnchorID = anchor(n.Label, "table-"+n.Number)
				if n.Label != "" {
					r.labels[n.Label] = labelTarget{AnchorID: n.AnchorID, Number: n.Number}
				}
				walkTableContent(n.Children)
				for i, sub := range n.Subtables {
					if sub == nil {
						continue
					}
					sub.Number = n.Number + "." + subfigureLetter(i)
					sub.AnchorID = anchor(sub.Label, "table-"+sub.Number)
					if sub.Label != "" {
						r.labels[sub.Label] = labelTarget{AnchorID: sub.AnchorID, Number: sub.Number}
					}
					walkTableContent(sub.Blocks)
				}
			case *ast.DisplayMath:
				eqNo++
				n.Number = intString(eqNo)
				n.AnchorID = anchor(n.Label, "equation-"+n.Number)
				if n.Label != "" {
					r.labels[n.Label] = labelTarget{AnchorID: n.AnchorID, Number: n.Number}
				}
			case *ast.ComplexHTML:
				if strings.Contains(n.EnvName, "algorithm") {
					algNo++
					n.Number = intString(algNo)
					n.AnchorID = anchor(n.Label, "algorithm-"+n.Number)
				} else {
					tableNo++
					n.Number = intString(tableNo)
					n.AnchorID = anchor(n.Label, "table-"+n.Number)
				}
				if n.Label != "" {
					r.labels[n.Label] = labelTarget{AnchorID: n.AnchorID, Number: n.Number}
				}
			case *ast.StyledBlock:
				walk(n.Children)
			case *ast.AbstractBlock:
				walk(n.Children)
			case *ast.EnvironmentBlock:
				walk(n.Children)
			case *ast.List:
				for _, item := range n.Items {
					walk(item.Blocks)
				}
			case *ast.TCB:
				walk(n.Children)
			}
		}
	}
	walkTableContent = func(blocks []ast.Block) {
		for _, block := range blocks {
			if complex, ok := block.(*ast.ComplexHTML); ok && isTabularComplex(complex) {
				continue
			}
			walk([]ast.Block{block})
		}
	}
	walk(doc.Children)
}

func (r *Renderer) renderBlocks(b *strings.Builder, blocks []ast.Block) {
	for _, block := range blocks {
		switch n := block.(type) {
		case *ast.Section:
			level := n.Level + 1
			if level > 6 {
				level = 6
			}
			b.WriteString(`<section id="`)
			b.WriteString(html.EscapeString(n.AnchorID))
			b.WriteString(`" class="section level-`)
			b.WriteString(intString(n.Level))
			b.WriteString(`"><h`)
			b.WriteString(intString(level))
			writeStyleAttr(b, alignStyle(n.TitleAlign))
			b.WriteString(`><span class="section-number">`)
			b.WriteString(html.EscapeString(sectionNumberLabel(n)))
			b.WriteString(`</span><span class="section-title">`)
			b.WriteString(r.renderInlines(n.Title))
			b.WriteString(`</span>`)
			b.WriteString(`</h`)
			b.WriteString(intString(level))
			b.WriteString(`>`)
			r.renderBlocks(b, n.Children)
			b.WriteString(`</section>`)
		case *ast.Paragraph:
			if len(n.Inlines) == 0 {
				continue
			}
			b.WriteString("<p")
			writeStyleAttr(b, alignStyle(n.Align))
			b.WriteString(">")
			b.WriteString(r.renderInlines(n.Inlines))
			b.WriteString("</p>")
		case *ast.AbstractBlock:
			r.renderAbstractBlock(b, n.Children)
		case *ast.KeywordsBlock:
			r.renderKeywordsBlock(b, n.Inlines)
		case *ast.EnvironmentBlock:
			r.renderBlocks(b, n.Children)
		case *ast.References:
			r.renderReferences(b)
		case *ast.DisplayMath:
			b.WriteString(`<div id="`)
			b.WriteString(html.EscapeString(n.AnchorID))
			b.WriteString(`" class="math display">\[`)
			b.WriteString(html.EscapeString(n.TeX))
			b.WriteString(`\]`)
			if n.Number != "" {
				b.WriteString(`<span class="equation-number">(`)
				b.WriteString(html.EscapeString(n.Number))
				b.WriteString(`)</span>`)
			}
			b.WriteString(`</div>`)
		case *ast.Figure:
			b.WriteString(`<figure id="`)
			b.WriteString(html.EscapeString(n.AnchorID))
			b.WriteString(`">`)
			images := n.Images
			if len(images) == 0 && n.Image != nil {
				images = []*ast.Image{n.Image}
			}
			if len(images) > 1 {
				b.WriteString(`<div class="figure-grid">`)
			}
			subfigures := subfigureByImageIndex(n.Subfigures)
			for i, image := range images {
				if image == nil {
					continue
				}
				widthStyle := imageWidthStyle(image)
				if sub := subfigures[i]; sub != nil {
					b.WriteString(`<div`)
					if sub.AnchorID != "" {
						b.WriteString(` id="`)
						b.WriteString(html.EscapeString(sub.AnchorID))
						b.WriteString(`"`)
					}
					b.WriteString(` class="subfigure"`)
					if widthStyle != "" {
						b.WriteString(` style="`)
						b.WriteString(html.EscapeString(subfigureWidthStyle(widthStyle)))
						b.WriteString(`"`)
					}
					b.WriteString(`>`)
				}
				b.WriteString(`<img src="`)
				src := image.OutputPath
				if src == "" {
					src = image.SourcePath
				}
				b.WriteString(html.EscapeString(src))
				b.WriteString(`" alt="`)
				b.WriteString(html.EscapeString(r.plainInlines(n.Caption)))
				b.WriteString(`"`)
				if subfigures[i] == nil && widthStyle != "" {
					b.WriteString(` style="`)
					b.WriteString(html.EscapeString(widthStyle))
					b.WriteString(`"`)
				}
				b.WriteString(`>`)
				if sub := subfigures[i]; sub != nil {
					b.WriteString(`<div class="subfigure-caption">(`)
					b.WriteString(html.EscapeString(subfigureDisplayNumber(sub.Number)))
					b.WriteString(`)`)
					if len(sub.Caption) > 0 {
						b.WriteString(` `)
						b.WriteString(r.renderInlines(sub.Caption))
					}
					b.WriteString(`</div>`)
					b.WriteString(`</div>`)
					if sub.BreakAfter {
						b.WriteString(`<div class="subfigure-break"></div>`)
					}
				}
			}
			if len(images) > 1 {
				b.WriteString(`</div>`)
			}
			if len(n.Caption) > 0 {
				b.WriteString(`<figcaption`)
				writeStyleAttr(b, alignStyle(n.CaptionAlign))
				b.WriteString(`><strong>Figure `)
				b.WriteString(html.EscapeString(n.Number))
				b.WriteString(`.</strong> `)
				b.WriteString(r.renderInlines(n.Caption))
				b.WriteString(`</figcaption>`)
			}
			b.WriteString(`</figure>`)
		case *ast.Table:
			b.WriteString(`<figure id="`)
			b.WriteString(html.EscapeString(n.AnchorID))
			b.WriteString(`" class="table-float">`)
			if len(n.Subtables) > 0 {
				b.WriteString(`<div class="table-grid">`)
				for _, sub := range n.Subtables {
					if sub == nil {
						continue
					}
					b.WriteString(`<div`)
					if sub.AnchorID != "" {
						b.WriteString(` id="`)
						b.WriteString(html.EscapeString(sub.AnchorID))
						b.WriteString(`"`)
					}
					b.WriteString(` class="subtable">`)
					r.renderBlocks(b, sub.Blocks)
					if len(sub.Caption) > 0 {
						b.WriteString(`<div class="subtable-caption">(`)
						b.WriteString(html.EscapeString(subfigureDisplayNumber(sub.Number)))
						b.WriteString(`) `)
						b.WriteString(r.renderInlines(sub.Caption))
						b.WriteString(`</div>`)
					}
					b.WriteString(`</div>`)
					if sub.BreakAfter {
						b.WriteString(`<div class="subfigure-break"></div>`)
					}
				}
				b.WriteString(`</div>`)
			} else {
				r.renderBlocks(b, n.Children)
			}
			if len(n.Caption) > 0 {
				b.WriteString(`<figcaption`)
				writeStyleAttr(b, alignStyle(n.CaptionAlign))
				b.WriteString(`><strong>Table `)
				b.WriteString(html.EscapeString(n.Number))
				b.WriteString(`.</strong> `)
				b.WriteString(r.renderInlines(n.Caption))
				b.WriteString(`</figcaption>`)
			}
			b.WriteString(`</figure>`)
		case *ast.List:
			if n.Kind == "description" {
				b.WriteString("<dl>")
				for _, item := range n.Items {
					b.WriteString("<dt>")
					b.WriteString(r.renderInlines(item.Label))
					b.WriteString("</dt><dd>")
					r.renderBlocks(b, item.Blocks)
					b.WriteString("</dd>")
				}
				b.WriteString("</dl>")
			} else if n.Ordered {
				b.WriteString("<ol>")
				for _, item := range n.Items {
					b.WriteString("<li>")
					r.renderBlocks(b, item.Blocks)
					b.WriteString("</li>")
				}
				b.WriteString("</ol>")
			} else {
				b.WriteString("<ul>")
				for _, item := range n.Items {
					b.WriteString("<li>")
					r.renderBlocks(b, item.Blocks)
					b.WriteString("</li>")
				}
				b.WriteString("</ul>")
			}
		case *ast.StyledBlock:
			style := blockStyle(n)
			if style == "" {
				r.renderBlocks(b, n.Children)
				continue
			}
			b.WriteString(`<div class="styled-block" style="`)
			b.WriteString(html.EscapeString(style))
			b.WriteString(`">`)
			r.renderBlocks(b, n.Children)
			b.WriteString(`</div>`)
		case *ast.TCB:
			b.WriteString(`<details class="tcb" open style="`)
			b.WriteString(html.EscapeString(tcbStyle(n)))
			b.WriteString(`"><summary class="tcb-title"><span class="tcb-title-text">`)
			b.WriteString(r.renderInlines(n.Title))
			b.WriteString(`</span><span class="tcb-toggle" aria-hidden="true"></span></summary><div class="tcb-body">`)
			r.renderBlocks(b, n.Children)
			b.WriteString(`</div></details>`)
		case *ast.CodeBlock:
			r.renderCodeBlock(b, n)
		case *ast.ComplexHTML:
			if n.Number != "" {
				b.WriteString(`<section id="`)
				b.WriteString(html.EscapeString(n.AnchorID))
				b.WriteString(`" class="complex-wrapper">`)
			} else {
				b.WriteString(`<div class="complex-wrapper">`)
			}
			if n.HTML != "" {
				htmlText := r.resolveComplexRefs(n.HTML, n.RawTeX)
				htmlText = r.applyComplexNumber(htmlText, n)
				b.WriteString(htmlText)
			} else {
				b.WriteString(`<pre><code>`)
				b.WriteString(html.EscapeString(n.RawTeX))
				b.WriteString(`</code></pre>`)
			}
			if n.Number != "" {
				b.WriteString(`</section>`)
			} else {
				b.WriteString(`</div>`)
			}
		}
	}
}

func (r *Renderer) renderAbstractBlock(b *strings.Builder, children []ast.Block) {
	b.WriteString(`<section class="abstract"><h2>Abstract</h2>`)
	r.renderBlocks(b, children)
	b.WriteString(`</section>`)
}

func (r *Renderer) renderKeywordsBlock(b *strings.Builder, inlines []ast.Inline) {
	if len(inlines) == 0 {
		return
	}
	b.WriteString(`<p class="keywords"><strong>Keywords:</strong> `)
	b.WriteString(r.renderInlines(inlines))
	b.WriteString(`</p>`)
}

func (r *Renderer) renderInlines(inlines []ast.Inline) string {
	var b strings.Builder
	for i, node := range inlines {
		if i > 0 && needsSpaceBetween(inlines[i-1], node) {
			b.WriteByte(' ')
		}
		switch n := node.(type) {
		case *ast.Text:
			b.WriteString(html.EscapeString(n.Value))
		case *ast.Bold:
			b.WriteString("<strong>")
			b.WriteString(r.renderInlines(n.Children))
			b.WriteString("</strong>")
		case *ast.Italic:
			b.WriteString("<em>")
			b.WriteString(r.renderInlines(n.Children))
			b.WriteString("</em>")
		case *ast.Styled:
			style := inlineStyle(n)
			if style == "" {
				b.WriteString(r.renderInlines(n.Children))
			} else {
				b.WriteString(`<span style="`)
				b.WriteString(html.EscapeString(style))
				b.WriteString(`">`)
				b.WriteString(r.renderInlines(n.Children))
				b.WriteString(`</span>`)
			}
		case *ast.InlineMath:
			b.WriteString(`<span class="math inline">\(`)
			b.WriteString(html.EscapeString(n.TeX))
			b.WriteString(`\)</span>`)
		case *ast.Link:
			b.WriteString(`<a href="`)
			b.WriteString(html.EscapeString(safeLinkURL(n.URL)))
			b.WriteString(`">`)
			b.WriteString(r.renderInlines(n.Children))
			b.WriteString(`</a>`)
		case *ast.Cite:
			b.WriteString(r.renderCites(n.Keys))
		case *ast.Ref:
			b.WriteString(r.renderRef(n.Key))
		case *ast.Footnote:
			b.WriteString(`<sup class="footnote"><button type="button" class="footnote-marker" aria-label="Footnote">`)
			b.WriteString(commentIcon())
			b.WriteString(`</button><span class="footnote-popover" role="tooltip">`)
			b.WriteString(r.renderInlines(n.Children))
			b.WriteString(`</span></sup>`)
		}
	}
	return b.String()
}

func (r *Renderer) renderTOCInlines(inlines []ast.Inline) string {
	var b strings.Builder
	for i, node := range inlines {
		if i > 0 && needsSpaceBetween(inlines[i-1], node) {
			b.WriteByte(' ')
		}
		switch n := node.(type) {
		case *ast.Text:
			b.WriteString(html.EscapeString(n.Value))
		case *ast.Bold:
			b.WriteString("<strong>")
			b.WriteString(r.renderTOCInlines(n.Children))
			b.WriteString("</strong>")
		case *ast.Italic:
			b.WriteString("<em>")
			b.WriteString(r.renderTOCInlines(n.Children))
			b.WriteString("</em>")
		case *ast.Styled:
			style := inlineStyle(n)
			if style == "" {
				b.WriteString(r.renderTOCInlines(n.Children))
			} else {
				b.WriteString(`<span style="`)
				b.WriteString(html.EscapeString(style))
				b.WriteString(`">`)
				b.WriteString(r.renderTOCInlines(n.Children))
				b.WriteString(`</span>`)
			}
		case *ast.InlineMath:
			b.WriteString(`<span class="math inline">\(`)
			b.WriteString(html.EscapeString(n.TeX))
			b.WriteString(`\)</span>`)
		case *ast.Link:
			b.WriteString(r.renderTOCInlines(n.Children))
		case *ast.Cite:
			b.WriteString(html.EscapeString(r.citesText(n.Keys)))
		case *ast.Ref:
			b.WriteString(html.EscapeString(r.refText(n.Key)))
		case *ast.Footnote:
			b.WriteString(r.renderTOCInlines(n.Children))
		}
	}
	return b.String()
}

func commentIcon() string {
	return `<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M21 15a4 4 0 0 1-4 4H8l-5 3V7a4 4 0 0 1 4-4h10a4 4 0 0 1 4 4z"></path></svg>`
}

func (r *Renderer) collectCitations(blocks []ast.Block) {
	var walkInlines func([]ast.Inline)
	walkInlines = func(inlines []ast.Inline) {
		for _, node := range inlines {
			switch n := node.(type) {
			case *ast.Cite:
				for _, key := range n.Keys {
					key = strings.TrimSpace(key)
					if key == "" || r.citeNums[key] != 0 {
						continue
					}
					r.citeOrder = append(r.citeOrder, key)
					r.citeNums[key] = len(r.citeOrder)
				}
			case *ast.Bold:
				walkInlines(n.Children)
			case *ast.Italic:
				walkInlines(n.Children)
			case *ast.Styled:
				walkInlines(n.Children)
			case *ast.Link:
				walkInlines(n.Children)
			case *ast.Footnote:
				walkInlines(n.Children)
			}
		}
	}
	var walkBlocks func([]ast.Block)
	walkBlocks = func(blocks []ast.Block) {
		for _, block := range blocks {
			switch n := block.(type) {
			case *ast.Section:
				walkInlines(n.Title)
				walkBlocks(n.Children)
			case *ast.Paragraph:
				walkInlines(n.Inlines)
			case *ast.Figure:
				walkInlines(n.Caption)
			case *ast.Table:
				walkInlines(n.Caption)
				walkBlocks(n.Children)
				for _, sub := range n.Subtables {
					if sub != nil {
						walkInlines(sub.Caption)
						walkBlocks(sub.Blocks)
					}
				}
			case *ast.StyledBlock:
				walkBlocks(n.Children)
			case *ast.AbstractBlock:
				walkBlocks(n.Children)
			case *ast.KeywordsBlock:
				walkInlines(n.Inlines)
			case *ast.EnvironmentBlock:
				walkBlocks(n.Children)
			case *ast.TCB:
				walkInlines(n.Title)
				walkBlocks(n.Children)
			case *ast.List:
				for _, item := range n.Items {
					walkBlocks(item.Blocks)
				}
			}
		}
	}
	walkBlocks(blocks)
}

func (r *Renderer) renderCite(key string) string {
	no := r.citeNums[key]
	if no == 0 {
		r.warnMissingCite(key)
		return `<span class="cite">[?]</span>`
	}
	if r.doc != nil && r.doc.References != nil {
		if _, ok := r.doc.References[key]; !ok {
			r.warnMissingCite(key)
		}
	}
	return `<a class="cite" href="#ref-` + html.EscapeString(anchor(key, key)) + `">[` + html.EscapeString(intString(no)) + `]</a>`
}

func (r *Renderer) renderCites(keys []string) string {
	parts := r.citationParts(keys)
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for i, part := range parts {
		if i > 0 {
			b.WriteString(",")
		}
		if part.missing {
			b.WriteString(`<span class="cite">[?]</span>`)
			continue
		}
		if part.start == part.end {
			b.WriteString(`<a class="cite" href="#ref-`)
			b.WriteString(html.EscapeString(anchor(part.startKey, part.startKey)))
			b.WriteString(`">[`)
			b.WriteString(html.EscapeString(intString(part.start)))
			b.WriteString(`]</a>`)
			continue
		}
		b.WriteString(`<span class="cite-range"><a class="cite" href="#ref-`)
		b.WriteString(html.EscapeString(anchor(part.startKey, part.startKey)))
		b.WriteString(`">[`)
		b.WriteString(html.EscapeString(intString(part.start)))
		b.WriteString(`]</a>-<a class="cite" href="#ref-`)
		b.WriteString(html.EscapeString(anchor(part.endKey, part.endKey)))
		b.WriteString(`">[`)
		b.WriteString(html.EscapeString(intString(part.end)))
		b.WriteString(`]</a></span>`)
	}
	return b.String()
}

func (r *Renderer) citeText(key string) string {
	no := r.citeNums[key]
	if no == 0 {
		r.warnMissingCite(key)
		return "[?]"
	}
	if r.doc != nil && r.doc.References != nil {
		if _, ok := r.doc.References[key]; !ok {
			r.warnMissingCite(key)
		}
	}
	return "[" + intString(no) + "]"
}

func (r *Renderer) citesText(keys []string) string {
	parts := r.citationParts(keys)
	if len(parts) == 0 {
		return ""
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.missing {
			out = append(out, "[?]")
		} else if part.start == part.end {
			out = append(out, "["+intString(part.start)+"]")
		} else {
			out = append(out, "["+intString(part.start)+"]-["+intString(part.end)+"]")
		}
	}
	return strings.Join(out, ",")
}

type citationPart struct {
	start    int
	end      int
	startKey string
	endKey   string
	missing  bool
}

type citationRef struct {
	key string
	no  int
}

func (r *Renderer) citationParts(keys []string) []citationPart {
	var refs []citationRef
	var parts []citationPart
	seen := map[int]bool{}
	for _, key := range keys {
		no := r.citeNums[key]
		if no == 0 {
			r.warnMissingCite(key)
			parts = append(parts, citationPart{missing: true})
			continue
		}
		if r.doc != nil && r.doc.References != nil {
			if _, ok := r.doc.References[key]; !ok {
				r.warnMissingCite(key)
			}
		}
		if seen[no] {
			continue
		}
		seen[no] = true
		refs = append(refs, citationRef{key: key, no: no})
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].no < refs[j].no })
	for i := 0; i < len(refs); {
		start := refs[i]
		end := start
		j := i + 1
		for j < len(refs) && refs[j].no == end.no+1 {
			end = refs[j]
			j++
		}
		if end.no-start.no+1 >= 3 {
			parts = append(parts, citationPart{
				start:    start.no,
				end:      end.no,
				startKey: start.key,
				endKey:   end.key,
			})
		} else {
			for k := i; k < j; k++ {
				parts = append(parts, citationPart{
					start:    refs[k].no,
					end:      refs[k].no,
					startKey: refs[k].key,
					endKey:   refs[k].key,
				})
			}
		}
		i = j
	}
	return parts
}

func (r *Renderer) renderReferences(b *strings.Builder) {
	if len(r.citeOrder) == 0 {
		return
	}
	b.WriteString(`<section id="references" class="references"><h2>References</h2><ol>`)
	for _, key := range r.citeOrder {
		no := r.citeNums[key]
		entry, ok := r.doc.References[key]
		if !ok {
			r.warnMissingCite(key)
		}
		b.WriteString(`<li id="ref-`)
		b.WriteString(html.EscapeString(anchor(key, key)))
		b.WriteString(`" value="`)
		b.WriteString(html.EscapeString(intString(no)))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(formatIEEE(entry, key)))
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ol></section>`)
}

func (r *Renderer) warnMissingCite(key string) {
	if r == nil || r.doc == nil || key == "" || r.missingCites[key] {
		return
	}
	r.missingCites[key] = true
	r.doc.Warnings = append(r.doc.Warnings, "missing bibliography entry: "+key)
}

func (r *Renderer) renderRef(key string) string {
	if target, ok := r.labels[key]; ok && target.AnchorID != "" {
		var b strings.Builder
		b.WriteString(`<a href="#`)
		b.WriteString(html.EscapeString(target.AnchorID))
		b.WriteString(`">`)
		if target.Number != "" {
			b.WriteString(html.EscapeString(target.Number))
		} else {
			b.WriteString(html.EscapeString(key))
		}
		b.WriteString(`</a>`)
		return b.String()
	}
	r.warnMissingRef(key)
	return html.EscapeString(key)
}

func (r *Renderer) refText(key string) string {
	if target, ok := r.labels[key]; ok {
		if target.Number != "" {
			return target.Number
		}
		return key
	}
	r.warnMissingRef(key)
	return key
}

var rawRefRE = regexp.MustCompile(`\\(?:eq)?ref\s*\{([^}]*)\}`)
var lateXMLMissingRefRE = regexp.MustCompile(`(?s)<span class="ltx_ref ltx_missing_label[^"]*">LABEL:[^<]*</span>`)
var lateXMLTableCaptionTagRE = regexp.MustCompile(`(?is)<span class="ltx_tag ltx_tag_table">Table[^<]*</span>`)
var lateXMLAlgorithmCaptionTagRE = regexp.MustCompile(`(?is)<span class="ltx_tag ltx_tag_float">\s*(?:<span class="[^"]*">)?Algorithm[^<]*(?:</span>)?\s*</span>`)

func (r *Renderer) resolveComplexRefs(htmlText, rawTeX string) string {
	if htmlText == "" || rawTeX == "" {
		return htmlText
	}
	matches := rawRefRE.FindAllStringSubmatch(rawTeX, -1)
	if len(matches) == 0 {
		return htmlText
	}
	i := 0
	return lateXMLMissingRefRE.ReplaceAllStringFunc(htmlText, func(_ string) string {
		if i >= len(matches) {
			return ""
		}
		key := strings.TrimSpace(matches[i][1])
		i++
		return r.renderRef(key)
	})
}

func (r *Renderer) applyComplexNumber(htmlText string, n *ast.ComplexHTML) string {
	if htmlText == "" || n == nil || n.Number == "" {
		return htmlText
	}
	if strings.Contains(n.EnvName, "algorithm") {
		replacement := `<span class="ltx_tag ltx_tag_float"><span class="ltx_text ltx_font_bold">Algorithm ` + html.EscapeString(n.Number) + `</span> </span>`
		return lateXMLAlgorithmCaptionTagRE.ReplaceAllString(htmlText, replacement)
	}
	replacement := `<span class="ltx_tag ltx_tag_table">Table ` + html.EscapeString(n.Number) + `: </span>`
	return lateXMLTableCaptionTagRE.ReplaceAllString(htmlText, replacement)
}

func (r *Renderer) warnMissingRef(key string) {
	if r == nil || r.doc == nil || key == "" || r.missingRefs[key] {
		return
	}
	r.missingRefs[key] = true
	r.doc.Warnings = append(r.doc.Warnings, "unresolved ref: "+key)
}

func subfigureByImageIndex(subs []*ast.Subfigure) map[int]*ast.Subfigure {
	out := map[int]*ast.Subfigure{}
	for _, sub := range subs {
		if sub != nil {
			out[sub.ImageIndex] = sub
		}
	}
	return out
}

func isTabularComplex(n *ast.ComplexHTML) bool {
	return n != nil && (n.EnvName == "tabular" || n.EnvName == "tabularx")
}

func subfigureLetter(i int) string {
	if i < 0 {
		return "?"
	}
	var buf [8]byte
	pos := len(buf)
	for {
		pos--
		buf[pos] = byte('a' + i%26)
		i = i/26 - 1
		if i < 0 {
			return string(buf[pos:])
		}
	}
}

func subfigureDisplayNumber(number string) string {
	if idx := strings.LastIndexByte(number, '.'); idx >= 0 && idx+1 < len(number) {
		return number[idx+1:]
	}
	return number
}

func imageWidthStyle(image *ast.Image) string {
	if image == nil || image.Options == nil {
		return ""
	}
	width := strings.TrimSpace(image.Options["width"])
	if width == "" {
		return ""
	}
	if cssWidth, ok := latexWidthToCSS(width); ok {
		return "width: " + cssWidth + "; max-width: 100%;"
	}
	return ""
}

func subfigureWidthStyle(imageStyle string) string {
	width := strings.TrimSpace(strings.TrimPrefix(imageStyle, "width:"))
	if idx := strings.IndexByte(width, ';'); idx >= 0 {
		width = strings.TrimSpace(width[:idx])
	}
	if width == "" {
		return imageStyle
	}
	return "flex: 0 0 " + width + "; max-width: " + width + ";"
}

func latexWidthToCSS(width string) (string, bool) {
	width = strings.ReplaceAll(width, " ", "")
	width = strings.ReplaceAll(width, "{", "")
	width = strings.ReplaceAll(width, "}", "")
	if strings.HasSuffix(width, `%`) {
		if _, err := strconv.ParseFloat(strings.TrimSuffix(width, `%`), 64); err == nil {
			return width, true
		}
	}
	for _, unit := range []string{`\textwidth`, `\linewidth`, `\columnwidth`} {
		if strings.Contains(width, unit) {
			factor := strings.TrimSpace(strings.TrimSuffix(width, unit))
			if factor == "" {
				return "100%", true
			}
			if strings.HasSuffix(factor, `*`) {
				factor = strings.TrimSuffix(factor, `*`)
			}
			f, err := strconv.ParseFloat(factor, 64)
			if err != nil {
				return "", false
			}
			return formatPercent(f * 100), true
		}
	}
	return "", false
}

func formatPercent(v float64) string {
	s := strconv.FormatFloat(v, 'f', 4, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s + "%"
}

func needsSpaceBetween(left, right ast.Inline) bool {
	if !isMathInline(left) && !isMathInline(right) {
		return false
	}
	leftRune, okLeft := trailingVisibleRune(left)
	rightRune, okRight := leadingVisibleRune(right)
	if !okLeft || !okRight {
		return false
	}
	if unicode.IsSpace(leftRune) || unicode.IsSpace(rightRune) {
		return false
	}
	if strings.ContainsRune("([{", leftRune) {
		return false
	}
	if strings.ContainsRune(",.;:!?)]}", rightRune) {
		return false
	}
	return true
}

func isMathInline(node ast.Inline) bool {
	_, ok := node.(*ast.InlineMath)
	return ok
}

func leadingVisibleRune(node ast.Inline) (rune, bool) {
	text := visibleInlineText(node)
	for _, r := range text {
		return r, true
	}
	return 0, false
}

func trailingVisibleRune(node ast.Inline) (rune, bool) {
	text := visibleInlineText(node)
	var last rune
	ok := false
	for _, r := range text {
		last = r
		ok = true
	}
	return last, ok
}

func visibleInlineText(node ast.Inline) string {
	switch n := node.(type) {
	case *ast.Text:
		return n.Value
	case *ast.Bold:
		return inlineText(n.Children)
	case *ast.Italic:
		return inlineText(n.Children)
	case *ast.Styled:
		return inlineText(n.Children)
	case *ast.InlineMath:
		return "x"
	case *ast.Link:
		return inlineText(n.Children)
	case *ast.Cite:
		return strings.Join(n.Keys, ", ")
	case *ast.Ref:
		return n.Key
	case *ast.Footnote:
		return inlineText(n.Children)
	default:
		return ""
	}
}

func inlineStyle(n *ast.Styled) string {
	if n == nil {
		return ""
	}
	return styleString(n.Color, n.Background, n.FontSize, "", n.Underline, n.Bold, n.Italic, n.Mono)
}

func blockStyle(n *ast.StyledBlock) string {
	if n == nil {
		return ""
	}
	return styleString(n.Color, n.Background, n.FontSize, n.Align, n.Underline, n.Bold, n.Italic, n.Mono)
}

func tcbStyle(n *ast.TCB) string {
	titleBg := n.TitleBackground
	if titleBg == "" {
		titleBg = "color-mix(in srgb, gray 70%, white)"
	}
	border := n.BorderColor
	if border == "" {
		border = "black"
	}
	bodyBg := n.BodyBackground
	if bodyBg == "" {
		bodyBg = "color-mix(in srgb, " + titleBg + " 20%, white)"
	}
	align := n.TitleAlign
	if align == "" {
		align = "left"
	}
	return "--tcb-title-bg: " + titleBg + "; --tcb-border: " + border + "; --tcb-title-color: " + border + "; --tcb-body-bg: " + bodyBg + "; --tcb-title-align: " + align + ";"
}

func styleString(color, background, fontSize, align string, underline, bold, italic, mono bool) string {
	var parts []string
	if color != "" {
		parts = append(parts, "color: "+color)
	}
	if background != "" {
		parts = append(parts, "background-color: "+background, "padding: 0 0.16em")
	}
	if fontSize != "" {
		parts = append(parts, "font-size: "+fontSize)
	}
	if align != "" {
		parts = append(parts, "text-align: "+align)
	}
	if underline {
		parts = append(parts, "text-decoration: underline")
	}
	if bold {
		parts = append(parts, "font-weight: 700")
	}
	if italic {
		parts = append(parts, "font-style: italic")
	}
	if mono {
		parts = append(parts, `font-family: Consolas, "Liberation Mono", "Courier New", monospace`)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ") + ";"
}

func alignStyle(align string) string {
	switch align {
	case "left", "center", "right", "justify":
		return "text-align: " + align + ";"
	default:
		return ""
	}
}

func writeStyleAttr(b *strings.Builder, style string) {
	if style == "" {
		return
	}
	b.WriteString(` style="`)
	b.WriteString(html.EscapeString(style))
	b.WriteString(`"`)
}

func safeLinkURL(raw string) string {
	u := strings.TrimSpace(raw)
	lower := strings.ToLower(u)
	switch {
	case strings.HasPrefix(lower, "http://"),
		strings.HasPrefix(lower, "https://"),
		strings.HasPrefix(lower, "mailto:"),
		strings.HasPrefix(lower, "ftp://"),
		strings.HasPrefix(u, "#"),
		strings.HasPrefix(u, "/"),
		strings.HasPrefix(u, "./"),
		strings.HasPrefix(u, "../"):
		return u
	default:
		if strings.Contains(u, ":") {
			return "#"
		}
		return u
	}
}

func (r *Renderer) plainInlines(inlines []ast.Inline) string {
	var b strings.Builder
	for _, node := range inlines {
		switch n := node.(type) {
		case *ast.Text:
			b.WriteString(n.Value)
		case *ast.Bold:
			b.WriteString(r.plainInlines(n.Children))
		case *ast.Italic:
			b.WriteString(r.plainInlines(n.Children))
		case *ast.Styled:
			b.WriteString(r.plainInlines(n.Children))
		case *ast.InlineMath:
			b.WriteString(n.TeX)
		case *ast.Link:
			b.WriteString(r.plainInlines(n.Children))
		case *ast.Cite:
			b.WriteString(strings.Join(n.Keys, ", "))
		case *ast.Ref:
			if target, ok := r.labels[n.Key]; ok && target.Number != "" {
				b.WriteString(target.Number)
			} else {
				r.warnMissingRef(n.Key)
				b.WriteString(n.Key)
			}
		case *ast.Footnote:
			b.WriteString(r.plainInlines(n.Children))
		}
	}
	return b.String()
}

func (r *Renderer) renderTOC(blocks []ast.Block) string {
	items := r.renderTOCItems(blocks, 0)
	if items == "" {
		return ""
	}
	return `<nav class="toc" id="toc" aria-label="Contents"><div class="toc-header"><h2>Contents</h2><button type="button" class="toc-toggle" aria-controls="toc-list" aria-expanded="true"><span class="toc-toggle-open">Hide</span><span class="toc-toggle-closed" aria-hidden="true"><svg viewBox="0 0 24 48" focusable="false"><path d="M7 12l8 12-8 12"></path><path d="M13 12l8 12-8 12"></path></svg></span></button></div><ol id="toc-list" class="toc-list">` + items + `</ol></nav>`
}

func (r *Renderer) renderTOCItems(blocks []ast.Block, parentLevel int) string {
	var b strings.Builder
	for _, block := range blocks {
		switch n := block.(type) {
		case *ast.Section:
			children := r.renderTOCItems(n.Children, n.Level)
			b.WriteString(`<li class="toc-level-`)
			b.WriteString(intString(n.Level))
			if children != "" {
				b.WriteString(` toc-has-children`)
			}
			b.WriteString(`"`)
			r.writeTOCMissingIndent(&b, n.Level, parentLevel)
			if children != "" {
				b.WriteString(`><details><summary><a href="#`)
				b.WriteString(html.EscapeString(n.AnchorID))
				b.WriteString(`">`)
				b.WriteString(`<span class="toc-number">`)
				b.WriteString(html.EscapeString(sectionNumberLabel(n)))
				b.WriteString(`</span><span class="toc-title">`)
				b.WriteString(r.renderTOCInlines(n.Title))
				b.WriteString(`</span></a></summary><ol class="toc-child-list">`)
				b.WriteString(children)
				b.WriteString(`</ol></details></li>`)
				continue
			}
			b.WriteString(`><a href="#`)
			b.WriteString(html.EscapeString(n.AnchorID))
			b.WriteString(`">`)
			b.WriteString(`<span class="toc-number">`)
			b.WriteString(html.EscapeString(sectionNumberLabel(n)))
			b.WriteString(`</span><span class="toc-title">`)
			b.WriteString(r.renderTOCInlines(n.Title))
			b.WriteString(`</span></a></li>`)
		case *ast.StyledBlock:
			b.WriteString(r.renderTOCItems(n.Children, parentLevel))
		case *ast.EnvironmentBlock:
			b.WriteString(r.renderTOCItems(n.Children, parentLevel))
		}
	}
	return b.String()
}

func (r *Renderer) writeTOCMissingIndent(b *strings.Builder, level, parentLevel int) {
	if b == nil || parentLevel <= 0 {
		return
	}
	missing := level - parentLevel - 1
	if missing <= 0 {
		return
	}
	const tocTreeStepPX = 10
	b.WriteString(` style="--toc-missing-indent: `)
	b.WriteString(intString(missing * tocTreeStepPX))
	b.WriteString(`px;"`)
}

func (r *Renderer) renderTOCFlat(blocks []ast.Block) string {
	var b strings.Builder
	var items func([]ast.Block)
	items = func(blocks []ast.Block) {
		for _, block := range blocks {
			sec, ok := block.(*ast.Section)
			if !ok {
				if styled, ok := block.(*ast.StyledBlock); ok {
					items(styled.Children)
				} else if env, ok := block.(*ast.EnvironmentBlock); ok {
					items(env.Children)
				}
				continue
			}
			b.WriteString(`<li class="toc-level-`)
			b.WriteString(intString(sec.Level))
			b.WriteString(`"><a href="#`)
			b.WriteString(html.EscapeString(sec.AnchorID))
			b.WriteString(`">`)
			b.WriteString(`<span class="toc-number">`)
			b.WriteString(html.EscapeString(sectionNumberLabel(sec)))
			b.WriteString(`</span><span class="toc-title">`)
			b.WriteString(r.renderTOCInlines(sec.Title))
			b.WriteString(`</span></a></li>`)
			items(sec.Children)
		}
	}
	items(blocks)
	return b.String()
}

func tocToggleScript() string {
	return `<script>
document.addEventListener("DOMContentLoaded", function () {
  var toc = document.getElementById("toc");
  var button = toc ? toc.querySelector(".toc-toggle") : null;
  if (!toc || !button) return;
  var key = "metablog-toc-collapsed";
  function setCollapsed(collapsed) {
    document.body.classList.toggle("toc-collapsed", collapsed);
    button.setAttribute("aria-expanded", collapsed ? "false" : "true");
  }
  try {
    setCollapsed(window.localStorage.getItem(key) === "1");
  } catch (e) {
    setCollapsed(false);
  }
  button.addEventListener("click", function () {
    var collapsed = !document.body.classList.contains("toc-collapsed");
    setCollapsed(collapsed);
    try {
      window.localStorage.setItem(key, collapsed ? "1" : "0");
    } catch (e) {}
  });
});
</script>
`
}

func inlineText(inlines []ast.Inline) string {
	var b strings.Builder
	for _, node := range inlines {
		switch n := node.(type) {
		case *ast.Text:
			b.WriteString(n.Value)
		case *ast.Bold:
			b.WriteString(inlineText(n.Children))
		case *ast.Italic:
			b.WriteString(inlineText(n.Children))
		case *ast.Styled:
			b.WriteString(inlineText(n.Children))
		case *ast.InlineMath:
			b.WriteString(n.TeX)
		case *ast.Link:
			b.WriteString(inlineText(n.Children))
		case *ast.Cite:
			b.WriteString(strings.Join(n.Keys, ", "))
		case *ast.Ref:
			b.WriteString(n.Key)
		case *ast.Footnote:
			b.WriteString(inlineText(n.Children))
		}
	}
	return b.String()
}

func sectionNumberLabel(sec *ast.Section) string {
	if sec != nil && sec.Appendix {
		return "App. " + sec.Number
	}
	if sec == nil {
		return ""
	}
	return sec.Number
}

func joinNumbers(nums []int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = intString(n)
	}
	return strings.Join(parts, ".")
}

func appendixNumber(nums []int) string {
	if len(nums) == 0 {
		return ""
	}
	parts := make([]string, len(nums))
	parts[0] = appendixLetter(nums[0] - 1)
	for i := 1; i < len(nums); i++ {
		parts[i] = intString(nums[i])
	}
	return strings.Join(parts, ".")
}

func appendixLetter(i int) string {
	if i < 0 {
		return "A"
	}
	var buf [8]byte
	pos := len(buf)
	for {
		pos--
		buf[pos] = byte('A' + i%26)
		i = i/26 - 1
		if i < 0 {
			return string(buf[pos:])
		}
	}
}

func intString(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

var anchorRE = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func anchor(label, fallback string) string {
	base := label
	if base == "" {
		base = fallback
	}
	base = strings.ReplaceAll(base, ":", "-")
	base = anchorRE.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		return "item"
	}
	return base
}

func formatIEEE(entry ast.ReferenceEntry, key string) string {
	if entry.Key == "" {
		return key + "."
	}
	f := entry.Fields
	authors := formatAuthors(f["author"])
	title := trimPeriod(f["title"])
	year := f["year"]
	var parts []string
	if authors != "" {
		parts = append(parts, authors)
	}
	if title != "" {
		parts = append(parts, `"`+title+`"`)
	}
	switch strings.ToLower(entry.Type) {
	case "article":
		if journal := f["journal"]; journal != "" {
			parts = append(parts, journal)
		}
		if vol := f["volume"]; vol != "" {
			parts = append(parts, "vol. "+vol)
		}
		if no := f["number"]; no != "" {
			parts = append(parts, "no. "+no)
		}
		if pages := f["pages"]; pages != "" {
			parts = append(parts, "pp. "+pages)
		}
	case "inproceedings", "conference":
		if booktitle := f["booktitle"]; booktitle != "" {
			parts = append(parts, "in "+booktitle)
		}
		if pages := f["pages"]; pages != "" {
			parts = append(parts, "pp. "+pages)
		}
	case "book":
		if publisher := f["publisher"]; publisher != "" {
			parts = append(parts, publisher)
		}
	default:
		if journal := firstNonEmpty(f["journal"], f["booktitle"]); journal != "" {
			parts = append(parts, journal)
		}
	}
	if year != "" {
		parts = append(parts, year)
	}
	if len(parts) == 0 {
		return key + "."
	}
	return strings.TrimSpace(strings.Join(parts, ", ")) + "."
}

func formatAuthors(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	authors := strings.Split(s, " and ")
	out := make([]string, 0, len(authors))
	for _, author := range authors {
		author = strings.TrimSpace(author)
		if author == "" {
			continue
		}
		out = append(out, formatAuthor(author))
	}
	return strings.Join(out, ", ")
}

func formatAuthor(author string) string {
	if strings.Contains(author, ",") {
		parts := strings.SplitN(author, ",", 2)
		last := strings.TrimSpace(parts[0])
		first := strings.TrimSpace(parts[1])
		if first == "" {
			return last
		}
		return initials(first) + " " + last
	}
	parts := strings.Fields(author)
	if len(parts) <= 1 {
		return author
	}
	last := parts[len(parts)-1]
	return initials(strings.Join(parts[:len(parts)-1], " ")) + " " + last
}

func initials(s string) string {
	var out []string
	for _, part := range strings.Fields(s) {
		r := []rune(part)
		if len(r) > 0 {
			out = append(out, strings.ToUpper(string(r[0]))+".")
		}
	}
	return strings.Join(out, " ")
}

func trimPeriod(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), ".")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
