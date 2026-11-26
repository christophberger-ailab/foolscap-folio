package web

import (
	"net/http"
	"slices"

	"github.com/christophberger-ailab/foolscap-folio/internal/project"
)

// bookmark is one entry of the Bookmarks pane.
type bookmark struct {
	Path  string
	Title string
}

// bookmarksHandler answers with the Bookmarks pane's list: the pages whose
// frontmatter says `bookmarked: true`, in the order of the tree, hidden
// ones included, since a bookmark is a way back to a page the tree may not
// show.
func (s *server) bookmarksHandler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == "" {
		http.Error(w, "no project open", http.StatusBadRequest)
		return
	}
	tree, err := project.Load(s.root)
	if tree == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	flattenTreeSpans(tree.Pages, spanSymbols(s.loadCSS(s.activeCSS())))
	s.render(w, "bookmarks-list", bookmarksIn(tree.Pages, nil))
}

// bookmarksIn appends the bookmarked pages among pages, depth first.
func bookmarksIn(pages []*project.Page, list []bookmark) []bookmark {
	for _, p := range pages {
		if p.Path != "" && slices.Contains(p.Flags, project.BookmarkFlag) {
			list = append(list, bookmark{p.Path, p.Title})
		}
		list = bookmarksIn(p.Children, list)
	}
	return list
}
