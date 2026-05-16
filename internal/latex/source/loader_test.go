package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSkipsCyclicInput(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.tex")
	aPath := filepath.Join(dir, "a.tex")

	if err := os.WriteFile(mainPath, []byte(`\documentclass{article}
\begin{document}
Before.
\input{a}
After.
\end{document}
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(aPath, []byte(`Inside A.
\input{a}
After recursive input.
`), 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(loaded.Document, "Inside A.") {
		t.Fatalf("expected first input expansion, got %q", loaded.Document)
	}
	if !strings.Contains(loaded.Document, "After recursive input.") {
		t.Fatalf("expected content after cyclic input to remain, got %q", loaded.Document)
	}
	for _, warning := range loaded.Warnings {
		if strings.Contains(warning, "skipped cyclic input") && strings.Contains(warning, aPath) {
			return
		}
	}
	t.Fatalf("expected cyclic input warning for %s, got %#v", aPath, loaded.Warnings)
}

func TestLoadIgnoresCommentedInput(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.tex")
	commentedPath := filepath.Join(dir, "commented.tex")
	livePath := filepath.Join(dir, "live.tex")

	if err := os.WriteFile(mainPath, []byte(`\documentclass{article}
\begin{document}
Before.
% \input{commented}
\input{live}
Literal percent \% should remain.
\end{document}
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(commentedPath, []byte(`Commented input should not appear.
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(livePath, []byte(`Live input should appear.
% Comment inside input should be stripped.
`), 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(loaded.Document, "Commented input should not appear.") {
		t.Fatalf("commented input was expanded: %q", loaded.Document)
	}
	if !strings.Contains(loaded.Document, "Live input should appear.") {
		t.Fatalf("live input was not expanded: %q", loaded.Document)
	}
	if strings.Contains(loaded.Document, "Comment inside input should be stripped.") {
		t.Fatalf("included file comments were not stripped before expansion: %q", loaded.Document)
	}
	if !strings.Contains(loaded.Document, `Literal percent \% should remain.`) {
		t.Fatalf("escaped percent was stripped as a comment: %q", loaded.Document)
	}
}

func TestLoadExpandsIncludeLikeInput(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.tex")
	sectionPath := filepath.Join(dir, "section.tex")

	if err := os.WriteFile(mainPath, []byte(`\documentclass{article}
\begin{document}
Before.
\include{section}
After.
\end{document}
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sectionPath, []byte(`Included section body.
`), 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(loaded.Document, "Included section body.") {
		t.Fatalf("include was not expanded: %q", loaded.Document)
	}
}

func TestLoadDoesNotTreatIncludegraphicsAsInclude(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.tex")

	if err := os.WriteFile(mainPath, []byte(`\documentclass{article}
\begin{document}
\includegraphics{fig/example}
\end{document}
`), 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(loaded.Document, `\includegraphics{fig/example}`) {
		t.Fatalf("includegraphics was unexpectedly modified: %q", loaded.Document)
	}
}

func TestLoadOnlyExpandsBracedInputSyntax(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.tex")
	bracedPath := filepath.Join(dir, "braced.tex")
	plainPath := filepath.Join(dir, "plain.tex")

	if err := os.WriteFile(mainPath, []byte(`\documentclass{article}
\begin{document}
\input{braced}
\input plain
\input2{plain}
\end{document}
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bracedPath, []byte(`Braced input body.
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plainPath, []byte(`Plain input body.
`), 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(loaded.Document, "Braced input body.") {
		t.Fatalf("braced input was not expanded: %q", loaded.Document)
	}
	if strings.Contains(loaded.Document, "Plain input body.") {
		t.Fatalf("unbraced input syntax was unexpectedly expanded: %q", loaded.Document)
	}
	if !strings.Contains(loaded.Document, `\input plain`) {
		t.Fatalf("unbraced input syntax should be preserved: %q", loaded.Document)
	}
	if !strings.Contains(loaded.Document, `\input2{plain}`) {
		t.Fatalf("non-command input prefix should be preserved: %q", loaded.Document)
	}
}

func TestLoadDoesNotExpandInputInsideRawEnvironments(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.tex")
	hiddenPath := filepath.Join(dir, "hidden.tex")

	if err := os.WriteFile(mainPath, []byte(`\documentclass{article}
\begin{document}
\begin{lstlisting}
100% stays
\input{hidden}
\end{lstlisting}
\begin{minted}{go}
fmt.Println("200% stays")
\input{hidden}
\end{minted}
\begin{html}
<div data-tex="\input{hidden}">300% stays</div>
\end{html}
\end{document}
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hiddenPath, []byte(`Hidden input body.
`), 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"100% stays", "200% stays", "300% stays", `\input{hidden}`} {
		if !strings.Contains(loaded.Document, want) {
			t.Fatalf("raw environment content was unexpectedly changed; missing %q in:\n%s", want, loaded.Document)
		}
	}
	if strings.Contains(loaded.Document, "Hidden input body.") {
		t.Fatalf("input inside raw environment was expanded:\n%s", loaded.Document)
	}
}

func TestLoadIgnoresDocumentMarkersInsideRawBeforeDocument(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.tex")
	if err := os.WriteFile(mainPath, []byte(`\begin{verbatim}
\begin{document}
wrong body
\end{verbatim}

\begin{document}
real body
\end{document}
`), 0644); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(loaded.Document, "real body") {
		t.Fatalf("real document body missing: %q", loaded.Document)
	}
	if strings.Contains(loaded.Document, "wrong body") {
		t.Fatalf("raw preamble document marker was used as real body: %q", loaded.Document)
	}
}

func TestLoadIgnoresEndDocumentInsideRawBody(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.tex")
	if err := os.WriteFile(mainPath, []byte(`\begin{document}
before
\begin{lstlisting}
\end{document}
\end{lstlisting}
after
\end{document}
`), 0644); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(loaded.Document, "after") {
		t.Fatalf("document ended at raw environment marker: %q", loaded.Document)
	}
}
