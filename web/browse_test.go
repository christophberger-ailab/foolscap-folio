package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The folder dialog lists the subfolders of the folder it shows -- not its
// files, not its hidden folders -- and marks the ones that are folios.
func TestBrowseListsSubfolders(t *testing.T) {
	srv := bareServer(t)
	dir := t.TempDir()
	for _, d := range []string{"alpha", "beta", ".hidden"} {
		os.Mkdir(filepath.Join(dir, d), 0o755)
	}
	os.WriteFile(filepath.Join(dir, "beta", "folio.yml"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "file.txt"), nil, 0o644)

	rec := get(t, srv, "/browse?dir="+url.QueryEscape(dir))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`data-path="` + filepath.Join(dir, "alpha") + `"`,
		`class="browse-entry folio" data-path="` + filepath.Join(dir, "beta") + `"`,
		`value="` + dir + `"`,
		`data-path="` + filepath.Dir(dir) + `"`, // the way up
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dialog missing %q:\n%s", want, body)
		}
	}
	for _, unwanted := range []string{"file.txt", ".hidden"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("dialog lists %q:\n%s", unwanted, body)
		}
	}
	if strings.Index(body, "alpha") > strings.Index(body, "beta") {
		t.Errorf("folders are not sorted:\n%s", body)
	}
}

// Without a folder named, the dialog shows where the last folio was opened
// from, with that folio selected.
func TestBrowseStartsAtLastOpenedLocation(t *testing.T) {
	srv, root := testServer(t)
	body := get(t, srv, "/browse").Body.String()
	if !strings.Contains(body, `value="`+filepath.Dir(root)+`"`) {
		t.Errorf("dialog does not show %q:\n%s", filepath.Dir(root), body)
	}
	if !strings.Contains(body, `class="browse-entry folio selected" data-path="`+root+`"`) {
		t.Errorf("last opened folio %q is not selected:\n%s", root, body)
	}
}

// With nothing opened yet, the dialog starts in the current directory.
func TestBrowseDefaultsToWorkingDirectory(t *testing.T) {
	srv := bareServer(t)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	body := get(t, srv, "/browse").Body.String()
	if !strings.Contains(body, `value="`+wd+`"`) {
		t.Errorf("dialog does not show the working directory %q:\n%s", wd, body)
	}
}

// A folder that is not there is reported in the dialog rather than
// replacing it, so the user can correct the path.
func TestBrowseMissingFolder(t *testing.T) {
	srv := bareServer(t)
	missing := filepath.Join(t.TempDir(), "gone")
	rec := get(t, srv, "/browse?dir="+url.QueryEscape(missing))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if body := rec.Body.String(); !strings.Contains(body, `class="browse-error"`) {
		t.Errorf("no error shown for %q:\n%s", missing, body)
	}
}
