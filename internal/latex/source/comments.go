package source

import (
	"strings"

	"MetaBlog/internal/latex/lexer"
)

func StripComments(s string) string {
	p := sourceParser{raw: s, tokens: lexer.Tokenize(s)}
	for _, tok := range p.tokens {
		if tok.Kind == lexer.EOF {
			break
		}
		if tok.Kind == lexer.Comment {
			p.skipComment()
			continue
		}
		p.writeText(s[tok.Start:tok.End])
	}
	return p.out.String()
}

func truncateBuilder(b *strings.Builder, n int) {
	if n == b.Len() {
		return
	}
	s := b.String()
	b.Reset()
	b.WriteString(s[:n])
}
