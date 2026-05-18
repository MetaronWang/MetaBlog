package render

import (
	"strings"
	"testing"

	"MetaBlog/internal/latex/ast"
)

func TestTOCTitleRefsRenderWithoutNestedAnchors(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.Section{
				Level: 1,
				Title: []ast.Inline{&ast.Text{Value: "Target"}},
				Label: "sec-target",
			},
			&ast.Section{
				Level: 1,
				Title: []ast.Inline{
					&ast.Text{Value: "Uses Sec."},
					&ast.Ref{Key: "sec-target"},
				},
			},
		},
	}

	got := Render(doc)
	navStart := strings.Index(got, `<nav class="toc"`)
	navEnd := strings.Index(got, `</nav>`)
	if navStart < 0 || navEnd < navStart {
		t.Fatalf("rendered HTML does not contain TOC nav")
	}
	nav := got[navStart:navEnd]
	if strings.Contains(nav, `Uses Sec.<a`) || strings.Contains(nav, `Uses Sec. <a`) {
		t.Fatalf("TOC contains nested ref anchor: %s", nav)
	}
	if !strings.Contains(nav, `Uses Sec.1`) {
		t.Fatalf("TOC does not contain resolved plain ref text: %s", nav)
	}
}

func TestSectionHeadingTitleRefsStayInSingleGridCell(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.Section{
				Level: 1,
				Title: []ast.Inline{&ast.Text{Value: "Target"}},
				Label: "sec-target",
			},
			&ast.Section{
				Level: 3,
				Title: []ast.Inline{
					&ast.Text{Value: "Uses Sec."},
					&ast.Ref{Key: "sec-target"},
					&ast.Text{Value: " in title"},
				},
			},
		},
	}

	got := Render(doc)
	want := `<span class="section-number">1.0.1</span><span class="section-title">Uses Sec.<a href="#sec-target">1</a> in title</span>`
	if !strings.Contains(got, want) {
		t.Fatalf("section heading title was not wrapped as one grid cell; want %q in:\n%s", want, got)
	}
}

func TestTOCMissingIntermediateLevelGetsVisualIndent(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.Section{
				Level: 1,
				Title: []ast.Inline{&ast.Text{Value: "Parent"}},
				Children: []ast.Block{
					&ast.Section{
						Level: 3,
						Title: []ast.Inline{&ast.Text{Value: "Direct level three"}},
					},
				},
			},
		},
	}

	got := Render(doc)
	if !strings.Contains(got, `class="toc-level-3" style="--toc-missing-indent: 10px;"`) {
		t.Fatalf("TOC level 3 child without level 2 did not receive missing indent: %s", got)
	}
}

func TestFourthLevelSectionRendersAsH5(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.Section{
				Level: 4,
				Title: []ast.Inline{&ast.Text{Value: "Fourth"}},
			},
		},
	}

	got := Render(doc)
	if !strings.Contains(got, `class="section level-4"><h5`) {
		t.Fatalf("fourth-level section did not render as h5:\n%s", got)
	}
	if !strings.Contains(got, `class="toc-level-4"`) {
		t.Fatalf("fourth-level section missing from TOC:\n%s", got)
	}
}

func TestUnnumberedDisplayMathDoesNotConsumeEquationNumber(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.DisplayMath{TeX: "x + y"},
			&ast.DisplayMath{TeX: "a + b", Numbered: true},
		},
	}

	got := Render(doc)
	if strings.Contains(got, `<span class="equation-number">(2)</span>`) {
		t.Fatalf("unnumbered display math consumed equation number:\n%s", got)
	}
	if !strings.Contains(got, `<span class="equation-number">(1)</span>`) {
		t.Fatalf("numbered display math did not start at 1:\n%s", got)
	}
	if strings.Count(got, `class="equation-number"`) != 1 {
		t.Fatalf("expected exactly one equation number:\n%s", got)
	}
}

func TestRenderSkipsKaTeXWhenDocumentHasNoMath(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.Paragraph{Inlines: []ast.Inline{&ast.Text{Value: "Plain text."}}},
		},
	}

	got := Render(doc)
	for _, unwanted := range []string{
		`katex.min.css`,
		`katex.min.js`,
		`auto-render.min.js`,
		`renderMathInElement`,
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("non-math document loaded KaTeX asset %q:\n%s", unwanted, got)
		}
	}
}

func TestRenderUsesScopedKaTeXRendererForMath(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.Paragraph{Inlines: []ast.Inline{
				&ast.Text{Value: "Inline "},
				&ast.InlineMath{TeX: "x + y"},
			}},
			&ast.DisplayMath{TeX: "a + b", Numbered: true},
		},
	}

	got := Render(doc)
	for _, want := range []string{
		`katex.min.css`,
		`katex.min.js`,
		`window.katex.render`,
		`function normalizeTeX(tex, displayMode)`,
		`function mathCopyText(node)`,
		`document.addEventListener("copy", function (event)`,
		`event.clipboardData.setData("text/plain", parts.join("\n"))`,
		`document.querySelectorAll(".math.inline")`,
		`document.querySelectorAll(".math.display")`,
		`<span class="math inline" data-tex="x + y">\(x + y\)</span>`,
		`<span class="math-render-target" data-tex="a + b">\[a + b\]</span><span class="equation-number">(1)</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered math HTML missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{
		`auto-render.min.js`,
		`renderMathInElement`,
		`{left: "$"`,
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("scoped math renderer still contains %q:\n%s", unwanted, got)
		}
	}
}

func TestRenderLoadsKaTeXForComplexHTMLMath(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.ComplexHTML{HTML: `<div class="metablog-latexml-fragment"><span class="math inline">\(x+y\)</span></div>`},
		},
	}

	got := Render(doc)
	for _, want := range []string{
		`katex.min.css`,
		`katex.min.js`,
		`window.katex.render`,
		`tex.slice(0, 2) === "\\("`,
		`<span class="math inline">\(x+y\)</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("complex HTML math did not trigger scoped KaTeX support %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `auto-render.min.js`) || strings.Contains(got, `renderMathInElement`) {
		t.Fatalf("complex HTML math should not use KaTeX auto-render:\n%s", got)
	}
}

func TestRenderMathCopyUsesLaTeXSource(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.Paragraph{Inlines: []ast.Inline{
				&ast.Text{Value: "Inline "},
				&ast.InlineMath{TeX: `\alpha+\beta`},
			}},
			&ast.DisplayMath{TeX: `x^2+y^2`, Numbered: true},
		},
	}

	got := Render(doc)
	for _, want := range []string{
		`function mathCopyText(node)`,
		`return displayMode ? "\\[" + tex + "\\]" : "\\(" + tex + "\\)";`,
		`function replaceMathWithSource(fragment)`,
		`function rangeIntersectsMath(range)`,
		`function installMathCopyHandler()`,
		`event.clipboardData.setData("text/plain", parts.join("\n"))`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("math copy handler missing %q:\n%s", want, got)
		}
	}
}

func TestRenderWithOptionsIncludesIcon(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
	}

	got := RenderWithOptions(doc, Options{
		AssetPrefix: "../..",
		IconHref:    "assets/site/figs/metaron_logo.svg",
	})
	want := `<link rel="icon" href="../../assets/site/figs/metaron_logo.svg">`
	if !strings.Contains(got, want) {
		t.Fatalf("rendered HTML does not include configured icon; want %q in:\n%s", want, got)
	}
}

func TestRenderWithOptionsInjectsFooterAndArticleStat(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Authors: []ast.Author{
			{Name: []ast.Inline{&ast.Text{Value: "Alice"}}},
		},
	}

	got := RenderWithOptions(doc, Options{
		ArticleStatHTML: `<div class="custom-article-stat">stat</div>`,
		FooterHTML:      `<footer class="custom-page-footing">footer</footer>`,
	})
	metaIdx := strings.Index(got, `class="article-meta"`)
	statIdx := strings.Index(got, `class="custom-article-stat"`)
	mainEndIdx := strings.Index(got, `</article></main>`)
	footerIdx := strings.Index(got, `class="custom-page-footing"`)
	bodyEndIdx := strings.Index(got, `</body>`)
	if metaIdx < 0 || statIdx < 0 || statIdx < metaIdx {
		t.Fatalf("article stat was not injected after author metadata:\n%s", got)
	}
	if mainEndIdx < 0 || footerIdx < mainEndIdx || bodyEndIdx < footerIdx {
		t.Fatalf("footer was not injected after article main and before body end:\n%s", got)
	}
}

func TestRenderLineBreakInline(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.Paragraph{Inlines: []ast.Inline{
				&ast.Text{Value: "First line"},
				&ast.LineBreak{},
				&ast.Text{Value: "Second line"},
			}},
		},
	}

	got := Render(doc)
	if !strings.Contains(got, `<p>First line<br>Second line</p>`) {
		t.Fatalf("line break inline did not render as br:\n%s", got)
	}
}

func TestRenderTCB(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.TCB{
				Title:           []ast.Inline{&ast.Bold{Children: []ast.Inline{&ast.Text{Value: "测试TCB环境"}}}},
				TitleAlign:      "center",
				TitleBackground: "color-mix(in srgb, gray 70%, white)",
				BorderColor:     "black",
				BodyBackground:  "color-mix(in srgb, gray 20%, white)",
				Children: []ast.Block{
					&ast.Paragraph{Inlines: []ast.Inline{&ast.Text{Value: "tcb_test。测试！！"}}},
				},
			},
		},
	}

	got := Render(doc)
	for _, want := range []string{
		`<details class="tcb" open style="--tcb-title-bg: color-mix(in srgb, gray 70%, white); --tcb-border: black; --tcb-title-color: black; --tcb-body-bg: color-mix(in srgb, gray 20%, white); --tcb-title-align: center;">`,
		`<summary class="tcb-title"><span class="tcb-title-text"><strong>测试TCB环境</strong></span><span class="tcb-toggle" aria-hidden="true"></span></summary>`,
		`<div class="tcb-body"><p>tcb_test。测试！！</p></div></details>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered TCB missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderCodeBlock(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.CodeBlock{
				EnvName:  "minted",
				Language: "go",
				Text:     `fmt.Println("100%")`,
			},
		},
	}

	got := Render(doc)
	for _, want := range []string{
		`<div class="code-block chchroma" data-wrap="false">`,
		`<span class="code-block-lang">go</span>`,
		`Println`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered code block missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderSubfigureCaptionsAndRefs(t *testing.T) {
	images := make([]*ast.Image, 27)
	subfigures := make([]*ast.Subfigure, 27)
	for i := range images {
		images[i] = &ast.Image{SourcePath: "fig.png"}
		subfigures[i] = &ast.Subfigure{ImageIndex: i}
	}
	subfigures[0].Label = "fig:first"
	subfigures[0].Caption = []ast.Inline{&ast.Text{Value: "First"}}
	subfigures[26].Label = "fig:aa"
	subfigures[26].Caption = []ast.Inline{&ast.Text{Value: "Twenty seven"}}
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.Figure{
				Label:      "fig:main",
				Images:     images,
				Subfigures: subfigures,
				Caption:    []ast.Inline{&ast.Text{Value: "Main"}},
			},
			&ast.Paragraph{Inlines: []ast.Inline{
				&ast.Text{Value: "See "},
				&ast.Ref{Key: "fig:first"},
				&ast.Text{Value: " and "},
				&ast.Ref{Key: "fig:aa"},
			}},
		},
	}

	got := Render(doc)
	for _, want := range []string{
		`<div id="fig-first" class="subfigure"`,
		`<div class="subfigure-caption">(a) First</div>`,
		`<div id="fig-aa" class="subfigure"`,
		`<div class="subfigure-caption">(aa) Twenty seven</div>`,
		`See <a href="#fig-first">1.a</a> and <a href="#fig-aa">1.aa</a>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered subfigure output missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderSubtableCaptionsAndRefs(t *testing.T) {
	subtables := make([]*ast.Subtable, 27)
	for i := range subtables {
		subtables[i] = &ast.Subtable{
			Blocks: []ast.Block{&ast.Paragraph{Inlines: []ast.Inline{&ast.Text{Value: "table body"}}}},
		}
	}
	subtables[0].Label = "tab:first"
	subtables[0].Caption = []ast.Inline{&ast.Text{Value: "First"}}
	subtables[26].Label = "tab:aa"
	subtables[26].Caption = []ast.Inline{&ast.Text{Value: "Twenty seven"}}
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.Table{
				Label:     "tab:main",
				Subtables: subtables,
				Caption:   []ast.Inline{&ast.Text{Value: "Main"}},
			},
			&ast.Paragraph{Inlines: []ast.Inline{
				&ast.Text{Value: "See "},
				&ast.Ref{Key: "tab:first"},
				&ast.Text{Value: " and "},
				&ast.Ref{Key: "tab:aa"},
			}},
		},
	}

	got := Render(doc)
	for _, want := range []string{
		`<figure id="tab-main" class="table-float">`,
		`<div id="tab-first" class="subtable">`,
		`<div class="subtable-caption">(a) First</div>`,
		`<div id="tab-aa" class="subtable">`,
		`<div class="subtable-caption">(aa) Twenty seven</div>`,
		`See <a href="#tab-first">1.a</a> and <a href="#tab-aa">1.aa</a>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered subtable output missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderBodyAbstractAndKeywordsBlocks(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.AbstractBlock{Children: []ast.Block{
				&ast.Paragraph{Inlines: []ast.Inline{&ast.Text{Value: "Body abstract."}}},
			}},
			&ast.KeywordsBlock{Inlines: []ast.Inline{
				&ast.Text{Value: "one, "},
				&ast.Bold{Children: []ast.Inline{&ast.Text{Value: "two"}}},
			}},
		},
	}

	got := Render(doc)
	for _, want := range []string{
		`<section class="abstract"><h2>Abstract</h2><p>Body abstract.</p></section>`,
		`<p class="keywords"><strong>Keywords:</strong> one, <strong>two</strong></p>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered output missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderFontSizeDeclarations(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.Paragraph{Inlines: []ast.Inline{
				&ast.Styled{
					FontSize: "0.9em",
					Children: []ast.Inline{&ast.Text{Value: "small text"}},
				},
			}},
			&ast.StyledBlock{
				FontSize: "1.44em",
				Children: []ast.Block{
					&ast.Paragraph{Inlines: []ast.Inline{&ast.Text{Value: "large block"}}},
				},
			},
		},
	}

	got := Render(doc)
	for _, want := range []string{
		`<span style="font-size: 0.9em;">small text</span>`,
		`<div class="styled-block" style="font-size: 1.44em;">`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered HTML missing font-size style %q in:\n%s", want, got)
		}
	}
}

func TestRenderMonoStyleUsesSourceCodePro(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.Paragraph{Inlines: []ast.Inline{
				&ast.Styled{
					Mono:     true,
					Children: []ast.Inline{&ast.Text{Value: "fmt.Println"}},
				},
			}},
		},
	}

	got := Render(doc)
	want := `<span style="font-family: &#34;Source Code Pro&#34;, Consolas, &#34;Liberation Mono&#34;, &#34;Courier New&#34;, monospace;">fmt.Println</span>`
	if !strings.Contains(got, want) {
		t.Fatalf("rendered HTML missing mono font style %q in:\n%s", want, got)
	}
}

func TestRenderRawHTMLBlockWithoutEscaping(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.RawHTML{HTML: `<div class="custom"><span data-x="1">raw & html</span></div>`},
		},
	}

	got := Render(doc)
	want := `<div class="custom"><span data-x="1">raw & html</span></div>`
	if !strings.Contains(got, want) {
		t.Fatalf("rendered HTML missing unescaped raw block %q in:\n%s", want, got)
	}
	if strings.Contains(got, `raw &amp; html`) {
		t.Fatalf("raw HTML block was escaped:\n%s", got)
	}
}

func TestRenderRawHTMLInlineWithoutExtraBreak(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.Paragraph{Inlines: []ast.Inline{
				&ast.Text{Value: "Before "},
				&ast.RawHTMLInline{HTML: `<span class="raw">raw</span>`},
				&ast.Text{Value: " after."},
			}},
		},
	}

	got := Render(doc)
	want := `<p>Before <span class="raw">raw</span> after.</p>`
	if !strings.Contains(got, want) {
		t.Fatalf("inline raw HTML did not stay inside paragraph; want %q in:\n%s", want, got)
	}
}

func TestRenderFontStyleCommands(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.Paragraph{Inlines: []ast.Inline{
				&ast.Styled{FontFamily: "sans", Children: []ast.Inline{&ast.Text{Value: "sans"}}},
				&ast.Text{Value: " "},
				&ast.Styled{FontVariant: "small-caps", Children: []ast.Inline{&ast.Text{Value: "caps"}}},
				&ast.Text{Value: " "},
				&ast.Styled{FontStyle: "oblique", Children: []ast.Inline{&ast.Text{Value: "slant"}}},
				&ast.Text{Value: " "},
				&ast.Styled{FontWeight: "400", Children: []ast.Inline{&ast.Text{Value: "medium"}}},
			}},
		},
	}

	got := Render(doc)
	for _, want := range []string{
		`font-family: &#34;HarmonyOS Sans&#34;, &#34;HarmonyOS Sans SC&#34;, &#34;Source Han Sans SC&#34;`,
		`font-variant: small-caps;`,
		`font-style: oblique;`,
		`font-weight: 400;`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered HTML missing font style %q in:\n%s", want, got)
		}
	}
}

func TestRenderLinks(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.Paragraph{Inlines: []ast.Inline{
				&ast.Text{Value: "See"},
				&ast.Link{URL: "https://example.com?a=1&b=2", Children: []ast.Inline{&ast.Text{Value: "Example"}}},
				&ast.Text{Value: "and"},
				&ast.Link{URL: "javascript:alert(1)", Children: []ast.Inline{&ast.Text{Value: "Bad"}}},
			}},
		},
	}

	got := Render(doc)
	for _, want := range []string{
		`<a href="https://example.com?a=1&amp;b=2">Example</a>`,
		`<a href="#">Bad</a>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered HTML missing link %q in:\n%s", want, got)
		}
	}
}

func TestRenderDescriptionList(t *testing.T) {
	doc := &ast.Document{
		Title: []ast.Inline{&ast.Text{Value: "Test"}},
		Children: []ast.Block{
			&ast.List{
				Kind: "description",
				Items: []*ast.ListItem{
					{
						Label: []ast.Inline{&ast.Bold{Children: []ast.Inline{&ast.Text{Value: "NLP"}}}},
						Blocks: []ast.Block{
							&ast.Paragraph{Inlines: []ast.Inline{&ast.Text{Value: "Compression"}}},
						},
					},
				},
			},
		},
	}

	got := Render(doc)
	want := `<dl><dt><strong>NLP</strong></dt><dd><p>Compression</p></dd></dl>`
	if !strings.Contains(got, want) {
		t.Fatalf("rendered HTML missing description list %q in:\n%s", want, got)
	}
}

func TestCitationsUseIEEECompressedRanges(t *testing.T) {
	allKeys := []string{"k1", "k2", "k3", "k4", "k5", "k6", "k7", "k8", "k9", "k10", "k11", "k12", "k13"}
	keys := []string{"k1", "k3", "k4", "k5", "k7", "k9", "k10", "k11", "k13"}
	refs := map[string]ast.ReferenceEntry{}
	for _, key := range allKeys {
		refs[key] = ast.ReferenceEntry{Key: key, Fields: map[string]string{"title": key}}
	}
	doc := &ast.Document{
		Title:      []ast.Inline{&ast.Text{Value: "Test"}},
		References: refs,
		Children: []ast.Block{
			&ast.Paragraph{Inlines: []ast.Inline{
				&ast.Text{Value: "Seed citation order"},
				&ast.Cite{Keys: allKeys},
			}},
			&ast.Paragraph{Inlines: []ast.Inline{
				&ast.Text{Value: "Cites"},
				&ast.Cite{Keys: keys},
			}},
			&ast.References{},
		},
	}

	got := Render(doc)
	want := `<a class="cite" href="#ref-k1">[1]</a>,<span class="cite-range"><a class="cite" href="#ref-k3">[3]</a>-<a class="cite" href="#ref-k5">[5]</a></span>,<a class="cite" href="#ref-k7">[7]</a>,<span class="cite-range"><a class="cite" href="#ref-k9">[9]</a>-<a class="cite" href="#ref-k11">[11]</a></span>,<a class="cite" href="#ref-k13">[13]</a>`
	if !strings.Contains(got, want) {
		t.Fatalf("citations were not compressed as IEEE ranges; want %q in:\n%s", want, got)
	}
}

func TestCitationsCompressFourConsecutiveNumbers(t *testing.T) {
	keys := []string{"k1", "k2", "k3", "k4"}
	refs := map[string]ast.ReferenceEntry{}
	for _, key := range keys {
		refs[key] = ast.ReferenceEntry{Key: key, Fields: map[string]string{"title": key}}
	}
	doc := &ast.Document{
		Title:      []ast.Inline{&ast.Text{Value: "Test"}},
		References: refs,
		Children: []ast.Block{
			&ast.Paragraph{Inlines: []ast.Inline{
				&ast.Text{Value: "Cites"},
				&ast.Cite{Keys: keys},
			}},
			&ast.References{},
		},
	}

	got := Render(doc)
	want := `<span class="cite-range"><a class="cite" href="#ref-k1">[1]</a>-<a class="cite" href="#ref-k4">[4]</a></span>`
	if !strings.Contains(got, want) {
		t.Fatalf("four consecutive citations were not compressed; want %q in:\n%s", want, got)
	}
}

func TestCitationsDoNotCompressTwoConsecutiveNumbers(t *testing.T) {
	keys := []string{"k1", "k2"}
	refs := map[string]ast.ReferenceEntry{}
	for _, key := range keys {
		refs[key] = ast.ReferenceEntry{Key: key, Fields: map[string]string{"title": key}}
	}
	doc := &ast.Document{
		Title:      []ast.Inline{&ast.Text{Value: "Test"}},
		References: refs,
		Children: []ast.Block{
			&ast.Paragraph{Inlines: []ast.Inline{
				&ast.Text{Value: "Cites"},
				&ast.Cite{Keys: keys},
			}},
			&ast.References{},
		},
	}

	got := Render(doc)
	want := `<a class="cite" href="#ref-k1">[1]</a>,<a class="cite" href="#ref-k2">[2]</a>`
	if !strings.Contains(got, want) {
		t.Fatalf("two consecutive citations should not be compressed; want %q in:\n%s", want, got)
	}
	if strings.Contains(got, `cite-range`) {
		t.Fatalf("two consecutive citations unexpectedly used a citation range: %s", got)
	}
}
