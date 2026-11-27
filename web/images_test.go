package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// imageList decodes a /images response.
func imageList(t *testing.T, srv http.Handler) []imageEntry {
	t.Helper()
	rec := get(t, srv, "/images")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var out struct {
		Images []imageEntry `json:"images"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Images
}

// UI.14-10: the Images pane lists every image under assets/images, in
// subfolders too, with its project path and when it was last modified;
// other files, the entries folio.yml's ignore patterns name (issue #181),
// and images elsewhere are left out.
func TestImagesListsAssetsImages(t *testing.T) {
	srv, root := testServer(t)
	writeConfig(t, root, testConfig+"ignore: [\"_*\", \".*\"]\n")
	writePNG(t, root, "assets/images/b.png")
	writePNG(t, root, "assets/images/shots/a photo.png")
	writePNG(t, root, "assets/images/_draft.png")
	writePNG(t, root, "assets/images/.hidden/c.png")
	writePNG(t, root, "assets/images/_old/d.png")
	writePNG(t, root, "elsewhere.png")
	os.WriteFile(filepath.Join(root, "assets/images/notes.txt"), []byte("x"), 0o644)
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	os.Chtimes(filepath.Join(root, "assets/images/b.png"), when, when)

	got := imageList(t, srv)
	if len(got) != 2 {
		t.Fatalf("images = %+v, want b.png and shots/a photo.png", got)
	}
	byPath := map[string]imageEntry{}
	for _, img := range got {
		byPath[img.Path] = img
	}
	b, ok := byPath["assets/images/b.png"]
	if !ok || b.Name != "b.png" || b.Modified != when.UnixMilli() {
		t.Errorf("b.png = %+v, want name b.png modified %d", b, when.UnixMilli())
	}
	if a, ok := byPath["assets/images/shots/a photo.png"]; !ok || a.Name != "shots/a photo.png" {
		t.Errorf("a photo.png = %+v", a)
	}
}

// A project without the folder has no images, which is no error.
func TestImagesWithoutTheFolder(t *testing.T) {
	srv, _ := testServer(t)
	if got := imageList(t, srv); len(got) != 0 {
		t.Errorf("images = %+v, want none", got)
	}
}

func TestImagesWithoutAProject(t *testing.T) {
	if rec := get(t, bareServer(t), "/images"); rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", rec.Code)
	}
}

// A symlink in the folder is not listed, so the pane cannot be pointed at
// a file outside the project.
func TestImagesSkipsSymlinks(t *testing.T) {
	srv, root := testServer(t)
	outside := filepath.Join(t.TempDir(), "x.png")
	writePNG(t, filepath.Dir(outside), "x.png")
	os.MkdirAll(filepath.Join(root, "assets/images"), 0o755)
	if err := os.Symlink(outside, filepath.Join(root, "assets/images/link.png")); err != nil {
		t.Skip("no symlinks:", err)
	}
	if got := imageList(t, srv); len(got) != 0 {
		t.Errorf("images = %+v, want none", got)
	}
}

// UI.14-10: the Images pane belongs to the project, like Render and Git.
func TestImagesPaneComesWithTheProject(t *testing.T) {
	srv, _ := testServer(t)
	body := post(t, srv, "/open", url.Values{"path": {fixture(t)}}).Body.String()
	for _, want := range []string{`id="images-pane" data-pane="images"`, `id="images-view"`, `id="images-sort"`, `id="images-search"`, `id="images-list"`} {
		if !strings.Contains(body, want) {
			t.Errorf("open does not bring %s", want)
		}
	}
	if body := get(t, bareServer(t), "/").Body.String(); strings.Contains(body, `id="images-pane"`) {
		t.Error("page without a project carries the Images pane")
	}
	for file, wants := range map[string][]string{
		"assets/static/dock.js":   {"images: { label: 'Images', id: 'images-pane'"},
		"assets/static/app.js":    {"function updateImages(", "insertImageLink(", "function filterImages("},
		"assets/static/editor.js": {"function insertImageLink("},
		"assets/static/app.css":   {".images-list", ".images-list.large", ".images-list.small", ".image-insert"},
	} {
		src, err := assets.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range wants {
			if !assetContains(file, string(src), want) {
				t.Errorf("%s does not carry %q", file, want)
			}
		}
	}
}

// jsFunc cuts a top-level function out of a script, for node to run.
func jsFunc(t *testing.T, file, name string) string {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	fn := regexp.MustCompile(`(?s)\nfunction ` + name + `\(.*?\n}\n`).Find(src)
	if fn == nil {
		t.Fatalf("%s has no function %s", file, name)
	}
	return string(fn)
}

func runNode(t *testing.T, script string, args ...string) string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	out, err := exec.Command(node, append([]string{"-e", script, "--"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	return string(out)
}

// UI.14-10-2: four orders -- by name or by date modified, each ascending
// or descending; images modified at once fall back to their names.
func TestSortImages(t *testing.T) {
	script := jsFunc(t, "assets/static/app.js", "sortImages") + `
const imgs = [{name: "b.png", modified: 2}, {name: "a.png", modified: 2}, {name: "C.png", modified: 1}];
for (const order of ["name-asc", "name-desc", "modified-asc", "modified-desc"]) {
  console.log(order + ":" + sortImages(imgs, order).map(i => i.name).join(","));
}`
	want := "name-asc:a.png,b.png,C.png\nname-desc:C.png,b.png,a.png\nmodified-asc:C.png,a.png,b.png\nmodified-desc:a.png,b.png,C.png\n"
	if got := runNode(t, script); got != want {
		t.Errorf("sortImages:\n%s\nwant:\n%s", got, want)
	}
}

// UI.14-10-5: the search box keeps the images whose name -- their path
// inside assets/images -- contains the text typed, ignoring case; a blank
// field keeps them all.
func TestFilterImages(t *testing.T) {
	script := jsFunc(t, "assets/static/app.js", "filterImages") + `
const imgs = [{name: "Cover.png"}, {name: "shots/cover-back.jpg"}, {name: "map.svg"}];
for (const q of ["cover", "SHOTS/", " ", "ver-b", "nope"]) {
  console.log(JSON.stringify(q) + ":" + filterImages(imgs, q).map(i => i.name).join(","));
}`
	want := `"cover":Cover.png,shots/cover-back.jpg
"SHOTS/":shots/cover-back.jpg
" ":Cover.png,shots/cover-back.jpg,map.svg
"ver-b":shots/cover-back.jpg
"nope":
`
	if got := runNode(t, script); got != want {
		t.Errorf("filterImages:\n%s\nwant:\n%s", got, want)
	}
}

// UI.14-10-3: the link is website-absolute, wrapped in <...> where the
// path would otherwise end it, and the cursor goes between the brackets.
func TestImageLink(t *testing.T) {
	script := jsFunc(t, "assets/static/editor.js", "imageLink") + `
console.log(imageLink("assets/images/a.png"));
console.log(imageLink("assets/images/a photo (1).png"));`
	want := "![](/assets/images/a.png)\n![](</assets/images/a photo (1).png>)\n"
	if got := runNode(t, script); got != want {
		t.Errorf("imageLink:\n%s\nwant:\n%s", got, want)
	}
}

// The event /watch raises when an image changes on disk is the one the
// page listens for, so the preview and the Images pane follow the change.
func TestMediaChangedEventIsHeard(t *testing.T) {
	src, err := assets.ReadFile("assets/static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(src), "addEventListener('"+mediaChangedEvent+"'"); n < 2 {
		t.Errorf("app.js listens for %q %d times, want the preview and the Images pane", mediaChangedEvent, n)
	}
}
