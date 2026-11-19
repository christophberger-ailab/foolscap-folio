package site

import (
	"encoding/json"
	"io/fs"
	"os"
	"regexp"
	"testing"
)

// The embedded Fuse.js is a copied file, so package.json and its lockfile
// exist only to let the vulnerability scan see it. They must name the
// version that is actually embedded.
func TestPackageJSONPinsTheEmbeddedFuse(t *testing.T) {
	b, err := os.ReadFile("package.json")
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		t.Fatal(err)
	}
	pinned := pkg.Dependencies["fuse.js"]
	if pinned == "" {
		t.Fatal("package.json does not list fuse.js")
	}

	header := regexp.MustCompile(`Fuse\.js v(\d+\.\d+\.\d+)`)
	files, err := fs.Glob(assetFS, "assets/fuse*js")
	if err != nil || len(files) == 0 {
		t.Fatalf("no embedded Fuse.js found (%v)", err)
	}
	for _, name := range files {
		data, err := assetFS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		m := header.FindSubmatch(data)
		if m == nil {
			t.Errorf("%s: no Fuse.js version in its header", name)
			continue
		}
		if string(m[1]) != pinned {
			t.Errorf("%s is Fuse.js %s, but package.json pins %s", name, m[1], pinned)
		}
	}
}
