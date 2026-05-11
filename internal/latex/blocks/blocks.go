package blocks

import (
	"fmt"
	"sort"
	"strings"

	"MetaBlog/internal/latex/lexer"
)

type ComplexBlock struct {
	ID      string
	EnvName string
	RawTeX  string
	HTML    string
	Caption string
	Label   string
}

type Result struct {
	Text   string
	Blocks map[string]*ComplexBlock
}

func Lift(s string) Result {
	var out strings.Builder
	blocks := map[string]*ComplexBlock{}
	count := 0
	for i := 0; i < len(s); {
		start, env, end, ok := nextBegin(s, i)
		if !ok {
			out.WriteString(s[i:])
			break
		}
		if !isComplexStart(env) {
			out.WriteString(s[i:end])
			i = end
			continue
		}
		rawEnd, ok := findEnvironmentEnd(s, start, env)
		if !ok {
			out.WriteString(s[i:end])
			i = end
			continue
		}
		out.WriteString(s[i:start])
		count++
		id := fmt.Sprintf("@@METABLOG_COMPLEX_BLOCK_%04d@@", count)
		raw := s[start:rawEnd]
		caption, label := complexMetadata(raw)
		blocks[id] = &ComplexBlock{
			ID:      id,
			EnvName: env,
			RawTeX:  raw,
			Caption: caption,
			Label:   label,
		}
		out.WriteString("\n")
		out.WriteString(id)
		out.WriteString("\n")
		i = rawEnd
	}
	return Result{Text: out.String(), Blocks: blocks}
}

func complexMetadata(raw string) (string, string) {
	tokens := lexer.Tokenize(raw)
	caption := ""
	label := ""
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.Kind == lexer.EOF {
			break
		}
		switch tok.Kind {
		case lexer.Raw:
			continue
		case lexer.LBrace:
			if _, end, ok := readTokenGroupAt(raw, tokens, tok.Start, lexer.LBrace, lexer.RBrace); ok {
				i = tokenIndexAt(tokens, end) - 1
				continue
			}
		case lexer.LBracket:
			if _, end, ok := readTokenGroupAt(raw, tokens, tok.Start, lexer.LBracket, lexer.RBracket); ok {
				i = tokenIndexAt(tokens, end) - 1
				continue
			}
		case lexer.Command:
			if tok.Value == "begin" && tok.Start != 0 {
				if env, _, ok := readBeginAt(raw, tok.Start); ok {
					if end, ok := findEnvironmentEnd(raw, tok.Start, env); ok {
						i = tokenIndexAt(tokens, end) - 1
						continue
					}
				}
			}
			switch tok.Value {
			case "caption":
				arg, end, ok := readCommandArgAt(raw, tokens, tok.Start, "caption")
				if ok {
					if caption == "" {
						caption = strings.TrimSpace(arg)
					}
					i = tokenIndexAt(tokens, end) - 1
					continue
				}
			case "label":
				arg, end, ok := readCommandArgAt(raw, tokens, tok.Start, "label")
				if ok {
					if label == "" {
						label = strings.TrimSpace(arg)
					}
					i = tokenIndexAt(tokens, end) - 1
					continue
				}
			default:
				if end, ok := skipCommandArgs(raw, tokens, tok.End); ok {
					i = tokenIndexAt(tokens, end) - 1
				}
			}
		}
	}
	return caption, label
}

func isComplexStart(env string) bool {
	switch env {
	case "tabular", "tabularx", "algorithm", "algorithm*":
		return true
	default:
		return false
	}
}

func findEnvironmentEnd(s string, start int, env string) (int, bool) {
	depth := 0
	for i := start; i < len(s); {
		kind, name, end, ok := readBeginEndAt(s, i)
		if !ok {
			if s[i] == '\\' {
				if _, cmdEnd, ok := lexer.CommandNameAt(s, i); ok {
					i = cmdEnd
					continue
				}
				i += 2
				continue
			}
			i++
			continue
		}
		if name != env {
			i = end
			continue
		}
		if kind == "begin" {
			depth++
		} else {
			depth--
			if depth == 0 {
				return end, true
			}
		}
		i = end
	}
	return start, false
}

func readCommandArgAt(s string, tokens []lexer.Token, idx int, cmd string) (string, int, bool) {
	tokIdx := tokenIndexAt(tokens, idx)
	if tokIdx >= len(tokens) || tokens[tokIdx].Start != idx || tokens[tokIdx].Kind != lexer.Command || tokens[tokIdx].Value != cmd {
		return "", idx, false
	}
	i := skipWhitespace(s, tokens[tokIdx].End)
	if i < len(s) && s[i] == '[' {
		if _, end, ok := readTokenGroupAt(s, tokens, i, lexer.LBracket, lexer.RBracket); ok {
			i = skipWhitespace(s, end)
		}
	}
	if i >= len(s) || s[i] != '{' {
		return "", idx, false
	}
	return readTokenGroupAt(s, tokens, i, lexer.LBrace, lexer.RBrace)
}

func skipCommandArgs(s string, tokens []lexer.Token, pos int) (int, bool) {
	i := skipWhitespace(s, pos)
	moved := false
	for i < len(s) && s[i] == '[' {
		_, end, ok := readTokenGroupAt(s, tokens, i, lexer.LBracket, lexer.RBracket)
		if !ok {
			return i, moved
		}
		i = skipWhitespace(s, end)
		moved = true
	}
	if i < len(s) && s[i] == '{' {
		_, end, ok := readTokenGroupAt(s, tokens, i, lexer.LBrace, lexer.RBrace)
		if ok {
			i = end
			moved = true
		}
	}
	return i, moved
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
	return env, end, true
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

func firstCommandArg(s, cmd string) string {
	idx := lexer.FindCommand(s, 0, cmd)
	if idx < 0 {
		return ""
	}
	i := idx + len(cmd) + 1
	for i < len(s) && (s[i] == ' ' || s[i] == '\n' || s[i] == '\r' || s[i] == '\t') {
		i++
	}
	if i < len(s) && s[i] == '[' {
		if _, end, ok := readBalanced(s, i, '[', ']'); ok {
			i = end
		}
	}
	for i < len(s) && (s[i] == ' ' || s[i] == '\n' || s[i] == '\r' || s[i] == '\t') {
		i++
	}
	if i >= len(s) || s[i] != '{' {
		return ""
	}
	arg, _, ok := readBalanced(s, i, '{', '}')
	if !ok {
		return ""
	}
	return strings.TrimSpace(arg)
}

func nextBegin(s string, start int) (int, string, int, bool) {
	for i := start; i < len(s); i++ {
		if kind, name, end, ok := readBeginEndAt(s, i); ok && kind == "begin" {
			return i, name, end, true
		}
		if s[i] == '\\' {
			if _, end, ok := lexer.CommandNameAt(s, i); ok {
				i = end - 1
				continue
			}
			i++
		}
	}
	return -1, "", start, false
}

func skipWhitespace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\n' || s[i] == '\r' || s[i] == '\t') {
		i++
	}
	return i
}

func readBeginEndAt(s string, i int) (string, string, int, bool) {
	kind := ""
	switch {
	case lexer.IsCommandAt(s, i, "begin"):
		kind = "begin"
	case lexer.IsCommandAt(s, i, "end"):
		kind = "end"
	default:
		return "", "", i, false
	}
	j := i + len(kind) + 1
	j = skipWhitespace(s, j)
	if j >= len(s) || s[j] != '{' {
		return "", "", i, false
	}
	env, end, ok := readBalanced(s, j, '{', '}')
	if !ok {
		return "", "", i, false
	}
	return kind, env, end, true
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
