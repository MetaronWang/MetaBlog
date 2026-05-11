package lexer

type Kind int

const (
	Text Kind = iota
	Command
	Comment
	Raw
	LBrace
	RBrace
	LBracket
	RBracket
	Dollar
	EOF
)

type Token struct {
	Kind  Kind
	Value string
	Start int
	End   int
}

func Tokenize(s string) []Token {
	var out []Token
	for i := 0; i < len(s); {
		if rawEnd, ok := rawEnvironmentEnd(s, i); ok {
			out = append(out, Token{Kind: Raw, Value: s[i:rawEnd], Start: i, End: rawEnd})
			i = rawEnd
			continue
		}
		if rawEnd, ok := verbCommandEnd(s, i); ok {
			out = append(out, Token{Kind: Raw, Value: s[i:rawEnd], Start: i, End: rawEnd})
			i = rawEnd
			continue
		}
		switch s[i] {
		case '\\':
			start := i
			i++
			if i < len(s) && isLetter(s[i]) {
				for i < len(s) && isLetter(s[i]) {
					i++
				}
				out = append(out, Token{Kind: Command, Value: s[start+1 : i], Start: start, End: i})
				continue
			}
			if i < len(s) {
				i++
			}
			out = append(out, Token{Kind: Command, Value: s[start+1 : i], Start: start, End: i})
		case '%':
			start := i
			for i < len(s) && s[i] != '\n' && s[i] != '\r' {
				i++
			}
			out = append(out, Token{Kind: Comment, Value: s[start:i], Start: start, End: i})
		case '{':
			out = append(out, Token{Kind: LBrace, Value: "{", Start: i, End: i + 1})
			i++
		case '}':
			out = append(out, Token{Kind: RBrace, Value: "}", Start: i, End: i + 1})
			i++
		case '[':
			out = append(out, Token{Kind: LBracket, Value: "[", Start: i, End: i + 1})
			i++
		case ']':
			out = append(out, Token{Kind: RBracket, Value: "]", Start: i, End: i + 1})
			i++
		case '$':
			out = append(out, Token{Kind: Dollar, Value: "$", Start: i, End: i + 1})
			i++
		default:
			start := i
			for i < len(s) && !isSpecial(s[i]) {
				i++
			}
			out = append(out, Token{Kind: Text, Value: s[start:i], Start: start, End: i})
		}
	}
	out = append(out, Token{Kind: EOF, Start: len(s), End: len(s)})
	return out
}

func CommandNameAt(s string, i int) (string, int, bool) {
	if i >= len(s) || s[i] != '\\' {
		return "", i, false
	}
	j := i + 1
	if j >= len(s) || !isLetter(s[j]) {
		return "", i, false
	}
	for j < len(s) && isLetter(s[j]) {
		j++
	}
	return s[i+1 : j], j, true
}

func IsCommandAt(s string, i int, cmd string) bool {
	name, _, ok := CommandNameAt(s, i)
	return ok && name == cmd
}

func FindCommand(s string, start int, cmd string) int {
	for i := start; i < len(s); i++ {
		if s[i] != '\\' {
			continue
		}
		name, end, ok := CommandNameAt(s, i)
		if !ok {
			i++
			continue
		}
		if name == cmd {
			return i
		}
		i = end - 1
	}
	return -1
}

func FindAnyCommand(s string, start int, cmds ...string) (int, string) {
	for i := start; i < len(s); i++ {
		if s[i] != '\\' {
			continue
		}
		name, end, ok := CommandNameAt(s, i)
		if !ok {
			i++
			continue
		}
		for _, cmd := range cmds {
			if name == cmd {
				return i, name
			}
		}
		i = end - 1
	}
	return -1, ""
}

func isSpecial(b byte) bool {
	switch b {
	case '\\', '%', '{', '}', '[', ']', '$':
		return true
	default:
		return false
	}
}

func isLetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func rawEnvironmentEnd(s string, i int) (int, bool) {
	env, beginEnd, ok := beginEnvironmentAt(s, i)
	if !ok || !isRawEnvironment(env) {
		return i, false
	}
	endTag := `\end{` + env + `}`
	if end := findRawEnvironmentClose(s, beginEnd, endTag); end >= 0 {
		return end + len(endTag), true
	}
	return len(s), true
}

func beginEnvironmentAt(s string, i int) (string, int, bool) {
	if !IsCommandAt(s, i, "begin") {
		return "", i, false
	}
	j := i + len(`\begin`)
	for j < len(s) && isSpace(s[j]) {
		j++
	}
	if j >= len(s) || s[j] != '{' {
		return "", i, false
	}
	start := j + 1
	for j = start; j < len(s) && s[j] != '}'; j++ {
	}
	if j >= len(s) {
		return "", i, false
	}
	return s[start:j], j + 1, true
}

func isRawEnvironment(env string) bool {
	switch env {
	case "verbatim", "lstlisting", "minted":
		return true
	default:
		return false
	}
}

func findRawEnvironmentClose(s string, start int, endTag string) int {
	for i := start; i <= len(s)-len(endTag); i++ {
		if s[i] == '\\' && !isEscapedAt(s, i) && startsWithAt(s, i, endTag) {
			return i
		}
	}
	return -1
}

func verbCommandEnd(s string, i int) (int, bool) {
	if !IsCommandAt(s, i, "verb") {
		return i, false
	}
	j := i + len(`\verb`)
	if j < len(s) && s[j] == '*' {
		j++
	}
	if j >= len(s) || isSpace(s[j]) {
		return i, false
	}
	delim := s[j]
	j++
	for j < len(s) {
		if s[j] == delim {
			return j + 1, true
		}
		if s[j] == '\n' || s[j] == '\r' {
			return i, false
		}
		j++
	}
	return len(s), true
}

func startsWithAt(s string, i int, needle string) bool {
	return i >= 0 && i+len(needle) <= len(s) && s[i:i+len(needle)] == needle
}

func isEscapedAt(s string, idx int) bool {
	count := 0
	for i := idx - 1; i >= 0 && s[i] == '\\'; i-- {
		count++
	}
	return count%2 == 1
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}
