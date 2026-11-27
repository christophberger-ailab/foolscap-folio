package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #181: the ignore setting lists gitignore patterns of the paths
// folio leaves out; the output directories and .git are always left out.
func TestIgnoreMatch(t *testing.T) {
	ig, err := NewIgnore([]string{"_*/", ".ignored/", "**/OLD/", "*.draft.md", "/top.md", "docs/tmp", "!_keep/", "# comment", ""}, "out")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		rel  string
		dir  bool
		want bool
	}{
		{"_drafts", true, true},
		{"_drafts/page.md", false, true},
		{"a/_drafts", true, true},
		{"_page.md", false, false}, // "_*/" names directories only
		{"_keep", true, false},     // negated
		{"_keep/page.md", false, false},
		{".ignored/x.md", false, true},
		{".other/x.md", false, false},
		{"a/b/OLD/x.md", false, true},
		{"OLD", true, true},
		{"OLD", false, false},
		{"a/b.draft.md", false, true},
		{"top.md", false, true},
		{"a/top.md", false, false},
		{"docs/tmp", true, true},
		{"x/docs/tmp", true, false},
		{"page.md", false, false},
		{"out/web/index.md", false, true},
		{"_build/book.md", false, true},
		{"a/.git", true, true},
		{"", true, false},
	} {
		if got := ig.Match(c.rel, c.dir); got != c.want {
			t.Errorf("Match(%q, %v) = %v, want %v", c.rel, c.dir, got, c.want)
		}
	}
	if (Ignore{}).Match("_drafts/x.md", false) {
		t.Error("the zero Ignore leaves out _drafts")
	}
	deep, _ := NewIgnore([]string{strings.Repeat("**/a/", 40) + "b"}, "")
	if deep.Match(strings.Repeat("a/", 60)+"c", false) { // fast, too
		t.Error("a deep pattern matches what it does not name")
	}
	if _, err := NewIgnore([]string{"[a"}, ""); err == nil {
		t.Error("a bad pattern is no error")
	}
}

// Without a setting nothing is left out; a bad pattern fails the load;
// ReadIgnore reads the setting of a project's folio.yml alone.
func TestIgnoreSetting(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"_drafts/a.md", "notes/b.md", "old/c.md"} {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Topics.Names(); len(got) != 3 {
		t.Errorf("topics without a setting = %v, want all three", got)
	}
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("ignore: [\"_*/\", old/]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if c, err = Load(root); err != nil {
		t.Fatal(err)
	}
	if got := c.Topics.Names(); len(got) != 1 || got[0] != "notes" {
		t.Errorf("topics = %v, want [notes]", got)
	}
	if !ReadIgnore(root).Match("old/c.md", false) || ReadIgnore(root).Match("notes/b.md", false) {
		t.Error("ReadIgnore does not read the setting")
	}
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("ignore: [\"[x\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil {
		t.Error("a bad pattern loads")
	}
}
