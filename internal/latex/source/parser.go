package source

import (
	"fmt"
	"path/filepath"
	"strings"

	"MetaBlog/internal/latex/lexer"
)

func parseAndExpandSource(s, baseDir, rootDir string, seen map[string]int, active map[string]bool) (string, []string, error) {
	tokens := lexer.Tokenize(s)
	p := sourceParser{
		raw:     s,
		tokens:  tokens,
		baseDir: baseDir,
		rootDir: rootDir,
		seen:    seen,
		active:  active,
	}
	out, err := p.parse()
	return out, p.warnings, err
}

type sourceParser struct {
	raw      string
	tokens   []lexer.Token
	baseDir  string
	rootDir  string
	seen     map[string]int
	active   map[string]bool
	warnings []string

	out               strings.Builder
	lineStartOut      int
	lineHasContent    bool
	skipNextLineBreak bool
}

func (p *sourceParser) parse() (string, error) {
	for i := 0; i < len(p.tokens); {
		tok := p.tokens[i]
		if tok.Kind == lexer.EOF {
			break
		}
		if tok.Kind == lexer.Comment {
			p.skipComment()
			i++
			continue
		}
		if tok.Kind == lexer.Command && (tok.Value == "input" || tok.Value == "include") {
			if next, ok, err := p.expandInputAt(i); err != nil {
				return "", err
			} else if ok {
				i = next
				continue
			}
		}
		p.writeText(p.raw[tok.Start:tok.End])
		i++
	}
	return p.out.String(), nil
}

func (p *sourceParser) expandInputAt(i int) (int, bool, error) {
	argIdx := p.nextNonSpaceToken(i + 1)
	if argIdx >= len(p.tokens) || p.tokens[argIdx].Kind != lexer.LBrace {
		return i, false, nil
	}
	name, next, ok := readTokenGroup(p.raw, p.tokens, argIdx)
	if !ok {
		return i, false, nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return i, false, nil
	}
	if filepath.Ext(name) == "" {
		name += ".tex"
	}
	inputPath := resolveInputPath(name, p.baseDir, p.rootDir)
	content, subWarnings, err := loadFile(inputPath, p.rootDir, p.seen, p.active)
	p.warnings = append(p.warnings, subWarnings...)
	if err != nil {
		return i, false, fmt.Errorf("expand \\%s %s: %w", p.tokens[i].Value, inputPath, err)
	}
	p.writeText("\n")
	p.writeText(content)
	p.writeText("\n")
	return next, true, nil
}

func (p *sourceParser) nextNonSpaceToken(i int) int {
	for i < len(p.tokens) {
		tok := p.tokens[i]
		switch tok.Kind {
		case lexer.Comment:
			i++
			continue
		case lexer.Text:
			if strings.TrimSpace(tok.Value) == "" {
				i++
				continue
			}
		}
		return i
	}
	return i
}

func readTokenGroup(raw string, tokens []lexer.Token, i int) (string, int, bool) {
	if i >= len(tokens) || tokens[i].Kind != lexer.LBrace {
		return "", i, false
	}
	depth := 0
	for j := i; j < len(tokens); j++ {
		switch tokens[j].Kind {
		case lexer.LBrace:
			depth++
		case lexer.RBrace:
			depth--
			if depth == 0 {
				return raw[tokens[i].End:tokens[j].Start], j + 1, true
			}
		case lexer.EOF:
			return "", i, false
		}
	}
	return "", i, false
}

func (p *sourceParser) skipComment() {
	if !p.lineHasContent {
		truncateBuilder(&p.out, p.lineStartOut)
		p.skipNextLineBreak = true
	}
}

func (p *sourceParser) writeText(s string) {
	if s == "" {
		return
	}
	if p.skipNextLineBreak {
		s = trimOneLeadingLineBreak(s)
		p.skipNextLineBreak = false
		if s == "" {
			return
		}
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		p.out.WriteByte(ch)
		if ch == '\r' {
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
				p.out.WriteByte(s[i])
			}
			p.lineStartOut = p.out.Len()
			p.lineHasContent = false
			continue
		}
		if ch == '\n' {
			p.lineStartOut = p.out.Len()
			p.lineHasContent = false
			continue
		}
		if ch != ' ' && ch != '\t' {
			p.lineHasContent = true
		}
	}
}

func trimOneLeadingLineBreak(s string) string {
	if strings.HasPrefix(s, "\r\n") {
		return s[2:]
	}
	if strings.HasPrefix(s, "\n") || strings.HasPrefix(s, "\r") {
		return s[1:]
	}
	return s
}
