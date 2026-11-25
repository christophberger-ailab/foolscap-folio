package web

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/internal/gitrepo"
)

// cloneRepo is what the clone form runs; tests stand a local repository in
// for the network.
var cloneRepo = gitrepo.Clone

// cloneView is the Open dialog's "From a repository" form: the URL to
// clone, the folder the clone goes into, and the GitHub sign-in that lets
// it reach a private repository and push back to it. An access token for
// another host is a field of the form, and never part of the view.
type cloneView struct {
	URL    string
	Dir    string
	GitHub githubView
	Error  string
	Output string
}

// cloneForm shows the form, cloning into the folder the dialog showed.
func (s *server) cloneForm(w http.ResponseWriter, r *http.Request) {
	dir := strings.TrimSpace(r.URL.Query().Get("dir"))
	if dir == "" {
		dir, _ = os.Getwd()
	}
	s.render(w, "clone", cloneView{Dir: dir, GitHub: s.githubView()})
}

// cloneHandler clones a repository into a new folder in the one named, and
// opens it. A clone with no folio.yml at its top may keep its book in a
// subfolder, so the folder dialog shows it to choose from instead.
//
// The clone runs without the server's lock: it can take minutes, and the
// tree handlers must stay answerable while it does.
func (s *server) cloneHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	v := cloneView{
		URL:    strings.TrimSpace(r.FormValue("url")),
		Dir:    strings.TrimSpace(r.FormValue("dir")),
		GitHub: s.githubView(),
	}
	fail := func(err error) {
		v.Error = err.Error()
		s.render(w, "clone", v)
	}
	if err := gitrepo.ValidRemote(v.URL); err != nil {
		fail(err)
		return
	}
	if !filepath.IsAbs(v.Dir) {
		fail(errors.New("name the folder to clone into by its full path"))
		return
	}
	name := gitrepo.RepoName(v.URL)
	if name == "" {
		fail(errors.New("the URL names no repository folder to clone into"))
		return
	}
	// The token is never put back into the form: a page is no place for it.
	token := strings.TrimSpace(r.FormValue("token"))
	origin := gitrepo.Origin(v.URL)
	if token != "" {
		if origin == "" {
			fail(errors.New("an access token is sent only over https://: use the repository's https:// address"))
			return
		}
		if origin == gitrepo.GitHubOrigin && v.GitHub.Login != "" {
			fail(errors.New("you are signed in to GitHub, and git uses that sign-in there: leave the token out"))
			return
		}
	}
	dest := filepath.Join(filepath.Clean(v.Dir), name)
	var prev string
	if token != "" {
		prev = s.setHostToken(origin, token)
	}
	out, err := cloneRepo(v.URL, dest)
	if err != nil {
		if token != "" {
			s.setHostToken(origin, prev)
		}
		v.Output = strings.TrimSpace(out)
		if hint := signInHint(out, origin, token != ""); hint != "" {
			err = errors.New(hint)
		}
		fail(err)
		return
	}
	if token != "" {
		if err := s.saveHostTokens(); err != nil {
			fmt.Fprintf(os.Stderr, "folio web: the access token for %s is not kept for the next start: %v\n", origin, err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if config.Find(dest) == "" {
		b := s.browseAt(dest)
		b.Notice = "Cloned into " + dest + ". There is no folio.yml at its top: choose the folder that holds the book."
		s.render(w, "browse", b)
		return
	}
	// The form was posted into the dialog; the project it opens belongs
	// in the main area, as an Open does.
	w.Header().Set("HX-Retarget", "#main")
	w.Header().Set("HX-Reswap", "innerHTML")
	if err := s.openProject(w, dest); err != nil {
		w.Header().Del("HX-Retarget")
		w.Header().Del("HX-Reswap")
		fail(err)
	}
}
