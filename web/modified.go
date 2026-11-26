package web

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
	changed, _ := gitChanged(root)
	return changed
}

// gitChanged reads the changed files; tests count the calls.
var gitChanged = gitrepo.Changed

// changedFiles is changedPages for the open project, read from git only
// when something it depends on may have moved: the pages, which s.fp
// fingerprints, or the repository's state, which repoStamp does. A poll
// of /watch that finds neither moved thus costs a few stat calls instead
// of two git processes. The caller must hold s.mu.
func (s *server) changedFiles() []string {
	if s.gitDirRoot != s.root {
		s.gitDirRoot, s.gitDir = s.root, gitDir(s.root)
	}
	key := s.root + "|" + s.fp + "|" + repoStamp(s.root, s.gitDir)
	if key == s.statusKey {
		return s.changed
	}
	if s.gitDir == "" {
		// Something that may be a new repository turned up above the
		// project; the stamp to keep from now on is that repository's.
		if s.gitDir = gitDir(s.root); s.gitDir != "" {
			key = s.root + "|" + s.fp + "|" + repoStamp(s.root, s.gitDir)
		}
	}
	s.statusKey, s.changed = key, changedPages(s.root)
	return s.changed
}

// gitDir is the git directory of the repository root is in, or "".
func gitDir(root string) string {
	if root == "" || !gitrepo.Available() {
		return ""
	}
	dir, _ := gitrepo.Dir(root)
	return dir
}

// repoStamp fingerprints the files git rewrites whenever the status can
// change without a page being touched: the index (staging, committing,
// checking out, resetting), HEAD (switching branches) and HEAD's log
// (every commit, also one made in a terminal). Outside a repository it
// notes which of root and the directories above it hold a .git, so that
// a `git init` is noticed as well.
func repoStamp(root, gitDir string) string {
	var b strings.Builder
	if gitDir == "" {
		for dir := root; ; dir = filepath.Dir(dir) {
			if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
				b.WriteString(dir + ";")
			}
			if filepath.Dir(dir) == dir {
				return b.String()
			}
		}
	}
	for _, name := range []string{"index", "HEAD", filepath.Join("logs", "HEAD")} {
		if fi, err := os.Stat(filepath.Join(gitDir, name)); err == nil {
			fmt.Fprintf(&b, "%d.%d;", fi.Size(), fi.ModTime().UnixNano())
		}
	}
	return b.String()
}

// markModified marks the pages whose files are among changed, which is
// sorted.
func markModified(pages []*project.Page, changed []string) {
	for _, p := range pages {
		_, p.Modified = slices.BinarySearch(changed, p.Path)
		markModified(p.Children, changed)
	}
}
