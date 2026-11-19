package web

import (
	"encoding/json"
	iofs "io/fs"
	"net/http"
	"path"
	"path/filepath"
	"strings"
)

// imagesDir is where the Images pane looks for images, relative to the
// project root: the folder the pages address their media in.
const imagesDir = "assets/images"

// imageEntry is one image the Images pane lists: its path relative to the
// project root, its name relative to imagesDir, and when it was last
// modified, in milliseconds since the epoch, as JavaScript counts time.
type imageEntry struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Modified int64  `json:"modified"`
}

// images lists the images under assets/images for the Images pane. The
// pane sorts them itself, so the order here is the walk's. Hidden and
// draft entries are skipped as everywhere else, and so is anything that is
// not a regular file, a symlink above all.
func (s *server) images(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	dir, err := s.resolvePath(imagesDir)
	s.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	list := []imageEntry{}
	filepath.WalkDir(dir, func(p string, d iofs.DirEntry, err error) error {
		if err != nil {
			return nil // a missing folder lists nothing; unreadable entries drop out
		}
		name := d.Name()
		hidden := p != dir && (strings.HasPrefix(name, "_") || strings.HasPrefix(name, "."))
		if d.IsDir() {
			if hidden {
				return filepath.SkipDir
			}
			return nil
		}
		if hidden || !d.Type().IsRegular() || !mediaExts[strings.ToLower(path.Ext(name))] {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		list = append(list, imageEntry{Path: imagesDir + "/" + rel, Name: rel, Modified: fi.ModTime().UnixMilli()})
		return nil
	})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(struct {
		Images []imageEntry `json:"images"`
	}{list})
}
