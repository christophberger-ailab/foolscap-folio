package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
)

// browseView is the folder dialog the Open button shows: one folder, its
// subfolders, and the projects opened before.
type browseView struct {
	// Dir is the folder shown, absolute.
	Dir string
	// Parent is the folder above Dir, "" at the top of the file system.
	Parent  string
	Entries []browseEntry
	Recent  []recentPath
	// Error says why Dir could not be listed.
	Error string
}

// browseEntry is one subfolder in the dialog.
type browseEntry struct {
	Name, Path string
	// Folio marks a folder that has a folio.yml (or a legacy qm.yml).
	Folio bool
	// Selected marks the folio opened last, so Open opens it again.
	Selected bool
}

// browse renders the folder dialog for the folder named by ?dir=. Without
// one it shows where the last project was opened from, with that project
// selected, and the working directory when none was opened yet.
func (s *server) browse(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := strings.TrimSpace(r.URL.Query().Get("dir"))
	last := s.root
	if last == "" && len(s.cfg.Recent) > 0 {
		last = s.cfg.Recent[0]
	}
	if dir == "" {
		dir = filepath.Dir(last)
		if fi, err := os.Stat(dir); last == "" || err != nil || !fi.IsDir() {
			dir, _ = os.Getwd()
		}
	}
	v := browseView{Recent: s.recentPaths()}
	var err error
	if v.Dir, err = filepath.Abs(dir); err != nil {
		v.Error = err.Error()
	} else {
		v.Entries, err = subfolders(v.Dir, last)
		if err != nil {
			v.Error = err.Error()
		}
	}
	if p := filepath.Dir(v.Dir); p != v.Dir {
		v.Parent = p
	}
	s.render(w, "browse", v)
}

// subfolders lists the folders in dir that are not hidden, in name order,
// selecting the one at the path selected.
func subfolders(dir, selected string) ([]browseEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []browseEntry
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		// Stat rather than e.IsDir, so a link to a folder counts as one.
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			continue
		}
		out = append(out, browseEntry{Name: e.Name(), Path: p, Folio: config.Find(p) != "", Selected: p == selected})
	}
	return out, nil
}
