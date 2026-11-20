package markdown

import (
	"fmt"
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
		x.h.InsertBefore(x.h, x.h.FirstChild(), ast.NewString([]byte(strings.Join(num, ".")+" ")))
	}
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
