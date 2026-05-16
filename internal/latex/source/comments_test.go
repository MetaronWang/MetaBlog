package source

import (
	"strings"
	"testing"
)

func TestStripCommentsDropsFullCommentLinesWithoutBlankLines(t *testing.T) {
	in := `In particular, DACE
% is a general-purpose PAP construction approach for binary optimization problems.
% It 
requires only a small set of training instances.`

	got := StripComments(in)
	want := `In particular, DACE
requires only a small set of training instances.`
	if got != want {
		t.Fatalf("unexpected comment stripping:\nwant: %q\n got: %q", want, got)
	}
	if strings.Contains(got, "\n\n") {
		t.Fatalf("full-line comments produced blank lines: %q", got)
	}
}

func TestStripCommentsKeepsInlineCommentLineBreakAndEscapedPercent(t *testing.T) {
	in := "Before % comment\n100\\% remains\nAfter"

	got := StripComments(in)
	want := "Before \n100\\% remains\nAfter"
	if got != want {
		t.Fatalf("unexpected comment stripping:\nwant: %q\n got: %q", want, got)
	}
}

func TestStripCommentsProtectsRawEnvironments(t *testing.T) {
	in := `Before
\begin{verbatim}
100% remains
\end{verbatim}
\begin{lstlisting}
200% remains
\end{lstlisting}
\begin{minted}{go}
fmt.Println("300% remains")
\end{minted}
\begin{html}
<div data-rate="400%">HTML percent remains</div>
\end{html}
After % comment`

	got := StripComments(in)
	for _, want := range []string{"100% remains", "200% remains", "300% remains", `data-rate="400%"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("raw environment content was stripped; missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "comment") {
		t.Fatalf("normal comment was not stripped:\n%s", got)
	}
}
