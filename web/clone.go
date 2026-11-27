package web

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/internal/gitrepo"
)

// cloneRepo is what the clone form runs; tests stand a local repository in
// for the network.
var cloneRepo = gitrepo.Clone

// systemProxy is the proxy the system sends web traffic through, which
// git does not use on its own; "" for none or for a system git reads it
// from the environment on. Tests stand one in.
var systemProxy = osProxy

// cloneView is the Open dialog's "From a repository" form: the URL to
// clone, the branch to check out and the folder of the book in it, the
// folder the clone goes into, and the GitHub sign-in that lets it reach a
// private repository and push back to it. An access token for another
// host is a field of the form, and never part of the view. Back is the
// folder the dialog showed, which Back returns to.
type cloneView struct {
	URL, Branch, Path string
	Dir, Back         string
	GitHub            githubView
	Error             string
	Output            string
}

// defaultCloneDir is where a clone goes unless the form says otherwise:
// folio's folder in the user's data folder, out of the way of a user who
// never means to open the clone anywhere but in folio; "" when there is
// no such folder.
func defaultCloneDir() string {
	var base string
	switch runtime.GOOS {
	case "windows":
		base = os.Getenv("LOCALAPPDATA")
	case "darwin", "ios":
		base, _ = os.UserConfigDir() // ~/Library/Application Support
	default:
		if base = os.Getenv("XDG_DATA_HOME"); !filepath.IsAbs(base) {
			if home, err := os.UserHomeDir(); err == nil {
				base = filepath.Join(home, ".local", "share")
			}
		}
	}
	if base == "" {
		if base, _ = os.UserConfigDir(); base == "" {
			return ""
		}
	}
	return filepath.Join(base, configDirName, "repositories")
}

// cloneForm shows the form; Back returns to the folder the dialog showed.
func (s *server) cloneForm(w http.ResponseWriter, r *http.Request) {
	back := strings.TrimSpace(r.URL.Query().Get("dir"))
	if back == "" {
		back, _ = os.Getwd()
	}
	s.render(w, "clone", cloneView{Dir: defaultCloneDir(), Back: back, GitHub: s.githubView()})
}

// cloneHandler clones a repository into a new folder in the one named, and
// opens the book in it: the folder the form names, else the clone's top. A
// book folder with no folio.yml may keep its book further down, so the
// folder dialog shows it to choose from instead. A clone of the same
// repository already in its place is opened rather than cloned again.
//
// The clone runs without the server's lock: it can take minutes, and the
// tree handlers must stay answerable while it does.
func (s *server) cloneHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	v := cloneView{
		URL:    strings.TrimSpace(r.FormValue("url")),
		Branch: strings.TrimSpace(r.FormValue("branch")),
		Path:   strings.Trim(strings.TrimSpace(r.FormValue("path")), `/\`),
		Dir:    strings.TrimSpace(r.FormValue("dir")),
		Back:   strings.TrimSpace(r.FormValue("back")),
		GitHub: s.githubView(),
	}
	fail := func(err error) {
		v.Error = err.Error()
		s.render(w, "clone", v)
	}
	if v.Dir == "" {
		v.Dir = defaultCloneDir()
	}
	sub := filepath.FromSlash(v.Path)
	switch err := gitrepo.ValidRemote(v.URL); {
	case err != nil:
		fail(err)
		return
	case gitrepo.ValidBranch(v.Branch) != nil:
		fail(gitrepo.ValidBranch(v.Branch))
		return
	case sub != "" && !filepath.IsLocal(sub):
		fail(errors.New("name the book's folder by its path inside the repository, such as docs"))
		return
	case !filepath.IsAbs(v.Dir):
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
	if !clonedFrom(dest, v.URL) {
		var prev string
		if token != "" {
			prev = s.setHostToken(origin, token)
		}
		out, err := cloneRepo(v.URL, dest, v.Branch)
		if err != nil {
			if token != "" {
				s.setHostToken(origin, prev)
			}
			v.Output = strings.TrimSpace(out)
			if hint := cmp.Or(signInHint(out, origin, token != ""), networkHint(out, origin)); hint != "" {
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
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	book := filepath.Join(dest, sub)
	if st, err := os.Stat(book); err != nil || !st.IsDir() {
		b := s.browseAt(dest)
		b.Notice = "Cloned into " + dest + ". It has no folder " + v.Path + ": choose the folder that holds the book."
		s.render(w, "browse", b)
		return
	}
	if config.Find(book) == "" {
		b := s.browseAt(book)
		b.Notice = "Cloned into " + dest + ". There is no folio.yml in " + book + ": choose the folder that holds the book."
		s.render(w, "browse", b)
		return
	}
	// The form was posted into the dialog; the project it opens belongs
	// in the main area, as an Open does.
	w.Header().Set("HX-Retarget", "#main")
	w.Header().Set("HX-Reswap", "innerHTML")
	if err := s.openProject(w, book); err != nil {
		w.Header().Del("HX-Retarget")
		w.Header().Del("HX-Reswap")
		fail(err)
	}
}

// clonedFrom reports whether dir is the top of a clone of url. A folder
// that is merely inside one, of another repository perhaps, is not.
func clonedFrom(dir, url string) bool {
	root, err := gitrepo.Root(dir)
	if err != nil {
		return false
	}
	a, errA := filepath.EvalSymlinks(root)
	b, errB := filepath.EvalSymlinks(dir)
	got, err := gitrepo.OriginURL(dir)
	return errA == nil && errB == nil && a == b && err == nil && got == url
}

// networkHint is what a clone that could not reach its remote, or lost it
// on the way, tells the user; "" for any other failure. The usual cause is
// one a browser does not suffer from: a company network's proxy, which the
// browser is set up for and git is not.
func networkHint(out, origin string) string {
	if !gitrepo.NetworkFailed(out) {
		return ""
	}
	host := cmp.Or(strings.TrimSuffix(strings.TrimPrefix(origin, "https://"), "/"), "the repository's host")
	hint := "git could not reach " + host + ", or lost the connection on the way. Where a browser gets through, the network most likely wants a proxy that git is not told about, or a VPN that is not on."
	if p := systemProxy(); p != "" {
		return hint + " This computer sends web traffic through the proxy " + p + ". To have git use it too, run `git config --global http.proxy http://" + p + "` in a terminal and clone again."
	}
	return hint + " If the network has a proxy, run `git config --global http.proxy http://<proxy>:<port>` in a terminal and clone again."
}

// proxyServer reads the proxy for https from a Windows ProxyServer
// setting: either one host:port for every protocol, or a list such as
// "http=p:80;https=p:443", whose https entry wins over its http one.
func proxyServer(setting string) string {
	if !strings.Contains(setting, "=") {
		return strings.TrimSpace(setting)
	}
	var http string
	for e := range strings.SplitSeq(setting, ";") {
		k, v, _ := strings.Cut(strings.TrimSpace(e), "=")
		switch strings.ToLower(k) {
		case "https":
			return v
		case "http":
			http = v
		}
	}
	return http
}
