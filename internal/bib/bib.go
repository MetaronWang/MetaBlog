package bib

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"MetaBlog/internal/latex/ast"
)

func Load(root string, files []string, warnings *[]string) map[string]ast.ReferenceEntry {
	out := map[string]ast.ReferenceEntry{}
	for _, name := range files {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		path := name
		if filepath.Ext(path) == "" {
			path += ".bib"
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			warn(warnings, "could not read bibliography: "+path)
			continue
		}
		for key, entry := range Parse(string(b)) {
			out[key] = entry
		}
	}
	return out
}

func Parse(s string) map[string]ast.ReferenceEntry {
	out := map[string]ast.ReferenceEntry{}
	for i := 0; i < len(s); {
		at := strings.IndexByte(s[i:], '@')
		if at < 0 {
			break
		}
		i += at
		j := i + 1
		for j < len(s) && isNameChar(s[j]) {
			j++
		}
		typ := strings.ToLower(strings.TrimSpace(s[i+1 : j]))
		for j < len(s) && isSpace(s[j]) {
			j++
		}
		if j >= len(s) || (s[j] != '{' && s[j] != '(') {
			i = j
			continue
		}
		open, close := s[j], byte('}')
		if open == '(' {
			close = ')'
		}
		body, end, ok := readBalanced(s, j, open, close)
		if !ok {
			i = j + 1
			continue
		}
		key, fields := parseEntryBody(body)
		if key != "" {
			out[key] = ast.ReferenceEntry{Key: key, Type: typ, Fields: fields}
		}
		i = end
	}
	return out
}

func parseEntryBody(body string) (string, map[string]string) {
	comma := strings.IndexByte(body, ',')
	if comma < 0 {
		return strings.TrimSpace(body), map[string]string{}
	}
	key := strings.TrimSpace(body[:comma])
	fields := map[string]string{}
	i := comma + 1
	for i < len(body) {
		for i < len(body) && (isSpace(body[i]) || body[i] == ',') {
			i++
		}
		start := i
		for i < len(body) && isNameChar(body[i]) {
			i++
		}
		if start == i {
			break
		}
		name := strings.ToLower(strings.TrimSpace(body[start:i]))
		for i < len(body) && isSpace(body[i]) {
			i++
		}
		if i >= len(body) || body[i] != '=' {
			break
		}
		i++
		for i < len(body) && isSpace(body[i]) {
			i++
		}
		val, end := readValue(body, i)
		if name != "" {
			fields[name] = cleanTeX(val)
		}
		i = end
	}
	return key, fields
}

func readValue(s string, i int) (string, int) {
	if i >= len(s) {
		return "", i
	}
	if s[i] == '{' {
		val, end, ok := readBalanced(s, i, '{', '}')
		if ok {
			return val, end
		}
	}
	if s[i] == '"' {
		var b strings.Builder
		for j := i + 1; j < len(s); j++ {
			if s[j] == '\\' && j+1 < len(s) {
				b.WriteByte(s[j])
				j++
				b.WriteByte(s[j])
				continue
			}
			if s[j] == '"' {
				return b.String(), j + 1
			}
			b.WriteByte(s[j])
		}
		return b.String(), len(s)
	}
	start := i
	for i < len(s) && s[i] != ',' && s[i] != '\n' && s[i] != '\r' {
		i++
	}
	return strings.TrimSpace(s[start:i]), i
}

func readBalanced(s string, start int, open, close byte) (string, int, bool) {
	depth := 0
	for i := start; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
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

var whitespaceRE = regexp.MustCompile(`\s+`)
var commandRE = regexp.MustCompile(`\\[a-zA-Z]+\*?(?:\s*\{([^{}]*)\})?`)

func cleanTeX(s string) string {
	replacer := strings.NewReplacer(
		`{\"{u}}`, "u", `{\"u}`, "u",
		`{\"{U}}`, "U", `{\"U}`, "U",
		`{\ss}`, "ss", `\ss`, "ss",
		`{-}`, "-", `{--}`, "--",
		"``", `"`, "''", `"`,
		"{", "", "}", "",
	)
	s = replacer.Replace(s)
	s = commandRE.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(whitespaceRE.ReplaceAllString(s, " "))
}

func isNameChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '-' || b == ':'
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

func warn(warnings *[]string, msg string) {
	if warnings != nil {
		*warnings = append(*warnings, msg)
	}
}
