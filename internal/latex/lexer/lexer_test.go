package lexer

import (
	"strings"
	"testing"
)

func TestCommandNameUsesTeXLetterBoundary(t *testing.T) {
	if IsCommandAt(`\citep{key}`, 0, "cite") {
		t.Fatal(`\citep was misidentified as \cite`)
	}
	if !IsCommandAt(`\cite{key}`, 0, "cite") {
		t.Fatal(`\cite was not identified`)
	}
	if IsCommandAt(`\includegraphics{a}`, 0, "include") {
		t.Fatal(`\includegraphics was misidentified as \include`)
	}
}

func TestProjectCommandNamesUseStandardControlWords(t *testing.T) {
	if !IsCommandAt(`\defInstitution{cse}{School}`, 0, "defInstitution") {
		t.Fatal(`\defInstitution was not identified`)
	}
	if IsCommandAt(`\defInstitutionMore{cse}{School}`, 0, "defInstitution") {
		t.Fatal(`\defInstitutionMore was misidentified as \defInstitution`)
	}
	if IsCommandAt(`\def_institution{cse}{School}`, 0, "def_institution") {
		t.Fatal(`\def_institution should not be treated as a standard control word`)
	}
}

func TestTokenizePreservesSpans(t *testing.T) {
	tokens := Tokenize(`a \textbf{x}`)
	if len(tokens) < 5 {
		t.Fatalf("too few tokens: %#v", tokens)
	}
	if tokens[1].Kind != Command || tokens[1].Value != "textbf" || tokens[1].Start != 2 {
		t.Fatalf("unexpected command token: %#v", tokens[1])
	}
	if tokens[len(tokens)-1].Kind != EOF {
		t.Fatalf("missing EOF token: %#v", tokens)
	}
}

func TestTokenizeComments(t *testing.T) {
	tokens := Tokenize("A % comment\nB")
	if len(tokens) < 4 {
		t.Fatalf("too few tokens: %#v", tokens)
	}
	if tokens[1].Kind != Comment || tokens[1].Value != "% comment" {
		t.Fatalf("unexpected comment token: %#v", tokens)
	}
}

func TestTokenizeProtectsRawEnvironments(t *testing.T) {
	in := `\begin{lstlisting}
100% stays
\input{hidden}
\end{lstlisting}
After`
	tokens := Tokenize(in)
	if len(tokens) < 3 {
		t.Fatalf("too few tokens: %#v", tokens)
	}
	if tokens[0].Kind != Raw {
		t.Fatalf("first token is %v, want Raw: %#v", tokens[0].Kind, tokens)
	}
	if tokens[0].Value[:len(`\begin{lstlisting}`)] != `\begin{lstlisting}` {
		t.Fatalf("unexpected raw token: %#v", tokens[0])
	}
}

func TestTokenizeProtectsHTMLEnvironment(t *testing.T) {
	in := `\begin{html}
<div data-tex="\section{Not A Section}">100% stays</div>
\end{html}
After`
	tokens := Tokenize(in)
	if len(tokens) < 3 {
		t.Fatalf("too few tokens: %#v", tokens)
	}
	if tokens[0].Kind != Raw {
		t.Fatalf("first token is %v, want Raw: %#v", tokens[0].Kind, tokens)
	}
	if !strings.Contains(tokens[0].Value, `\section{Not A Section}`) || !strings.Contains(tokens[0].Value, "100% stays") {
		t.Fatalf("html environment content was not protected: %#v", tokens[0])
	}
}

func TestTokenizeProtectsVerbCommand(t *testing.T) {
	tokens := Tokenize(`Before \verb|100% \input{x}| after`)
	foundRaw := false
	for _, tok := range tokens {
		if tok.Kind == Raw && tok.Value == `\verb|100% \input{x}|` {
			foundRaw = true
		}
		if tok.Kind == Comment {
			t.Fatalf("verb content produced comment token: %#v", tokens)
		}
	}
	if !foundRaw {
		t.Fatalf("verb command was not protected: %#v", tokens)
	}
}
