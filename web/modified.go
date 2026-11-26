package web

import (
	"slices"

	"github.com/christophberger-ailab/foolscap-folio/internal/gitrepo"
	"github.com/christophberger-ailab/foolscap-folio/internal/project"
)

// changedPages lists the files of the project at root changed since the
// last commit, sorted, by their paths inside the project. A project
// outside a Git working tree, and a machine without git, have none.
func changedPages(root string) []string {
	if root == "" || !gitrepo.Available() {
		return nil
	}
	changed, _ := gitrepo.Changed(root)
	return changed
}

// markModified marks the pages whose files are among changed, which is
// sorted.
func markModified(pages []*project.Page, changed []string) {
	for _, p := range pages {
		_, p.Modified = slices.BinarySearch(changed, p.Path)
		markModified(p.Children, changed)
	}
}
