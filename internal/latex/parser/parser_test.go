package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"MetaBlog/internal/latex/ast"
	"MetaBlog/internal/latex/blocks"
	"MetaBlog/internal/latex/lexer"
)

func TestStyledSectionDoesNotBecomeChildOfPreviousSection(t *testing.T) {
	doc, err := Parse(`\appendices
\section{Experiment on CCP}
Before.

{
\color{blue}
\section{Experiments on OneMax}
Inside.
}

After.`, nil, "", "")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(doc.Children) != 2 {
		t.Fatalf("expected 2 top-level sections, got %d", len(doc.Children))
	}
	first, ok := doc.Children[0].(*ast.Section)
	if !ok {
		t.Fatalf("first child is %T, want *ast.Section", doc.Children[0])
	}
	second, ok := doc.Children[1].(*ast.Section)
	if !ok {
		t.Fatalf("second child is %T, want *ast.Section", doc.Children[1])
	}
	if len(first.Children) != 1 {
		t.Fatalf("expected first section to contain only its paragraph, got %d children", len(first.Children))
	}
	if len(second.Title) != 1 {
		t.Fatalf("expected styled title wrapper, got %d title inline nodes", len(second.Title))
	}
	titleStyle, ok := second.Title[0].(*ast.Styled)
	if !ok {
		t.Fatalf("second section title is %T, want *ast.Styled", second.Title[0])
	}
	if titleStyle.Color != "blue" {
		t.Fatalf("expected blue styled section title, got %q", titleStyle.Color)
	}
	if len(second.Children) != 2 {
		t.Fatalf("expected styled body plus following unstyled paragraph, got %d children", len(second.Children))
	}
	if _, ok := second.Children[0].(*ast.StyledBlock); !ok {
		t.Fatalf("first child of second section is %T, want *ast.StyledBlock", second.Children[0])
	}
}

func TestTokenIndexAtUsesParserBoundarySemantics(t *testing.T) {
	tokens := []lexer.Token{
		{Kind: lexer.Text, Start: 0, End: 5},
		{Kind: lexer.Command, Start: 10, End: 18},
		{Kind: lexer.EOF, Start: 18, End: 18},
	}
	cases := []struct {
		pos  int
		want int
	}{
		{pos: 0, want: 0},
		{pos: 4, want: 0},
		{pos: 5, want: 1},
		{pos: 9, want: 1},
		{pos: 10, want: 1},
		{pos: 17, want: 1},
		{pos: 18, want: 2},
		{pos: 30, want: 2},
	}
	for _, tc := range cases {
		if got := tokenIndexAt(tokens, tc.pos); got != tc.want {
			t.Fatalf("tokenIndexAt(pos=%d) = %d, want %d", tc.pos, got, tc.want)
		}
	}
}

func TestKeywordsEnvironmentAlias(t *testing.T) {
	doc, err := Parse(`\title{Alias Test}
\begin{abstract}
Abstract body with \textbf{style}.
\end{abstract}

\begin{keywords}
one, \textbf{two}
\end{keywords}

\section{Body}
Text.`, nil, "", "")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(doc.Keywords) != 0 {
		t.Fatalf("keywords environment should render in body, got metadata: %#v", doc.Keywords)
	}
	if len(doc.Children) != 3 {
		t.Fatalf("expected abstract, keywords environment plus section, got %d children", len(doc.Children))
	}
	abstract, ok := doc.Children[0].(*ast.AbstractBlock)
	if !ok {
		t.Fatalf("first child is %T, want *ast.AbstractBlock", doc.Children[0])
	}
	if len(abstract.Children) != 1 || blockText(abstract.Children[0]) != "Abstract body with style." {
		t.Fatalf("abstract body parsed incorrectly: %#v", abstract.Children)
	}
	keywords, ok := doc.Children[1].(*ast.KeywordsBlock)
	if !ok {
		t.Fatalf("second child is %T, want *ast.KeywordsBlock", doc.Children[1])
	}
	if len(keywords.Inlines) != 2 {
		t.Fatalf("expected keywords inline nodes, got %#v", keywords.Inlines)
	}
	if blockInlineText(keywords.Inlines) != "one, two" {
		t.Fatalf("keywords body parsed incorrectly: %#v", keywords.Inlines)
	}
	if _, ok := keywords.Inlines[1].(*ast.Bold); !ok {
		t.Fatalf("second keyword inline is %T, want *ast.Bold", keywords.Inlines[1])
	}
	sec, ok := doc.Children[2].(*ast.Section)
	if !ok {
		t.Fatalf("third child is %T, want *ast.Section", doc.Children[2])
	}
	if len(sec.Children) != 1 {
		t.Fatalf("expected one paragraph in section, got %d", len(sec.Children))
	}
}

func TestParseAuthorInstitutionMetadata(t *testing.T) {
	doc, err := Parse(`\author{\textbf{Alice}}{cse}{alice@example.com}
\defInstitution{cse}{\small Department}

Body.`, nil, "main.tex", ".")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(doc.Authors) != 1 || len(doc.Institutions) != 1 {
		t.Fatalf("metadata not parsed: authors=%#v institutions=%#v", doc.Authors, doc.Institutions)
	}
	if _, ok := doc.Authors[0].Name[0].(*ast.Bold); !ok {
		t.Fatalf("author text argument formatting not parsed: %#v", doc.Authors[0].Name)
	}
	if styledFontSize(doc.Institutions[0].Info, 0) != "0.9em" {
		t.Fatalf("institution text argument declarations not parsed: %#v", doc.Institutions[0].Info)
	}
}

func TestInstitutionDedupAndDuplicateCodeWarning(t *testing.T) {
	doc, err := Parse(`\author{Alice}{cse,sds}{alice@example.com}
\defInstitution{cse}{Department, University}
\defInstitution{sds}{Department, University}
\defInstitution{cse}{Other Department}

Body.`, nil, "main.tex", ".")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(doc.Institutions) != 1 {
		t.Fatalf("expected duplicated institution content to be merged, got %#v", doc.Institutions)
	}
	inst := doc.Institutions[0]
	if inst.Code != "cse" || inst.Number != 1 || len(inst.Aliases) != 1 || inst.Aliases[0] != "sds" {
		t.Fatalf("unexpected merged institution: %#v", inst)
	}
	found := false
	aliasFound := false
	for _, warning := range doc.Warnings {
		if strings.Contains(warning, "duplicate institution code cse ignored") {
			found = true
		}
		if strings.Contains(warning, "institution code sds aliases cse as institution 1") {
			aliasFound = true
		}
	}
	if !found {
		t.Fatalf("duplicate institution code warning missing: %#v", doc.Warnings)
	}
	if !aliasFound {
		t.Fatalf("institution alias warning missing: %#v", doc.Warnings)
	}
}

func TestParseTCBEnvironment(t *testing.T) {
	doc, err := Parse(`\begin{tcb}[gray!70][black][gray!20]{\centering \textbf{测试TCB环境}}
tcb_test。测试！！
\end{tcb}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 1 {
		t.Fatalf("expected one block, got %d", len(doc.Children))
	}
	box, ok := doc.Children[0].(*ast.TCB)
	if !ok {
		t.Fatalf("expected TCB block, got %#v", doc.Children[0])
	}
	if box.TitleAlign != "center" {
		t.Fatalf("expected centered title, got %q", box.TitleAlign)
	}
	if box.TitleBackground != "color-mix(in srgb, gray 70%, white)" {
		t.Fatalf("unexpected title background: %q", box.TitleBackground)
	}
	if box.BorderColor != "black" {
		t.Fatalf("unexpected border color: %q", box.BorderColor)
	}
	if box.BodyBackground != "color-mix(in srgb, gray 20%, white)" {
		t.Fatalf("unexpected body background: %q", box.BodyBackground)
	}
	if len(box.Title) != 1 {
		t.Fatalf("unexpected title nodes: %#v", box.Title)
	}
	if _, ok := box.Title[0].(*ast.Bold); !ok {
		t.Fatalf("title did not preserve inline formatting: %#v", box.Title)
	}
	if len(box.Children) != 1 || blockText(box.Children[0]) != "tcb_test。测试！！" {
		t.Fatalf("unexpected tcb body: %#v", box.Children)
	}
}

func TestParseTCBDefaultColors(t *testing.T) {
	doc, err := Parse(`\begin{tcb}{Title}
Body
\end{tcb}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	box := doc.Children[0].(*ast.TCB)
	if box.TitleBackground != "color-mix(in srgb, gray 70%, white)" {
		t.Fatalf("unexpected default title background: %q", box.TitleBackground)
	}
	if box.BodyBackground != "#ededed" {
		t.Fatalf("unexpected default body background: %q", box.BodyBackground)
	}
	if box.BorderColor != "#aeaeae" {
		t.Fatalf("unexpected default border color: %q", box.BorderColor)
	}
}

func TestFontSizeDeclarations(t *testing.T) {
	inlines := ParseInline(`{\small small text} and {\Huge huge text}`)
	if len(inlines) != 3 {
		t.Fatalf("unexpected inline nodes: %#v", inlines)
	}
	small, ok := inlines[0].(*ast.Styled)
	if !ok || small.FontSize != "0.9em" {
		t.Fatalf("small declaration not parsed: %#v", inlines[0])
	}
	huge, ok := inlines[2].(*ast.Styled)
	if !ok || huge.FontSize != "2.488em" {
		t.Fatalf("Huge declaration not parsed: %#v", inlines[2])
	}
}

func TestTextTTCommandUsesMonoStyle(t *testing.T) {
	inlines := ParseInline(`Use \texttt{fmt.Println("hi")} here.`)
	if len(inlines) != 3 {
		t.Fatalf("unexpected inline nodes: %#v", inlines)
	}
	mono, ok := inlines[1].(*ast.Styled)
	if !ok || !mono.Mono {
		t.Fatalf("texttt command did not produce mono style: %#v", inlines[1])
	}
	if blockInlineText(mono.Children) != `fmt.Println("hi")` {
		t.Fatalf("texttt content changed: %#v", mono.Children)
	}
}

func TestFontStyleCommandsUseStyledInline(t *testing.T) {
	tests := []struct {
		src         string
		fontFamily  string
		fontStyle   string
		fontWeight  string
		fontVariant string
	}{
		{src: `\textrm{roman}`, fontFamily: "serif"},
		{src: `\textsf{sans}`, fontFamily: "sans"},
		{src: `\textsc{caps}`, fontVariant: "small-caps"},
		{src: `\textsl{slanted}`, fontStyle: "oblique"},
		{src: `\textup{upright}`, fontStyle: "normal"},
		{src: `\textmd{medium}`, fontWeight: "400"},
		{src: `\textnormal{normal}`, fontFamily: "serif", fontStyle: "normal", fontWeight: "400", fontVariant: "normal"},
	}
	for _, tt := range tests {
		inlines := ParseInline(tt.src)
		if len(inlines) != 1 {
			t.Fatalf("%s parsed to unexpected inline nodes: %#v", tt.src, inlines)
		}
		styled, ok := inlines[0].(*ast.Styled)
		if !ok {
			t.Fatalf("%s parsed as %T, want *ast.Styled", tt.src, inlines[0])
		}
		if styled.FontFamily != tt.fontFamily || styled.FontStyle != tt.fontStyle ||
			styled.FontWeight != tt.fontWeight || styled.FontVariant != tt.fontVariant {
			t.Fatalf("%s style mismatch: %#v", tt.src, styled)
		}
	}
}

func TestFontStyleDeclarations(t *testing.T) {
	inlines := ParseInline(`{\sffamily sans} {\scshape caps} {\slshape slant} {\mdseries medium} {\rmfamily roman} {\normalfont normal}`)
	if len(inlines) != 11 {
		t.Fatalf("unexpected inline nodes: %#v", inlines)
	}
	cases := []struct {
		idx         int
		fontFamily  string
		fontStyle   string
		fontWeight  string
		fontVariant string
	}{
		{idx: 0, fontFamily: "sans"},
		{idx: 2, fontVariant: "small-caps"},
		{idx: 4, fontStyle: "oblique"},
		{idx: 6, fontWeight: "400"},
		{idx: 8, fontFamily: "serif"},
		{idx: 10, fontFamily: "serif", fontStyle: "normal", fontWeight: "400", fontVariant: "normal"},
	}
	for _, tt := range cases {
		styled, ok := inlines[tt.idx].(*ast.Styled)
		if !ok {
			t.Fatalf("inline %d parsed as %T, want *ast.Styled", tt.idx, inlines[tt.idx])
		}
		if styled.FontFamily != tt.fontFamily || styled.FontStyle != tt.fontStyle ||
			styled.FontWeight != tt.fontWeight || styled.FontVariant != tt.fontVariant {
			t.Fatalf("inline %d style mismatch: %#v", tt.idx, styled)
		}
	}
}

func TestParseURLAndHrefCommands(t *testing.T) {
	inlines := ParseInline(`See \url{https://example.com/a\_b?x=1\&y=2} and \href{mailto:test@example.com}{\small Email us}.`)
	if len(inlines) != 5 {
		t.Fatalf("unexpected inline nodes: %#v", inlines)
	}
	url, ok := inlines[1].(*ast.Link)
	if !ok {
		t.Fatalf("url command parsed as %T, want *ast.Link", inlines[1])
	}
	if url.URL != "https://example.com/a_b?x=1&y=2" {
		t.Fatalf("unexpected url: %q", url.URL)
	}
	href, ok := inlines[3].(*ast.Link)
	if !ok {
		t.Fatalf("href command parsed as %T, want *ast.Link", inlines[3])
	}
	if href.URL != "mailto:test@example.com" {
		t.Fatalf("unexpected href url: %q", href.URL)
	}
	if styledFontSize(href.Children, 0) != "0.9em" {
		t.Fatalf("href text argument declarations not parsed: %#v", href.Children)
	}
}

func TestInlineCommandNamesRequireExactLexicalBoundary(t *testing.T) {
	inlines := ParseInline(`\citep{key} \refstepcounter{figure} \urlstyle{same} \cite{real}`)
	if len(inlines) != 2 {
		t.Fatalf("unexpected inline nodes: %#v", inlines)
	}
	text, ok := inlines[0].(*ast.Text)
	if !ok {
		t.Fatalf("first inline is %T, want *ast.Text", inlines[0])
	}
	if strings.Contains(text.Value, "[") {
		t.Fatalf("unsupported citation command was parsed as cite: %#v", inlines)
	}
	cite, ok := inlines[1].(*ast.Cite)
	if !ok || len(cite.Keys) != 1 || cite.Keys[0] != "real" {
		t.Fatalf("exact cite command was not parsed: %#v", inlines)
	}
}

func TestEscapedDollarDoesNotOpenInlineMath(t *testing.T) {
	inlines := ParseInline(`Price is \$5 and math is $x+1$.`)
	if len(inlines) != 3 {
		t.Fatalf("unexpected inline nodes: %#v", inlines)
	}
	if _, ok := inlines[1].(*ast.InlineMath); !ok {
		t.Fatalf("expected only real dollar pair to become inline math: %#v", inlines)
	}
}

func TestParseNestedListsAndDescription(t *testing.T) {
	doc, err := Parse(`\begin{itemize}
\item Back-End Development intern:
\begin{itemize}
\item Alarm management platform.
\item Alarm history maintenance.
\end{itemize}
\item Research intern:
\begin{description}
\item[\textbf{NLP}] Compression of a neural network model.
\end{description}
\end{itemize}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 1 {
		t.Fatalf("expected one top-level block, got %d", len(doc.Children))
	}
	outer, ok := doc.Children[0].(*ast.List)
	if !ok {
		t.Fatalf("top-level block is %T, want *ast.List", doc.Children[0])
	}
	if outer.Kind != "itemize" || outer.Ordered || len(outer.Items) != 2 {
		t.Fatalf("unexpected outer list: %#v", outer)
	}
	firstNested, ok := outer.Items[0].Blocks[1].(*ast.List)
	if !ok {
		t.Fatalf("first item nested block is %T, want *ast.List; blocks=%#v", outer.Items[0].Blocks[1], outer.Items[0].Blocks)
	}
	if firstNested.Kind != "itemize" || len(firstNested.Items) != 2 {
		t.Fatalf("nested itemize was not preserved: %#v", firstNested)
	}
	secondNested, ok := outer.Items[1].Blocks[1].(*ast.List)
	if !ok {
		t.Fatalf("second item nested block is %T, want *ast.List; blocks=%#v", outer.Items[1].Blocks[1], outer.Items[1].Blocks)
	}
	if secondNested.Kind != "description" || secondNested.Ordered || len(secondNested.Items) != 1 {
		t.Fatalf("description list was not parsed: %#v", secondNested)
	}
	if _, ok := secondNested.Items[0].Label[0].(*ast.Bold); !ok {
		t.Fatalf("description label did not preserve inline style: %#v", secondNested.Items[0].Label)
	}
	if blockText(secondNested.Items[0].Blocks[0]) != "Compression of a neural network model." {
		t.Fatalf("unexpected description body: %#v", secondNested.Items[0].Blocks)
	}
}

func TestListParserIgnoresNestedOrGroupedItemCommands(t *testing.T) {
	doc, err := Parse(`\begin{itemize}
\item First {not \item fake} still first.
\item[\textbf{A [B]}] Second item.
\item Third item.
\begin{verbatim}
\item not a real item
\end{verbatim}
Done.
\end{itemize}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	list, ok := doc.Children[0].(*ast.List)
	if !ok {
		t.Fatalf("top-level block is %T, want *ast.List", doc.Children[0])
	}
	if len(list.Items) != 3 {
		t.Fatalf("expected three top-level items, got %#v", list.Items)
	}
	if blockText(list.Items[0].Blocks[0]) != "First not fake still first." {
		t.Fatalf("grouped fake item changed first body: %#v", list.Items[0].Blocks)
	}
	label, ok := list.Items[1].Label[0].(*ast.Bold)
	if !ok || len(label.Children) != 1 || label.Children[0].(*ast.Text).Value != "A [B]" {
		t.Fatalf("description-style label with nested brackets parsed incorrectly: %#v", list.Items[1].Label)
	}
	if len(list.Items[2].Blocks) != 3 {
		t.Fatalf("raw environment with item command split list item: %#v", list.Items[2].Blocks)
	}
	if _, ok := list.Items[2].Blocks[1].(*ast.CodeBlock); !ok {
		t.Fatalf("verbatim block inside item not preserved: %#v", list.Items[2].Blocks)
	}
}

func TestListParserKeepsItemInsideUnknownEnvironmentOutOfParentList(t *testing.T) {
	doc, err := Parse(`\begin{enumerate}
\item First item.
\begin{unknownenv}
\item Not parent item.
\end{unknownenv}
\item Second item.
\end{enumerate}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	list := doc.Children[0].(*ast.List)
	if len(list.Items) != 2 {
		t.Fatalf("unknown environment item split parent list: %#v", list.Items)
	}
	if len(doc.Warnings) == 0 || !strings.Contains(doc.Warnings[0], "unsupported environment unknownenv") {
		t.Fatalf("unknown environment warning missing: %#v", doc.Warnings)
	}
}

func TestRawTextEnvironmentsBecomeCodeBlocks(t *testing.T) {
	doc, err := Parse(`Before.

\begin{verbatim}
\section{Not A Section}
100% remains
\input{hidden}
\end{verbatim}

After.`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 3 {
		t.Fatalf("expected paragraph, code block, paragraph; got %#v", doc.Children)
	}
	code, ok := doc.Children[1].(*ast.CodeBlock)
	if !ok {
		t.Fatalf("middle block is %T, want *ast.CodeBlock", doc.Children[1])
	}
	if code.EnvName != "verbatim" || !strings.Contains(code.Text, `\section{Not A Section}`) ||
		!strings.Contains(code.Text, "100% remains") || !strings.Contains(code.Text, `\input{hidden}`) {
		t.Fatalf("raw environment content was not preserved: %#v", code)
	}
	if blockText(doc.Children[0]) != "Before." || blockText(doc.Children[2]) != "After." {
		t.Fatalf("surrounding paragraphs changed: %#v", doc.Children)
	}
}

func TestHTMLEnvironmentBecomesRawHTMLBlock(t *testing.T) {
	doc, err := Parse(`Before.

\begin{html}
<section class="custom">
  <h2>\section{Not A Section}</h2>
  <p>100% remains and \input{hidden}</p>
</section>
\end{html}

After.`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 3 {
		t.Fatalf("expected paragraph, raw html, paragraph; got %#v", doc.Children)
	}
	raw, ok := doc.Children[1].(*ast.RawHTML)
	if !ok {
		t.Fatalf("middle block is %T, want *ast.RawHTML", doc.Children[1])
	}
	for _, want := range []string{`<section class="custom">`, `\section{Not A Section}`, `100% remains`, `\input{hidden}`} {
		if !strings.Contains(raw.HTML, want) {
			t.Fatalf("raw HTML missing %q: %#v", want, raw)
		}
	}
	if blockText(doc.Children[0]) != "Before." || blockText(doc.Children[2]) != "After." {
		t.Fatalf("surrounding paragraphs changed: %#v", doc.Children)
	}
	if len(doc.Warnings) != 0 {
		t.Fatalf("html environment should not emit parser warnings: %#v", doc.Warnings)
	}
}

func TestImportHTMLCommandEmbedsRawHTMLFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "partials"), 0755); err != nil {
		t.Fatal(err)
	}
	htmlPath := filepath.Join(dir, "partials", "snippet.html")
	htmlBody := `<aside class="note"><strong>Raw HTML</strong></aside>`
	if err := os.WriteFile(htmlPath, []byte(htmlBody), 0644); err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(dir, "main.tex")
	doc, err := Parse(`Before.

\importHTML{partials/snippet.html}

After.`, nil, inputPath, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 3 {
		t.Fatalf("expected paragraph, raw html, paragraph; got %#v", doc.Children)
	}
	raw, ok := doc.Children[1].(*ast.RawHTML)
	if !ok {
		t.Fatalf("middle block is %T, want *ast.RawHTML", doc.Children[1])
	}
	if raw.HTML != htmlBody {
		t.Fatalf("imported HTML changed: %q", raw.HTML)
	}
	if len(doc.Warnings) != 0 {
		t.Fatalf("importHTML emitted unexpected warnings: %#v", doc.Warnings)
	}
}

func TestImportHTMLCommandWarnsForMissingAndNonHTMLFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plain.txt"), []byte("plain text"), 0644); err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(dir, "main.tex")
	doc, err := Parse(`\importHTML{missing.html}

\importHTML{plain.txt}`, nil, inputPath, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 1 {
		t.Fatalf("expected non-HTML import to still emit RawHTML, got %#v", doc.Children)
	}
	raw, ok := doc.Children[0].(*ast.RawHTML)
	if !ok || raw.HTML != "plain text" {
		t.Fatalf("non-HTML import should be embedded as raw HTML text: %#v", doc.Children)
	}
	if len(doc.Warnings) != 2 {
		t.Fatalf("expected missing-file and non-HTML warnings, got %#v", doc.Warnings)
	}
	if !strings.Contains(doc.Warnings[0], `\importHTML file not found missing.html`) {
		t.Fatalf("missing-file warning not found: %#v", doc.Warnings)
	}
	if !strings.Contains(doc.Warnings[1], `\importHTML content does not look like HTML: plain.txt`) {
		t.Fatalf("non-HTML warning not found: %#v", doc.Warnings)
	}
}

func TestMintedCodeBlockLanguage(t *testing.T) {
	doc, err := Parse(`\begin{minted}[linenos]{go}
fmt.Println("hi")
\end{minted}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	code, ok := doc.Children[0].(*ast.CodeBlock)
	if !ok {
		t.Fatalf("block is %T, want *ast.CodeBlock", doc.Children[0])
	}
	if code.EnvName != "minted" || code.Language != "go" || code.Text != `fmt.Println("hi")` {
		t.Fatalf("unexpected minted block: %#v", code)
	}
}

func TestUnsupportedEnvironmentIsPreservedAsTransparentBlock(t *testing.T) {
	doc, err := Parse(`Before.

\begin{unknownenv}
\section{Not A Section}
Inside text.
\end{unknownenv}

After.`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 3 {
		t.Fatalf("expected paragraph, environment, paragraph; got %#v", doc.Children)
	}
	env, ok := doc.Children[1].(*ast.EnvironmentBlock)
	if !ok || env.EnvName != "unknownenv" {
		t.Fatalf("unsupported environment was not preserved: %#v", doc.Children[1])
	}
	if len(env.Children) != 1 {
		t.Fatalf("expected one section inside unsupported environment, got %#v", env.Children)
	}
	sec, ok := env.Children[0].(*ast.Section)
	if !ok || blockInlineText(sec.Title) != "Not A Section" || blockText(sec.Children[0]) != "Inside text." {
		t.Fatalf("unsupported environment content parsed incorrectly: %#v", env.Children)
	}
	if len(doc.Warnings) == 0 || !strings.Contains(doc.Warnings[0], "unsupported environment unknownenv") {
		t.Fatalf("unsupported environment warning missing: %#v", doc.Warnings)
	}
}

func TestUnknownInlineCommandEmitsWarning(t *testing.T) {
	doc, err := Parse(`Known text \unknowncmd{kept}.`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Warnings) == 0 || !strings.Contains(doc.Warnings[0], `unsupported inline command \unknowncmd`) {
		t.Fatalf("unknown inline warning missing: %#v", doc.Warnings)
	}
	if len(doc.Children) != 1 || blockText(doc.Children[0]) != "Known text kept." {
		t.Fatalf("unknown inline fallback changed: %#v", doc.Children)
	}
}

func TestInlineParserUsesTokenGroupsForUnknownCommandFallback(t *testing.T) {
	doc, err := Parse(`Text \unknowncmd{\textbf{kept}} \% \& \_.`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	para := doc.Children[0].(*ast.Paragraph)
	if len(doc.Warnings) == 0 || !strings.Contains(doc.Warnings[0], `unsupported inline command \unknowncmd`) {
		t.Fatalf("unknown inline warning missing: %#v", doc.Warnings)
	}
	if _, ok := para.Inlines[1].(*ast.Bold); !ok {
		t.Fatalf("unknown command braced argument was not parsed as inline content: %#v", para.Inlines)
	}
	if blockText(para) != "Text kept % & _." {
		t.Fatalf("escaped command text changed: %q", blockText(para))
	}
}

func TestMetadataParserScansTopLevelDocument(t *testing.T) {
	doc, err := Parse(`\title{First Title}

\section{Body}
Body.

\title{Second Title}
\author{Alice}{sustech}{alice@example.com}
\defInstitution{sustech}{SUSTech}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if blockInlineText(doc.Title) != "Second Title" {
		t.Fatalf("title metadata parsed incorrectly: %#v", doc.Title)
	}
	if len(doc.Warnings) == 0 || !strings.Contains(doc.Warnings[0], `multiple \title`) {
		t.Fatalf("multiple title warning missing: %#v", doc.Warnings)
	}
	if len(doc.Authors) != 1 || len(doc.Institutions) != 1 {
		t.Fatalf("metadata after body was not extracted: authors=%#v institutions=%#v", doc.Authors, doc.Institutions)
	}
	sec := doc.Children[0].(*ast.Section)
	if blockText(sec.Children[0]) != "Body." {
		t.Fatalf("metadata commands leaked into body: %#v", sec.Children)
	}
}

func TestTransparentLayoutEnvironmentsAndEmptyDeclarationGroups(t *testing.T) {
	doc, err := Parse(`\begin{center}
Centered text.
\end{center}

{\color{blue}}

\begin{flushright}
Right text.
\end{flushright}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 2 {
		t.Fatalf("expected two rendered blocks; empty declaration group should be consumed: %#v", doc.Children)
	}
	center := doc.Children[0].(*ast.StyledBlock)
	right := doc.Children[1].(*ast.StyledBlock)
	if center.Align != "center" || blockText(center.Children[0]) != "Centered text." {
		t.Fatalf("center environment parsed incorrectly: %#v", center)
	}
	if right.Align != "right" || blockText(right.Children[0]) != "Right text." {
		t.Fatalf("flushright environment parsed incorrectly: %#v", right)
	}
}

func TestTextArgumentsAcceptDeclarations(t *testing.T) {
	doc, err := Parse(`\title{\Huge Big Title}

\section{\small Small Section}

\begin{figure}
\caption{\large Figure Caption}
\end{figure}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if styledFontSize(doc.Title, 0) != "2.488em" {
		t.Fatalf("title declaration not parsed: %#v", doc.Title)
	}
	sec := doc.Children[0].(*ast.Section)
	if styledFontSize(sec.Title, 0) != "0.9em" {
		t.Fatalf("section declaration not parsed: %#v", sec.Title)
	}
	fig := sec.Children[0].(*ast.Figure)
	if styledFontSize(fig.Caption, 0) != "1.2em" {
		t.Fatalf("caption declaration not parsed: %#v", fig.Caption)
	}
}

func TestTCBTitleAcceptsTextArgumentDeclarations(t *testing.T) {
	doc, err := Parse(`\begin{tcb}[#2BB7B3][#003e42][]{\Huge TCB Title}
Body
\end{tcb}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 1 {
		t.Fatalf("expected one block, got %d", len(doc.Children))
	}
	box, ok := doc.Children[0].(*ast.TCB)
	if !ok {
		t.Fatalf("expected TCB block, got %#v", doc.Children[0])
	}
	if styledFontSize(box.Title, 0) != "2.488em" {
		t.Fatalf("tcb title declaration not parsed: %#v", box.Title)
	}
}

func TestTCBTitleAcceptsCenteringAndSizeDeclarationsInAnyOrder(t *testing.T) {
	doc, err := Parse(`\begin{tcb}[#2BB7B3][#003e42][]{\tiny\centering First Title}
Body
\end{tcb}

\begin{tcb}[#2BB7B3][#003e42][]{\centering\tiny Second Title}
Body
\end{tcb}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 2 {
		t.Fatalf("expected two tcb blocks, got %d", len(doc.Children))
	}
	for i, child := range doc.Children {
		box, ok := child.(*ast.TCB)
		if !ok {
			t.Fatalf("child %d is %T, want *ast.TCB", i, child)
		}
		if box.TitleAlign != "center" {
			t.Fatalf("box %d title align = %q, want center", i, box.TitleAlign)
		}
		if styledFontSize(box.Title, 0) != "0.5em" {
			t.Fatalf("box %d title font size not parsed: %#v", i, box.Title)
		}
	}
}

func TestTextContainersAcceptAlignmentAndSizeDeclarationsInAnyOrder(t *testing.T) {
	doc, err := Parse(`\title{\centering\tiny Doc Title}

\section{\tiny\centering Section Title}

\begin{figure}
\caption{\centering\tiny Figure Caption}
\end{figure}

\tiny\centering Center paragraph.`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if doc.TitleAlign != "center" || styledFontSize(doc.Title, 0) != "0.5em" {
		t.Fatalf("document title declarations not parsed: align=%q title=%#v", doc.TitleAlign, doc.Title)
	}
	sec := doc.Children[0].(*ast.Section)
	if sec.TitleAlign != "center" || styledFontSize(sec.Title, 0) != "0.5em" {
		t.Fatalf("section title declarations not parsed: align=%q title=%#v", sec.TitleAlign, sec.Title)
	}
	fig := sec.Children[0].(*ast.Figure)
	if fig.CaptionAlign != "center" || styledFontSize(fig.Caption, 0) != "0.5em" {
		t.Fatalf("caption declarations not parsed: align=%q caption=%#v", fig.CaptionAlign, fig.Caption)
	}
	para := sec.Children[1].(*ast.Paragraph)
	if para.Align != "center" || styledFontSize(para.Inlines, 0) != "0.5em" {
		t.Fatalf("paragraph declarations not parsed: align=%q inlines=%#v", para.Align, para.Inlines)
	}
}

func TestNonTextArgumentsDoNotUseTextArgumentParser(t *testing.T) {
	doc, err := Parse(`\section{Title}\label{\Huge sec:x}
See \ref{\Huge sec:x} and \cite{\Huge key}.`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	sec := doc.Children[0].(*ast.Section)
	if sec.Label != `\Huge sec:x` {
		t.Fatalf("label argument was changed: %q", sec.Label)
	}
	para := sec.Children[0].(*ast.Paragraph)
	ref := para.Inlines[1].(*ast.Ref)
	if ref.Key != `\Huge sec:x` {
		t.Fatalf("ref argument was changed: %q", ref.Key)
	}
	cite := para.Inlines[3].(*ast.Cite)
	if len(cite.Keys) != 1 || cite.Keys[0] != `\Huge key` {
		t.Fatalf("cite argument was changed: %#v", cite.Keys)
	}
}

func TestBlockFontSizeDeclaration(t *testing.T) {
	doc, err := Parse(`{
\Large
\section{Large Title}
Large body.
}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	sec := doc.Children[0].(*ast.Section)
	title, ok := sec.Title[0].(*ast.Styled)
	if !ok || title.FontSize != "1.44em" {
		t.Fatalf("section title font size not styled: %#v", sec.Title)
	}
	body, ok := sec.Children[0].(*ast.StyledBlock)
	if !ok || body.FontSize != "1.44em" {
		t.Fatalf("section body font size not styled: %#v", sec.Children)
	}
}

func TestBlockFontFamilyDeclaration(t *testing.T) {
	tests := []struct {
		name        string
		decl        string
		fontFamily  string
		fontStyle   string
		fontWeight  string
		fontVariant string
	}{
		{name: "sans", decl: `\sffamily`, fontFamily: "sans"},
		{name: "serif", decl: `\rmfamily`, fontFamily: "serif"},
		{name: "oblique", decl: `\slshape`, fontStyle: "oblique"},
		{name: "small caps", decl: `\scshape`, fontVariant: "small-caps"},
		{name: "medium", decl: `\mdseries`, fontWeight: "400"},
	}
	for _, tt := range tests {
		doc, err := Parse(`{
`+tt.decl+`
\section{Styled Title}
Styled body.
}`, nil, "main.tex", ".")
		if err != nil {
			t.Fatal(err)
		}
		sec := doc.Children[0].(*ast.Section)
		title, ok := sec.Title[0].(*ast.Styled)
		if !ok {
			t.Fatalf("%s section title not styled: %#v", tt.name, sec.Title)
		}
		if title.FontFamily != tt.fontFamily || title.FontStyle != tt.fontStyle ||
			title.FontWeight != tt.fontWeight || title.FontVariant != tt.fontVariant {
			t.Fatalf("%s title style mismatch: %#v", tt.name, title)
		}
		body, ok := sec.Children[0].(*ast.StyledBlock)
		if !ok {
			t.Fatalf("%s section body not styled: %#v", tt.name, sec.Children)
		}
		if body.FontFamily != tt.fontFamily || body.FontStyle != tt.fontStyle ||
			body.FontWeight != tt.fontWeight || body.FontVariant != tt.fontVariant {
			t.Fatalf("%s body style mismatch: %#v", tt.name, body)
		}
	}
}

func TestBlockFontStyleDeclarationsCompose(t *testing.T) {
	doc, err := Parse(`{
\slshape\scshape
\section{Styled Title}
Styled body.
}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	sec := doc.Children[0].(*ast.Section)
	title, ok := sec.Title[0].(*ast.Styled)
	if !ok || title.FontStyle != "oblique" || title.FontVariant != "small-caps" {
		t.Fatalf("section title style combination not preserved: %#v", sec.Title)
	}
	body, ok := sec.Children[0].(*ast.StyledBlock)
	if !ok || body.FontStyle != "oblique" || body.FontVariant != "small-caps" {
		t.Fatalf("section body style combination not preserved: %#v", sec.Children)
	}
}

func TestTransparentBlockGroupCanContainSection(t *testing.T) {
	doc, err := Parse(`Before.

{
\section{Grouped Section}
Grouped body.
}

After.`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 2 {
		t.Fatalf("expected paragraph and section, got %#v", doc.Children)
	}
	if blockText(doc.Children[0]) != "Before." {
		t.Fatalf("prefix paragraph changed: %#v", doc.Children[0])
	}
	sec, ok := doc.Children[1].(*ast.Section)
	if !ok {
		t.Fatalf("grouped section not parsed: %#v", doc.Children[1])
	}
	if blockInlineText(sec.Title) != "Grouped Section" {
		t.Fatalf("section title parsed incorrectly: %#v", sec.Title)
	}
	if len(sec.Children) != 2 || blockText(sec.Children[0]) != "Grouped body." || blockText(sec.Children[1]) != "After." {
		t.Fatalf("section children parsed incorrectly: %#v", sec.Children)
	}
}

func TestTransparentLooseBlockGroupDoesNotRenderBraces(t *testing.T) {
	doc, err := Parse(`{
Grouped body.
}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 1 || blockText(doc.Children[0]) != "Grouped body." {
		t.Fatalf("transparent group rendered incorrectly: %#v", doc.Children)
	}
}

func TestInlineGroupSectionCommandDoesNotAffectSectionTree(t *testing.T) {
	doc, err := Parse(`Intro {not \section{Fake}} text.

\section{Real}
Real body.`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 2 {
		t.Fatalf("expected paragraph and real section, got %#v", doc.Children)
	}
	if blockText(doc.Children[0]) != "Intro not Fake text." {
		t.Fatalf("inline group paragraph parsed incorrectly: %#v", doc.Children[0])
	}
	sec, ok := doc.Children[1].(*ast.Section)
	if !ok || blockInlineText(sec.Title) != "Real" {
		t.Fatalf("real section not parsed: %#v", doc.Children[1])
	}
}

func TestBlockParserHandlesAppendicesCommandInTextFlow(t *testing.T) {
	doc, err := Parse(`\section{Main}
Main body.

\appendices

\section{Appendix Section}
Appendix body.`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 2 {
		t.Fatalf("expected main and appendix sections, got %#v", doc.Children)
	}
	mainSec := doc.Children[0].(*ast.Section)
	appSec := doc.Children[1].(*ast.Section)
	if mainSec.Appendix {
		t.Fatalf("main section marked as appendix")
	}
	if !appSec.Appendix || blockInlineText(appSec.Title) != "Appendix Section" {
		t.Fatalf("appendix section parsed incorrectly: %#v", appSec)
	}
}

func TestSubSubSubSectionBuildsFourthLevelSection(t *testing.T) {
	doc, err := Parse(`\section{Main}
\subsection{Sub}
\subsubsection{Sub Sub}
\subsubsubsection{Fourth}
Body.`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 1 {
		t.Fatalf("expected one top-level section, got %#v", doc.Children)
	}
	mainSec := doc.Children[0].(*ast.Section)
	subSec := mainSec.Children[0].(*ast.Section)
	subSubSec := subSec.Children[0].(*ast.Section)
	fourthSec, ok := subSubSec.Children[0].(*ast.Section)
	if !ok {
		t.Fatalf("fourth-level section not parsed: %#v", subSubSec.Children)
	}
	if fourthSec.Level != 4 || blockInlineText(fourthSec.Title) != "Fourth" {
		t.Fatalf("unexpected fourth-level section: %#v", fourthSec)
	}
	if blockText(fourthSec.Children[0]) != "Body." {
		t.Fatalf("fourth-level section body parsed incorrectly: %#v", fourthSec.Children)
	}
}

func TestDisplayMathBracketBlockUsesBlockParser(t *testing.T) {
	doc, err := Parse(`Before.

\[
x + y
\]

After.`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 3 {
		t.Fatalf("expected paragraph, display math, paragraph; got %#v", doc.Children)
	}
	math, ok := doc.Children[1].(*ast.DisplayMath)
	if !ok || math.TeX != "x + y" || math.Numbered {
		t.Fatalf("display math parsed incorrectly: %#v", doc.Children[1])
	}
}

func TestDisplayMathDollarBlockPreservesMathCommands(t *testing.T) {
	doc, err := Parse(`Before.

$$
\forall{i,j\in X}, i>j \Rightarrow y \quad z
$$

After.`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Warnings) != 0 {
		t.Fatalf("display math produced warnings: %#v", doc.Warnings)
	}
	if len(doc.Children) != 3 {
		t.Fatalf("expected paragraph, display math, paragraph; got %#v", doc.Children)
	}
	math, ok := doc.Children[1].(*ast.DisplayMath)
	if !ok {
		t.Fatalf("dollar display math parsed as %T", doc.Children[1])
	}
	if math.Numbered {
		t.Fatalf("dollar display math should be unnumbered: %#v", math)
	}
	if !strings.Contains(math.TeX, `\forall{i,j\in X}`) ||
		!strings.Contains(math.TeX, `\Rightarrow y \quad z`) {
		t.Fatalf("display math TeX was not preserved: %q", math.TeX)
	}
}

func TestDisplayMathDollarBlockPreservesEscapedDollar(t *testing.T) {
	doc, err := Parse(`$$
x + \$5 + y
$$`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	math, ok := doc.Children[0].(*ast.DisplayMath)
	if !ok {
		t.Fatalf("dollar display math parsed as %T", doc.Children[0])
	}
	if math.TeX != `x + \$5 + y` {
		t.Fatalf("display math TeX did not preserve escaped dollar: %q", math.TeX)
	}
	if math.Numbered {
		t.Fatalf("dollar display math should be unnumbered: %#v", math)
	}
}

func TestInlineDollarWithDisplayMathDelimiterStaysText(t *testing.T) {
	doc, err := Parse(`\textbf{$$x=y$$}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 1 {
		t.Fatalf("expected one paragraph, got %#v", doc.Children)
	}
	para, ok := doc.Children[0].(*ast.Paragraph)
	if !ok {
		t.Fatalf("block is %T, want *ast.Paragraph", doc.Children[0])
	}
	if blockInlineText(para.Inlines) != "$$x=y$$" {
		t.Fatalf("display math delimiter in inline context changed: %#v", para.Inlines)
	}
	if containsEmptyInlineMath(para.Inlines) {
		t.Fatalf("display math delimiter produced empty inline math: %#v", para.Inlines)
	}
}

func TestDisplayMathDollarBlockInsideTextFlow(t *testing.T) {
	doc, err := Parse(`Before.
$$
\forall{x\in X}
$$
After.`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Warnings) != 0 {
		t.Fatalf("display math produced warnings: %#v", doc.Warnings)
	}
	if len(doc.Children) != 3 {
		t.Fatalf("expected paragraph, display math, paragraph; got %#v", doc.Children)
	}
	if _, ok := doc.Children[1].(*ast.DisplayMath); !ok {
		t.Fatalf("dollar display math parsed as %T", doc.Children[1])
	}
}

func TestDisplayMathDollarBlockAfterChineseParagraphIsUnnumbered(t *testing.T) {
	doc, err := Parse(`基于这些设置，我们想要证明的内容可以写作：

$$
\forall{i,j\in \mathrm{2ast}}, i>j \Rightarrow y
$$

在正式证明之前继续正文。`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Warnings) != 0 {
		t.Fatalf("display math produced warnings: %#v", doc.Warnings)
	}
	if len(doc.Children) != 3 {
		t.Fatalf("expected paragraph, display math, paragraph; got %#v", doc.Children)
	}
	math, ok := doc.Children[1].(*ast.DisplayMath)
	if !ok {
		t.Fatalf("dollar display math parsed as %T", doc.Children[1])
	}
	if math.Numbered {
		t.Fatalf("dollar display math should be unnumbered: %#v", math)
	}
}

func TestAlignedEnvironmentIsPreservedAsDisplayMath(t *testing.T) {
	doc, err := Parse(`\begin{aligned}
\forall x \in X &\Rightarrow x \quad y
\end{aligned}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Warnings) != 0 {
		t.Fatalf("math environment produced warnings: %#v", doc.Warnings)
	}
	if len(doc.Children) != 1 {
		t.Fatalf("expected one display math block, got %#v", doc.Children)
	}
	math, ok := doc.Children[0].(*ast.DisplayMath)
	if !ok {
		t.Fatalf("aligned environment was not display math: %#v", doc.Children[0])
	}
	if !strings.Contains(math.TeX, `\begin{aligned}`) ||
		!strings.Contains(math.TeX, `\forall x \in X`) ||
		!strings.Contains(math.TeX, `\Rightarrow x \quad y`) {
		t.Fatalf("aligned TeX was not preserved: %q", math.TeX)
	}
	if math.Numbered {
		t.Fatalf("aligned fallback display math should be unnumbered: %#v", math)
	}
}

func TestHTMLEnvironmentClosesAtFirstEndTag(t *testing.T) {
	doc, err := Parse(`\begin{html}<p>hello</p>\end{html}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := doc.Children[0].(*ast.RawHTML)
	if !ok {
		t.Fatalf("html environment parsed as %T", doc.Children[0])
	}
	if raw.HTML != `<p>hello</p>` {
		t.Fatalf("raw HTML = %q, want %q", raw.HTML, `<p>hello</p>`)
	}
}

func TestStarredEquationIsUnnumbered(t *testing.T) {
	doc, err := Parse(`\begin{equation*}
x + y
\end{equation*}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	math, ok := doc.Children[0].(*ast.DisplayMath)
	if !ok {
		t.Fatalf("equation* parsed as %T", doc.Children[0])
	}
	if math.Numbered {
		t.Fatalf("equation* should be unnumbered: %#v", math)
	}
}

func TestEquationIsNumbered(t *testing.T) {
	doc, err := Parse(`\begin{equation}
x + y
\end{equation}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	math, ok := doc.Children[0].(*ast.DisplayMath)
	if !ok {
		t.Fatalf("equation parsed as %T", doc.Children[0])
	}
	if !math.Numbered {
		t.Fatalf("equation should be numbered: %#v", math)
	}
}

func TestFigureParserHandlesSubfloatsAndNestedOptions(t *testing.T) {
	doc, err := Parse(`\begin{figure}
\centering
\subfloat[First]{\includegraphics[width=0.45\textwidth, trim={1 2 3 4}, clip]{fig/a.pdf}\label{fig:a}}\\
\subfloat[Second]{\includegraphics[width={0.4\linewidth}]{fig/b.pdf}\caption{Nested caption}\label{fig:b}}
\caption{\large Figure Caption}
\label{fig:main}
\end{figure}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	fig := doc.Children[0].(*ast.Figure)
	if len(fig.Images) != 2 {
		t.Fatalf("expected two images, got %#v", fig.Images)
	}
	if fig.Images[0].SourcePath != "fig/a.pdf" || fig.Images[0].Options["trim"] != "{1 2 3 4}" || fig.Images[0].Options["clip"] != "" {
		t.Fatalf("first image parsed incorrectly: %#v", fig.Images[0])
	}
	if fig.Images[1].SourcePath != "fig/b.pdf" || fig.Images[1].Options["width"] != "{0.4\\linewidth}" {
		t.Fatalf("second image parsed incorrectly: %#v", fig.Images[1])
	}
	if len(fig.Subfigures) != 2 {
		t.Fatalf("expected two subfigures, got %#v", fig.Subfigures)
	}
	if fig.Subfigures[0].ImageIndex != 0 || fig.Subfigures[0].Label != "fig:a" || !fig.Subfigures[0].BreakAfter {
		t.Fatalf("first subfigure parsed incorrectly: %#v", fig.Subfigures[0])
	}
	if fig.Subfigures[1].ImageIndex != 1 || fig.Subfigures[1].Label != "fig:b" || fig.Subfigures[1].BreakAfter {
		t.Fatalf("second subfigure parsed incorrectly: %#v", fig.Subfigures[1])
	}
	if blockInlineText(fig.Subfigures[0].Caption) != "First" || blockInlineText(fig.Subfigures[1].Caption) != "Nested caption" {
		t.Fatalf("subfigure captions parsed incorrectly: %#v", fig.Subfigures)
	}
	if fig.Label != "fig:main" {
		t.Fatalf("figure label = %q, want fig:main", fig.Label)
	}
	if styledFontSize(fig.Caption, 0) != "1.2em" {
		t.Fatalf("caption declaration not parsed: %#v", fig.Caption)
	}
}

func TestFigureParserIgnoresCaptionArgumentLabelsForFigureLabel(t *testing.T) {
	doc, err := Parse(`\begin{figure}
\includegraphics{fig/a.pdf}
\caption{A caption mentioning \label{not-real}}
\label{fig:real}
\end{figure}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	fig := doc.Children[0].(*ast.Figure)
	if fig.Label != "fig:real" {
		t.Fatalf("figure label = %q, want fig:real", fig.Label)
	}
}

func TestFigureParserFindsImageInsideSimpleWrapperEnvironment(t *testing.T) {
	doc, err := Parse(`\begin{figure}
\begin{center}
\includegraphics[width=0.6\textwidth]{fig/wrapped.pdf}
\end{center}
\caption{Wrapped}
\end{figure}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	fig := doc.Children[0].(*ast.Figure)
	if len(fig.Images) != 1 || fig.Images[0].SourcePath != "fig/wrapped.pdf" {
		t.Fatalf("wrapped image not parsed: %#v", fig.Images)
	}
}

func TestFigureParserFindsImageInsideScopedGroup(t *testing.T) {
	doc, err := Parse(`\begin{figure}
{\centering
\includegraphics{fig/grouped.pdf}
}
\caption{Grouped}
\end{figure}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	fig := doc.Children[0].(*ast.Figure)
	if len(fig.Images) != 1 || fig.Images[0].SourcePath != "fig/grouped.pdf" {
		t.Fatalf("grouped image not parsed: %#v", fig.Images)
	}
}

func TestFigureParserDoesNotTreatDeclarationFollowedByGroupAsCommandArgument(t *testing.T) {
	doc, err := Parse(`\begin{figure}
\centering {
\includegraphics{fig/decl-group.pdf}
}
\caption{Grouped}
\end{figure}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	fig := doc.Children[0].(*ast.Figure)
	if len(fig.Images) != 1 || fig.Images[0].SourcePath != "fig/decl-group.pdf" {
		t.Fatalf("declaration grouped image not parsed: %#v", fig.Images)
	}
}

func TestFigureParserUnwrapsLayoutCommands(t *testing.T) {
	doc, err := Parse(`\begin{figure}
\resizebox{\linewidth}{!}{\includegraphics{fig/a.pdf}}
\caption{Wrapped}
\label{fig:wrapped}
\end{figure}`, nil, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	fig := doc.Children[0].(*ast.Figure)
	if len(fig.Images) != 1 || fig.Images[0].SourcePath != "fig/a.pdf" {
		t.Fatalf("wrapped image not parsed: %#v", fig.Images)
	}
	if fig.Label != "fig:wrapped" || blockInlineText(fig.Caption) != "Wrapped" {
		t.Fatalf("figure metadata changed: %#v", fig)
	}
}

func TestTableParserHandlesSubfloatsWithTabularBlocks(t *testing.T) {
	lifted := blocks.Lift(`\begin{table}
\setlength{\tabcolsep}{1pt}
\subfloat[Left]{\resizebox{\linewidth}{!}{\begin{tabular}{c}
A
\end{tabular}}\label{tab:left}}\\
\subfloat{\begin{tabularx}{\linewidth}{c}
B
\end{tabularx}\caption{Right}\label{tab:right}}
\caption{\centering Main Table}
\label{tab:main}
\end{table}`)
	doc, err := Parse(lifted.Text, lifted.Blocks, "main.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 1 {
		t.Fatalf("expected one table, got %#v", doc.Children)
	}
	if len(doc.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", doc.Warnings)
	}
	table, ok := doc.Children[0].(*ast.Table)
	if !ok {
		t.Fatalf("child is %T, want *ast.Table", doc.Children[0])
	}
	if table.Label != "tab:main" || table.CaptionAlign != "center" || blockInlineText(table.Caption) != "Main Table" {
		t.Fatalf("table metadata parsed incorrectly: %#v", table)
	}
	if len(table.Subtables) != 2 {
		t.Fatalf("expected two subtables, got %#v", table.Subtables)
	}
	if table.Subtables[0].Label != "tab:left" || !table.Subtables[0].BreakAfter {
		t.Fatalf("first subtable parsed incorrectly: %#v", table.Subtables[0])
	}
	if blockInlineText(table.Subtables[0].Caption) != "Left" {
		t.Fatalf("first subtable caption parsed incorrectly: %#v", table.Subtables[0].Caption)
	}
	if table.Subtables[1].Label != "tab:right" || table.Subtables[1].BreakAfter {
		t.Fatalf("second subtable parsed incorrectly: %#v", table.Subtables[1])
	}
	if blockInlineText(table.Subtables[1].Caption) != "Right" {
		t.Fatalf("second subtable caption parsed incorrectly: %#v", table.Subtables[1].Caption)
	}
	firstBlock, ok := table.Subtables[0].Blocks[0].(*ast.ComplexHTML)
	if !ok || firstBlock.EnvName != "tabular" {
		t.Fatalf("first subtable block parsed incorrectly: %#v", table.Subtables[0].Blocks)
	}
	secondBlock, ok := table.Subtables[1].Blocks[0].(*ast.ComplexHTML)
	if !ok || secondBlock.EnvName != "tabularx" {
		t.Fatalf("second subtable block parsed incorrectly: %#v", table.Subtables[1].Blocks)
	}
}

func styledFontSize(inlines []ast.Inline, idx int) string {
	if idx >= len(inlines) {
		return ""
	}
	styled, ok := inlines[idx].(*ast.Styled)
	if !ok {
		return ""
	}
	return styled.FontSize
}

func blockText(b ast.Block) string {
	p, ok := b.(*ast.Paragraph)
	if !ok {
		return ""
	}
	var out strings.Builder
	for _, in := range p.Inlines {
		out.WriteString(inlineText(in))
	}
	return out.String()
}

func blockInlineText(inlines []ast.Inline) string {
	var out strings.Builder
	for _, in := range inlines {
		out.WriteString(inlineText(in))
	}
	return out.String()
}

func inlineText(in ast.Inline) string {
	switch n := in.(type) {
	case *ast.Text:
		return n.Value
	case *ast.Bold:
		return blockInlineText(n.Children)
	case *ast.Italic:
		return blockInlineText(n.Children)
	case *ast.Styled:
		return blockInlineText(n.Children)
	case *ast.Link:
		return blockInlineText(n.Children)
	case *ast.Footnote:
		return blockInlineText(n.Children)
	default:
		return ""
	}
}

func containsEmptyInlineMath(inlines []ast.Inline) bool {
	for _, in := range inlines {
		switch n := in.(type) {
		case *ast.InlineMath:
			if n.TeX == "" {
				return true
			}
		case *ast.Bold:
			if containsEmptyInlineMath(n.Children) {
				return true
			}
		case *ast.Italic:
			if containsEmptyInlineMath(n.Children) {
				return true
			}
		case *ast.Styled:
			if containsEmptyInlineMath(n.Children) {
				return true
			}
		case *ast.Link:
			if containsEmptyInlineMath(n.Children) {
				return true
			}
		case *ast.Footnote:
			if containsEmptyInlineMath(n.Children) {
				return true
			}
		}
	}
	return false
}

func TestParseLstlistingLanguageFromOptionalArgs(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{`\begin{lstlisting}[language=C]
int main() { return 0; }
\end{lstlisting}`, "C"},
		{`\begin{lstlisting}[language=Python]
print("hello")
\end{lstlisting}`, "Python"},
		{`\begin{lstlisting}[language={C++}]
#include <iostream>
\end{lstlisting}`, "C++"},
		{`\begin{lstlisting}[language=go,other=ignored]
package main
\end{lstlisting}`, "go"},
		{`\begin{lstlisting}[other=ignored,language=Rust]
fn main() {}
\end{lstlisting}`, "Rust"},
		{`\begin{lstlisting}[frame,language=Go,numbers=left]
package main
\end{lstlisting}`, "Go"},
		{`\begin{lstlisting} [language=JavaScript]
console.log("x")
\end{lstlisting}`, "JavaScript"},
		{`\begin{lstlisting}[language = bash]
echo hello
\end{lstlisting}`, "bash"},
		{`\begin{lstlisting}[LANGUAGE=Python]
x = 1
\end{lstlisting}`, "Python"},
		{`\begin{lstlisting}[]
no language
\end{lstlisting}`, ""},
		{`\begin{lstlisting}
no args at all
\end{lstlisting}`, ""},
	}
	for _, tt := range tests {
		got, _ := Parse(tt.raw, nil, "test.tex", ".")
		if got == nil {
			t.Errorf("Parse returned nil for %q", tt.raw)
			continue
		}
		var lang string
		for _, block := range got.Children {
			if cb, ok := block.(*ast.CodeBlock); ok {
				lang = cb.Language
				break
			}
		}
		if lang != tt.want {
			t.Errorf("language = %q, want %q for raw: %s", lang, tt.want, tt.raw)
		}
	}
}

func TestParseLstlistingWhitespaceBeforeOptionalArgsDoesNotLeakOptions(t *testing.T) {
	doc, err := Parse(`\begin{lstlisting} [language=JavaScript]
console.log("x")
\end{lstlisting}`, nil, "test.tex", ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 1 {
		t.Fatalf("expected one block, got %#v", doc.Children)
	}
	cb, ok := doc.Children[0].(*ast.CodeBlock)
	if !ok {
		t.Fatalf("expected code block, got %#v", doc.Children[0])
	}
	if cb.Language != "JavaScript" {
		t.Fatalf("language = %q", cb.Language)
	}
	if strings.Contains(cb.Text, "language=JavaScript") {
		t.Fatalf("optional args leaked into code text: %q", cb.Text)
	}
	if !strings.Contains(cb.Text, `console.log("x")`) {
		t.Fatalf("code text missing source line: %q", cb.Text)
	}
}

func TestParseMintedLanguageFromRequiredArgs(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{`\begin{minted}{python}
print("hello")
\end{minted}`, "python"},
		{`\begin{minted}[]{c}
int main() {}
\end{minted}`, "c"},
		{`\begin{minted}[ignore=this]{go}
package main
\end{minted}`, "go"},
		{`\begin{minted}{Rust}
fn main() {}
\end{minted}`, "Rust"},
	}
	for _, tt := range tests {
		got, _ := Parse(tt.raw, nil, "test.tex", ".")
		if got == nil {
			t.Errorf("Parse returned nil for %q", tt.raw)
			continue
		}
		var lang string
		for _, block := range got.Children {
			if cb, ok := block.(*ast.CodeBlock); ok {
				lang = cb.Language
				break
			}
		}
		if lang != tt.want {
			t.Errorf("language = %q, want %q for raw: %s", lang, tt.want, tt.raw)
		}
	}
}

func TestExtractListingsOptionEdgeCases(t *testing.T) {
	tests := []struct{ opts, key, want string }{
		{"language=C", "language", "C"},
		{"language=C++", "language", "C++"},
		{"language={C, C++}", "language", "C, C++"},
		{"other=ignored,language=Python", "language", "Python"},
		{"frame,language=Go,numbers=left", "language", "Go"},
		{"language = go", "language", "go"},
		{"x=y,language=Rust,z=w", "language", "Rust"},
		{"", "language", ""},
		{"x=y", "language", ""},
		{"language", "language", ""},
	}
	for _, tt := range tests {
		got := extractListingsOption(tt.opts, tt.key)
		if got != tt.want {
			t.Errorf("extractListingsOption(%q, %q) = %q, want %q", tt.opts, tt.key, got, tt.want)
		}
	}
}
