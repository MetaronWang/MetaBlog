package blocks

import (
	"strings"
	"testing"
)

func TestLiftExtractsComplexMetadataWithTokenBoundaries(t *testing.T) {
	res := Lift(`Before.

\begin{algorithm}
\caption{Main caption mentioning \label{not:block}}
\begin{tabular}{c}
\label{not:block:either}
x
\end{tabular}
\label{alg:real}
\end{algorithm}

After.`)
	if len(res.Blocks) != 1 {
		t.Fatalf("expected one complex block, got %#v", res.Blocks)
	}
	var block *ComplexBlock
	for _, b := range res.Blocks {
		block = b
	}
	if block.Caption != `Main caption mentioning \label{not:block}` {
		t.Fatalf("caption parsed incorrectly: %q", block.Caption)
	}
	if block.Label != "alg:real" {
		t.Fatalf("label parsed incorrectly: %q", block.Label)
	}
}

func TestLiftDoesNotExtractComplexBlocksInsideRawTextEnvironments(t *testing.T) {
	res := Lift(`Before.

\begin{minted}{latex}
\begin{algorithm}
\caption{This is code, not a real algorithm block}
\end{algorithm}
\end{minted}

\begin{lstlisting}
\begin{tabular}{c}
x
\end{tabular}
\end{lstlisting}

\begin{html}
<pre>\begin{algorithm}\end{algorithm}</pre>
\end{html}

After.`)
	if len(res.Blocks) != 0 {
		t.Fatalf("expected no complex blocks inside raw environments, got %#v", res.Blocks)
	}
	if !strings.Contains(res.Text, `\begin{algorithm}`) || strings.Contains(res.Text, "@@METABLOG_COMPLEX_BLOCK_") {
		t.Fatalf("raw environment content was not preserved:\n%s", res.Text)
	}
}
