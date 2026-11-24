package docx

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/markdown"
)

// A draft's comments are Word comments on their passages, replies
// included; its todos are highlighted.
func TestDraftComments(t *testing.T) {
	d := markdown.Parse([]byte(`Say <!-- << {author="Ann" date="2026-11-01"} Check <it> --><!-- <<+ {author="Bob"} Done -->this<!-- >> --> and ==TODO== <!-- ==TODO== later -->.` + "\n"))
	d.Prepare(markdown.Options{Draft: true})
	path := filepath.Join(t.TempDir(), "out.docx")
	if err := Write(path, d, Options{}); err != nil {
		t.Fatal(err)
	}
	parts := readParts(t, path)
	has(t, parts["word/document.xml"],
		`<w:commentRangeStart w:id="0"/>`, `this</w:t></w:r><w:commentRangeEnd w:id="0"/><w:r><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:commentReference w:id="0"/></w:r>`,
		`<w:highlight w:val="yellow"/></w:rPr><w:t xml:space="preserve">TODO</w:t>`,
		`<w:commentReference w:id="1"/>`)
	has(t, parts["word/comments.xml"],
		`<w:comment w:id="0" w:author="Ann" w:date="2026-11-01T00:00:00Z" w:initials="A">`,
		`Check &lt;it&gt;`, `Bob: Done`,
		`<w:comment w:id="1" w:author="TODO" w:initials="T">`, `==TODO== later`)
	has(t, parts["[Content_Types].xml"], `PartName="/word/comments.xml"`)
	has(t, parts["word/_rels/document.xml.rels"], `Target="comments.xml"`)
}

// Without a draft there are no comments to write.
func TestNoComments(t *testing.T) {
	parts := build(t, "Say <!-- << c -->this<!-- >> -->.\n", Options{})
	if _, ok := parts["word/comments.xml"]; ok {
		t.Error("comments part written without a draft")
	}
}

// A passage across paragraphs is one comment's range, which ends where
// the passage does.
func TestDraftCommentAcrossParagraphs(t *testing.T) {
	d := markdown.Parse([]byte("<!-- << Long -->First\n\nMiddle\n\n<!-- >> -->Last\n"))
	d.Prepare(markdown.Options{Draft: true})
	path := filepath.Join(t.TempDir(), "out.docx")
	if err := Write(path, d, Options{}); err != nil {
		t.Fatal(err)
	}
	doc := readParts(t, path)["word/document.xml"]
	start, mid, end := strings.Index(doc, `<w:commentRangeStart w:id="0"/>`), strings.Index(doc, "Middle"), strings.Index(doc, `<w:commentRangeEnd w:id="0"/>`)
	if start < 0 || end < 0 || !(start < mid && mid < end) || end > strings.Index(doc, "Last") {
		t.Errorf("range does not span the passage:\n%s", doc)
	}
	if strings.Count(doc, "commentRangeEnd") != 1 {
		t.Errorf("range ended more than once:\n%s", doc)
	}
}
