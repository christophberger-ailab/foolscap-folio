package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/yuin/goldmark/ast"

	"github.com/christophberger-ailab/foolscap-folio/internal/bookmaker"
	"github.com/christophberger-ailab/foolscap-folio/internal/build"
	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/internal/markdown"
)

// xref is what the preview shows of a reference's target. ID is the
// identifier of a heading of the page itself, which the reference
// scrolls to; "" for a target on another page.
type xref struct {
	Name   string `json:"name"`
	Number string `json:"number,omitempty"`
	ID     string `json:"id,omitempty"`
}

// xrefsView answers /xrefs: the targets of the references, by the key
// they are written with, and the identifiers of the page's headings, in
// order, for the preview to put on its headings.
type xrefsView struct {
	Refs     map[string]xref `json:"refs"`
	Headings []string        `json:"headings"`
}

// xrefs resolves the cross-references of the text the editor holds for
// the page at path, as a website does: against the text's own headings
// and figures and those of the project's other pages, numbered as the
// first format that numbers anything numbers them (issue #161).
func (s *server) xrefs(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rel := r.FormValue("path")
	if _, err := s.resolvePath(rel); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := config.Load(s.root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var f config.Format
	for _, e := range cfg.Formats {
		if e.Value.NumberedHeadings || e.Value.NumberedFigures {
			f = e.Value
			break
		}
	}
	fm, body, _ := bookmaker.ParseFrontMatter(r.FormValue("text"))
	page := pageKey(rel)
	doc := markdown.Parse([]byte(body))
	markdown.AssignIDs(doc)
	local := markdown.Targets(doc, page, fm.PageID(), nil)

	// The page's own targets come from its text, the numbers of its
	// headings from the book, by key; the other pages' from the project.
	numbers := map[string]string{}
	var others []markdown.Target
	for _, t := range build.ProjectTargets(cfg, f) {
		if t.Page == page {
			numbers[t.Key] = t.Number
		} else {
			others = append(others, t)
		}
	}
	v := xrefsView{Refs: map[string]xref{}, Headings: []string{}}
	for i, t := range local {
		if t.Figure {
			continue
		}
		local[i].Number = numbers[t.Key]
		v.Headings = append(v.Headings, t.ID)
	}
	idx := markdown.NewIndex(local, others)
	_ = ast.Walk(doc.Root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		l, ok := n.(*ast.Link)
		if !entering || !ok || len(l.Destination) < 2 || l.Destination[0] != '#' {
			return ast.WalkContinue, nil
		}
		key := string(l.Destination[1:])
		if u, err := url.PathUnescape(key); err == nil {
			key = u
		}
		if t, n := idx.Lookup(key, page); n > 0 {
			x := xref{Name: t.Name, Number: t.Number}
			if t.Page == page && !t.Figure {
				x.ID = t.ID
			}
			v.Refs[key] = x
		}
		return ast.WalkContinue, nil
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// pageKey is the name the targets give the page at rel: its path without
// the extension, a folder's index page by the folder's path.
func pageKey(rel string) string {
	p := strings.TrimSuffix(rel, path.Ext(rel))
	if path.Base(p) == "index" {
		p = strings.TrimSuffix(path.Dir(p), ".")
	}
	return p
}
