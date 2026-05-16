package parser

import (
	"fmt"
	"strings"

	"MetaBlog/internal/latex/ast"
	"MetaBlog/internal/latex/lexer"
)

func ParseInline(s string) []ast.Inline {
	inlines, _ := parseInlineWithWarnings(s)
	return inlines
}

func parseInlineWithWarnings(s string) ([]ast.Inline, []string) {
	p := &inlineParser{s: s, tokens: lexer.Tokenize(s)}
	inlines := p.parseUntil("")
	return inlines, p.warnings
}

type ParsedTextArgument struct {
	Inlines  []ast.Inline
	Align    string
	Warnings []string
}

func ParseTextArgument(s string) []ast.Inline {
	return ParseTextArgumentWithDeclarations(s).Inlines
}

func ParseTextArgumentWithDeclarations(s string) ParsedTextArgument {
	return parseTextArgumentWithWarnings(s)
}

func parseTextArgumentWithWarnings(s string) ParsedTextArgument {
	body := strings.TrimSpace(s)
	decl := readTextDeclarations(body)
	if !decl.HasDeclarations {
		inlines, warnings := parseInlineWithWarnings(s)
		return ParsedTextArgument{Inlines: inlines, Align: decl.Align, Warnings: warnings}
	}
	if strings.TrimSpace(decl.Rest) == "" {
		return ParsedTextArgument{Align: decl.Align}
	}
	if !decl.HasStyle {
		inlines, warnings := parseInlineWithWarnings(decl.Rest)
		return ParsedTextArgument{Inlines: inlines, Align: decl.Align, Warnings: warnings}
	}
	inlines, warnings := parseInlineWithWarnings(decl.Rest)
	decl.Style.Children = inlines
	return ParsedTextArgument{Inlines: []ast.Inline{decl.Style}, Align: decl.Align, Warnings: warnings}
}

type inlineParser struct {
	s        string
	tokens   []lexer.Token
	i        int
	warnings []string
}

func (p *inlineParser) parseUntil(endToken string) []ast.Inline {
	var out []ast.Inline
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			out = append(out, &ast.Text{Value: normalizeInlineText(text.String())})
			text.Reset()
		}
	}
	for p.i < len(p.s) {
		if endToken != "" && strings.HasPrefix(p.s[p.i:], endToken) {
			break
		}
		tok := p.currentToken()
		if tok.Kind == lexer.EOF {
			break
		}
		if tok.End <= p.i {
			p.i++
			continue
		}
		if tok.Start > p.i {
			p.writeTextSegment(&text, p.s[p.i:tok.Start])
			p.i = tok.Start
			continue
		}
		switch tok.Kind {
		case lexer.Text:
			p.writeTextSegment(&text, tok.Value)
			p.i = tok.End
			continue
		case lexer.Comment:
			p.i = tok.End
			continue
		case lexer.LBrace:
			flush()
			if arg, end, ok := p.readGroupAt(tok.Start, lexer.LBrace, lexer.RBrace); ok {
				out = append(out, p.parseInlineGroup(arg)...)
				p.i = end
				continue
			}
		case lexer.LBracket:
			text.WriteByte('[')
			p.i = tok.End
			continue
		case lexer.RBracket:
			text.WriteByte(']')
			p.i = tok.End
			continue
		case lexer.RBrace:
			text.WriteByte('}')
			p.i = tok.End
			continue
		case lexer.Dollar:
			flush()
			start := p.i + 1
			if end := findInlineDollarEnd(p.s, start); end >= 0 {
				out = append(out, &ast.InlineMath{TeX: strings.TrimSpace(p.s[start:end])})
				p.i = end + 1
				continue
			}
			text.WriteByte('$')
			p.i = tok.End
			continue
		case lexer.Command:
			if nodes, textValue, ok := p.readCommandInline(tok); ok {
				if len(nodes) > 0 {
					flush()
					out = append(out, nodes...)
				} else {
					text.WriteString(textValue)
				}
				continue
			}
		}
		text.WriteByte(p.s[p.i])
		p.i++
	}
	flush()
	return mergeText(out)
}

func (p *inlineParser) parseTextArgument(s string) []ast.Inline {
	parsed := parseTextArgumentWithWarnings(s)
	p.warnings = append(p.warnings, parsed.Warnings...)
	return parsed.Inlines
}

func (p *inlineParser) parseInlineGroup(s string) []ast.Inline {
	return p.parseTextArgument(s)
}

func (p *inlineParser) currentToken() lexer.Token {
	idx := tokenIndexAt(p.tokens, p.i)
	return p.tokens[idx]
}

func (p *inlineParser) writeTextSegment(text *strings.Builder, s string) {
	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], "``") || strings.HasPrefix(s[i:], "''") {
			text.WriteByte('"')
			i += 2
			continue
		}
		if strings.HasPrefix(s[i:], "--") {
			text.WriteString("-")
			i += 2
			continue
		}
		if s[i] == '~' {
			text.WriteRune('\u00a0')
			i++
			continue
		}
		text.WriteByte(s[i])
		i++
	}
}

func (p *inlineParser) readCommandInline(tok lexer.Token) ([]ast.Inline, string, bool) {
	if tok.Kind != lexer.Command || tok.Start != p.i {
		return nil, "", false
	}
	switch tok.Value {
	case "textbf":
		if arg, ok := p.readCommandArg("textbf"); ok {
			return []ast.Inline{&ast.Bold{Children: p.parseTextArgument(arg)}}, "", true
		}
	case "textit", "emph":
		if arg, ok := p.readCommandArg(tok.Value); ok {
			return []ast.Inline{&ast.Italic{Children: p.parseTextArgument(arg)}}, "", true
		}
	case "textcolor":
		if color, ok := p.readCommandArg("textcolor"); ok {
			if arg, ok := p.readNextBraceArg(); ok {
				return []ast.Inline{&ast.Styled{Children: p.parseTextArgument(arg), Color: normalizeLaTeXColor(color)}}, "", true
			}
		}
	case "colorbox":
		if color, ok := p.readCommandArg("colorbox"); ok {
			if arg, ok := p.readNextBraceArg(); ok {
				return []ast.Inline{&ast.Styled{Children: p.parseTextArgument(arg), Background: normalizeLaTeXColor(color)}}, "", true
			}
		}
	case "href":
		if url, ok := p.readCommandArg("href"); ok {
			if arg, ok := p.readNextBraceArg(); ok {
				return []ast.Inline{&ast.Link{URL: normalizeURLArg(url), Children: p.parseTextArgument(arg)}}, "", true
			}
		}
	case "url":
		if url, ok := p.readCommandArg("url"); ok {
			url = normalizeURLArg(url)
			return []ast.Inline{&ast.Link{URL: url, Children: []ast.Inline{&ast.Text{Value: url}}}}, "", true
		}
	case "cite":
		if arg, ok := p.readCommandArg("cite"); ok {
			return []ast.Inline{&ast.Cite{Keys: splitCSV(arg)}}, "", true
		}
	case "ref":
		if arg, ok := p.readCommandArg("ref"); ok {
			return []ast.Inline{&ast.Ref{Key: strings.TrimSpace(arg)}}, "", true
		}
	case "footnote":
		if arg, ok := p.readCommandArg("footnote"); ok {
			return []ast.Inline{&ast.Footnote{Children: p.parseTextArgument(arg)}}, "", true
		}
	case "IEEEPARstart":
		if first, ok := p.readCommandArg("IEEEPARstart"); ok {
			if second, ok := p.readNextBraceArg(); ok {
				return nil, first + second, true
			}
		}
	case "(":
		start := tok.End
		if end := findUnescapedSequence(p.s, start, `\)`); end >= 0 {
			p.i = end + len(`\)`)
			return []ast.Inline{&ast.InlineMath{TeX: strings.TrimSpace(p.s[start:end])}}, "", true
		}
	case ",", ";", ":":
		p.i = tok.End
		return nil, " ", true
	case "%":
		p.i = tok.End
		return nil, "%", true
	case "&":
		p.i = tok.End
		return nil, "&", true
	case "_":
		p.i = tok.End
		return nil, "_", true
	case "#":
		p.i = tok.End
		return nil, "#", true
	case "$":
		p.i = tok.End
		return nil, "$", true
	case "{":
		p.i = tok.End
		return nil, "{", true
	case "}":
		p.i = tok.End
		return nil, "}", true
	case "underline":
		if arg, ok := p.readCommandArg("underline"); ok {
			return []ast.Inline{&ast.Styled{Children: p.parseTextArgument(arg), Underline: true}}, "", true
		}
	case "textrm":
		if arg, ok := p.readCommandArg("textrm"); ok {
			return p.parseTextArgument(arg), "", true
		}
	case "LaTeX":
		p.i = tok.End
		return nil, "LaTeX", true
	}
	if tex, ok := p.readInlineMathCommand(); ok {
		return []ast.Inline{&ast.InlineMath{TeX: tex}}, "", true
	}
	if isLetterCommand(tok.Value) {
		p.warnings = append(p.warnings, fmt.Sprintf("unsupported inline command \\%s", tok.Value))
		p.i = tok.End
		p.skipInlineSpaces()
		if p.i < len(p.s) && p.s[p.i] == '{' {
			if arg, end, ok := p.readGroupAt(p.i, lexer.LBrace, lexer.RBrace); ok {
				p.i = end
				return p.parseTextArgument(arg), "", true
			}
		}
		return nil, "", true
	}
	p.i = tok.End
	return nil, "", true
}

func (p *inlineParser) readCommandArg(cmd string) (string, bool) {
	tok := p.currentToken()
	if tok.Start != p.i || tok.Kind != lexer.Command || tok.Value != cmd {
		return "", false
	}
	i := p.skipSpacesFrom(tok.End)
	if i < len(p.s) && p.s[i] == '[' {
		if _, end, ok := p.readGroupAt(i, lexer.LBracket, lexer.RBracket); ok {
			i = end
		}
	}
	i = p.skipSpacesFrom(i)
	if i >= len(p.s) || p.s[i] != '{' {
		return "", false
	}
	arg, end, ok := p.readGroupAt(i, lexer.LBrace, lexer.RBrace)
	if !ok {
		return "", false
	}
	p.i = end
	return arg, true
}

func (p *inlineParser) readNextBraceArg() (string, bool) {
	p.skipInlineSpaces()
	if p.i >= len(p.s) || p.s[p.i] != '{' {
		return "", false
	}
	arg, end, ok := p.readGroupAt(p.i, lexer.LBrace, lexer.RBrace)
	if !ok {
		return "", false
	}
	p.i = end
	return arg, true
}

func (p *inlineParser) readInlineMathCommand() (string, bool) {
	tok := p.currentToken()
	if tok.Start != p.i || tok.Kind != lexer.Command || !isInlineMathCommand(tok.Value) {
		return "", false
	}
	k := p.skipSpacesFrom(tok.End)
	if k >= len(p.s) || p.s[k] != '{' {
		return "", false
	}
	_, end, ok := p.readGroupAt(k, lexer.LBrace, lexer.RBrace)
	if !ok {
		return "", false
	}
	end = readScripts(p.s, end)
	tex := p.s[p.i:end]
	p.i = end
	return strings.TrimSpace(tex), true
}

func (p *inlineParser) readGroupAt(pos int, open, close lexer.Kind) (string, int, bool) {
	idx := tokenIndexAt(p.tokens, pos)
	if idx >= len(p.tokens) || p.tokens[idx].Start != pos || p.tokens[idx].Kind != open {
		return "", pos, false
	}
	start := p.tokens[idx].End
	if open == lexer.LBrace {
		depth := 1
		for i := idx + 1; i < len(p.tokens); i++ {
			tok := p.tokens[i]
			switch tok.Kind {
			case lexer.LBrace:
				depth++
			case lexer.RBrace:
				depth--
				if depth == 0 {
					return p.s[start:tok.Start], tok.End, true
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
	for i := idx + 1; i < len(p.tokens); i++ {
		tok := p.tokens[i]
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
					return p.s[start:tok.Start], tok.End, true
				}
			}
		case lexer.EOF:
			return "", pos, false
		}
	}
	return "", pos, false
}

func (p *inlineParser) skipSpacesFrom(i int) int {
	for i < len(p.s) && (p.s[i] == ' ' || p.s[i] == '\t' || p.s[i] == '\r' || p.s[i] == '\n') {
		i++
	}
	return i
}

func (p *inlineParser) skipInlineSpaces() {
	p.i = p.skipSpacesFrom(p.i)
}

func isLetterCommand(cmd string) bool {
	if cmd == "" {
		return false
	}
	for _, r := range cmd {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

func isInlineMathCommand(cmd string) bool {
	switch cmd {
	case "boldsymbol", "mathbf", "mathit", "mathrm", "mathbb", "mathcal", "mathsf", "mathtt", "mathfrak", "operatorname":
		return true
	default:
		return false
	}
}

func readScripts(s string, i int) int {
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n') {
			i++
		}
		if i >= len(s) || (s[i] != '^' && s[i] != '_') {
			return i
		}
		i++
		if i < len(s) && s[i] == '{' {
			_, end, ok := readBalanced(s, i, '{', '}')
			if !ok {
				return i
			}
			i = end
			continue
		}
		if i < len(s) {
			i++
		}
	}
	return i
}

func findUnescapedSequence(s string, start int, seq string) int {
	for i := start; i <= len(s)-len(seq); i++ {
		if s[i] == '\\' {
			if strings.HasPrefix(s[i:], seq) && !isEscapedAt(s, i) {
				return i
			}
			i++
			continue
		}
		if strings.HasPrefix(s[i:], seq) && !isEscapedAt(s, i) {
			return i
		}
	}
	return -1
}

func findInlineDollarEnd(s string, start int) int {
	for i := start; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] != '$' {
			continue
		}
		if i+1 < len(s) && s[i+1] == '$' {
			return -1
		}
		return i
	}
	return -1
}

func findDisplayDollarEnd(s string, start int) int {
	for i := start; i < len(s)-1; i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == '$' && s[i+1] == '$' {
			return i
		}
	}
	return -1
}

func isEscapedAt(s string, idx int) bool {
	count := 0
	for i := idx - 1; i >= 0 && s[i] == '\\'; i-- {
		count++
	}
	return count%2 == 1
}

type textDeclarations struct {
	Style           *ast.Styled
	Rest            string
	Align           string
	HasStyle        bool
	HasDeclarations bool
}

func readTextDeclarations(s string) textDeclarations {
	style := &ast.Styled{}
	i := 0
	decl := textDeclarations{Style: style}
	for {
		i = skipInlineSpaces(s, i)
		if i >= len(s) || s[i] != '\\' {
			break
		}
		cmd, next, ok := readInlineCommandName(s, i)
		if !ok {
			break
		}
		switch cmd {
		case "centering":
			decl.Align = "center"
			i = next
			decl.HasDeclarations = true
		case "raggedright":
			decl.Align = "left"
			i = next
			decl.HasDeclarations = true
		case "raggedleft":
			decl.Align = "right"
			i = next
			decl.HasDeclarations = true
		case "color":
			arg, end, ok := readCommandArgAtInline(s, next)
			if !ok {
				decl.Rest = strings.TrimSpace(s[i:])
				return decl
			}
			style.Color = normalizeLaTeXColor(arg)
			i = end
			decl.HasStyle = true
			decl.HasDeclarations = true
		case "pagecolor", "backgroundcolor", "bgcolor":
			arg, end, ok := readCommandArgAtInline(s, next)
			if !ok {
				decl.Rest = strings.TrimSpace(s[i:])
				return decl
			}
			style.Background = normalizeLaTeXColor(arg)
			i = end
			decl.HasStyle = true
			decl.HasDeclarations = true
		case "colorbox":
			color, afterColor, ok := readCommandArgAtInline(s, next)
			if !ok {
				decl.Rest = strings.TrimSpace(s[i:])
				return decl
			}
			content, end, ok := readCommandArgAtInline(s, afterColor)
			if !ok {
				decl.Rest = strings.TrimSpace(s[i:])
				return decl
			}
			style.Background = normalizeLaTeXColor(color)
			i = end
			decl.Rest = strings.TrimSpace(content + " " + s[i:])
			decl.HasStyle = true
			decl.HasDeclarations = true
			return decl
		case "underline", "ul":
			style.Underline = true
			i = next
			decl.HasStyle = true
			decl.HasDeclarations = true
		case "bfseries", "bf":
			style.Bold = true
			i = next
			decl.HasStyle = true
			decl.HasDeclarations = true
		case "itshape", "it", "em":
			style.Italic = true
			i = next
			decl.HasStyle = true
			decl.HasDeclarations = true
		case "ttfamily", "tt":
			style.Mono = true
			i = next
			decl.HasStyle = true
			decl.HasDeclarations = true
		case "tiny", "scriptsize", "footnotesize", "small", "normalsize", "large", "Large", "LARGE", "huge", "Huge":
			style.FontSize = latexFontSize(cmd)
			i = next
			decl.HasStyle = true
			decl.HasDeclarations = true
		case "normalfont", "upshape":
			style.Bold = false
			style.Italic = false
			style.Mono = false
			style.FontSize = ""
			i = next
			decl.HasStyle = true
			decl.HasDeclarations = true
		default:
			decl.Rest = strings.TrimSpace(s[i:])
			return decl
		}
	}
	decl.Rest = strings.TrimSpace(s[i:])
	return decl
}

func latexFontSize(cmd string) string {
	switch cmd {
	case "tiny":
		return "0.5em"
	case "scriptsize":
		return "0.7em"
	case "footnotesize":
		return "0.8em"
	case "small":
		return "0.9em"
	case "normalsize":
		return "1em"
	case "large":
		return "1.2em"
	case "Large":
		return "1.44em"
	case "LARGE":
		return "1.728em"
	case "huge":
		return "2.074em"
	case "Huge":
		return "2.488em"
	default:
		return ""
	}
}

func readInlineCommandName(s string, i int) (string, int, bool) {
	return lexer.CommandNameAt(s, i)
}

func readCommandArgAtInline(s string, i int) (string, int, bool) {
	i = skipInlineSpaces(s, i)
	if i >= len(s) || s[i] != '{' {
		return "", i, false
	}
	return readBalanced(s, i, '{', '}')
}

func skipInlineSpaces(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n') {
		i++
	}
	return i
}

func normalizeLaTeXColor(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "{}")
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "#") {
		return s
	}
	if strings.Contains(s, "!") {
		parts := strings.Split(s, "!")
		if len(parts) >= 2 {
			base := normalizeNamedColor(parts[0])
			percent := strings.TrimSpace(parts[1])
			if base != "" && percent != "" {
				return "color-mix(in srgb, " + base + " " + percent + "%, white)"
			}
		}
	}
	return normalizeNamedColor(s)
}

func normalizeNamedColor(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "black", "white", "red", "green", "blue", "cyan", "magenta", "yellow", "gray", "grey", "orange", "purple", "brown", "pink":
		if strings.EqualFold(s, "grey") {
			return "gray"
		}
		return strings.ToLower(s)
	default:
		return ""
	}
}

func normalizeURLArg(s string) string {
	s = strings.TrimSpace(s)
	replacements := map[string]string{
		`\%`: "%",
		`\&`: "&",
		`\_`: "_",
		`\#`: "#",
		`\~`: "~",
	}
	for old, repl := range replacements {
		s = strings.ReplaceAll(s, old, repl)
	}
	return s
}

func normalizeInlineText(s string) string {
	var b strings.Builder
	lastWasSpace := false
	for _, r := range s {
		if r == '\u00a0' {
			b.WriteRune(r)
			lastWasSpace = false
			continue
		}
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			if !lastWasSpace {
				b.WriteByte(' ')
				lastWasSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastWasSpace = false
	}
	return b.String()
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func mergeText(in []ast.Inline) []ast.Inline {
	var out []ast.Inline
	for _, node := range in {
		t, ok := node.(*ast.Text)
		if !ok || len(out) == 0 {
			out = append(out, node)
			continue
		}
		if prev, ok := out[len(out)-1].(*ast.Text); ok {
			if prev.Value == "" {
				prev.Value = t.Value
			} else if t.Value != "" {
				prev.Value += t.Value
			}
			continue
		}
		out = append(out, node)
	}
	return out
}
