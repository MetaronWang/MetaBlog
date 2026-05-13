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
