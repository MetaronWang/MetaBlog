package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"MetaBlog/internal/latex/lexer"
)

type Loaded struct {
	InputFile string
	RootDir   string
	Document  string
	Warnings  []string
}

func Load(input string) (*Loaded, error) {
	abs, err := filepath.Abs(input)
	if err != nil {
		return nil, err
	}
	seen := map[string]int{}
	active := map[string]bool{}
	rootDir := filepath.Dir(abs)
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	raw := string(b)
	bodyStart, bodyEnd, ok := locateDocumentBounds(raw, lexer.Tokenize(raw))
	if !ok {
		return nil, fmt.Errorf("missing \\begin{document} or \\end{document} in %s", input)
	}
	seen[abs]++
	active[abs] = true
	doc, warnings, err := parseAndExpandSource(raw[bodyStart:bodyEnd], rootDir, rootDir, seen, active)
	delete(active, abs)
	if err != nil {
		return nil, err
	}
	return &Loaded{
		InputFile: abs,
		RootDir:   rootDir,
		Document:  doc,
		Warnings:  warnings,
	}, nil
}

func locateDocumentBounds(raw string, tokens []lexer.Token) (int, int, bool) {
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.Kind == lexer.EOF {
			break
		}
		if tok.Kind != lexer.Command || tok.Value != "begin" {
			continue
		}
		argIdx := nextNonSpaceToken(tokens, i+1)
		name, next, ok := readTokenGroup(raw, tokens, argIdx)
		if !ok || strings.TrimSpace(name) != "document" {
			continue
		}
		bodyStart := tokens[next-1].End
		for j := next; j < len(tokens); j++ {
			endTok := tokens[j]
			if endTok.Kind == lexer.EOF {
				break
			}
			if endTok.Kind != lexer.Command || endTok.Value != "end" {
				continue
			}
			endArgIdx := nextNonSpaceToken(tokens, j+1)
			endName, _, ok := readTokenGroup(raw, tokens, endArgIdx)
			if ok && strings.TrimSpace(endName) == "document" {
				return bodyStart, endTok.Start, true
			}
		}
		return 0, 0, false
	}
	return 0, 0, false
}

func nextNonSpaceToken(tokens []lexer.Token, i int) int {
	for i < len(tokens) {
		tok := tokens[i]
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

func loadFile(path, rootDir string, seen map[string]int, active map[string]bool) (string, []string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	if active[abs] {
		return "", []string{fmt.Sprintf("skipped cyclic input %s", abs)}, nil
	}
	var warnings []string
	if seen[abs] > 0 {
		warnings = append(warnings, fmt.Sprintf("repeated input %s", abs))
	}
	seen[abs]++
	active[abs] = true
	defer delete(active, abs)

	b, err := os.ReadFile(abs)
	if err != nil {
		return "", nil, err
	}
	expanded, subWarnings, err := parseAndExpandSource(string(b), filepath.Dir(abs), rootDir, seen, active)
	warnings = append(warnings, subWarnings...)
	if err != nil {
		return "", warnings, err
	}
	return expanded, warnings, nil
}

func resolveInputPath(name, baseDir, rootDir string) string {
	rel := filepath.FromSlash(name)
	candidates := []string{
		filepath.Clean(filepath.Join(baseDir, rel)),
		filepath.Clean(filepath.Join(rootDir, rel)),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return candidates[0]
}

func extractDocument(s string) (string, bool) {
	begin := strings.Index(s, `\begin{document}`)
	end := strings.LastIndex(s, `\end{document}`)
	if begin < 0 || end < 0 || end <= begin {
		return "", false
	}
	begin += len(`\begin{document}`)
	return s[begin:end], true
}

func skipSpaces(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n') {
		i++
	}
	return i
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
