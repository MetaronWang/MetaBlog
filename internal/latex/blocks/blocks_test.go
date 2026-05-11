package blocks

import "testing"

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
