package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/gitrepo"
)

// fakeGitHub stands in for github.com and api.github.com: it hands out a
// device code, answers "pending" to the first poll and a token to the
// next, and says whose the token is.
func fakeGitHub(t *testing.T) *atomic.Int32 {
	t.Helper()
	var polls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/device/code", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.FormValue("client_id") != "cid" || r.FormValue("scope") != "repo" {
			t.Errorf("device code asked for with %v", r.Form)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"device_code": "dev", "user_code": "ABCD-1234",
			"verification_uri": "https://github.com/login/device", "interval": 5, "expires_in": 900,
		})
	})
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.FormValue("device_code") != "dev" || r.FormValue("client_id") != "cid" {
			t.Errorf("token asked for with %v", r.Form)
		}
		if polls.Add(1) == 1 {
			json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"access_token": "gho_tok", "token_type": "bearer"})
	})
	mux.HandleFunc("GET /user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer gho_tok" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"login": "octocat"})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	prevBase, prevAPI, prevID, prevUnit := githubBase, githubAPI, githubClientID, pollUnit
	githubBase, githubAPI, githubClientID, pollUnit = ts.URL, ts.URL, "cid", 0
	t.Setenv("FOLIO_GITHUB_CLIENT_ID", "")
	t.Cleanup(func() {
		githubBase, githubAPI, githubClientID, pollUnit = prevBase, prevAPI, prevID, prevUnit
		gitrepo.SetGitHubToken("")
	})
	return &polls
}

func TestGitHubSignInNeedsAClientID(t *testing.T) {
	prev := githubClientID
	githubClientID = ""
	t.Cleanup(func() { githubClientID = prev })
	t.Setenv("FOLIO_GITHUB_CLIENT_ID", "")
	srv := bareServer(t)
	body := get(t, srv, "/clone").Body.String()
	if strings.Contains(body, "/github/login") {
		t.Errorf("sign-in offered without a client ID:\n%s", body)
	}
	if rec := post(t, srv, "/github/login", nil); !strings.Contains(rec.Body.String(), "not set up") {
		t.Errorf("login without a client ID: %s", rec.Body)
	}
}

// The device flow end to end: the code is shown, a poll before the user
// approved keeps waiting, the next one signs in, and the token is kept for
// the next start, readable by its owner alone.
func TestGitHubDeviceFlow(t *testing.T) {
	fakeGitHub(t)
	dir := t.TempDir()
	srv, err := newServer(filepath.Join(dir, "settings.cue"))
	if err != nil {
		t.Fatal(err)
	}

	body := get(t, srv, "/clone").Body.String()
	if !strings.Contains(body, `hx-post="/github/login"`) {
		t.Fatalf("no sign-in offered:\n%s", body)
	}

	body = post(t, srv, "/github/login", nil).Body.String()
	for _, want := range []string{"ABCD-1234", "https://github.com/login/device", `hx-post="/github/poll"`} {
		if !strings.Contains(body, want) {
			t.Errorf("device step lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "dev\"") || strings.Contains(body, ">dev<") {
		t.Errorf("device code leaked into the page:\n%s", body)
	}

	body = post(t, srv, "/github/poll", nil).Body.String()
	if !strings.Contains(body, `hx-post="/github/poll"`) {
		t.Errorf("pending poll stopped polling:\n%s", body)
	}
	body = post(t, srv, "/github/poll", nil).Body.String()
	if !strings.Contains(body, "octocat") || strings.Contains(body, `hx-post="/github/poll"`) {
		t.Errorf("approved poll did not sign in:\n%s", body)
	}
	if strings.Contains(body, "gho_tok") {
		t.Errorf("token shown in the page:\n%s", body)
	}

	file := filepath.Join(dir, githubTokenFile)
	fi, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		t.Errorf("token file mode %v, want owner only", fi.Mode().Perm())
	}

	// The next start is still signed in.
	srv2, err := newServer(filepath.Join(dir, "settings.cue"))
	if err != nil {
		t.Fatal(err)
	}
	if body := get(t, srv2, "/clone").Body.String(); !strings.Contains(body, "octocat") {
		t.Errorf("sign-in not kept:\n%s", body)
	}

	// Signing out forgets the token on disk and in git.
	body = post(t, srv2, "/github/logout", nil).Body.String()
	if strings.Contains(body, "octocat") {
		t.Errorf("still signed in after sign-out:\n%s", body)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("token file kept after sign-out: %v", err)
	}
}

func TestGitHubPollWithoutLogin(t *testing.T) {
	fakeGitHub(t)
	srv := bareServer(t)
	body := post(t, srv, "/github/poll", nil).Body.String()
	if strings.Contains(body, `hx-post="/github/poll"`) {
		t.Errorf("polls with no sign-in under way:\n%s", body)
	}
}

// The clone form clones into folio's own data folder unless told
// otherwise; Back returns to the folder the Open dialog showed.
func TestCloneForm(t *testing.T) {
	data := dataHome(t)
	srv := bareServer(t)
	dir := t.TempDir()
	body := get(t, srv, "/clone?dir="+url.QueryEscape(dir)).Body.String()
	for _, want := range []string{`name="url"`, `name="dir"`, `name="branch"`, `name="path"`,
		`value="` + filepath.Join(data, "folio", "repositories") + `"`, `data-path="` + dir + `"`} {
		if !strings.Contains(body, want) {
			t.Errorf("clone form lacks %q:\n%s", want, body)
		}
	}
}

// The Open dialog offers the way to a repository.
func TestBrowseOffersClone(t *testing.T) {
	srv := bareServer(t)
	if body := get(t, srv, "/browse").Body.String(); !strings.Contains(body, `id="browse-clone"`) {
		t.Errorf("no clone button in the Open dialog:\n%s", body)
	}
}

func TestCloneRefusesBadInput(t *testing.T) {
	srv := bareServer(t)
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "book"), 0o755)
	for name, form := range map[string]url.Values{
		"local path":    {"url": {"/etc"}, "dir": {dir}},
		"option":        {"url": {"--upload-pack=x"}, "dir": {dir}},
		"bad branch":    {"url": {"https://github.com/o/x"}, "dir": {dir}, "branch": {"-x"}},
		"path up":       {"url": {"https://github.com/o/x"}, "dir": {dir}, "path": {"../x"}},
		"path absolute": {"url": {"https://github.com/o/x"}, "dir": {dir}, "path": {dir}},
		"relative":      {"url": {"https://github.com/o/book"}, "dir": {"rel"}},
		"already there": {"url": {"https://github.com/o/book"}, "dir": {dir}},
	} {
		t.Run(name, func(t *testing.T) {
			rec := post(t, srv, "/clone", form)
			if !strings.Contains(rec.Body.String(), `class="browse-error"`) {
				t.Errorf("not refused: %d %s", rec.Code, rec.Body)
			}
			if rec.Header().Get("HX-Retarget") != "" {
				t.Errorf("refusal retargeted to %q", rec.Header().Get("HX-Retarget"))
			}
		})
	}
}

// A clone with a folio.yml at its top is opened at once; the open dialog
// is not the target any more, the main area is.
func TestCloneOpensTheFolio(t *testing.T) {
	remote := bareRemote(t, map[string]string{"folio.yml": testConfig, "index.qmd": "---\ntitle: Home\n---\n# Home\n"})
	srv := bareServer(t)
	dir := t.TempDir()
	rec := post(t, srv, "/clone", url.Values{"url": {"https://github.com/o/book.git"}, "dir": {dir}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if got := remote.asked; got != "https://github.com/o/book.git" {
		t.Errorf("cloned %q", got)
	}
	if rec.Header().Get("HX-Retarget") != "#main" {
		t.Errorf("retarget = %q, want #main", rec.Header().Get("HX-Retarget"))
	}
	if srv.gitRoot() != filepath.Join(dir, "book") {
		t.Errorf("open project = %q", srv.gitRoot())
	}
	if !strings.Contains(rec.Body.String(), "Home") {
		t.Errorf("project not rendered:\n%s", rec.Body)
	}
}

// A clone without a folio.yml at its top may keep its book further down,
// so the folder dialog shows the clone to choose from.
func TestCloneWithoutAFolioBrowsesIt(t *testing.T) {
	bareRemote(t, map[string]string{"docs/folio.yml": testConfig})
	srv := bareServer(t)
	dir := t.TempDir()
	rec := post(t, srv, "/clone", url.Values{"url": {"git@github.com:o/repo.git"}, "dir": {dir}})
	if rec.Header().Get("HX-Retarget") != "" {
		t.Errorf("retargeted to %q", rec.Header().Get("HX-Retarget"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="browse"`) || !strings.Contains(body, filepath.Join(dir, "repo")) {
		t.Errorf("clone not shown in the folder dialog:\n%s", body)
	}
}

// A clone named with no folder goes into folio's data folder, and a
// second clone of the same repository opens the one already there.
func TestCloneIntoTheDataFolder(t *testing.T) {
	data := dataHome(t)
	bareRemote(t, map[string]string{"folio.yml": testConfig, "index.qmd": "# Home\n"})
	srv := bareServer(t)
	form := url.Values{"url": {"https://github.com/o/book.git"}}
	for range 2 {
		rec := post(t, srv, "/clone", form)
		if rec.Header().Get("HX-Retarget") != "#main" {
			t.Fatalf("not opened: %s", rec.Body)
		}
	}
	if want := filepath.Join(data, "folio", "repositories", "book"); srv.gitRoot() != want {
		t.Errorf("open project = %q, want %q", srv.gitRoot(), want)
	}
	// Nor is a plain folder inside a clone of the repository.
	nested := filepath.Join(srv.gitRoot(), "sub")
	os.MkdirAll(filepath.Join(nested, "book"), 0o755)
	if clonedFrom(filepath.Join(nested, "book"), "https://github.com/o/book.git") {
		t.Error("a folder inside the clone taken for a clone")
	}
	// Another repository of the same name is not taken for it.
	rec := post(t, srv, "/clone", url.Values{"url": {"https://example.com/other/book.git"}})
	if !strings.Contains(rec.Body.String(), `class="browse-error"`) {
		t.Errorf("another repository's clone opened: %s", rec.Body)
	}
}

// The branch named is checked out, and the folder named in the
// repository is the one opened.
func TestCloneBranchAndPath(t *testing.T) {
	remote := bareRemote(t, map[string]string{"README.md": "code\n"})
	gitCmd(t, remote.src, "checkout", "-q", "-b", "docs")
	os.MkdirAll(filepath.Join(remote.src, "doc", "book"), 0o755)
	os.WriteFile(filepath.Join(remote.src, "doc", "book", "folio.yml"), []byte(testConfig), 0o644)
	gitCmd(t, remote.src, "add", ".")
	gitCmd(t, remote.src, "-c", "user.email=f@example.com", "-c", "user.name=F", "commit", "-q", "-m", "docs")
	gitCmd(t, remote.src, "checkout", "-q", "main")

	srv := bareServer(t)
	dir := t.TempDir()
	rec := post(t, srv, "/clone", url.Values{"url": {"https://github.com/o/code"}, "dir": {dir},
		"branch": {"docs"}, "path": {"doc/book/"}})
	if rec.Header().Get("HX-Retarget") != "#main" {
		t.Fatalf("not opened: %s", rec.Body)
	}
	if remote.branch != "docs" {
		t.Errorf("cloned branch %q", remote.branch)
	}
	if want := filepath.Join(dir, "code", "doc", "book"); srv.gitRoot() != want {
		t.Errorf("open project = %q, want %q", srv.gitRoot(), want)
	}
}

// A folder named that the clone does not have is reported, and the clone
// shown in the folder dialog.
func TestCloneMissingPath(t *testing.T) {
	bareRemote(t, map[string]string{"folio.yml": testConfig})
	srv := bareServer(t)
	dir := t.TempDir()
	rec := post(t, srv, "/clone", url.Values{"url": {"https://github.com/o/book"}, "dir": {dir}, "path": {"docs"}})
	body := rec.Body.String()
	if rec.Header().Get("HX-Retarget") != "" || !strings.Contains(body, `class="browse"`) || !strings.Contains(body, "docs") {
		t.Errorf("missing folder not reported in the folder dialog:\n%s", body)
	}
}

// A connection that failed says what usually stands behind it, and names
// the proxy the system uses when there is one.
func TestCloneNetworkFailure(t *testing.T) {
	prev, prevProxy := cloneRepo, systemProxy
	cloneRepo = func(u, dest, branch, proxy string) (string, error) {
		return "fatal: unable to access 'https://dev.azure.com/o/_git/r/': Recv failure: Connection was reset", errors.New("exit status 128")
	}
	systemProxy = func() string { return "proxy.corp:8080" }
	t.Cleanup(func() { cloneRepo, systemProxy = prev, prevProxy })
	noGitProxy(t)
	srv := bareServer(t)
	rec := post(t, srv, "/clone", url.Values{"url": {"https://dev.azure.com/o/_git/r"}, "dir": {t.TempDir()}})
	body := rec.Body.String()
	for _, want := range []string{"dev.azure.com", "also through the proxy proxy.corp:8080", "Connection was reset"} {
		if !strings.Contains(body, want) {
			t.Errorf("no %q in:\n%s", want, body)
		}
	}
}

// A clone that could not reach its host goes again through the proxy the
// system uses, which git is not told about, and keeps it.
func TestCloneRetriesThroughTheSystemProxy(t *testing.T) {
	stub := bareRemote(t, map[string]string{"folio.yml": testConfig})
	reach := cloneRepo
	var proxies []string
	cloneRepo = func(u, dest, branch, proxy string) (string, error) {
		proxies = append(proxies, proxy)
		if proxy == "" {
			return "fatal: unable to access 'https://dev.azure.com/o/_git/r/': Recv failure: Connection was reset", errors.New("exit status 128")
		}
		return reach(u, dest, branch, proxy)
	}
	prevProxy := systemProxy
	systemProxy = func() string { return "proxy.corp:8080" }
	t.Cleanup(func() { systemProxy = prevProxy })
	noGitProxy(t)
	srv := bareServer(t)
	rec := post(t, srv, "/clone", url.Values{"url": {"https://dev.azure.com/o/_git/r"}, "dir": {t.TempDir()}})
	if rec.Header().Get("HX-Retarget") != "#main" {
		t.Errorf("the book did not open:\n%s", rec.Body.String())
	}
	if !slices.Equal(proxies, []string{"", "http://proxy.corp:8080"}) || stub.asked == "" {
		t.Errorf("cloned through %q, want directly, then through http://proxy.corp:8080", proxies)
	}
}

// A proxy git has of its own is the one it goes through: the system's is
// not tried in its place.
func TestCloneKeepsGitsOwnProxy(t *testing.T) {
	prev, prevProxy := cloneRepo, systemProxy
	var n int
	cloneRepo = func(u, dest, branch, proxy string) (string, error) {
		n++
		return "fatal: Recv failure: Connection was reset", errors.New("exit status 128")
	}
	systemProxy = func() string { return "proxy.corp:8080" }
	t.Cleanup(func() { cloneRepo, systemProxy = prev, prevProxy })
	noGitProxy(t)
	t.Setenv("HTTPS_PROXY", "http://mine:3128")
	srv := bareServer(t)
	post(t, srv, "/clone", url.Values{"url": {"https://dev.azure.com/o/_git/r"}, "dir": {t.TempDir()}})
	if n != 1 {
		t.Errorf("cloned %d times, want once", n)
	}
}

// noGitProxy gives git no proxy of its own for the test.
func noGitProxy(t *testing.T) {
	t.Helper()
	for _, v := range []string{"https_proxy", "HTTPS_PROXY", "all_proxy", "ALL_PROXY"} {
		t.Setenv(v, "")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

// dataHome points the user's data folder at a temporary one and returns
// it.
func dataHome(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LOCALAPPDATA", d)
	case "darwin", "ios":
		t.Setenv("HOME", d)
		return filepath.Join(d, "Library", "Application Support")
	default:
		t.Setenv("XDG_DATA_HOME", d)
	}
	return d
}

type remoteStub struct {
	src           string // the repository cloned from
	asked, branch string
}

// bareRemote makes a repository holding files and points the clone
// handler at it whatever URL is asked for, which it records: the URL
// check is ValidRemote's to make, and a real network remote is not to be
// had in a test.
func bareRemote(t *testing.T, files map[string]string) *remoteStub {
	t.Helper()
	if !gitrepo.Available() {
		t.Skip("git is not installed")
	}
	src := t.TempDir()
	for name, content := range files {
		p := filepath.Join(src, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.email=f@example.com", "-c", "user.name=F", "add", "."},
		{"-c", "user.email=f@example.com", "-c", "user.name=F", "commit", "-q", "-m", "first"},
	} {
		gitCmd(t, src, args...)
	}
	stub := &remoteStub{src: src}
	prev := cloneRepo
	cloneRepo = func(u, dest, branch, proxy string) (string, error) {
		stub.asked, stub.branch = u, branch
		out, err := gitrepo.Clone(src, dest, branch, proxy)
		if err == nil {
			// The clone remembers the URL asked for, as a real one would.
			gitCmd(t, dest, "remote", "set-url", "origin", u)
		}
		return out, err
	}
	t.Cleanup(func() { cloneRepo = prev })
	return stub
}

// gitCmd runs git in dir for a test's own setup.
func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// GitHub's reason for refusing comes through, not only its status.
func TestGitHubRefusalSaysWhy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"device_flow_disabled","error_description":"Device Flow must be explicitly enabled for this App"}`))
	}))
	t.Cleanup(ts.Close)
	prevBase, prevID := githubBase, githubClientID
	githubBase, githubClientID = ts.URL, "cid"
	t.Cleanup(func() { githubBase, githubClientID = prevBase, prevID })
	t.Setenv("FOLIO_GITHUB_CLIENT_ID", "")
	body := post(t, bareServer(t), "/github/login", nil).Body.String()
	if !strings.Contains(body, "Device Flow must be explicitly enabled") {
		t.Errorf("reason not shown:\n%s", body)
	}
}

func TestProxyServer(t *testing.T) {
	for in, want := range map[string]string{
		"":                               "",
		"proxy:8080":                     "proxy:8080",
		"http=p:80;https=s:443;ftp=f:21": "s:443",
		"ftp=f:21;http=p:80":             "p:80",
		"socks=s:1080":                   "",
	} {
		if got := proxyServer(in); got != want {
			t.Errorf("proxyServer(%q) = %q, want %q", in, got, want)
		}
	}
}
