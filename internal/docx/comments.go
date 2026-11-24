package docx

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/christophberger-ailab/foolscap-folio/internal/markdown"
)

// A draft's comments are Word comments (spec-render.yaml DRAFT), its
// todos highlighted runs; a todo in an HTML comment carries that comment
// as a Word comment of its own.

const relComments = relBase + "comments"

var reCommentID = regexp.MustCompile(`<w:comment\s[^>]*?w:id="(\d+)"`)

// comment writes a commented passage: the start of its range, the
// passage, and, unless it goes on in a later block, the range's end and
// the reference to the comment.
func (w *writer) comment(buf *bytes.Buffer, n *markdown.Comment, rp runProps, c ctx) {
	if c.note {
		w.inlines(buf, n, rp, c) // footnotes and footers take no comments
		return
	}
	if len(n.Thread) > 0 {
		w.commentOpen = w.newComment(n.Thread)
		fmt.Fprintf(buf, `<w:commentRangeStart w:id="%d"/>`, w.commentOpen)
	}
	w.inlines(buf, n, rp, c)
	if !n.Open && w.commentOpen >= 0 {
		w.commentEnd(buf, w.commentOpen)
		w.commentOpen = -1
	}
}

// todo writes a todo as a highlighted TODO, with its HTML comment as a
// Word comment for a hidden one.
func (w *writer) todo(buf *bytes.Buffer, n *markdown.Todo, rp runProps, c ctx) {
	rp.bold, rp.highlight = true, "yellow"
	if !n.Hidden || c.note {
		buf.WriteString(w.run(rp, "TODO"))
		return
	}
	id := w.newComment([]markdown.Note{{Author: "TODO", Text: n.Note}})
	fmt.Fprintf(buf, `<w:commentRangeStart w:id="%d"/>%s`, id, w.run(rp, "TODO"))
	w.commentEnd(buf, id)
}

func (w *writer) commentEnd(buf *bytes.Buffer, id int) {
	style := esc(w.styles.need("character", "CommentReference", `<w:rPr><w:sz w:val="16"/><w:szCs w:val="16"/></w:rPr>`,
		"annotation reference"))
	fmt.Fprintf(buf, `<w:commentRangeEnd w:id="%d"/><w:r><w:rPr><w:rStyle w:val="%s"/></w:rPr><w:commentReference w:id="%d"/></w:r>`, id, style, id)
}

// newComment adds a comment with its thread, the replies as paragraphs
// below the comment, and returns its ID.
func (w *writer) newComment(thread []markdown.Note) int {
	id := w.commentBase + len(w.commentXML)
	first := thread[0]
	author := cmpOr(first.Author, "folio")
	attrs := fmt.Sprintf(`w:id="%d" w:author="%s"`, id, esc(author))
	if d, err := time.Parse(time.DateOnly, first.Date); err == nil {
		attrs += ` w:date="` + d.Format(time.RFC3339) + `"`
	}
	attrs += ` w:initials="` + esc(initials(author)) + `"`
	var b strings.Builder
	for i, note := range thread {
		text := note.Text
		if i > 0 {
			text = cmpOr(note.Meta(), "↳") + ": " + text
		}
		b.WriteString(`<w:p>`)
		if i == 0 {
			b.WriteString(`<w:r><w:annotationRef/></w:r>`)
		}
		b.WriteString(w.run(runProps{}, text) + `</w:p>`)
	}
	w.commentXML = append(w.commentXML, `<w:comment `+attrs+`>`+b.String()+`</w:comment>`)
	return id
}

// initials are the first letters of the words of a name.
func initials(name string) string {
	var b strings.Builder
	for _, f := range strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) }) {
		b.WriteRune([]rune(f)[0])
	}
	return b.String()
}

// commentsPart writes the comments into the package, after a template's
// own.
func (w *writer) commentsPart(p *pkg) []relationship {
	if len(w.commentXML) == 0 {
		return nil
	}
	name := relTarget(p, relComments)
	if x := p.get(name); name != "" && x != nil {
		p.set(name, []byte(insertBefore(string(x), "</w:comments>", strings.Join(w.commentXML, ""))))
		return nil
	}
	name = "word/comments.xml"
	p.set(name, []byte(xmlDecl+`<w:comments xmlns:w="`+nsW+`" xmlns:r="`+nsR+`">`+strings.Join(w.commentXML, "")+`</w:comments>`))
	ensureContentType(p, "/"+name, "application/vnd.openxmlformats-officedocument.wordprocessingml.comments+xml", true)
	return []relationship{{"rIdQmComments", relComments, "comments.xml"}}
}
