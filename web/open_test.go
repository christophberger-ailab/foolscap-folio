package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The top bar names the open folio by the title its folio.yml gives it, left
// of the Open button, with the full path as the tooltip. There is no path
// field any more: the Open button opens the folder dialog instead.
func TestTopBarShowsFolioTitle(t *testing.T) {
	root := fixture(t)
	if err := os.WriteFile(filepath.Join(root, "folio.yml"), []byte("title: The Handbook\n"+testConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := bareServer(t)
	if rec := post(t, srv, "/open", url.Values{"path": {root}}); rec.Code != http.StatusOK {
		t.Fatalf("open: status %d: %s", rec.Code, rec.Body)
	}
	body := get(t, srv, "/").Body.String()
	if !strings.Contains(body, `<span class="folio-title" id="folio-title" title="`+root+`">The Handbook</span>`) {
		t.Errorf("top bar does not show the folio's title:\n%s", body)
	}
	if strings.Contains(body, `id="open-path"`) {
		t.Errorf("top bar still has the path field:\n%s", body)
	}
}

// Without a title, the folio is named by its folder.
func TestTopBarTitleDefaultsToFolderName(t *testing.T) {
	srv, root := testServer(t)
	body := get(t, srv, "/").Body.String()
	if !strings.Contains(body, `title="`+root+`">`+filepath.Base(root)+`</span>`) {
		t.Errorf("top bar does not show the folder name %q:\n%s", filepath.Base(root), body)
	}
}

// The title comes first, then Open, then Reload, which reads the open page
// from disk again.
func TestTopBarOrder(t *testing.T) {
	srv, _ := testServer(t)
	body := get(t, srv, "/").Body.String()
	title := strings.Index(body, `id="folio-title"`)
	open := strings.Index(body, `id="open-button"`)
	reload := strings.Index(body, `id="content-reload"`)
	if title < 0 || open < 0 || reload < 0 || !(title < open && open < reload) {
		t.Errorf("want title < open < reload, got %d, %d, %d", title, open, reload)
	}
	if strings.Count(body, `id="content-reload"`) != 1 {
		t.Errorf("Reload button appears more than once:\n%s", body)
	}
}

// A path with nothing to shorten -- the root directory -- is shown as it
// is, rather than as an empty field.
func TestPathLabel(t *testing.T) {
	tests := []struct{ path, want string }{
		{"/home/u/book", "book"},
		{"/home/u/book/", "book"},
		{"/", "/"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := pathLabel(tc.path); got != tc.want {
			t.Errorf("pathLabel(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// Every project opened lands in the dialog's recent list, most recent first, and none
// of them twice.
func TestRecentPathsAreOffered(t *testing.T) {
	srv, first := testServer(t)
	second := fixture(t)
	if rec := post(t, srv, "/open", url.Values{"path": {second}}); rec.Code != http.StatusOK {
		t.Fatalf("open: status %d: %s", rec.Code, rec.Body)
	}
	// Opening the first one again moves it back to the head instead of
	// listing it twice.
	if rec := post(t, srv, "/open", url.Values{"path": {first}}); rec.Code != http.StatusOK {
		t.Fatalf("reopen: status %d: %s", rec.Code, rec.Body)
	}

	if got := srv.cfg.Recent; len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("recent = %v, want [%s %s]", got, first, second)
	}
	body := get(t, srv, "/browse").Body.String()
	for _, want := range []string{
		`data-path="` + first + `"`,
		`data-path="` + second + `"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("recent list missing %q:\n%s", want, body)
		}
	}
}

// The list is capped, so the dialog stays a list one can look through.
func TestRecentPathsAreCapped(t *testing.T) {
	srv := bareServer(t)
	for i := 0; i < maxRecent+5; i++ {
		srv.rememberRoot(filepath.Join(t.TempDir(), "p"))
	}
	if len(srv.cfg.Recent) != maxRecent {
		t.Errorf("recent holds %d paths, want %d", len(srv.cfg.Recent), maxRecent)
	}
}

// The recent list survives a restart: it is written beside the render prefs
// and read back by the next server.
func TestRecentPathsPersist(t *testing.T) {
	dir := t.TempDir()
	prefs := filepath.Join(dir, configFileName)
	root := fixture(t)

	srv, err := newServer(prefs)
	if err != nil {
		t.Fatal(err)
	}
	if rec := post(t, srv, "/open", url.Values{"path": {root}}); rec.Code != http.StatusOK {
		t.Fatalf("open: status %d: %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(filepath.Join(dir, configFileName)); err != nil {
		t.Fatalf("%s not written: %v", configFileName, err)
	}

	again, err := newServer(prefs)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.cfg.Recent) != 1 || again.cfg.Recent[0] != root {
		t.Errorf("restored recent = %v, want [%s]", again.cfg.Recent, root)
	}
}

// /open replaces the top bar's title along with the panes, so it names
// the folio just opened.
func TestOpenRefreshesTheTitle(t *testing.T) {
	srv, _ := testServer(t)
	other := fixture(t)
	body := post(t, srv, "/open", url.Values{"path": {other}}).Body.String()
	if !strings.Contains(body, `hx-swap-oob="innerHTML:#open-panel"`) {
		t.Errorf("open does not refresh the top bar:\n%s", body)
	}
	if !strings.Contains(body, `title="`+other+`">`+filepath.Base(other)+`</span>`) {
		t.Errorf("refreshed title does not name %q:\n%s", other, body)
	}
}

// bareServer is a server with no project open, which is what the app
// starts as when it is given no path.
func bareServer(t *testing.T) *server {
	t.Helper()
	srv, err := newServer("")
	if err != nil {
		t.Fatal(err)
	}
	return srv
}
