package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/yuin/goldmark/ast"

	"github.com/christophberger-ailab/foolscap-folio/internal/bookmaker"
	"github.com/christophberger-ailab/foolscap-folio/internal/build"
	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/internal/markdown"
)

// xref is what the preview shows of a reference's target, and where a
// click on it leads: to the heading or figure with Key on its page, the
// page itself or, for a target on another page, the file Path.
type xref struct {
	Name   string `json:"name"`
	Number string `json:"number,omitempty"`
	Key    string `json:"key"`
	Path   string `json:"path,omitempty"`
}

// mark is what the preview puts on one of the page's headings or figures:
// the key a reference names it by, and its number, if it has one.
type mark struct {
	Key    string `json:"key"`
	Number string `json:"number,omitempty"`
}

// xrefsView answers /xrefs: the targets of the references, by the key
// they are written with, and the page's headings and figures, in order,
// for the preview to number and to scroll to.
type xrefsView struct {
	Refs     map[string]xref `json:"refs"`
	Headings []mark          `json:"headings"`
	Figures  []mark          `json:"figures"`
}

// xrefs resolves the cross-references of the text the editor holds for
// the page at path, as a website does: against the text's own headings
// and figures and those of the project's other pages, numbered as the
// first format that numbers anything numbers them (issue #161). It
// numbers the page's headings and figures as that format does (#178).
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

	// The page's own targets come from its text, their numbers from the
	// book: a heading's by key, a figure's by its place on the page. The
	// other pages' targets come from the project. The book is only built
	// when it numbers something or a reference needs it.
	numbers := map[string]string{}
	var labels []string
	var others []markdown.Target
	if f.NumberedHeadings || f.NumberedFigures || strings.Contains(body, "](#") {
		for _, t := range build.ProjectTargets(cfg, f) {
			switch {
			case t.Page != page:
				others = append(others, t)
			case t.Figure && t.Number != "":
				labels = append(labels, t.Number)
			case !t.Figure:
				numbers[t.Key] = t.Number
			}
		}
	}
	figures := map[ast.Node]int{}
	for i, img := range markdown.Figures(doc) {
		figures[img] = i
	}
	v := xrefsView{Refs: map[string]xref{}, Headings: []mark{}, Figures: []mark{}}
	for i, t := range local {
		if !t.Figure {
			local[i].Number = numbers[t.Key]
			v.Headings = append(v.Headings, mark{t.Key, local[i].Number})
			continue
		}
		if n, ok := figures[t.Node]; ok {
			if n < len(labels) {
				local[i].Number = labels[n]
			}
			v.Figures = append(v.Figures, mark{t.Key, local[i].Number})
		}
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
			x := xref{Name: t.Name, Number: t.Number, Key: t.Key}
			if t.Page != page {
				x.Path = s.pageFile(t.Page)
			}
			v.Refs[key] = x
		}
		return ast.WalkContinue, nil
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// pageFile is the file of the page the targets name p (see pageKey), ""
// if there is none.
func (s *server) pageFile(p string) string {
	for _, name := range []string{p + ".qmd", p + ".md", path.Join(p, "index.qmd"), path.Join(p, "index.md")} {
		if abs, err := s.resolvePath(name); err == nil {
			if fi, err := os.Stat(abs); err == nil && fi.Mode().IsRegular() {
				return name
			}
		}
	}
	return ""
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
