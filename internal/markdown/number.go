package markdown

import (
	"fmt"
	"slices"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// NumberHeadings puts numbers before the headings of a book, as "1.2 ":
// its chapters are numbered through the book, as the chapter prefix counts
// them, and their headings below them. The preliminaries before the
// contents marker, the sections' pages and the headings with the class
// `unnumbered` get no number. A book without chapters numbers its
// sections instead. A title whose prefix numbers it already, as
// sectionPrefixed and chapterPrefixed say, keeps it alone.
func NumberHeadings(d *Document, sectionPrefixed, chapterPrefixed bool) {
	type heading struct {
		h       *ast.Heading
		section bool
	}
	var hs []heading
	chapters := false
	marker := TOCMarker(d.Root)
	listing := marker == nil
	_ = ast.Walk(d.Root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if n == marker {
			listing = true
		}
		if h, ok := n.(*ast.Heading); ok {
			if listing {
				s := InSection(h)
				chapters = chapters || h.Level == 1 && !s
				hs = append(hs, heading{h, s})
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	prefixed := chapterPrefixed
	if !chapters {
		prefixed = sectionPrefixed
	}
	var count [6]int
	for _, x := range hs {
		if x.section && chapters || AttrsOf(x.h).Has("unnumbered") {
			continue
		}
		level := min(max(x.h.Level, 1), 6)
		count[level-1]++
		clear(count[level:])
		if level == 1 && prefixed {
			continue
		}
		num := make([]string, level)
		for i, c := range count[:level] {
			num[i] = fmt.Sprint(c)
		}
		SetHeadingNumber(x.h, strings.Join(num, "."))
	}
}

var numberKey = []byte("folio-number")

// SetHeadingNumber puts num, as "1.2", before a heading's text.
func SetHeadingNumber(h *ast.Heading, num string) {
	h.InsertBefore(h, h.FirstChild(), ast.NewString([]byte(num+" ")))
	h.SetAttribute(numberKey, num)
}

// HeadingNumber is the number SetHeadingNumber put before a heading, or
// the prefix that numbers a section's or chapter's title ("Chapter 3"),
// "" for neither.
func HeadingNumber(h *ast.Heading) string {
	if p, ok := AttrsOf(h).Get(PrefixAttr); ok {
		return strings.TrimRight(strings.TrimSpace(p), ":")
	}
	if v, ok := h.Attribute(numberKey); ok {
		return v.(string)
	}
	return ""
}

// HeadingPrefix is the prefix before a section's or chapter's title,
// "Chapter 3: ", "" for none.
func HeadingPrefix(h *ast.Heading) string {
	p, _ := AttrsOf(h).Get(PrefixAttr)
	return p
}

// SetHeadingPrefix puts prefix, as "Chapter 3: ", before a heading's text,
// as the flattener puts it before a chapter's title.
func SetHeadingPrefix(h *ast.Heading, prefix string) {
	h.InsertBefore(h, h.FirstChild(), ast.NewString([]byte(prefix)))
	a := AttrsOf(h)
	a.KV = append(slices.Clone(a.KV), [2]string{PrefixAttr, prefix})
	setAttrs(h, a)
}

// HeadingName is a heading's text without its number and prefix.
func HeadingName(h *ast.Heading, src []byte) string {
	s := strings.TrimSpace(PlainText(h, src))
	if v, ok := h.Attribute(numberKey); ok {
		s = strings.TrimPrefix(s, v.(string)+" ")
	}
	if p, ok := AttrsOf(h).Get(PrefixAttr); ok {
		s = strings.TrimPrefix(s, p)
	}
	return strings.TrimSpace(s)
}

// PrefixAttr is the heading attribute that holds the prefix the
// flattener put before a section's or chapter's title, "Chapter 3: ".
const PrefixAttr = "folio-prefix"

// NumberFigures labels the figures of a document "<label> 1", "<label> 2"
// and so on through the document (see FigureLabel); the caption shows the
// label before the alt text.
func NumberFigures(d *Document, label string) {
	for i, img := range Figures(d) {
		SetFigureLabel(img, fmt.Sprintf("%s %d", label, i+1))
	}
}

// Figures lists the images of a document that have a caption, in document
// order, leaving out those in table cells and footnotes, which show none.
func Figures(d *Document) []*ast.Image {
	var out []*ast.Image
	_ = ast.Walk(d.Root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.Kind().String() {
		case "TableCell", "Footnote":
			return ast.WalkSkipChildren, nil
		}
		if _, ok := FigureCaption(n, d.Source); ok {
			out = append(out, figureImage(n, d.Source))
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return out
}

var figureKey = []byte("folio-figure")

// SetFigureLabel gives a figure's image its label, as "Figure 3".
func SetFigureLabel(img *ast.Image, label string) { img.SetAttribute(figureKey, label) }

// FigureLabel is the label SetFigureLabel gave an image, "" for none.
func FigureLabel(img ast.Node) string {
	if v, ok := img.Attribute(figureKey); ok {
		return v.(string)
	}
	return ""
}

// TOCMarker returns the div that marks where a book's contents go, or nil.
func TOCMarker(root ast.Node) ast.Node {
	var marker ast.Node
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if d, ok := n.(*Div); ok && entering && d.Attrs.Has(TOCClass) {
			marker = d
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return marker
}

// InSection reports whether n is part of a section's own page.
func InSection(n ast.Node) bool {
	for p := n.Parent(); p != nil; p = p.Parent() {
		if d, ok := p.(*Div); ok && d.Attrs.Has(SectionClass) {
			return true
		}
	}
	return false
}
