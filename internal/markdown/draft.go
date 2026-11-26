package markdown

import (
	"bytes"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// What a draft render makes visible (spec-render.yaml DRAFT): todos, and
// comments with the passages they are about.

// KindTodo and KindComment are the node kinds of Todo and Comment.
var (
	KindTodo    = ast.NewNodeKind("Todo")
	KindComment = ast.NewNodeKind("Comment")
)

// TodoMark is what marks a todo in a page.
const TodoMark = "==TODO=="

// Todo is a `==TODO==` made visible. Hidden is one inside an ordinary
// HTML comment, whose text is Note.
type Todo struct {
	ast.BaseInline
	Hidden bool
	Note   string
}

// Kind implements ast.Node.
func (n *Todo) Kind() ast.NodeKind { return KindTodo }

// Dump implements ast.Node.
func (n *Todo) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

// Comment is a commented passage, its children: the comment and its
// replies in Thread. A passage across blocks is a Comment in each; all
// but the first have no Thread, and all but the last are Open.
type Comment struct {
	ast.BaseInline
	Thread []Note
	Open   bool
}

// Kind implements ast.Node.
func (n *Comment) Kind() ast.NodeKind { return KindComment }

// Dump implements ast.Node.
func (n *Comment) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

// Note is one entry of a comment's thread.
type Note struct{ Author, Date, Text string }

// Meta is who wrote the note and when, as far as it says.
func (n Note) Meta() string {
	switch {
	case n.Author == "" || n.Date == "":
		return n.Author + n.Date
	default:
		return n.Author + ", " + n.Date
	}
}

// marker is a comment marker (spec-web.yaml 3-8): `<!-- << c -->` opens a
// passage, `<!-- <<+ r -->` replies, `<!-- >> -->` closes one.
type marker struct {
	kind string // "<<", "<<+" or ">>"
	id   string
	note Note
}

var (
	htmlComment  = regexp.MustCompile(`^<!--([\s\S]*?)-->$`)
	markerSyntax = regexp.MustCompile(`^\s*(<<\+|<<|>>)[ \t]?`)
	markerAttrs  = regexp.MustCompile(`^[ \t]*(\{[^{}]*\})[ \t]?`)
)

// commentText returns what an HTML comment holds, and whether s is one.
func commentText(s string) (string, bool) {
	m := htmlComment.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", false
	}
	return m[1], true
}

// parseMarker reads the text of an HTML comment as a marker.
func parseMarker(data string) (marker, bool) {
	m := markerSyntax.FindStringSubmatch(data)
	if m == nil {
		return marker{}, false
	}
	mk := marker{kind: m[1]}
	rest := data[len(m[0]):]
	if a := markerAttrs.FindStringSubmatch(rest); a != nil {
		if attrs, ok := ParseAttrs(a[1]); ok {
			mk.id = attrs.ID
			mk.note.Author, _ = attrs.Get("author")
			mk.note.Date, _ = attrs.Get("date")
			rest = rest[len(a[0]):]
		}
	}
	mk.note.Text = strings.TrimSpace(rest)
	return mk, true
}

// lineComment is an HTML comment at the start of a line with text after
// it on the line, which Markdown would take for an HTML block.
var lineComment = regexp.MustCompile(`^ {0,3}<!--[\s\S]*?-->[ \t]*\S`)

// unblock goes before every comment marker that starts a line and has
// text after it, outside fenced code, so that the line is a paragraph
// holding the marker and its passage, as in the editor's preview, rather
// than an HTML block that keeps the passage from being read as Markdown.
// Parse takes it out again.
const unblock = "\u200b"

func unblockComments(src []byte) []byte {
	if !bytes.Contains(src, []byte("<!--")) {
		return src
	}
	var out bytes.Buffer
	var code fenceTracker
	for off := 0; off < len(src); {
		end := bytes.IndexByte(src[off:], '\n') + 1
		if end == 0 {
			end = len(src) - off
		}
		line := src[off : off+end]
		if !code.step(line) && bytes.Contains(line, []byte("<!--")) {
			// A comment may run on over the next lines before its passage.
			if m := lineComment.Find(src[off:]); m != nil {
				if c, ok := commentText(string(m[:len(m)-1])); ok {
					if _, ok := parseMarker(c); ok {
						out.WriteString(unblock)
					}
				}
			}
		}
		out.Write(line)
		off += end
	}
	return out.Bytes()
}

// reblock takes out the unblock characters that start paragraphs.
func reblock(d *Document) {
	_ = ast.Walk(d.Root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if p, ok := n.(*ast.Paragraph); ok && entering {
			if t, ok := p.FirstChild().(*ast.Text); ok && bytes.HasPrefix(t.Segment.Value(d.Source), []byte(unblock)) {
				t.Segment = t.Segment.WithStart(t.Segment.Start + len(unblock))
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
}

// fenceTracker follows the fenced code blocks of a text, line by line.
type fenceTracker struct{ fence string }

// step reads the next line and reports whether it is code: in a fenced
// code block or one of its fences.
func (t *fenceTracker) step(line []byte) bool {
	trimmed := strings.TrimLeft(string(line), " ")
	f := codeFence(trimmed)
	switch {
	case f != "" && t.fence == "":
		t.fence = f
	case f != "" && strings.HasPrefix(f, t.fence) && strings.TrimSpace(trimmed) == f:
		t.fence = ""
	default:
		return t.fence != ""
	}
	return true
}

// codeFence returns the run of backticks or tildes a line opens with, if
// it is three or more long.
func codeFence(line string) string {
	for _, c := range "`~" {
		n := len(line) - len(strings.TrimLeft(line, string(c)))
		if n >= 3 {
			return line[:n]
		}
	}
	return ""
}

// drafts makes the document's todos and comments visible.
func drafts(d *Document) {
	var texts []*ast.Text
	var raws []ast.Node // inline RawHTML and HTMLBlocks, in document order
	var blocks []ast.Node
	_ = ast.Walk(d.Root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.CodeSpan, *ast.FencedCodeBlock, *ast.CodeBlock:
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			if bytes.Contains(n.Segment.Value(d.Source), []byte(TodoMark)) {
				texts = append(texts, n)
			}
		case *ast.RawHTML, *ast.HTMLBlock:
			raws = append(raws, n)
		}
		if n.Type() == ast.TypeBlock && n.FirstChild() != nil && n.FirstChild().Type() == ast.TypeInline {
			blocks = append(blocks, n)
		}
		return ast.WalkContinue, nil
	})
	for _, t := range texts {
		splitTodos(t, d.Source)
	}
	comments(d, raws, blocks)
}

// splitTodos replaces every todo in t by a Todo.
func splitTodos(t *ast.Text, src []byte) {
	parent := t.Parent()
	seg := t.Segment
	for {
		v := seg.Value(src)
		i := bytes.Index(v, []byte(TodoMark))
		if i < 0 {
			break
		}
		if i > 0 {
			parent.InsertBefore(parent, t, ast.NewTextSegment(text.NewSegment(seg.Start, seg.Start+i)))
		}
		parent.InsertBefore(parent, t, &Todo{})
		seg = text.NewSegment(seg.Start+i+len(TodoMark), seg.Stop)
	}
	if seg.Len() > 0 || t.SoftLineBreak() || t.HardLineBreak() {
		t.Segment = seg
	} else {
		parent.RemoveChild(parent, t)
	}
}

// rawText is the source of an inline RawHTML or an HTMLBlock.
func rawText(n ast.Node, src []byte) string {
	var b strings.Builder
	var segs *text.Segments
	switch n := n.(type) {
	case *ast.RawHTML:
		segs = n.Segments
	case *ast.HTMLBlock:
		segs = n.Lines()
	}
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		b.Write(seg.Value(src))
	}
	if h, ok := n.(*ast.HTMLBlock); ok && h.HasClosure() {
		b.Write(h.ClosureLine.Value(src))
	}
	return b.String()
}

var allComments = regexp.MustCompile(`<!--[\s\S]*?-->`)

// opening is a passage's opening marker and its thread.
type opening struct {
	at     ast.Node
	id     string
	thread []Note
}

// comments turns the comment markers among raws into Comments, and the
// ordinary HTML comments holding a todo into hidden Todos. blocks are the
// document's inline containers in document order.
func comments(d *Document, raws, blocks []ast.Node) {
	type pair struct {
		o *opening
		c ast.Node // nil: unclosed
	}
	var open []*opening
	var pairs []pair
	var drop []ast.Node // the markers, and the spaces between a comment's
	var last *opening   // the opening a reply may follow
	for _, r := range raws {
		if b, ok := r.(*ast.HTMLBlock); ok {
			last = nil
			blockComments(d, b)
			continue
		}
		data, ok := commentText(rawText(r, d.Source))
		if !ok {
			last = nil
			continue
		}
		mk, ok := parseMarker(data)
		if ok {
			drop = append(drop, r)
		}
		switch {
		case !ok:
			if strings.Contains(data, TodoMark) {
				r.Parent().ReplaceChild(r.Parent(), r, &Todo{Hidden: true, Note: strings.TrimSpace(data)})
			}
			last = nil
		case mk.kind == "<<":
			last = &opening{at: r, id: mk.id, thread: []Note{mk.note}}
			open = append(open, last)
		case mk.kind == "<<+":
			if spaces, ok := spaceBetween(last, r, d.Source); ok {
				last.thread = append(last.thread, mk.note)
				last.at = r
				drop = append(drop, spaces...)
			}
		default:
			last = nil
			i := len(open) - 1
			if mk.id != "" {
				for i >= 0 && open[i].id != mk.id {
					i--
				}
			}
			if i >= 0 {
				pairs = append(pairs, pair{open[i], r})
				open = append(open[:i], open[i+1:]...)
			}
		}
	}
	for _, o := range open {
		pairs = append(pairs, pair{o, nil})
	}
	blockOf := func(n ast.Node) int {
		for ; n != nil; n = n.Parent() {
			for i, b := range blocks {
				if b == n {
					return i
				}
			}
		}
		return -1
	}
	for _, p := range pairs {
		o, c := p.o.at, p.c
		first := &Comment{Thread: p.o.thread}
		if c != nil && c.Parent() == o.Parent() {
			wrap(first, o, c, o.Parent())
			continue
		}
		from, to := blockOf(o), -1
		if c != nil {
			to = blockOf(c)
		}
		first.Open = to > from
		wrap(first, o, nil, o.Parent())
		for i := from + 1; i < to; i++ {
			wrap(&Comment{Open: true}, nil, nil, blocks[i])
		}
		if to > from {
			wrap(&Comment{}, nil, c, c.Parent())
		}
	}
	for _, n := range drop {
		if p := n.Parent(); p != nil {
			p.RemoveChild(p, n)
		}
	}
}

// wrap moves the children of p after after (from the first, when nil)
// up to before (to the last, when nil) into c, which takes their place.
func wrap(c *Comment, after, before, p ast.Node) {
	n := p.FirstChild()
	if after != nil {
		n = after.NextSibling()
	}
	for n != nil && n != before {
		next := n.NextSibling()
		p.RemoveChild(p, n)
		c.AppendChild(c, n)
		n = next
	}
	if c.FirstChild() == nil && len(c.Thread) == 0 && c.Open {
		return // a block between that holds nothing
	}
	if before != nil {
		p.InsertBefore(p, before, c)
	} else {
		p.AppendChild(p, c)
	}
}

// spaceBetween returns the text nodes between o's last marker and its
// sibling r, and whether they are all spaces, which makes r part of o.
func spaceBetween(o *opening, r ast.Node, src []byte) ([]ast.Node, bool) {
	if o == nil || o.at.Parent() != r.Parent() {
		return nil, false
	}
	var spaces []ast.Node
	for n := o.at.NextSibling(); n != r; n = n.NextSibling() {
		t, ok := n.(*ast.Text)
		if n == nil || !ok || len(bytes.TrimSpace(t.Segment.Value(src))) > 0 {
			return nil, false
		}
		spaces = append(spaces, n)
	}
	return spaces, true
}

// blockComments replaces an HTML block of comments alone by a paragraph
// of what a draft shows of them: a comment's note (its passage is the
// blocks below, which the markers do not span) and the todos of ordinary
// comments.
func blockComments(d *Document, b *ast.HTMLBlock) {
	src := rawText(b, d.Source)
	if strings.TrimSpace(allComments.ReplaceAllString(src, "")) != "" {
		return
	}
	para := ast.NewParagraph()
	for _, c := range allComments.FindAllString(src, -1) {
		data, _ := commentText(c)
		mk, ok := parseMarker(data)
		switch {
		case ok && mk.kind == "<<":
			para.AppendChild(para, &Comment{Thread: []Note{mk.note}})
		case ok && mk.kind == "<<+":
			if last, ok := para.LastChild().(*Comment); ok {
				last.Thread = append(last.Thread, mk.note)
			}
		case !ok && strings.Contains(data, TodoMark):
			para.AppendChild(para, &Todo{Hidden: true, Note: strings.TrimSpace(data)})
		}
	}
	if para.FirstChild() != nil {
		b.Parent().ReplaceChild(b.Parent(), b, para)
	}
}

func renderTodo(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if t := n.(*Todo); entering {
		if t.Hidden {
			w.WriteString(`<mark class="folio-todo folio-todo-hidden" title="` + html.EscapeString(t.Note) + `">TODO</mark>`)
		} else {
			w.WriteString(`<mark class="folio-todo">TODO</mark>`)
		}
	}
	return ast.WalkContinue, nil
}

func renderComment(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	c := n.(*Comment)
	if entering {
		if c.FirstChild() != nil {
			w.WriteString(`<mark class="folio-comment">`)
		}
		return ast.WalkContinue, nil
	}
	if c.FirstChild() != nil {
		w.WriteString(`</mark>`)
	}
	if len(c.Thread) == 0 {
		return ast.WalkContinue, nil
	}
	w.WriteString(`<span class="folio-note" role="note">`)
	for i, note := range c.Thread {
		class := "folio-note-entry"
		if i > 0 {
			class += " folio-note-reply"
		}
		w.WriteString(`<span class="` + class + `">`)
		if m := note.Meta(); m != "" {
			w.WriteString(`<span class="folio-note-meta">` + html.EscapeString(m) + `</span> `)
		}
		w.WriteString(html.EscapeString(note.Text) + `</span>`)
	}
	w.WriteString(`</span>`)
	return ast.WalkContinue, nil
}
