package markdown

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// KindDiv is the node kind of a fenced div.
var KindDiv = ast.NewNodeKind("Div")

// Div is a Pandoc fenced div: `::: {.class}` … `:::`.
type Div struct {
	ast.BaseBlock
	Attrs Attrs
}

// Kind implements ast.Node.
func (n *Div) Kind() ast.NodeKind { return KindDiv }

// Dump implements ast.Node.
func (n *Div) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Attrs": dumpAttrs(n.Attrs)}, nil)
}

func dumpAttrs(a Attrs) string {
	var b strings.Builder
	if a.ID != "" {
		b.WriteString("#" + a.ID + " ")
	}
	for _, c := range a.Classes {
		b.WriteString("." + c + " ")
	}
	for _, kv := range a.KV {
		b.WriteString(kv[0] + "=" + kv[1] + " ")
	}
	return strings.TrimSpace(b.String())
}

var (
	// openFence is `:::` (or longer) followed by attributes or one bare
	// class, optionally closed by more colons.
	openFence = regexp.MustCompile(`^ {0,3}:{3,}[ \t]*(\{.*\}|[^\s{}:][^\s{}]*)[ \t]*:*[ \t]*$`)
	// closeFence is a line of colons and nothing else.
	closeFence = regexp.MustCompile(`^ {0,3}:{3,}[ \t]*$`)
)

var (
	divStackKey  = parser.NewContextKey()
	codeDepthKey = parser.NewContextKey()
)

func divStack(pc parser.Context) []*Div {
	s, _ := pc.Get(divStackKey).([]*Div)
	return s
}

type divParser struct{}

func (divParser) Trigger() []byte { return []byte{':'} }

func (divParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	m := openFence.FindSubmatch(util.TrimRightSpace(line))
	if m == nil {
		return nil, parser.NoChildren
	}
	spec := string(m[1])
	var a Attrs
	if strings.HasPrefix(spec, "{") {
		var ok bool
		if a, ok = ParseAttrs(spec); !ok {
			return nil, parser.NoChildren
		}
	} else {
		a.Classes = []string{spec}
	}
	d := &Div{Attrs: a}
	pc.Set(divStackKey, append(divStack(pc), d))
	reader.AdvanceToEOL()
	return d, parser.HasChildren
}

func (divParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, _ := reader.PeekLine()
	if closeFence.Match(util.TrimRightSpace(line)) {
		depth, _ := pc.Get(codeDepthKey).(int)
		s := divStack(pc)
		// A closing fence belongs to the innermost open div, and never to a
		// div around an open code block.
		if depth == 0 && len(s) > 0 && s[len(s)-1] == node {
			reader.AdvanceToEOL()
			return parser.Close
		}
	}
	return parser.Continue | parser.HasChildren
}

func (divParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {
	s := divStack(pc)
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == node {
			s = append(s[:i], s[i+1:]...)
			break
		}
	}
	pc.Set(divStackKey, s)
}

func (divParser) CanInterruptParagraph() bool { return false }
func (divParser) CanAcceptIndentedLine() bool { return false }

// codeTracker wraps the fenced code and HTML block parsers to count the
// open blocks of verbatim lines, so that a `:::` line inside code or an
// HTML comment does not close a div. An HTML block that ends on its first
// line is not counted: goldmark asks the div about the next line before
// the block says it is over, and a fence there would be held back for it
// (issue #175).
type codeTracker struct{ parser.BlockParser }

func (c codeTracker) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	n, st := c.BlockParser.Open(parent, reader, pc)
	if n != nil && !oneLine(n, reader.Source()) {
		d, _ := pc.Get(codeDepthKey).(int)
		pc.Set(codeDepthKey, d+1)
	}
	return n, st
}

func (c codeTracker) Close(node ast.Node, reader text.Reader, pc parser.Context) {
	c.BlockParser.Close(node, reader, pc)
	if d, _ := pc.Get(codeDepthKey).(int); d > 0 && !oneLine(node, reader.Source()) {
		pc.Set(codeDepthKey, d-1)
	}
}

var (
	htmlType1Close = regexp.MustCompile(`(?i)</(?:script|pre|style|textarea)>`)
	htmlCloses     = map[ast.HTMLBlockType]string{
		ast.HTMLBlockType2: "-->",
		ast.HTMLBlockType3: "?>",
		ast.HTMLBlockType4: ">",
		ast.HTMLBlockType5: "]]>",
	}
)

// oneLine reports whether n is an HTML block that ends on its first line,
// as goldmark's HTML block parser reads it.
func oneLine(n ast.Node, source []byte) bool {
	h, ok := n.(*ast.HTMLBlock)
	if !ok || h.Lines().Len() != 1 {
		return false
	}
	seg := h.Lines().At(0)
	first := seg.Value(source)
	if h.HTMLBlockType == ast.HTMLBlockType1 {
		return htmlType1Close.Match(first)
	}
	end, ok := htmlCloses[h.HTMLBlockType]
	return ok && bytes.Contains(first, []byte(end))
}
