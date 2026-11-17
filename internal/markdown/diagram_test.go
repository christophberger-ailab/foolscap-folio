package markdown

import (
	"fmt"
	"strings"
	"testing"
)

// A ```{mermaid} block is drawn as an SVG diagram; ```mermaid, without
// braces, stays a code block highlighted as Mermaid.
func TestMermaidDiagram(t *testing.T) {
	src := "```{mermaid}\nflowchart LR\n  A[Start] --> B[End]\n```\n\n" +
		"```{.mermaid}\nflowchart LR\n  C --> D\n  accTitle: Second\n```\n\n" +
		"```mermaid\nflowchart LR\n  E --> F\n```\n"
	var warned []string
	got := render(t, src, Options{Warn: func(f string, a ...any) { warned = append(warned, fmt.Sprintf(f, a...)) }})
	if n := strings.Count(got, `<div class="mermaid"><svg `); n != 2 {
		t.Errorf("%d diagrams, want 2:\n%s", n, got)
	}
	if !strings.Contains(got, ">Start</text>") || !strings.Contains(got, `id="mermaid-2-title"`) {
		t.Errorf("diagrams not drawn from their source, or without their own ids:\n%s", got)
	}
	if !strings.Contains(got, `<pre class="sourceCode mermaid"><code class="language-mermaid">flowchart LR`) {
		t.Errorf("```mermaid is not a code block:\n%s", got)
	}
	if len(warned) > 0 {
		t.Errorf("warnings %q", warned)
	}
}

// A diagram that cannot be drawn stays a code block, with a warning.
func TestMermaidError(t *testing.T) {
	var warned []string
	got := render(t, "```{mermaid}\nflowchart LR\n  A -->\n```\n", Options{Warn: func(f string, a ...any) { warned = append(warned, fmt.Sprintf(f, a...)) }})
	if !strings.Contains(got, `<pre class="sourceCode mermaid">`) || strings.Contains(got, "<svg") {
		t.Errorf("not left a code block:\n%s", got)
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "Mermaid") {
		t.Errorf("warnings %q, want one about the diagram", warned)
	}
	render(t, "```{mermaid}\nnonsense\n```\n", Options{}) // no Warn, no panic
}
