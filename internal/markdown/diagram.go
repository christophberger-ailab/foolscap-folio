package markdown

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/util"

	"github.com/christophberger-ailab/foolscap-folio/internal/mermaid"
)

// KindDiagram is the node kind of a Diagram.
var KindDiagram = ast.NewNodeKind("Diagram")

// Diagram is a Mermaid diagram, ```{mermaid}, drawn as SVG.
type Diagram struct {
	ast.BaseBlock
	SVG string
}

// Kind implements ast.Node.
func (n *Diagram) Kind() ast.NodeKind { return KindDiagram }

// Dump implements ast.Node.
func (n *Diagram) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

// drawDiagrams replaces the ```{mermaid} blocks with their diagrams. A
// block that cannot be drawn stays a code block, and warn says why.
func drawDiagrams(d *Document, warn func(string, ...any)) {
	var blocks []*ast.FencedCodeBlock
	_ = ast.Walk(d.Root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if cb, ok := n.(*ast.FencedCodeBlock); ok && entering && isMermaid(cb, d.Source) {
			blocks = append(blocks, cb)
		}
		return ast.WalkContinue, nil
	})
	for i, cb := range blocks {
		src := Code(cb, d.Source)
		svg, err := mermaid.Render(src, fmt.Sprintf("mermaid-%d", i+1))
		if err != nil {
			if warn != nil {
				first, _, _ := strings.Cut(strings.TrimSpace(src), "\n")
				warn("Mermaid diagram %q stays a code block: %s", first, strings.Join(strings.Fields(err.Error()), " "))
			}
			continue
		}
		cb.Parent().ReplaceChild(cb.Parent(), cb, &Diagram{SVG: svg})
	}
}

// isMermaid reports whether a code block is a diagram: Mermaid in braces,
// ```{mermaid} or ```{.mermaid}.
func isMermaid(cb *ast.FencedCodeBlock, src []byte) bool {
	return CodeInfo(cb, src).Lang == "mermaid" && strings.HasPrefix(strings.TrimSpace(string(cb.Info.Segment.Value(src))), "{")
}

func renderDiagram(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		w.WriteString(`<div class="mermaid">` + n.(*Diagram).SVG + "</div>\n")
	}
	return ast.WalkSkipChildren, nil
}
