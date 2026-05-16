package parser

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"MetaBlog/internal/latex/ast"
	latexblocks "MetaBlog/internal/latex/blocks"
	"MetaBlog/internal/latex/lexer"
	"MetaBlog/internal/pathutil"
)

type Parser struct {
	text     string
	complex  map[string]*latexblocks.ComplexBlock
	doc      *ast.Document
	warnings []string
	instCode map[string]int
	instInfo map[string]int
}

func Parse(text string, complex map[string]*latexblocks.ComplexBlock, inputFile, root string) (*ast.Document, error) {
	doc := &ast.Document{
		InputFile:  inputFile,
		SourceRoot: root,
	}
	p := &Parser{text: text, complex: complex, doc: doc}

	body := p.extractDocumentMetadata(text)

	doc.Children = p.parseSectionedBlocks(body)
	doc.Warnings = append(doc.Warnings, p.warnings...)
	return doc, nil
}

func (p *Parser) extractDocumentMetadata(s string) string {
	tokens := lexer.Tokenize(s)
	var out strings.Builder
	pos := 0
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.Kind == lexer.EOF {
			break
		}
		if tok.End <= pos {
			continue
		}
		if tok.Start < pos {
			continue
		}
		if tok.Kind == lexer.LBrace || tok.Kind == lexer.LBracket {
			if _, end, ok := readTokenGroupAt(s, tokens, tok.Start, tok.Kind, matchingGroupClose(tok.Kind)); ok {
				i = tokenIndexAt(tokens, end) - 1
				continue
			}
		}
		if tok.Kind != lexer.Command {
			continue
		}
		if tok.Value == "begin" {
			env, _, ok := readBeginAt(s, tok.Start)
			if !ok {
				continue
			}
			end, ok := findEnvironmentEnd(s, tok.Start, env)
			if !ok {
				continue
			}
			i = tokenIndexAt(tokens, end) - 1
			continue
		}
		switch tok.Value {
		case "title":
			title, end, ok := readOneCommandArg(s, tok.Start, "title")
			if ok {
				out.WriteString(s[pos:tok.Start])
				out.WriteByte('\n')
				if len(p.doc.Title) > 0 {
					p.warnings = append(p.warnings, "multiple \\title metadata commands; using the last one")
				}
				parsed := p.parseTextArgumentWithDeclarations(title)
				p.doc.Title = parsed.Inlines
				p.doc.TitleAlign = parsed.Align
				pos = end
				i = tokenIndexAt(tokens, end) - 1
				continue
			}
			p.warnings = append(p.warnings, "could not parse \\title metadata")
		case "author":
			author, end, ok := p.parseAuthorCommand(s, tok.Start)
			if ok {
				out.WriteString(s[pos:tok.Start])
				out.WriteByte('\n')
				p.doc.Authors = append(p.doc.Authors, author)
				pos = end
				i = tokenIndexAt(tokens, end) - 1
				continue
			}
			p.warnings = append(p.warnings, "could not parse \\author metadata")
		case "defInstitution":
			inst, infoKey, end, ok := p.parseInstitutionCommand(s, tok.Start)
			if ok {
				out.WriteString(s[pos:tok.Start])
				out.WriteByte('\n')
				p.addInstitution(inst, infoKey)
				pos = end
				i = tokenIndexAt(tokens, end) - 1
				continue
			}
			p.warnings = append(p.warnings, "could not parse \\defInstitution metadata")
		}
	}
	out.WriteString(s[pos:])
	return out.String()
}

func (p *Parser) parseAuthorCommand(s string, idx int) (ast.Author, int, bool) {
	i := idx + len(`\author`)
	i = skipWhitespace(s, i)
	var attrs []string
	if i < len(s) && s[i] == '[' {
		arg, end, ok := readBalanced(s, i, '[', ']')
		if !ok {
			return ast.Author{}, idx, false
		}
		attrs = splitCSV(arg)
		i = skipWhitespace(s, end)
	}
	args, end, ok := readRequiredArgs(s, i, 3)
	if !ok {
		return ast.Author{}, idx, false
	}
	return ast.Author{
		Attributes:       attrs,
		Name:             p.parseTextArgument(strings.TrimSpace(args[0])),
		InstitutionCodes: splitCSV(args[1]),
		Email:            strings.TrimSpace(args[2]),
	}, end, true
}

func (p *Parser) parseInstitutionCommand(s string, idx int) (ast.Institution, string, int, bool) {
	i := idx + len(`\defInstitution`)
	i = skipWhitespace(s, i)
	args, end, ok := readRequiredArgs(s, i, 2)
	if !ok {
		return ast.Institution{}, "", idx, false
	}
	infoRaw := strings.TrimSpace(args[1])
	return ast.Institution{
		Code: strings.TrimSpace(args[0]),
		Info: p.parseTextArgument(infoRaw),
	}, institutionInfoKey(infoRaw), end, true
}

func (p *Parser) addInstitution(inst ast.Institution, infoKey string) {
	if p.instCode == nil {
		p.instCode = map[string]int{}
	}
	if p.instInfo == nil {
		p.instInfo = map[string]int{}
	}
	if inst.Code == "" {
		p.warnings = append(p.warnings, "empty institution code")
		return
	}
	if existing, ok := p.instCode[inst.Code]; ok {
		p.warnings = append(p.warnings, fmt.Sprintf("duplicate institution code %s ignored; first definition is institution %d", inst.Code, p.doc.Institutions[existing].Number))
		return
	}
	if infoKey != "" {
		if existing, ok := p.instInfo[infoKey]; ok {
			p.doc.Institutions[existing].Aliases = append(p.doc.Institutions[existing].Aliases, inst.Code)
			p.instCode[inst.Code] = existing
			primary := p.doc.Institutions[existing]
			p.warnings = append(p.warnings, fmt.Sprintf("institution code %s aliases %s as institution %d because their definitions have identical content", inst.Code, primary.Code, primary.Number))
			return
		}
	}
	inst.Number = len(p.doc.Institutions) + 1
	p.doc.Institutions = append(p.doc.Institutions, inst)
	idx := len(p.doc.Institutions) - 1
	p.instCode[inst.Code] = idx
	if infoKey != "" {
		p.instInfo[infoKey] = idx
	}
}

func institutionInfoKey(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func (p *Parser) parseTextArgument(s string) []ast.Inline {
	return p.parseTextArgumentWithDeclarations(s).Inlines
}

func (p *Parser) parseTextArgumentWithDeclarations(s string) ParsedTextArgument {
	parsed := parseTextArgumentWithParser(s, p)
	p.warnings = append(p.warnings, parsed.Warnings...)
	return parsed
}

func readRequiredArgs(s string, i, count int) ([]string, int, bool) {
	args := make([]string, 0, count)
	for len(args) < count {
		i = skipWhitespace(s, i)
		if i >= len(s) || s[i] != '{' {
			return nil, i, false
		}
		arg, end, ok := readBalanced(s, i, '{', '}')
		if !ok {
			return nil, i, false
		}
		args = append(args, arg)
		i = end
	}
	return args, i, true
}

func (p *Parser) parseSectionedBlocks(s string) []ast.Block {
	return p.parseSectionedBlocksWithAppendix(s, false)
}

func (p *Parser) parseSectionedBlocksWithAppendix(s string, appendix bool) []ast.Block {
	return newBlockParser(p, s, appendix).parseSectioned()
}

func isBlockStyledGroupCandidate(s string, start, idx int) bool {
	prefix := s[start:idx]
	if strings.TrimSpace(prefix) == "" {
		return true
	}
	if !hasBlankLine(prefix) {
		return false
	}
	j := idx - 1
	for j >= start && (s[j] == ' ' || s[j] == '\t' || s[j] == '\r') {
		j--
	}
	return j < start || s[j] == '\n'
}

func groupBodyContainsBlockSyntax(body string) bool {
	if hasBlankLine(body) {
		return true
	}
	tokens := lexer.Tokenize(body)
	braceDepth := 0
	bracketDepth := 0
	for _, tok := range tokens {
		if tok.Kind == lexer.EOF {
			break
		}
		switch tok.Kind {
		case lexer.LBrace:
			braceDepth++
			continue
		case lexer.RBrace:
			if braceDepth > 0 {
				braceDepth--
			}
			continue
		case lexer.LBracket:
			bracketDepth++
			continue
		case lexer.RBracket:
			if bracketDepth > 0 {
				bracketDepth--
			}
			continue
		}
		if braceDepth != 0 || bracketDepth != 0 {
			continue
		}
		if tok.Kind == lexer.Raw {
			env, _, ok := readBeginAt(body, tok.Start)
			if !ok || env != "html" {
				return true
			}
			continue
		}
		if tok.Kind != lexer.Command {
			continue
		}
		switch tok.Value {
		case "begin", "[", "bibliographystyle", "bibliography", "appendices", "section", "subsection", "subsubsection", "subsubsubsection":
			return true
		}
	}
	return false
}

func groupFollowedByParagraphBoundary(s string, end int) bool {
	if end >= len(s) {
		return true
	}
	next := skipWhitespace(s, end)
	if next >= len(s) {
		return true
	}
	return hasBlankLine(s[end:next])
}

func (p *Parser) readStyledBlockAt(s string, i int, appendix bool) (*ast.StyledBlock, int, bool) {
	body, end, ok := readBalanced(s, i, '{', '}')
	if !ok {
		return nil, i, false
	}
	decl := readTextDeclarations(strings.TrimSpace(body))
	if !decl.HasDeclarations || strings.TrimSpace(decl.Rest) == "" {
		return nil, i, false
	}
	style := decl.Style
	return &ast.StyledBlock{
		Color:       style.Color,
		Background:  style.Background,
		Align:       decl.Align,
		Underline:   style.Underline,
		Bold:        style.Bold,
		Italic:      style.Italic,
		Mono:        style.Mono,
		FontSize:    style.FontSize,
		FontFamily:  style.FontFamily,
		FontStyle:   style.FontStyle,
		FontWeight:  style.FontWeight,
		FontVariant: style.FontVariant,
		Children:    p.parseSectionedBlocksWithAppendix(decl.Rest, appendix),
	}, end, true
}

func (p *Parser) readTransparentBlockGroupAt(s string, i int, appendix bool) ([]ast.Block, int, bool) {
	body, end, ok := readBalanced(s, i, '{', '}')
	if !ok {
		return nil, i, false
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, i, false
	}
	decl := readTextDeclarations(body)
	if decl.HasDeclarations && strings.TrimSpace(decl.Rest) == "" {
		return []ast.Block{}, end, true
	}
	if decl.HasDeclarations {
		return nil, i, false
	}
	return p.parseSectionedBlocksWithAppendix(body, appendix), end, true
}

func (p *Parser) appendBlocks(c *sectionContainer, blocks []ast.Block) {
	if c.section != nil {
		c.section.Children = append(c.section.Children, blocks...)
		return
	}
	c.children = append(c.children, blocks...)
}

func (p *Parser) appendSectionedBlocks(stack []*sectionContainer, blocks []ast.Block) []*sectionContainer {
	for _, block := range blocks {
		sec, ok := block.(*ast.Section)
		if !ok {
			p.appendBlocks(stack[len(stack)-1], []ast.Block{block})
			continue
		}
		for len(stack) > 1 && stack[len(stack)-1].section.Level >= sec.Level {
			stack = stack[:len(stack)-1]
		}
		parent := stack[len(stack)-1]
		if parent.section != nil {
			parent.section.Children = append(parent.section.Children, sec)
		} else {
			parent.children = append(parent.children, sec)
		}
		stack = append(stack, &sectionContainer{section: sec})
	}
	return stack
}

func styleSectionedBlocks(blocks []ast.Block, style *ast.StyledBlock) []ast.Block {
	out := make([]ast.Block, 0, len(blocks))
	for _, block := range blocks {
		out = append(out, styleSectionedBlock(block, style))
	}
	return out
}

func styleSectionedBlock(block ast.Block, style *ast.StyledBlock) ast.Block {
	switch n := block.(type) {
	case *ast.Section:
		if len(n.Title) > 0 {
			n.Title = []ast.Inline{&ast.Styled{
				Color:       style.Color,
				Background:  style.Background,
				Underline:   style.Underline,
				Bold:        style.Bold,
				Italic:      style.Italic,
				Mono:        style.Mono,
				FontSize:    style.FontSize,
				FontFamily:  style.FontFamily,
				FontStyle:   style.FontStyle,
				FontWeight:  style.FontWeight,
				FontVariant: style.FontVariant,
				Children:    n.Title,
			}}
		}
		if style.Align != "" {
			n.TitleAlign = style.Align
		}
		n.Children = styleSectionedBlocks(n.Children, style)
		return n
	default:
		return &ast.StyledBlock{
			Color:       style.Color,
			Background:  style.Background,
			Align:       style.Align,
			Underline:   style.Underline,
			Bold:        style.Bold,
			Italic:      style.Italic,
			Mono:        style.Mono,
			FontSize:    style.FontSize,
			FontFamily:  style.FontFamily,
			FontStyle:   style.FontStyle,
			FontWeight:  style.FontWeight,
			FontVariant: style.FontVariant,
			Children:    []ast.Block{block},
		}
	}
}

type sectionContainer struct {
	section  *ast.Section
	children []ast.Block
}

type blockParser struct {
	parser   *Parser
	raw      string
	tokens   []lexer.Token
	pos      int
	appendix bool
}

func newBlockParser(p *Parser, raw string, appendix bool) *blockParser {
	return &blockParser{
		parser:   p,
		raw:      raw,
		tokens:   lexer.Tokenize(raw),
		appendix: appendix,
	}
}

func (bp *blockParser) parseSectioned() []ast.Block {
	root := &sectionContainer{children: []ast.Block{}}
	stack := []*sectionContainer{root}
	for {
		bp.skipInsignificant()
		if bp.eof() {
			break
		}
		if bp.commandAt("appendices") {
			bp.appendix = true
			stack = []*sectionContainer{root}
			bp.pos = bp.currentToken().End
			continue
		}
		if sec, ok := bp.readSection(); ok {
			for len(stack) > 1 && stack[len(stack)-1].section.Level >= sec.Level {
				stack = stack[:len(stack)-1]
			}
			parent := stack[len(stack)-1]
			if parent.section != nil {
				parent.section.Children = append(parent.section.Children, sec)
			} else {
				parent.children = append(parent.children, sec)
			}
			stack = append(stack, &sectionContainer{section: sec})
			continue
		}
		blocks := bp.readBlocksUntilSection()
		stack = bp.parser.appendSectionedBlocks(stack, blocks)
	}
	return root.children
}

func (bp *blockParser) parseLoose() []ast.Block {
	var out []ast.Block
	for {
		bp.skipInsignificant()
		if bp.eof() {
			break
		}
		out = append(out, bp.readNextLooseBlocks()...)
	}
	return out
}

func (bp *blockParser) readBlocksUntilSection() []ast.Block {
	var out []ast.Block
	for {
		bp.skipInsignificant()
		if bp.eof() || bp.commandAt("appendices") || bp.atSectionCommand() {
			break
		}
		if blocks, ok := bp.readSectionedGroupBlocks(); ok {
			out = append(out, blocks...)
			continue
		}
		out = append(out, bp.readNextLooseBlocks()...)
	}
	return out
}

func (bp *blockParser) readSectionedGroupBlocks() ([]ast.Block, bool) {
	tok := bp.currentToken()
	if tok.Start != bp.pos || tok.Kind != lexer.LBrace {
		return nil, false
	}
	if !bp.shouldReadGroupAsBlock(tok.Start) {
		return nil, false
	}
	if styled, end, ok := bp.parser.readStyledBlockAt(bp.raw, bp.pos, bp.appendix); ok {
		bp.pos = end
		return styleSectionedBlocks(styled.Children, styled), true
	}
	if grouped, end, ok := bp.parser.readTransparentBlockGroupAt(bp.raw, bp.pos, bp.appendix); ok {
		bp.pos = end
		return grouped, true
	}
	return nil, false
}

func (bp *blockParser) readNextLooseBlocks() []ast.Block {
	if bp.eof() {
		return nil
	}
	if block, ok := bp.readPlaceholder(); ok {
		return []ast.Block{block}
	}
	if blocks, ok := bp.readGroupBlocks(); ok {
		return blocks
	}
	if block, ok := bp.readEnvironmentBlock(); ok {
		if block == nil {
			return nil
		}
		return []ast.Block{block}
	}
	if block, ok := bp.readDisplayMathBlock(); ok {
		return []ast.Block{block}
	}
	if block, ok := bp.readImportHTMLBlock(); ok {
		if block == nil {
			return nil
		}
		return []ast.Block{block}
	}
	if block, ok := bp.readBibliographyBlock(); ok {
		if block == nil {
			return nil
		}
		return []ast.Block{block}
	}
	if bp.commandAt("appendices") {
		bp.appendix = true
		bp.pos = bp.currentToken().End
		return nil
	}
	if bp.atSectionCommand() {
		if sec, ok := bp.readSection(); ok {
			return []ast.Block{sec}
		}
	}
	if block, ok := bp.readParagraph(); ok {
		return []ast.Block{block}
	}
	if tok := bp.currentToken(); tok.Kind != lexer.EOF && tok.End > bp.pos {
		bp.pos = tok.End
	} else {
		bp.pos++
	}
	return nil
}

func (bp *blockParser) skipInsignificant() {
	for {
		bp.pos = skipWhitespace(bp.raw, bp.pos)
		tok := bp.currentToken()
		if tok.Kind != lexer.Comment || tok.Start != bp.pos {
			return
		}
		bp.pos = tok.End
	}
}

func (bp *blockParser) eof() bool {
	return bp.pos >= len(bp.raw) || bp.currentToken().Kind == lexer.EOF
}

func (bp *blockParser) currentToken() lexer.Token {
	idx := bp.tokenIndexAt(bp.pos)
	return bp.tokens[idx]
}

func (bp *blockParser) tokenIndexAt(pos int) int {
	return tokenIndexAt(bp.tokens, pos)
}

func (bp *blockParser) commandAt(cmd string) bool {
	tok := bp.currentToken()
	return tok.Start == bp.pos && tok.Kind == lexer.Command && tok.Value == cmd
}

func (bp *blockParser) atSectionCommand() bool {
	tok := bp.currentToken()
	if tok.Start != bp.pos || tok.Kind != lexer.Command {
		return false
	}
	switch tok.Value {
	case "section", "subsection", "subsubsection", "subsubsubsection":
		return true
	default:
		return false
	}
}

func (bp *blockParser) readSection() (*ast.Section, bool) {
	tok := bp.currentToken()
	if tok.Start != bp.pos || tok.Kind != lexer.Command {
		return nil, false
	}
	if !bp.atSectionCommand() {
		return nil, false
	}
	title, end, ok := readOneCommandArg(bp.raw, bp.pos, tok.Value)
	if !ok {
		bp.parser.warnings = append(bp.parser.warnings, fmt.Sprintf("could not parse \\%s title", tok.Value))
		bp.pos = tok.End
		return nil, false
	}
	parsedTitle := bp.parser.parseTextArgumentWithDeclarations(title)
	sec := &ast.Section{
		Level:      sectionLevel(tok.Value),
		Title:      parsedTitle.Inlines,
		TitleAlign: parsedTitle.Align,
		Appendix:   bp.appendix,
	}
	labelStart := skipWhitespace(bp.raw, end)
	if lexer.IsCommandAt(bp.raw, labelStart, "label") {
		if label, labelEnd, ok := readOneCommandArg(bp.raw, labelStart, "label"); ok {
			sec.Label = strings.TrimSpace(label)
			end = labelEnd
		}
	}
	bp.pos = end
	return sec, true
}

func (bp *blockParser) readPlaceholder() (ast.Block, bool) {
	id, end, ok := bp.parser.readPlaceholderAt(bp.raw, bp.pos)
	if !ok {
		return nil, false
	}
	cb := bp.parser.complex[id]
	bp.pos = end
	return &ast.ComplexHTML{
		BlockID: id,
		EnvName: cb.EnvName,
		RawTeX:  cb.RawTeX,
		HTML:    cb.HTML,
		Caption: cb.Caption,
		Label:   cb.Label,
	}, true
}

func (bp *blockParser) readGroupBlocks() ([]ast.Block, bool) {
	tok := bp.currentToken()
	if tok.Start != bp.pos || tok.Kind != lexer.LBrace {
		return nil, false
	}
	if !bp.shouldReadGroupAsBlock(tok.Start) {
		return nil, false
	}
	if styled, end, ok := bp.parser.readStyledBlockAt(bp.raw, bp.pos, bp.appendix); ok {
		bp.pos = end
		return []ast.Block{styled}, true
	}
	if grouped, end, ok := bp.parser.readTransparentBlockGroupAt(bp.raw, bp.pos, bp.appendix); ok {
		bp.pos = end
		return grouped, true
	}
	return nil, false
}

func (bp *blockParser) shouldReadGroupAsBlock(start int) bool {
	body, end, ok := readBalanced(bp.raw, start, '{', '}')
	if !ok {
		return false
	}
	if groupBodyContainsBlockSyntax(body) {
		return true
	}
	return groupFollowedByParagraphBoundary(bp.raw, end)
}

func (bp *blockParser) readEnvironmentBlock() (ast.Block, bool) {
	tok := bp.currentToken()
	if tok.Start != bp.pos {
		return nil, false
	}
	if tok.Kind == lexer.Raw {
		env, _, ok := readBeginAt(bp.raw, bp.pos)
		if !ok {
			return nil, false
		}
		switch env {
		case "verbatim", "lstlisting", "minted":
			bp.pos = tok.End
			return parseCodeBlock(bp.raw[tok.Start:tok.End], env), true
		case "html":
			bp.pos = tok.End
			return &ast.RawHTML{HTML: stripEnvironmentShell(bp.raw[tok.Start:tok.End], env)}, true
		default:
			return nil, false
		}
	}
	if tok.Kind != lexer.Command || tok.Value != "begin" {
		return nil, false
	}
	env, startEnd, ok := readBeginAt(bp.raw, bp.pos)
	if !ok {
		return nil, false
	}
	rawEnd, hasEnd := findEnvironmentEnd(bp.raw, bp.pos, env)
	if !hasEnd {
		bp.parser.warnings = append(bp.parser.warnings, fmt.Sprintf("could not parse environment %s end", env))
		bp.pos = startEnd
		return nil, false
	}
	raw := bp.raw[bp.pos:rawEnd]
	bp.pos = rawEnd
	switch env {
	case "tcb":
		return bp.parser.parseTCB(raw), true
	case "verbatim", "lstlisting", "minted":
		return parseCodeBlock(raw, env), true
	case "html":
		return &ast.RawHTML{HTML: stripEnvironmentShell(raw, env)}, true
	case "figure", "figure*":
		return bp.parser.parseFigure(raw, env == "figure*"), true
	case "table", "table*":
		return bp.parser.parseTable(raw, env == "table*"), true
	case "equation", "equation*", "align", "align*":
		inner := stripEnvironmentShell(raw, env)
		return &ast.DisplayMath{
			TeX:      strings.TrimSpace(removeLabelCommands(inner)),
			Label:    firstCommandArg(inner, "label"),
			Numbered: !strings.HasSuffix(env, "*"),
		}, true
	case "aligned", "aligned*", "alignedat", "alignedat*", "gathered", "split", "matrix", "pmatrix", "bmatrix", "Bmatrix", "vmatrix", "Vmatrix":
		return &ast.DisplayMath{
			TeX: strings.TrimSpace(raw),
		}, true
	case "enumerate", "itemize", "description":
		return bp.parser.parseList(stripEnvironmentShell(raw, env), env), true
	case "center":
		return &ast.StyledBlock{Align: "center", Children: bp.parser.parseLooseBlocks(stripEnvironmentShell(raw, env))}, true
	case "flushleft":
		return &ast.StyledBlock{Align: "left", Children: bp.parser.parseLooseBlocks(stripEnvironmentShell(raw, env))}, true
	case "flushright":
		return &ast.StyledBlock{Align: "right", Children: bp.parser.parseLooseBlocks(stripEnvironmentShell(raw, env))}, true
	case "quote", "quotation":
		return &ast.StyledBlock{Children: bp.parser.parseLooseBlocks(stripEnvironmentShell(raw, env))}, true
	case "abstract":
		return &ast.AbstractBlock{Children: bp.parser.parseLooseBlocks(stripEnvironmentShell(raw, env))}, true
	case "keywords", "IEEEkeywords":
		return &ast.KeywordsBlock{Inlines: bp.parser.parseTextArgument(strings.TrimSpace(stripEnvironmentShell(raw, env)))}, true
	default:
		bp.parser.warnings = append(bp.parser.warnings, fmt.Sprintf("unsupported environment %s", env))
		return &ast.EnvironmentBlock{EnvName: env, Children: bp.parser.parseSectionedBlocksWithAppendix(stripEnvironmentShell(raw, env), bp.appendix)}, true
	}
}

func (bp *blockParser) readDisplayMathBlock() (ast.Block, bool) {
	if strings.HasPrefix(bp.raw[bp.pos:], "$$") {
		start := bp.pos + len("$$")
		end := findDisplayDollarEnd(bp.raw, start)
		if end < 0 {
			return nil, false
		}
		bp.pos = end + len("$$")
		return &ast.DisplayMath{TeX: strings.TrimSpace(bp.raw[start:end])}, true
	}
	if !bp.commandAt("[") {
		return nil, false
	}
	start := bp.currentToken().End
	end := findUnescapedSequence(bp.raw, start, `\]`)
	if end < 0 {
		return nil, false
	}
	bp.pos = end + len(`\]`)
	return &ast.DisplayMath{TeX: strings.TrimSpace(bp.raw[start:end])}, true
}

func (bp *blockParser) readImportHTMLBlock() (ast.Block, bool) {
	cmd := ""
	if bp.commandAt("importHTML") {
		cmd = "importHTML"
	} else if bp.commandAt("inputHTML") {
		cmd = "inputHTML"
	}
	if cmd == "" {
		return nil, false
	}
	arg, end, ok := readOneCommandArg(bp.raw, bp.pos, cmd)
	if !ok {
		bp.parser.warnings = append(bp.parser.warnings, fmt.Sprintf("could not parse \\%s path", cmd))
		bp.pos = bp.currentToken().End
		return nil, true
	}
	bp.pos = end
	if html, ok := bp.parser.importHTML(cmd, arg); ok {
		return &ast.RawHTML{HTML: html}, true
	}
	return nil, true
}

func (p *Parser) importHTML(cmd, arg string) (string, bool) {
	rel := strings.TrimSpace(arg)
	if rel == "" {
		p.warnings = append(p.warnings, fmt.Sprintf("\\%s path is empty", cmd))
		return "", false
	}
	cleanRel, err := pathutil.CleanRelativePath(rel)
	if err != nil {
		p.warnings = append(p.warnings, fmt.Sprintf("\\%s path not allowed %s: %v", cmd, rel, err))
		return "", false
	}
	baseDir := p.importHTMLBaseDir()
	path := filepath.Join(baseDir, cleanRel)
	data, err := os.ReadFile(path)
	if err != nil {
		p.warnings = append(p.warnings, fmt.Sprintf("\\%s file not found %s: %v", cmd, rel, err))
		return "", false
	}
	html := string(data)
	if !looksLikeHTML(html) {
		p.warnings = append(p.warnings, fmt.Sprintf("\\%s content does not look like HTML: %s", cmd, rel))
	}
	return html, true
}

func (p *Parser) importHTMLBaseDir() string {
	if p.doc != nil && strings.TrimSpace(p.doc.InputFile) != "" {
		return filepath.Dir(p.doc.InputFile)
	}
	if p.doc != nil && strings.TrimSpace(p.doc.SourceRoot) != "" {
		return p.doc.SourceRoot
	}
	return "."
}

func looksLikeHTML(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html") {
		return true
	}
	if strings.HasPrefix(lower, "<!--") && strings.Contains(lower, "-->") {
		return true
	}
	for i := 0; i < len(s); i++ {
		if s[i] != '<' {
			continue
		}
		j := i + 1
		if j < len(s) && s[j] == '/' {
			j++
		}
		if j >= len(s) || !isHTMLNameStart(s[j]) {
			continue
		}
		for j < len(s) && isHTMLNameChar(s[j]) {
			j++
		}
		if j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r' || s[j] == '\f') {
			return true
		}
		if j < len(s) && (s[j] == '>' || s[j] == '/') {
			return true
		}
	}
	return false
}

func isHTMLNameStart(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isHTMLNameChar(c byte) bool {
	return isHTMLNameStart(c) || (c >= '0' && c <= '9') || c == '-' || c == ':' || c == '_'
}

func (bp *blockParser) readBibliographyBlock() (ast.Block, bool) {
	if bp.commandAt("bibliographystyle") {
		if _, end, ok := readOneCommandArg(bp.raw, bp.pos, "bibliographystyle"); ok {
			bp.pos = end
			return nil, true
		}
	}
	if !bp.commandAt("bibliography") {
		return nil, false
	}
	arg, end, ok := readOneCommandArg(bp.raw, bp.pos, "bibliography")
	if !ok {
		return nil, false
	}
	files := splitCSV(arg)
	if bp.parser.doc != nil {
		bp.parser.doc.BibliographyFiles = append(bp.parser.doc.BibliographyFiles, files...)
	}
	bp.pos = end
	return &ast.References{Files: files}, true
}

func (bp *blockParser) readParagraph() (ast.Block, bool) {
	end := bp.nextParagraphBoundary()
	if end <= bp.pos {
		return nil, false
	}
	para := normalizeParagraph(bp.raw[bp.pos:end])
	bp.pos = end
	if para == "" {
		return nil, true
	}
	parsed := bp.parser.parseTextArgumentWithDeclarations(para)
	return &ast.Paragraph{Inlines: parsed.Inlines, Align: parsed.Align}, true
}

func (bp *blockParser) nextParagraphBoundary() int {
	best := len(bp.raw)
	start := bp.pos
	braceDepth := 0
	bracketDepth := 0
	for _, tok := range bp.tokens[bp.tokenIndexAt(start):] {
		if tok.Kind == lexer.EOF {
			break
		}
		if tok.End <= start {
			continue
		}
		if tok.Kind == lexer.Text && braceDepth == 0 && bracketDepth == 0 {
			searchStart := 0
			if start > tok.Start {
				searchStart = start - tok.Start
			}
			if idx := firstBlankLine(tok.Value[searchStart:]); idx >= 0 {
				pos := tok.Start + searchStart + idx
				if pos > start && pos < best {
					best = pos
					break
				}
			}
		}
		if tok.Start <= start {
			continue
		}
		if braceDepth == 0 && bracketDepth == 0 && bp.isBlockBoundaryToken(tok, start) {
			best = tok.Start
			break
		}
		switch tok.Kind {
		case lexer.LBrace:
			braceDepth++
		case lexer.RBrace:
			if braceDepth > 0 {
				braceDepth--
			}
		case lexer.LBracket:
			bracketDepth++
		case lexer.RBracket:
			if bracketDepth > 0 {
				bracketDepth--
			}
		}
	}
	return best
}

func (bp *blockParser) isBlockBoundaryToken(tok lexer.Token, paragraphStart int) bool {
	switch tok.Kind {
	case lexer.Raw:
		if isVerbRawToken(tok.Value) {
			return false
		}
		env, _, ok := readBeginAt(bp.raw, tok.Start)
		return !ok || env != "html"
	case lexer.LBrace:
		return isBlockStyledGroupCandidate(bp.raw, paragraphStart, tok.Start)
	case lexer.Dollar:
		return strings.HasPrefix(bp.raw[tok.Start:], "$$")
	case lexer.Command:
		switch tok.Value {
		case "begin", "[", "bibliographystyle", "bibliography", "appendices", "section", "subsection", "subsubsection", "subsubsubsection":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func isVerbRawToken(value string) bool {
	return strings.HasPrefix(value, `\verb`)
}

func (p *Parser) parseLooseBlocks(s string) []ast.Block {
	return newBlockParser(p, s, false).parseLoose()
}

func (p *Parser) parseTCB(raw string) ast.Block {
	if !strings.HasPrefix(raw, `\begin{tcb}`) {
		p.warnings = append(p.warnings, "could not parse tcb environment")
		return &ast.TCB{}
	}
	beginEnd := len(`\begin{tcb}`)
	endStart := strings.LastIndex(raw, `\end{tcb}`)
	if endStart < beginEnd {
		p.warnings = append(p.warnings, "could not parse tcb environment end")
		return &ast.TCB{}
	}
	inner := raw[beginEnd:endStart]
	i := skipWhitespace(inner, 0)
	var opts []string
	for len(opts) < 3 {
		i = skipWhitespace(inner, i)
		if i >= len(inner) || inner[i] != '[' {
			break
		}
		arg, end, ok := readBalanced(inner, i, '[', ']')
		if !ok {
			break
		}
		opts = append(opts, strings.TrimSpace(arg))
		i = end
	}
	i = skipWhitespace(inner, i)
	title := ""
	if i < len(inner) && inner[i] == '{' {
		arg, end, ok := readBalanced(inner, i, '{', '}')
		if ok {
			title = arg
			i = end
		} else {
			p.warnings = append(p.warnings, "could not parse tcb title")
		}
	} else {
		p.warnings = append(p.warnings, "missing tcb title")
	}
	titleInlines, titleAlign := p.parseTCBTitle(title)
	titleRaw := "gray!70"
	if len(opts) > 0 && strings.TrimSpace(opts[0]) != "" {
		titleRaw = opts[0]
	}
	titleColor := normalizeLaTeXColor(titleRaw)
	if titleColor == "" {
		titleColor = normalizeLaTeXColor("gray!70")
	}
	borderColor := ""
	if len(opts) > 1 && strings.TrimSpace(opts[1]) != "" {
		borderColor = normalizeLaTeXColor(opts[1])
	}
	if borderColor == "" {
		borderColor = contrastSaturateLaTeXColor(titleRaw, titleColor)
	}
	bodyColor := ""
	if len(opts) > 2 && strings.TrimSpace(opts[2]) != "" {
		bodyColor = normalizeLaTeXColor(opts[2])
	}
	if bodyColor == "" {
		bodyColor = lightenLaTeXColor(titleRaw, titleColor, 0.8)
	}
	return &ast.TCB{
		Title:           titleInlines,
		TitleAlign:      titleAlign,
		TitleBackground: titleColor,
		BorderColor:     borderColor,
		BodyBackground:  bodyColor,
		Children:        p.parseLooseBlocks(inner[i:]),
	}
}

func (p *Parser) parseTCBTitle(s string) ([]ast.Inline, string) {
	parsed := p.parseTextArgumentWithDeclarations(s)
	return parsed.Inlines, parsed.Align
}

func parseCodeBlock(raw, env string) ast.Block {
	_, start, ok := readBeginAt(raw, 0)
	if !ok {
		return &ast.CodeBlock{EnvName: env, Text: raw}
	}
	language := ""
	if env == "minted" {
		i := skipWhitespace(raw, start)
		if i < len(raw) && raw[i] == '[' {
			if _, end, ok := readBalanced(raw, i, '[', ']'); ok {
				i = skipWhitespace(raw, end)
			}
		}
		if i < len(raw) && raw[i] == '{' {
			if arg, end, ok := readBalanced(raw, i, '{', '}'); ok {
				language = strings.TrimSpace(arg)
				start = end
			}
		}
	}
	if env == "lstlisting" {
		// readBeginAt already consumed \begin{lstlisting}[...], setting start
		// past the optional args. Look for [...] between \begin{lstlisting} and start.
		beginIdx := strings.Index(raw, `\begin{lstlisting}`)
		if beginIdx >= 0 {
			j := beginIdx + len(`\begin{lstlisting}`)
			j = skipWhitespace(raw, j)
			if j < len(raw) && raw[j] == '{' {
				if _, end, ok := readBalanced(raw, j, '{', '}'); ok {
					j = end
				}
			}
			if j < len(raw) && raw[j] == '[' {
				opts, end, ok := readBalanced(raw, j, '[', ']')
				if ok {
					language = extractListingsOption(opts, "language")
					start = end
				}
			}
		}
	}
	endStart, _, ok := findEnvironmentClose(raw, 0, env)
	if !ok || endStart < start {
		return &ast.CodeBlock{EnvName: env, Language: language, Text: strings.Trim(raw[start:], "\r\n")}
	}
	return &ast.CodeBlock{
		EnvName:  env,
		Language: language,
		Text:     strings.Trim(raw[start:endStart], "\r\n"),
	}
}

func extractListingsOption(opts, key string) string {
	for _, part := range splitTopLevel(opts, ',') {
		k, v, ok := cutTopLevel(part, '=')
		if !ok || !strings.EqualFold(strings.TrimSpace(k), key) {
			continue
		}
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, "{") {
			if unwrapped, end, ok := readBalanced(v, 0, '{', '}'); ok && strings.TrimSpace(v[end:]) == "" {
				return strings.TrimSpace(unwrapped)
			}
		}
		return v
	}
	return ""
}

func lightenLaTeXColor(raw, fallback string, amount float64) string {
	r, g, b, ok := latexColorRGB(raw)
	if !ok {
		return "color-mix(in srgb, " + fallback + " 20%, white)"
	}
	r = mixChannel(r, 255, amount)
	g = mixChannel(g, 255, amount)
	b = mixChannel(b, 255, amount)
	return rgbHex(r, g, b)
}

func contrastSaturateLaTeXColor(raw, fallback string) string {
	r, g, b, ok := latexColorRGB(raw)
	if !ok {
		return "color-mix(in srgb, " + fallback + " 80%, black)"
	}
	r = clampInt(int(math.Round((float64(r)-128)*1.2 + 128)))
	g = clampInt(int(math.Round((float64(g)-128)*1.2 + 128)))
	b = clampInt(int(math.Round((float64(b)-128)*1.2 + 128)))
	h, s, l := rgbToHSL(r, g, b)
	s *= 1.2
	if s > 1 {
		s = 1
	}
	r, g, b = hslToRGB(h, s, l)
	return rgbHex(r, g, b)
}

func latexColorRGB(raw string) (int, int, int, bool) {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, "{}")
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return 0, 0, 0, false
	}
	if strings.HasPrefix(s, "#") {
		return hexColorRGB(s)
	}
	if strings.Contains(s, "!") {
		parts := strings.Split(s, "!")
		if len(parts) >= 2 {
			r, g, b, ok := namedColorRGB(parts[0])
			if !ok {
				return 0, 0, 0, false
			}
			percent := 0.0
			if _, err := fmt.Sscanf(parts[1], "%f", &percent); err != nil {
				return 0, 0, 0, false
			}
			weight := percent / 100
			r = clampInt(int(math.Round(float64(r)*weight + 255*(1-weight))))
			g = clampInt(int(math.Round(float64(g)*weight + 255*(1-weight))))
			b = clampInt(int(math.Round(float64(b)*weight + 255*(1-weight))))
			return r, g, b, true
		}
	}
	return namedColorRGB(s)
}

func namedColorRGB(s string) (int, int, int, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "black":
		return 0, 0, 0, true
	case "white":
		return 255, 255, 255, true
	case "red":
		return 255, 0, 0, true
	case "green":
		return 0, 128, 0, true
	case "blue":
		return 0, 0, 255, true
	case "cyan":
		return 0, 255, 255, true
	case "magenta":
		return 255, 0, 255, true
	case "yellow":
		return 255, 255, 0, true
	case "gray", "grey":
		return 128, 128, 128, true
	case "orange":
		return 255, 165, 0, true
	case "purple":
		return 128, 0, 128, true
	case "brown":
		return 165, 42, 42, true
	case "pink":
		return 255, 192, 203, true
	default:
		return 0, 0, 0, false
	}
}

func hexColorRGB(s string) (int, int, int, bool) {
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	var r, g, b int
	if _, err := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b); err != nil {
		return 0, 0, 0, false
	}
	return r, g, b, true
}

func mixChannel(a, b int, amount float64) int {
	return clampInt(int(math.Round(float64(a)*(1-amount) + float64(b)*amount)))
}

func rgbHex(r, g, b int) string {
	return fmt.Sprintf("#%02x%02x%02x", clampInt(r), clampInt(g), clampInt(b))
}

func clampInt(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

func rgbToHSL(r, g, b int) (float64, float64, float64) {
	rf := float64(r) / 255
	gf := float64(g) / 255
	bf := float64(b) / 255
	maxv := math.Max(rf, math.Max(gf, bf))
	minv := math.Min(rf, math.Min(gf, bf))
	l := (maxv + minv) / 2
	if maxv == minv {
		return 0, 0, l
	}
	d := maxv - minv
	s := d / (1 - math.Abs(2*l-1))
	h := 0.0
	switch maxv {
	case rf:
		h = math.Mod((gf-bf)/d, 6)
	case gf:
		h = (bf-rf)/d + 2
	case bf:
		h = (rf-gf)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, l
}

func hslToRGB(h, s, l float64) (int, int, int) {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var rf, gf, bf float64
	switch {
	case h < 60:
		rf, gf, bf = c, x, 0
	case h < 120:
		rf, gf, bf = x, c, 0
	case h < 180:
		rf, gf, bf = 0, c, x
	case h < 240:
		rf, gf, bf = 0, x, c
	case h < 300:
		rf, gf, bf = x, 0, c
	default:
		rf, gf, bf = c, 0, x
	}
	return clampInt(int(math.Round((rf + m) * 255))),
		clampInt(int(math.Round((gf + m) * 255))),
		clampInt(int(math.Round((bf + m) * 255)))
}

func (p *Parser) readPlaceholderAt(s string, i int) (string, int, bool) {
	if !strings.HasPrefix(s[i:], "@@METABLOG_COMPLEX_BLOCK_") {
		return "", i, false
	}
	end := strings.Index(s[i+2:], "@@")
	if end < 0 {
		return "", i, false
	}
	id := s[i : i+2+end+2]
	if _, ok := p.complex[id]; !ok {
		return "", i, false
	}
	return id, i + len(id), true
}

func (p *Parser) parseFigure(raw string, starred bool) ast.Block {
	env := "figure"
	if starred {
		env = "figure*"
	}
	inner := stripEnvironmentShell(raw, env)
	return newFigureParser(p, inner).parse(starred)
}

func (p *Parser) parseTable(raw string, starred bool) ast.Block {
	env := "table"
	if starred {
		env = "table*"
	}
	inner := stripEnvironmentShell(raw, env)
	return newTableParser(p, inner).parse(starred)
}

type figureParser struct {
	parser       *Parser
	raw          string
	tokens       []lexer.Token
	images       []*ast.Image
	subfigures   []*ast.Subfigure
	caption      []ast.Inline
	captionAlign string
	captionSeen  bool
	firstLabel   string
	captionLabel string
}

func newFigureParser(p *Parser, raw string) *figureParser {
	return &figureParser{
		parser: p,
		raw:    raw,
		tokens: lexer.Tokenize(raw),
	}
}

func (fp *figureParser) parse(starred bool) *ast.Figure {
	fp.parseContent()
	var first *ast.Image
	if len(fp.images) > 0 {
		first = fp.images[0]
	}
	return &ast.Figure{
		Starred:      starred,
		Image:        first,
		Images:       fp.images,
		Subfigures:   fp.subfigures,
		Caption:      fp.caption,
		CaptionAlign: fp.captionAlign,
		Label:        fp.label(),
	}
}

func (fp *figureParser) parseContent() {
	for i := 0; i < len(fp.tokens); {
		tok := fp.tokens[i]
		switch tok.Kind {
		case lexer.EOF:
			return
		case lexer.LBracket:
			i = fp.skipBalanced(i)
			continue
		case lexer.Command:
			switch tok.Value {
			case "includegraphics":
				image, next, ok := fp.readIncludeGraphics(i)
				if ok {
					fp.images = append(fp.images, image)
					i = next
					continue
				}
			case "subfloat":
				next := fp.readSubfloat(i)
				if next > i {
					i = next
					continue
				}
			case "resizebox":
				next := fp.readLayoutWrapper(i, 3)
				if next > i {
					i = next
					continue
				}
			case "scalebox", "rotatebox", "adjustbox":
				next := fp.readLayoutWrapper(i, 2)
				if next > i {
					i = next
					continue
				}
			case "makebox", "fbox":
				next := fp.readLayoutWrapper(i, 1)
				if next > i {
					i = next
					continue
				}
			case "caption":
				next := fp.readCaption(i)
				if next > i {
					i = next
					continue
				}
			case "label":
				label, next, ok := fp.readRequiredArgAfter(i)
				if ok {
					label = strings.TrimSpace(label)
					if label != "" {
						if fp.firstLabel == "" {
							fp.firstLabel = label
						}
						if fp.captionSeen && fp.captionLabel == "" {
							fp.captionLabel = label
						}
					}
					i = next
					continue
				}
			}
			if isFigureDeclaration(tok.Value) {
				i++
				continue
			}
			next := fp.skipCommandArguments(i)
			if next > i+1 {
				i = next
				continue
			}
		}
		i++
	}
}

func (fp *figureParser) label() string {
	if fp.captionLabel != "" {
		return fp.captionLabel
	}
	return fp.firstLabel
}

func isFigureDeclaration(cmd string) bool {
	switch cmd {
	case "centering", "raggedright", "raggedleft",
		"tiny", "scriptsize", "footnotesize", "small", "normalsize", "large", "Large", "LARGE", "huge", "Huge",
		"normalfont", "upshape", "itshape", "slshape", "scshape", "bfseries", "mdseries", "rmfamily", "sffamily", "ttfamily":
		return true
	default:
		return false
	}
}

func (fp *figureParser) readIncludeGraphics(cmdIdx int) (*ast.Image, int, bool) {
	i := fp.skipSpaces(cmdIdx + 1)
	options := ""
	if i < len(fp.tokens) && fp.tokens[i].Kind == lexer.LBracket {
		arg, next, ok := fp.readGroup(i, lexer.LBracket, lexer.RBracket)
		if !ok {
			return nil, cmdIdx + 1, false
		}
		options = arg
		i = fp.skipSpaces(next)
	}
	if i >= len(fp.tokens) || fp.tokens[i].Kind != lexer.LBrace {
		return nil, cmdIdx + 1, false
	}
	source, next, ok := fp.readGroup(i, lexer.LBrace, lexer.RBrace)
	if !ok {
		return nil, cmdIdx + 1, false
	}
	return &ast.Image{
		SourcePath: filepath.ToSlash(strings.TrimSpace(source)),
		Options:    parseOptions(options),
	}, next, true
}

func (fp *figureParser) readSubfloat(cmdIdx int) int {
	i := fp.skipSpaces(cmdIdx + 1)
	var optionalCaption []ast.Inline
	for i < len(fp.tokens) && fp.tokens[i].Kind == lexer.LBracket {
		arg, next, ok := fp.readGroup(i, lexer.LBracket, lexer.RBracket)
		if !ok {
			return cmdIdx + 1
		}
		if strings.TrimSpace(arg) != "" {
			optionalCaption = fp.parser.parseTextArgument(arg)
		}
		i = fp.skipSpaces(next)
	}
	if i >= len(fp.tokens) || fp.tokens[i].Kind != lexer.LBrace {
		return cmdIdx + 1
	}
	body, next, ok := fp.readGroup(i, lexer.LBrace, lexer.RBrace)
	if !ok {
		return cmdIdx + 1
	}
	nested := newFigureParser(fp.parser, body)
	nested.parseContent()
	imageIndex := len(fp.images)
	caption := optionalCaption
	if len(nested.caption) > 0 {
		caption = nested.caption
	}
	fp.subfigures = append(fp.subfigures, &ast.Subfigure{
		ImageIndex: imageIndex,
		Label:      nested.label(),
		Caption:    caption,
		BreakAfter: fp.subfloatBreakAfter(fp.tokens[next-1].End, next),
	})
	fp.images = append(fp.images, nested.images...)
	return next
}

func (fp *figureParser) readLayoutWrapper(cmdIdx int, requiredArgs int) int {
	i := fp.skipSpaces(cmdIdx + 1)
	for i < len(fp.tokens) && fp.tokens[i].Kind == lexer.LBracket {
		_, next, ok := fp.readGroup(i, lexer.LBracket, lexer.RBracket)
		if !ok {
			return cmdIdx + 1
		}
		i = fp.skipSpaces(next)
	}
	body := ""
	bodyEnd := cmdIdx + 1
	for arg := 0; arg < requiredArgs; arg++ {
		if i >= len(fp.tokens) || fp.tokens[i].Kind != lexer.LBrace {
			return cmdIdx + 1
		}
		group, next, ok := fp.readGroup(i, lexer.LBrace, lexer.RBrace)
		if !ok {
			return cmdIdx + 1
		}
		if arg == requiredArgs-1 {
			body = group
			bodyEnd = next
		}
		i = fp.skipSpaces(next)
	}
	nested := newFigureParser(fp.parser, body)
	nested.parseContent()
	fp.mergeNested(nested)
	return bodyEnd
}

func (fp *figureParser) mergeNested(nested *figureParser) {
	if nested == nil {
		return
	}
	imageOffset := len(fp.images)
	fp.images = append(fp.images, nested.images...)
	for _, sub := range nested.subfigures {
		if sub == nil {
			continue
		}
		cp := *sub
		cp.ImageIndex += imageOffset
		fp.subfigures = append(fp.subfigures, &cp)
	}
	if !fp.captionSeen && nested.captionSeen {
		fp.caption = nested.caption
		fp.captionAlign = nested.captionAlign
		fp.captionSeen = true
	}
	if fp.firstLabel == "" && nested.firstLabel != "" {
		fp.firstLabel = nested.firstLabel
	}
	if fp.captionLabel == "" && nested.captionLabel != "" {
		fp.captionLabel = nested.captionLabel
	}
}

func (fp *figureParser) readCaption(cmdIdx int) int {
	i := fp.skipSpaces(cmdIdx + 1)
	if i < len(fp.tokens) && fp.tokens[i].Kind == lexer.LBracket {
		_, next, ok := fp.readGroup(i, lexer.LBracket, lexer.RBracket)
		if !ok {
			return cmdIdx + 1
		}
		i = fp.skipSpaces(next)
	}
	if i >= len(fp.tokens) || fp.tokens[i].Kind != lexer.LBrace {
		return cmdIdx + 1
	}
	caption, next, ok := fp.readGroup(i, lexer.LBrace, lexer.RBrace)
	if !ok {
		return cmdIdx + 1
	}
	if !fp.captionSeen {
		parsed := fp.parser.parseTextArgumentWithDeclarations(caption)
		fp.caption = parsed.Inlines
		fp.captionAlign = parsed.Align
		fp.captionSeen = true
	}
	return next
}

func (fp *figureParser) readRequiredArgAfter(cmdIdx int) (string, int, bool) {
	i := fp.skipSpaces(cmdIdx + 1)
	if i >= len(fp.tokens) || fp.tokens[i].Kind != lexer.LBrace {
		return "", cmdIdx + 1, false
	}
	return fp.readGroup(i, lexer.LBrace, lexer.RBrace)
}

func (fp *figureParser) readGroup(idx int, open, close lexer.Kind) (string, int, bool) {
	if idx >= len(fp.tokens) || fp.tokens[idx].Kind != open {
		return "", idx, false
	}
	if close != lexer.RBrace && close != lexer.RBracket {
		return "", idx, false
	}
	start := fp.tokens[idx].End
	if open == lexer.LBrace {
		depth := 1
		for i := idx + 1; i < len(fp.tokens); i++ {
			tok := fp.tokens[i]
			switch tok.Kind {
			case lexer.LBrace:
				depth++
			case lexer.RBrace:
				depth--
				if depth == 0 {
					return fp.raw[start:tok.Start], i + 1, true
				}
			case lexer.EOF:
				return "", idx, false
			}
		}
		return "", idx, false
	}
	if open != lexer.LBracket {
		return "", idx, false
	}
	braceDepth := 0
	bracketDepth := 1
	for i := idx + 1; i < len(fp.tokens); i++ {
		tok := fp.tokens[i]
		switch tok.Kind {
		case lexer.LBrace:
			braceDepth++
		case lexer.RBrace:
			if braceDepth > 0 {
				braceDepth--
			}
		case lexer.LBracket:
			if braceDepth == 0 {
				bracketDepth++
			}
		case lexer.RBracket:
			if braceDepth == 0 {
				bracketDepth--
				if bracketDepth == 0 {
					return fp.raw[start:tok.Start], i + 1, true
				}
			}
		case lexer.EOF:
			return "", idx, false
		}
	}
	return "", idx, false
}

func (fp *figureParser) skipBalanced(idx int) int {
	tok := fp.tokens[idx]
	switch tok.Kind {
	case lexer.LBrace:
		_, next, ok := fp.readGroup(idx, lexer.LBrace, lexer.RBrace)
		if ok {
			return next
		}
	case lexer.LBracket:
		_, next, ok := fp.readGroup(idx, lexer.LBracket, lexer.RBracket)
		if ok {
			return next
		}
	}
	return idx + 1
}

func (fp *figureParser) skipCommandArguments(cmdIdx int) int {
	i := fp.skipSpaces(cmdIdx + 1)
	for i < len(fp.tokens) && fp.tokens[i].Kind == lexer.LBracket {
		_, next, ok := fp.readGroup(i, lexer.LBracket, lexer.RBracket)
		if !ok {
			return cmdIdx + 1
		}
		i = fp.skipSpaces(next)
	}
	if i < len(fp.tokens) && fp.tokens[i].Kind == lexer.LBrace {
		_, next, ok := fp.readGroup(i, lexer.LBrace, lexer.RBrace)
		if ok {
			return next
		}
	}
	return cmdIdx + 1
}

func (fp *figureParser) skipSpaces(idx int) int {
	for idx < len(fp.tokens) {
		tok := fp.tokens[idx]
		switch tok.Kind {
		case lexer.Comment:
			idx++
		case lexer.Text:
			if strings.TrimSpace(tok.Value) == "" {
				idx++
				continue
			}
			return idx
		default:
			return idx
		}
	}
	return idx
}

func (fp *figureParser) subfloatBreakAfter(endOffset int, endIdx int) bool {
	i := skipWhitespace(fp.raw, endOffset)
	if strings.HasPrefix(fp.raw[i:], `\\`) {
		return true
	}
	boundary := fp.nextSubfloatBoundary(endIdx)
	return hasBlankLine(fp.raw[endOffset:boundary])
}

func (fp *figureParser) nextSubfloatBoundary(startIdx int) int {
	for i := startIdx; i < len(fp.tokens); i++ {
		tok := fp.tokens[i]
		switch tok.Kind {
		case lexer.Command:
			if tok.Value == "subfloat" || tok.Value == "caption" {
				return tok.Start
			}
		case lexer.LBrace, lexer.LBracket:
			i = fp.skipBalanced(i) - 1
		}
	}
	return len(fp.raw)
}

type tableParser struct {
	parser       *Parser
	raw          string
	tokens       []lexer.Token
	skipSpans    []span
	subtables    []*ast.Subtable
	caption      []ast.Inline
	captionAlign string
	captionSeen  bool
	firstLabel   string
	captionLabel string
}

func newTableParser(p *Parser, raw string) *tableParser {
	return &tableParser{
		parser: p,
		raw:    raw,
		tokens: lexer.Tokenize(raw),
	}
}

func (tp *tableParser) parse(starred bool) *ast.Table {
	tp.parseContent()
	return &ast.Table{
		Starred:      starred,
		Children:     tp.parser.parseLooseBlocks(removeSpans(tp.raw, tp.skipSpans)),
		Subtables:    tp.subtables,
		Caption:      tp.caption,
		CaptionAlign: tp.captionAlign,
		Label:        tp.label(),
	}
}

func (tp *tableParser) parseSubtable() *ast.Subtable {
	tp.parseContent()
	return &ast.Subtable{
		Blocks:  tp.parser.parseLooseBlocks(removeSpans(tp.raw, tp.skipSpans)),
		Label:   tp.label(),
		Caption: tp.caption,
	}
}

func (tp *tableParser) parseContent() {
	for i := 0; i < len(tp.tokens); {
		tok := tp.tokens[i]
		switch tok.Kind {
		case lexer.EOF:
			return
		case lexer.LBracket:
			i = tp.skipBalanced(i)
			continue
		case lexer.Command:
			switch tok.Value {
			case "subfloat":
				next := tp.readSubfloat(i)
				if next > i {
					i = next
					continue
				}
			case "resizebox":
				next := tp.unwrapLayoutCommand(i, 3)
				if next > i {
					i = next
					continue
				}
			case "scalebox", "rotatebox", "adjustbox":
				next := tp.unwrapLayoutCommand(i, 2)
				if next > i {
					i = next
					continue
				}
			case "makebox", "fbox":
				next := tp.unwrapLayoutCommand(i, 1)
				if next > i {
					i = next
					continue
				}
			case "setlength", "addtolength", "renewcommand":
				next := tp.removeLayoutCommand(i, 2)
				if next > i {
					i = next
					continue
				}
			case "vspace", "hspace":
				next := tp.removeLayoutCommand(i, 1)
				if next > i {
					i = next
					continue
				}
			case "caption":
				next := tp.readCaption(i)
				if next > i {
					i = next
					continue
				}
			case "label":
				label, next, ok := tp.readRequiredArgAfter(i)
				if ok {
					label = strings.TrimSpace(label)
					if label != "" {
						if tp.firstLabel == "" {
							tp.firstLabel = label
						}
						if tp.captionSeen && tp.captionLabel == "" {
							tp.captionLabel = label
						}
					}
					tp.skipSpans = append(tp.skipSpans, span{start: tok.Start, end: tp.tokens[next-1].End})
					i = next
					continue
				}
			}
			if isFigureDeclaration(tok.Value) {
				i++
				continue
			}
			next := tp.skipCommandArguments(i)
			if next > i+1 {
				i = next
				continue
			}
		}
		i++
	}
}

func (tp *tableParser) label() string {
	if tp.captionLabel != "" {
		return tp.captionLabel
	}
	return tp.firstLabel
}

func (tp *tableParser) readSubfloat(cmdIdx int) int {
	i := tp.skipSpaces(cmdIdx + 1)
	var optionalCaption []ast.Inline
	for i < len(tp.tokens) && tp.tokens[i].Kind == lexer.LBracket {
		arg, next, ok := tp.readGroup(i, lexer.LBracket, lexer.RBracket)
		if !ok {
			return cmdIdx + 1
		}
		if strings.TrimSpace(arg) != "" {
			optionalCaption = tp.parser.parseTextArgument(arg)
		}
		i = tp.skipSpaces(next)
	}
	if i >= len(tp.tokens) || tp.tokens[i].Kind != lexer.LBrace {
		return cmdIdx + 1
	}
	body, next, ok := tp.readGroup(i, lexer.LBrace, lexer.RBrace)
	if !ok {
		return cmdIdx + 1
	}
	nested := newTableParser(tp.parser, body)
	sub := nested.parseSubtable()
	if len(sub.Caption) == 0 {
		sub.Caption = optionalCaption
	}
	sub.BreakAfter = tp.subfloatBreakAfter(tp.tokens[next-1].End, next)
	tp.subtables = append(tp.subtables, sub)
	tp.skipSpans = append(tp.skipSpans, span{start: tp.tokens[cmdIdx].Start, end: tp.tokens[next-1].End})
	return next
}

func (tp *tableParser) readCaption(cmdIdx int) int {
	i := tp.skipSpaces(cmdIdx + 1)
	if i < len(tp.tokens) && tp.tokens[i].Kind == lexer.LBracket {
		_, next, ok := tp.readGroup(i, lexer.LBracket, lexer.RBracket)
		if !ok {
			return cmdIdx + 1
		}
		i = tp.skipSpaces(next)
	}
	if i >= len(tp.tokens) || tp.tokens[i].Kind != lexer.LBrace {
		return cmdIdx + 1
	}
	caption, next, ok := tp.readGroup(i, lexer.LBrace, lexer.RBrace)
	if !ok {
		return cmdIdx + 1
	}
	if !tp.captionSeen {
		parsed := tp.parser.parseTextArgumentWithDeclarations(caption)
		tp.caption = parsed.Inlines
		tp.captionAlign = parsed.Align
		tp.captionSeen = true
	}
	tp.skipSpans = append(tp.skipSpans, span{start: tp.tokens[cmdIdx].Start, end: tp.tokens[next-1].End})
	return next
}

func (tp *tableParser) readRequiredArgAfter(cmdIdx int) (string, int, bool) {
	i := tp.skipSpaces(cmdIdx + 1)
	if i >= len(tp.tokens) || tp.tokens[i].Kind != lexer.LBrace {
		return "", cmdIdx + 1, false
	}
	return tp.readGroup(i, lexer.LBrace, lexer.RBrace)
}

func (tp *tableParser) readGroup(idx int, open, close lexer.Kind) (string, int, bool) {
	if idx >= len(tp.tokens) || tp.tokens[idx].Kind != open {
		return "", idx, false
	}
	start := tp.tokens[idx].End
	if open == lexer.LBrace {
		depth := 1
		for i := idx + 1; i < len(tp.tokens); i++ {
			tok := tp.tokens[i]
			switch tok.Kind {
			case lexer.LBrace:
				depth++
			case lexer.RBrace:
				depth--
				if depth == 0 {
					return tp.raw[start:tok.Start], i + 1, true
				}
			case lexer.EOF:
				return "", idx, false
			}
		}
		return "", idx, false
	}
	if open != lexer.LBracket || close != lexer.RBracket {
		return "", idx, false
	}
	braceDepth := 0
	bracketDepth := 1
	for i := idx + 1; i < len(tp.tokens); i++ {
		tok := tp.tokens[i]
		switch tok.Kind {
		case lexer.LBrace:
			braceDepth++
		case lexer.RBrace:
			if braceDepth > 0 {
				braceDepth--
			}
		case lexer.LBracket:
			if braceDepth == 0 {
				bracketDepth++
			}
		case lexer.RBracket:
			if braceDepth == 0 {
				bracketDepth--
				if bracketDepth == 0 {
					return tp.raw[start:tok.Start], i + 1, true
				}
			}
		case lexer.EOF:
			return "", idx, false
		}
	}
	return "", idx, false
}

func (tp *tableParser) skipBalanced(idx int) int {
	tok := tp.tokens[idx]
	switch tok.Kind {
	case lexer.LBrace:
		_, next, ok := tp.readGroup(idx, lexer.LBrace, lexer.RBrace)
		if ok {
			return next
		}
	case lexer.LBracket:
		_, next, ok := tp.readGroup(idx, lexer.LBracket, lexer.RBracket)
		if ok {
			return next
		}
	}
	return idx + 1
}

func (tp *tableParser) skipCommandArguments(cmdIdx int) int {
	i := tp.skipSpaces(cmdIdx + 1)
	for i < len(tp.tokens) && tp.tokens[i].Kind == lexer.LBracket {
		_, next, ok := tp.readGroup(i, lexer.LBracket, lexer.RBracket)
		if !ok {
			return cmdIdx + 1
		}
		i = tp.skipSpaces(next)
	}
	if i < len(tp.tokens) && tp.tokens[i].Kind == lexer.LBrace {
		_, next, ok := tp.readGroup(i, lexer.LBrace, lexer.RBrace)
		if ok {
			return next
		}
	}
	return cmdIdx + 1
}

func (tp *tableParser) unwrapLayoutCommand(cmdIdx int, requiredArgs int) int {
	i := tp.skipSpaces(cmdIdx + 1)
	for i < len(tp.tokens) && tp.tokens[i].Kind == lexer.LBracket {
		_, next, ok := tp.readGroup(i, lexer.LBracket, lexer.RBracket)
		if !ok {
			return cmdIdx + 1
		}
		i = tp.skipSpaces(next)
	}
	for arg := 0; arg < requiredArgs-1; arg++ {
		if i >= len(tp.tokens) || tp.tokens[i].Kind != lexer.LBrace {
			return cmdIdx + 1
		}
		_, next, ok := tp.readGroup(i, lexer.LBrace, lexer.RBrace)
		if !ok {
			return cmdIdx + 1
		}
		i = tp.skipSpaces(next)
	}
	if i >= len(tp.tokens) || tp.tokens[i].Kind != lexer.LBrace {
		return cmdIdx + 1
	}
	_, next, ok := tp.readGroup(i, lexer.LBrace, lexer.RBrace)
	if !ok {
		return cmdIdx + 1
	}
	tp.skipSpans = append(tp.skipSpans,
		span{start: tp.tokens[cmdIdx].Start, end: tp.tokens[i].End},
		span{start: tp.tokens[next-1].Start, end: tp.tokens[next-1].End},
	)
	return i + 1
}

func (tp *tableParser) removeLayoutCommand(cmdIdx int, requiredArgs int) int {
	i := tp.skipSpaces(cmdIdx + 1)
	if i < len(tp.tokens) && tp.tokens[i].Kind == lexer.LBracket {
		_, next, ok := tp.readGroup(i, lexer.LBracket, lexer.RBracket)
		if !ok {
			return cmdIdx + 1
		}
		i = tp.skipSpaces(next)
	}
	for arg := 0; arg < requiredArgs; arg++ {
		if i >= len(tp.tokens) || tp.tokens[i].Kind != lexer.LBrace {
			return cmdIdx + 1
		}
		_, next, ok := tp.readGroup(i, lexer.LBrace, lexer.RBrace)
		if !ok {
			return cmdIdx + 1
		}
		i = tp.skipSpaces(next)
	}
	tp.skipSpans = append(tp.skipSpans, span{start: tp.tokens[cmdIdx].Start, end: tp.tokens[i-1].End})
	return i
}

func (tp *tableParser) skipSpaces(idx int) int {
	for idx < len(tp.tokens) {
		tok := tp.tokens[idx]
		switch tok.Kind {
		case lexer.Comment:
			idx++
		case lexer.Text:
			if strings.TrimSpace(tok.Value) == "" {
				idx++
				continue
			}
			return idx
		default:
			return idx
		}
	}
	return idx
}

func (tp *tableParser) subfloatBreakAfter(endOffset int, endIdx int) bool {
	i := skipWhitespace(tp.raw, endOffset)
	if strings.HasPrefix(tp.raw[i:], `\\`) {
		return true
	}
	boundary := tp.nextSubfloatBoundary(endIdx)
	return hasBlankLine(tp.raw[endOffset:boundary])
}

func (tp *tableParser) nextSubfloatBoundary(startIdx int) int {
	for i := startIdx; i < len(tp.tokens); i++ {
		tok := tp.tokens[i]
		switch tok.Kind {
		case lexer.Command:
			if tok.Value == "subfloat" || tok.Value == "caption" {
				return tok.Start
			}
		case lexer.LBrace, lexer.LBracket:
			i = tp.skipBalanced(i) - 1
		}
	}
	return len(tp.raw)
}

func removeSpans(s string, spans []span) string {
	if len(spans) == 0 {
		return s
	}
	sort.Slice(spans, func(i, j int) bool {
		return spans[i].start < spans[j].start
	})
	var out strings.Builder
	pos := 0
	for _, sp := range spans {
		if sp.start < pos {
			if sp.end > pos {
				pos = sp.end
			}
			continue
		}
		out.WriteString(s[pos:sp.start])
		out.WriteByte('\n')
		pos = sp.end
	}
	out.WriteString(s[pos:])
	return out.String()
}

func hasBlankLine(s string) bool {
	return firstBlankLine(s) >= 0
}

func firstBlankLine(s string) int {
	seenNewline := false
	firstNewline := -1
	for i, r := range s {
		switch r {
		case '\n':
			if seenNewline {
				return firstNewline
			}
			seenNewline = true
			firstNewline = i
		case ' ', '\t', '\r':
			continue
		default:
			seenNewline = false
			firstNewline = -1
		}
	}
	return -1
}

func (p *Parser) parseList(raw, env string) ast.Block {
	items := newListParser(raw).parse()
	list := &ast.List{Ordered: env == "enumerate", Kind: env}
	for _, item := range items {
		blocks := p.parseLooseBlocks(item.Body)
		if len(blocks) == 0 && strings.TrimSpace(item.Body) != "" {
			parsed := p.parseTextArgumentWithDeclarations(normalizeParagraph(item.Body))
			blocks = []ast.Block{&ast.Paragraph{Inlines: parsed.Inlines, Align: parsed.Align}}
		}
		list.Items = append(list.Items, &ast.ListItem{Label: p.parseTextArgument(item.Label), Blocks: blocks})
	}
	return list
}

type listParser struct {
	raw    string
	tokens []lexer.Token
}

func newListParser(raw string) *listParser {
	return &listParser{raw: raw, tokens: lexer.Tokenize(raw)}
}

func (lp *listParser) parse() []rawListItem {
	var items []rawListItem
	pos := 0
	current := -1
	for {
		idx, label, bodyStart, ok := lp.nextTopLevelItem(pos)
		if !ok {
			if current >= 0 {
				items[current].Body = lp.raw[pos:]
			} else if strings.TrimSpace(lp.raw[pos:]) != "" {
				items = append(items, rawListItem{Body: lp.raw[pos:]})
			}
			return items
		}
		if current >= 0 {
			items[current].Body = lp.raw[pos:idx]
		}
		pos = bodyStart
		items = append(items, rawListItem{Label: label})
		current = len(items) - 1
	}
}

func (lp *listParser) nextTopLevelItem(start int) (int, string, int, bool) {
	for i := lp.tokenIndexAt(start); i < len(lp.tokens); i++ {
		tok := lp.tokens[i]
		if tok.Kind == lexer.EOF {
			break
		}
		if tok.End <= start {
			continue
		}
		if tok.Start < start {
			continue
		}
		switch tok.Kind {
		case lexer.Raw:
			continue
		case lexer.LBrace:
			if _, end, ok := lp.readGroupAt(tok.Start, lexer.LBrace, lexer.RBrace); ok {
				i = lp.tokenIndexAt(end) - 1
				continue
			}
		case lexer.LBracket:
			if _, end, ok := lp.readGroupAt(tok.Start, lexer.LBracket, lexer.RBracket); ok {
				i = lp.tokenIndexAt(end) - 1
				continue
			}
		case lexer.Command:
			if tok.Value == "begin" {
				if env, _, ok := readBeginAt(lp.raw, tok.Start); ok {
					if end, ok := findEnvironmentEnd(lp.raw, tok.Start, env); ok {
						i = lp.tokenIndexAt(end) - 1
						continue
					}
				}
			}
			if tok.Value == "item" {
				label, bodyStart := lp.readItemLabel(tok.End)
				return tok.Start, label, bodyStart, true
			}
		}
	}
	return -1, "", start, false
}

func (lp *listParser) readItemLabel(pos int) (string, int) {
	i := skipWhitespace(lp.raw, pos)
	if i >= len(lp.raw) || lp.raw[i] != '[' {
		return "", i
	}
	arg, end, ok := lp.readGroupAt(i, lexer.LBracket, lexer.RBracket)
	if !ok {
		return "", i
	}
	return strings.TrimSpace(arg), end
}

func (lp *listParser) readGroupAt(pos int, open, close lexer.Kind) (string, int, bool) {
	idx := lp.tokenIndexAt(pos)
	if idx >= len(lp.tokens) || lp.tokens[idx].Start != pos || lp.tokens[idx].Kind != open {
		return "", pos, false
	}
	start := lp.tokens[idx].End
	if open == lexer.LBrace {
		depth := 1
		for i := idx + 1; i < len(lp.tokens); i++ {
			tok := lp.tokens[i]
			switch tok.Kind {
			case lexer.LBrace:
				depth++
			case lexer.RBrace:
				depth--
				if depth == 0 {
					return lp.raw[start:tok.Start], tok.End, true
				}
			case lexer.EOF:
				return "", pos, false
			}
		}
		return "", pos, false
	}
	if open != lexer.LBracket || close != lexer.RBracket {
		return "", pos, false
	}
	braceDepth := 0
	bracketDepth := 1
	for i := idx + 1; i < len(lp.tokens); i++ {
		tok := lp.tokens[i]
		switch tok.Kind {
		case lexer.LBrace:
			braceDepth++
		case lexer.RBrace:
			if braceDepth > 0 {
				braceDepth--
			}
		case lexer.LBracket:
			if braceDepth == 0 {
				bracketDepth++
			}
		case lexer.RBracket:
			if braceDepth == 0 {
				bracketDepth--
				if bracketDepth == 0 {
					return lp.raw[start:tok.Start], tok.End, true
				}
			}
		case lexer.EOF:
			return "", pos, false
		}
	}
	return "", pos, false
}

func (lp *listParser) tokenIndexAt(pos int) int {
	return tokenIndexAt(lp.tokens, pos)
}

func tokenIndexAt(tokens []lexer.Token, pos int) int {
	if len(tokens) == 0 {
		return 0
	}
	idx := sort.Search(len(tokens), func(i int) bool {
		tok := tokens[i]
		return tok.Kind == lexer.EOF || tok.End > pos
	})
	if idx < len(tokens) {
		return idx
	}
	return len(tokens) - 1
}

func matchingGroupClose(open lexer.Kind) lexer.Kind {
	switch open {
	case lexer.LBrace:
		return lexer.RBrace
	case lexer.LBracket:
		return lexer.RBracket
	default:
		return lexer.EOF
	}
}

func readTokenGroupAt(s string, tokens []lexer.Token, pos int, open, close lexer.Kind) (string, int, bool) {
	idx := tokenIndexAt(tokens, pos)
	if idx >= len(tokens) || tokens[idx].Start != pos || tokens[idx].Kind != open {
		return "", pos, false
	}
	start := tokens[idx].End
	if open == lexer.LBrace {
		depth := 1
		for i := idx + 1; i < len(tokens); i++ {
			tok := tokens[i]
			switch tok.Kind {
			case lexer.LBrace:
				depth++
			case lexer.RBrace:
				depth--
				if depth == 0 {
					return s[start:tok.Start], tok.End, true
				}
			case lexer.EOF:
				return "", pos, false
			}
		}
		return "", pos, false
	}
	if open != lexer.LBracket || close != lexer.RBracket {
		return "", pos, false
	}
	braceDepth := 0
	bracketDepth := 1
	for i := idx + 1; i < len(tokens); i++ {
		tok := tokens[i]
		switch tok.Kind {
		case lexer.LBrace:
			braceDepth++
		case lexer.RBrace:
			if braceDepth > 0 {
				braceDepth--
			}
		case lexer.LBracket:
			if braceDepth == 0 {
				bracketDepth++
			}
		case lexer.RBracket:
			if braceDepth == 0 {
				bracketDepth--
				if bracketDepth == 0 {
					return s[start:tok.Start], tok.End, true
				}
			}
		case lexer.EOF:
			return "", pos, false
		}
	}
	return "", pos, false
}

func sectionLevel(cmd string) int {
	switch cmd {
	case "section":
		return 1
	case "subsection":
		return 2
	case "subsubsection":
		return 3
	case "subsubsubsection":
		return 4
	default:
		return 1
	}
}

type span struct{ start, end int }

func extractCommand(s, cmd string) (string, span, bool) {
	idx := lexer.FindCommand(s, 0, cmd)
	if idx < 0 {
		return "", span{}, false
	}
	arg, end, ok := readOneCommandArg(s, idx, cmd)
	return arg, span{start: idx, end: end}, ok
}

func readOneCommandArg(s string, idx int, cmd string) (string, int, bool) {
	if !lexer.IsCommandAt(s, idx, cmd) {
		return "", idx, false
	}
	i := idx + len(cmd) + 1
	i = skipWhitespace(s, i)
	if i < len(s) && s[i] == '[' {
		if _, end, ok := readBalanced(s, i, '[', ']'); ok {
			i = skipWhitespace(s, end)
		}
	}
	if i >= len(s) || s[i] != '{' {
		return "", idx, false
	}
	arg, end, ok := readBalanced(s, i, '{', '}')
	return arg, end, ok
}

func firstCommandArg(s, cmd string) string {
	arg, _, ok := extractCommand(s, cmd)
	if !ok {
		return ""
	}
	return strings.TrimSpace(arg)
}

func firstOptionalArgAt(s string, idx int, cmd string) string {
	i := idx + len(cmd) + 1
	i = skipWhitespace(s, i)
	if i >= len(s) || s[i] != '[' {
		return ""
	}
	arg, _, ok := readBalanced(s, i, '[', ']')
	if !ok {
		return ""
	}
	return arg
}

func parseOptions(s string) map[string]string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	out := map[string]string{}
	for _, part := range splitTopLevel(s, ',') {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if k, v, ok := cutTopLevel(part, '='); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		} else {
			out[part] = ""
		}
	}
	return out
}

func splitTopLevel(s string, sep rune) []string {
	var parts []string
	start := 0
	braceDepth := 0
	bracketDepth := 0
	parenDepth := 0
	escaped := false
	for i, r := range s {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		switch r {
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		default:
			if r == sep && braceDepth == 0 && bracketDepth == 0 && parenDepth == 0 {
				parts = append(parts, s[start:i])
				start = i + len(string(r))
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func cutTopLevel(s string, sep rune) (string, string, bool) {
	braceDepth := 0
	bracketDepth := 0
	parenDepth := 0
	escaped := false
	for i, r := range s {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		switch r {
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		default:
			if r == sep && braceDepth == 0 && bracketDepth == 0 && parenDepth == 0 {
				return s[:i], s[i+len(string(r)):], true
			}
		}
	}
	return "", "", false
}

func readBeginAt(s string, i int) (string, int, bool) {
	if !lexer.IsCommandAt(s, i, "begin") {
		return "", i, false
	}
	j := i + len(`\begin`)
	j = skipWhitespace(s, j)
	if j >= len(s) || s[j] != '{' {
		return "", i, false
	}
	env, end, ok := readBalanced(s, j, '{', '}')
	if !ok {
		return "", i, false
	}
	j = end
	if j < len(s) && s[j] == '[' {
		if _, optEnd, ok := readBalanced(s, j, '[', ']'); ok {
			j = optEnd
		}
	}
	return env, j, true
}

func readEndAt(s string, i int) (string, int, bool) {
	if !lexer.IsCommandAt(s, i, "end") {
		return "", i, false
	}
	j := i + len(`\end`)
	j = skipWhitespace(s, j)
	if j >= len(s) || s[j] != '{' {
		return "", i, false
	}
	env, end, ok := readBalanced(s, j, '{', '}')
	if !ok {
		return "", i, false
	}
	return env, end, true
}

func findEnvironmentEnd(s string, start int, env string) (int, bool) {
	endStart, end, ok := findEnvironmentClose(s, start, env)
	if !ok {
		return start, false
	}
	_ = endStart
	return end, true
}

func findEnvironmentClose(s string, start int, env string) (int, int, bool) {
	depth := 0
	for i := start; i < len(s); {
		if name, end, ok := readBeginAt(s, i); ok {
			if name == env {
				depth++
			}
			i = end
			continue
		}
		if name, end, ok := readEndAt(s, i); ok {
			if name == env {
				depth--
				if depth == 0 {
					return i, end, true
				}
			}
			i = end
			continue
		}
		if s[i] == '\\' {
			if _, end, ok := lexer.CommandNameAt(s, i); ok {
				i = end
				continue
			}
			i += 2
			continue
		}
		i++
	}
	return start, start, false
}

func stripEnvironmentShell(raw, env string) string {
	name, start, ok := readBeginAt(raw, 0)
	if !ok || name != env {
		return raw
	}
	endStart, _, ok := findEnvironmentClose(raw, 0, env)
	if !ok || endStart < start {
		return raw[start:]
	}
	return raw[start:endStart]
}

func removeLabelCommands(s string) string {
	for {
		idx := lexer.FindCommand(s, 0, "label")
		if idx < 0 {
			return s
		}
		_, end, ok := readOneCommandArg(s, idx, "label")
		if !ok {
			return s
		}
		s = s[:idx] + s[end:]
	}
}

type rawListItem struct {
	Label string
	Body  string
}

func skipWhitespace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n') {
		i++
	}
	return i
}

func normalizeParagraph(s string) string {
	return strings.TrimSpace(s)
}

func readBalanced(s string, start int, open, close byte) (string, int, bool) {
	if start >= len(s) || s[start] != open {
		return "", start, false
	}
	depth := 0
	for i := start; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == open {
			depth++
		}
		if s[i] == close {
			depth--
			if depth == 0 {
				return s[start+1 : i], i + 1, true
			}
		}
	}
	return "", start, false
}
