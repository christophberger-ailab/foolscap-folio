package markdown

import (
	"bytes"
	"regexp"
	"slices"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// KindSpan is the node kind of a bracketed span.
var KindSpan = ast.NewNodeKind("Span")

// Span is a Pandoc bracketed span: `[text]{.class}`.
type Span struct {
	ast.BaseInline
	Attrs Attrs
}

// Kind implements ast.Node.
func (n *Span) Kind() ast.NodeKind { return KindSpan }

// Dump implements ast.Node.
func (n *Span) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Attrs": dumpAttrs(n.Attrs)}, nil)
}

// pandocTransformer reads the Pandoc inline syntax goldmark leaves as
// text: heading attributes, attributes after links and images, and
// bracketed spans.
type pandocTransformer struct{}

func (pandocTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	src := reader.Source()
	var containers []ast.Node
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if n.Type() == ast.TypeBlock && n.FirstChild() != nil && n.FirstChild().Type() == ast.TypeInline {
			containers = append(containers, n)
		}
		return ast.WalkContinue, nil
	})
	for _, c := range containers {
		if h, ok := c.(*ast.Heading); ok {
			headingAttrs(h, src)
		}
		inlineContainer(c, src)
	}
}

// inlineContainer applies the inline rules to the children of n and,
// recursively, of every inline container below it.
func inlineContainer(n ast.Node, src []byte) {
	trailingLinkAttrs(n, src)
	for spans(n, src) {
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c.(type) {
		case *ast.Emphasis, *ast.Link, *Span, *ast.Image:
			inlineContainer(c, src)
		default:
			if c.Kind().String() == "Strikethrough" {
				inlineContainer(c, src)
			}
		}
	}
}

// headingAttrs moves a trailing `{#id .class}` out of the heading text.
func headingAttrs(h *ast.Heading, src []byte) {
	t, ok := h.LastChild().(*ast.Text)
	if !ok {
		return
	}
	val := string(t.Segment.Value(src))
	switch t.PreviousSibling().(type) {
	case *ast.Image, *ast.Link:
		if strings.HasPrefix(val, "{") {
			return // the image's or link's own, see trailingLinkAttrs
		}
	}
	rest, a, ok := trailingAttrs(val)
	if !ok {
		return
	}
	t.Segment = t.Segment.WithStop(t.Segment.Start + len(rest))
	if t.Segment.Len() == 0 {
		h.RemoveChild(h, t)
	}
	setAttrs(h, a)
	if a.ID != "" {
		h.SetAttributeString("id", a.ID)
	}
	if len(a.Classes) > 0 {
		h.SetAttributeString("class", strings.Join(a.Classes, " "))
	}
}

// trailingLinkAttrs reads `{…}` straight after a link or image.
func trailingLinkAttrs(parent ast.Node, src []byte) {
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		switch c.(type) {
		case *ast.Link, *ast.Image:
		default:
			continue
		}
		t, ok := c.NextSibling().(*ast.Text)
		if !ok {
			continue
		}
		a, rest, ok := leadingAttrs(string(t.Segment.Value(src)))
		if !ok {
			continue
		}
		t.Segment = t.Segment.WithStart(t.Segment.Stop - len(rest))
		if t.Segment.Len() == 0 && !t.SoftLineBreak() && !t.HardLineBreak() {
			parent.RemoveChild(parent, t)
		}
		setAttrs(c, a)
		applyHTMLAttrs(c, a)
	}
}

// applyHTMLAttrs mirrors Pandoc attributes as goldmark attributes so the
// standard HTML renderers write them.
func applyHTMLAttrs(n ast.Node, a Attrs) {
	if a.ID != "" {
		n.SetAttributeString("id", a.ID)
	}
	if len(a.Classes) > 0 {
		n.SetAttributeString("class", strings.Join(a.Classes, " "))
	}
	var style []string
	for _, kv := range a.KV {
		switch kv[0] {
		case "width", "height":
			v := kv[1]
			if strings.IndexFunc(v, func(r rune) bool { return r < '0' || r > '9' }) < 0 {
				v += "px"
			}
			style = append(style, kv[0]+":"+v)
		case "style":
			style = append(style, kv[1])
		default:
			n.SetAttributeString(kv[0], kv[1])
		}
	}
	if len(style) > 0 {
		n.SetAttributeString("style", strings.Join(style, ";"))
	}
}

// bracket is an unmatched `[` in the inline text.
type bracket struct {
	node *ast.Text
	off  int
}

// spans finds one `[…]{attrs}` among the children of parent and turns it
// into a Span. It reports whether it changed anything, so the caller can
// look again.
func spans(parent ast.Node, src []byte) bool {
	var open []bracket
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		t, ok := c.(*ast.Text)
		if !ok {
			continue
		}
		val := t.Segment.Value(src)
		for i := 0; i < len(val); i++ {
			switch val[i] {
			case '\\':
				i++
			case '[':
				open = append(open, bracket{t, i})
			case ']':
				if len(open) == 0 {
					continue
				}
				o := open[len(open)-1]
				open = open[:len(open)-1]
				a, rest, ok := leadingAttrs(string(val[i+1:]))
				if !ok || a.Has("=") {
					continue
				}
				makeSpan(parent, o, t, i, len(val)-len(rest), a)
				return true
			}
		}
	}
	return false
}

// makeSpan moves everything between the bracket at o and the `]` at
// closeOff of closeNode into a new Span; the attributes end at attrsEnd.
func makeSpan(parent ast.Node, o bracket, closeNode *ast.Text, closeOff, attrsEnd int, a Attrs) {
	span := &Span{Attrs: a}
	seg := o.node.Segment
	cseg := closeNode.Segment

	if o.node == closeNode {
		inner := ast.NewTextSegment(text.NewSegment(seg.Start+o.off+1, seg.Start+closeOff))
		span.AppendChild(span, inner)
		after := ast.NewTextSegment(text.NewSegment(seg.Start+attrsEnd, seg.Stop))
		after.SetSoftLineBreak(o.node.SoftLineBreak())
		after.SetHardLineBreak(o.node.HardLineBreak())
		o.node.Segment = seg.WithStop(seg.Start + o.off)
		o.node.SetSoftLineBreak(false)
		o.node.SetHardLineBreak(false)
		parent.InsertAfter(parent, o.node, span)
		parent.InsertAfter(parent, span, after)
		return
	}

	// Collect the nodes strictly between the two texts.
	var middle []ast.Node
	for c := o.node.NextSibling(); c != nil && c != ast.Node(closeNode); c = c.NextSibling() {
		middle = append(middle, c)
	}
	first := ast.NewTextSegment(text.NewSegment(seg.Start+o.off+1, seg.Stop))
	first.SetSoftLineBreak(o.node.SoftLineBreak())
	first.SetHardLineBreak(o.node.HardLineBreak())
	o.node.Segment = seg.WithStop(seg.Start + o.off)
	o.node.SetSoftLineBreak(false)
	o.node.SetHardLineBreak(false)
	if first.Segment.Len() > 0 || first.SoftLineBreak() {
		span.AppendChild(span, first)
	}
	for _, m := range middle {
		span.AppendChild(span, m)
	}
	last := ast.NewTextSegment(text.NewSegment(cseg.Start, cseg.Start+closeOff))
	if last.Segment.Len() > 0 {
		span.AppendChild(span, last)
	}
	closeNode.Segment = cseg.WithStart(cseg.Start + attrsEnd)
	parent.InsertAfter(parent, o.node, span)
	if closeNode.Segment.Len() == 0 && !closeNode.SoftLineBreak() && !closeNode.HardLineBreak() {
		parent.RemoveChild(parent, closeNode)
	}
}

// lineSpan matches a line that is all one span whose text starts with
// what opens a block: a list item's marker or a quote's.
var lineSpan = regexp.MustCompile(`^( {0,3})\[((?:[-*+]|\d{1,9}[.)])[ \t]+|>[ \t]?)(.*)\](\{[^{}]*\})[ \t]*$`)

// blockSpans moves the marker of a block out of a span that holds the
// whole line, outside fenced code: `[- Only for POL]{.pol}` becomes
// `- [Only for POL]{.pol}`, so that the span is read before the list and
// the item is the span's; Filter takes out the item with the span
// (issue #163).
func blockSpans(src []byte) []byte {
	if !bytes.Contains(src, []byte("]{")) {
		return src
	}
	lines := bytes.SplitAfter(src, []byte("\n"))
	var code fenceTracker
	for i, line := range lines {
		if code.step(line) {
			continue
		}
		body := bytes.TrimRight(line, "\r\n")
		m := lineSpan.FindSubmatch(body)
		if m == nil || !balanced(m[3]) {
			continue
		}
		if _, ok := ParseAttrs(string(m[4])); !ok {
			continue
		}
		lines[i] = slices.Concat(m[1], m[2], []byte("["), m[3], []byte("]"), m[4], line[len(body):])
	}
	return bytes.Join(lines, nil)
}

// balanced reports whether the brackets of s pair up.
func balanced(s []byte) bool {
	depth := 0
	for _, c := range s {
		switch c {
		case '[':
			depth++
		case ']':
			if depth--; depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}
