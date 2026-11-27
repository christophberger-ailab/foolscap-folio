package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
)

// openFolder opens dir, which holds no folio.yml, as the Open dialog does.
func openFolder(t *testing.T, dir string) (*server, string) {
	t.Helper()
	srv := bareServer(t)
	rec := post(t, srv, "/open", url.Values{"path": {dir}})
	if rec.Code != http.StatusOK {
		t.Fatalf("open: status %d: %s", rec.Code, rec.Body)
	}
	return srv, rec.Body.String()
}

// A folder without folio.yml offers to start a project in it; an empty one
// offers the demo project too, and a project offers neither.
func TestOpenOffersInit(t *testing.T) {
	_, body := openFolder(t, t.TempDir())
	for _, want := range []string{`hx-post="/init"`, "Start a new project here", `name="demo"`} {
		if !strings.Contains(body, want) {
			t.Errorf("empty folder: no %q:\n%s", want, body)
		}
	}

	pages := t.TempDir()
	os.MkdirAll(filepath.Join(pages, "guide"), 0o755)
	os.WriteFile(filepath.Join(pages, "guide", "index.md"), []byte("# Guide\n"), 0o644)
	_, body = openFolder(t, pages)
	if !strings.Contains(body, `hx-post="/init"`) || strings.Contains(body, `name="demo"`) {
		t.Errorf("folder of pages: init offered wrongly:\n%s", body)
	}

	srv, _ := testServer(t)
	if body := get(t, srv, "/").Body.String(); strings.Contains(body, `hx-post="/init"`) {
		t.Errorf("project offers init:\n%s", body)
	}
}

// Init starts the project in the open folder and shows it.
func TestInitStartsAProject(t *testing.T) {
	for name, form := range map[string]url.Values{"new": {}, "demo": {"demo": {"1"}}} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			srv, _ := openFolder(t, dir)
			rec := post(t, srv, "/init", form)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
			if config.Find(dir) == "" {
				t.Fatal("no folio.yml written")
			}
			_, err := os.Stat(filepath.Join(dir, "showcase"))
			if demo := name == "demo"; demo != (err == nil) {
				t.Errorf("demo %v, showcase: %v", demo, err)
			}
			body := rec.Body.String()
			if strings.Contains(body, `hx-post="/init"`) || !strings.Contains(body, `id="tree"`) {
				t.Errorf("project not shown:\n%s", body)
			}
		})
	}
}

// Init is refused without an open folder and for a project; the demo is
// refused in a folder that is not empty, with the reason shown.
func TestInitRefusals(t *testing.T) {
	if rec := post(t, bareServer(t), "/init", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("no folder open: status %d", rec.Code)
	}
	srv, root := testServer(t)
	before, _ := os.ReadFile(filepath.Join(root, config.FileName))
	if rec := post(t, srv, "/init", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("project: status %d", rec.Code)
	}
	if after, _ := os.ReadFile(filepath.Join(root, config.FileName)); string(after) != string(before) {
		t.Error("project's folio.yml changed")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Notes\n"), 0o644)
	srv, _ = openFolder(t, dir)
	rec := post(t, srv, "/init", url.Values{"demo": {"1"}})
	if !strings.Contains(rec.Body.String(), "not empty") || config.Find(dir) != "" {
		t.Errorf("demo in a folder with pages: %d %s", rec.Code, rec.Body)
	}
}
