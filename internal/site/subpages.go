package site

import (
	"html"
	"strings"

	"github.com/christophberger-ailab/foolscap-folio/internal/markdown"
	"github.com/yuin/goldmark/ast"
)

// subpages lists the pages one level below p: the files beside a folder's
// index page and the folders with an index page of their own, in sidebar
// order. The home page lists the topics.
func (b *builder) subpages(p *pageInfo) []*pageInfo {
	var subs []*pageInfo
	if p.topic == nil {
		for _, t := range b.topics {
			subs = append(subs, t.home)
		}
		return subs
	}
	for _, c := range p.node.Children {
		if sub := p.topic.byNod[c]; sub != nil {
			subs = append(subs, sub)
		}
	}
	return subs
}

// expandSubpages turns every `::: {.subpages title="…"}` block of a page
// into a table of links to its subpages, as a Quarto listing of type
// table with the title field does. The block's own content is dropped,
// and so is the whole block on a page without subpages.
func (b *builder) expandSubpages(doc *markdown.Document, p *pageInfo) {
	var divs []*markdown.Div
	_ = ast.Walk(doc.Root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if d, ok := n.(*markdown.Div); ok && entering && d.Attrs.Has("subpages") {
			divs = append(divs, d)
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	if len(divs) == 0 {
		return
	}
	subs := b.subpages(p)
	for _, d := range divs {
		parent := d.Parent()
		if len(subs) == 0 {
			parent.RemoveChild(parent, d)
			continue
		}
		var s strings.Builder
		s.WriteString("<table class=\"subpages\">\n")
		if title, _ := d.Attrs.Get("title"); title != "" {
			s.WriteString("<thead><tr><th>" + html.EscapeString(title) + "</th></tr></thead>\n")
		}
		s.WriteString("<tbody>\n")
		for _, sub := range subs {
			s.WriteString(`<tr><td><a href="` + html.EscapeString(relURL(p.out, sub.out)) + `">` +
				html.EscapeString(sub.title) + "</a></td></tr>\n")
		}
		s.WriteString("</tbody>\n</table>\n")
		parent.ReplaceChild(parent, d, &markdown.Raw{HTML: s.String()})
	}
}
