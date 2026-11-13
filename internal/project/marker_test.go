package project

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// writeMarkerFixture builds a project exercising the _FW/_POL marker rules,
// including a folder-marked subtree, an own-suffix override, multi-level
// inheritance, and a synthetic (index-less) directory node.
func writeMarkerFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	pages := map[string]string{
		"index.qmd":                "---\ntitle: Home\norder: 1\n---\n# Home\n",
		"station_FW/index.qmd":     "---\ntitle: Station\norder: 2\n---\n# Station\n",
		"station_FW/alpha.qmd":     "---\ntitle: Alpha\norder: 1\n---\n# Alpha\n",
		"station_FW/bravo_POL.qmd": "---\ntitle: Bravo\norder: 2\n---\n# Bravo\n",
		"report_POL.qmd":           "---\ntitle: Report\norder: 3\n---\n# Report\n",
		"ops_POL/index.qmd":        "---\ntitle: Ops\norder: 4\n---\n# Ops\n",
		"ops_POL/sub/index.qmd":    "---\ntitle: Sub\norder: 1\n---\n# Sub\n",
		"ops_POL/sub/leaf.qmd":     "---\ntitle: Leaf\norder: 1\n---\n# Leaf\n",
		"plain.qmd":                "---\ntitle: Plain\norder: 5\n---\n# Plain\n",
		"zone_FW/only.qmd":         "---\ntitle: Only\norder: 1\n---\n# Only\n",
	}
	for name, content := range pages {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestMarkers(t *testing.T) {
	tree, err := Load(writeMarkerFixture(t))
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"index.qmd":                "",        // no suffix, no marked ancestor
		"plain.qmd":                "",        // no suffix
		"station_FW/index.qmd":     markerFW,  // folder's own suffix
		"station_FW/alpha.qmd":     markerFW,  // inherited from folder
		"station_FW/bravo_POL.qmd": markerPOL, // own suffix overrides folder
		"report_POL.qmd":           markerPOL, // own suffix
		"ops_POL/index.qmd":        markerPOL, // folder's own suffix
		"ops_POL/sub/index.qmd":    markerPOL, // inherited one level down
		"ops_POL/sub/leaf.qmd":     markerPOL, // inherited two levels down
		"zone_FW/only.qmd":         markerFW,  // inherited from synthetic dir node
	}
	for p, want := range cases {
		pg := tree.Find(p)
		if pg == nil {
			t.Fatalf("Find(%q) = nil", p)
		}
		if pg.Marker != want {
			t.Errorf("%s marker = %q, want %q", p, pg.Marker, want)
		}
	}

	// The synthetic directory node itself (Path == "", Dir == "zone_FW")
	// must carry the folder marker derived from its directory name.
	var zone *Page
	for _, p := range tree.Pages {
		if p.Path == "" && p.Dir == "zone_FW" {
			zone = p
		}
	}
	if zone == nil {
		t.Fatal("synthetic zone_FW node not found")
	}
	if zone.Marker != markerFW {
		t.Errorf("synthetic zone_FW marker = %q, want %q", zone.Marker, markerFW)
	}
}

// TestLoadHeadingFallback verifies the title falls back to the first body
// heading when frontmatter has no title, and to the filename only when there
// is no heading either.
func TestLoadHeadingFallback(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"heading.qmd":  "---\norder: 1\n---\n\n# From Heading\n\nbody\n",
		"fenced.qmd":   "---\norder: 2\n---\n\n```\n# In Code\n```\n\n# Outside\n",
		"nohead.qmd":   "---\norder: 3\n---\n\njust text\n",
		"codeonly.qmd": "---\norder: 4\n---\n\n```\n# hidden\n```\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tree, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"heading.qmd":  "From Heading",
		"fenced.qmd":   "Outside",
		"nohead.qmd":   "nohead",   // filename fallback
		"codeonly.qmd": "codeonly", // heading only in code fence -> filename
	}
	for p, w := range want {
		pg := tree.Find(p)
		if pg == nil {
			t.Fatalf("Find(%q) = nil", p)
		}
		if pg.Title != w {
			t.Errorf("%s title = %q, want %q", p, pg.Title, w)
		}
	}
}

func TestPageFlags(t *testing.T) {
	todo := Flag{"\U0001F6A7", "Contains todos"}
	review := Flag{"\U0001F440", "Needs review"}
	comment := Flag{"\U0001F4AC", "Contains comments"}
	cases := []struct {
		name string
		src  string
		want []Flag
	}{
		{"none", "---\ntitle: A\n---\n# A\n", nil},
		{"no frontmatter", "# A\n", nil},
		{"todo", "---\nfolio:\n  todo: true\n---\n", []Flag{todo}},
		{"review", "---\nfolio:\n  review: true\n---\n", []Flag{review}},
		{"both", "---\nfolio:\n  review: true\n  todo: true\n---\n", []Flag{todo, review}},
		{"false", "---\nfolio:\n  todo: false\n  review: false\n---\n", nil},
		{"not a bool", "---\nfolio:\n  todo: yes please\n---\n", nil},
		{"unknown key", "---\nfolio:\n  later: true\n---\n", nil},
		{"legacy qm", "---\nqm:\n  review: true\n---\n", []Flag{review}},
		{"folio wins over qm", "---\nfolio:\n  review: false\nqm:\n  review: true\n  todo: true\n---\n", []Flag{todo}},
		{"text", "# A\n\nSome ==TODO== here.\n", []Flag{todo}},
		{"text in comment", "# A\n\n<!-- ==TODO== fix -->\n", []Flag{todo}},
		{"text twice and key", "---\nfolio:\n  todo: true\n---\n==TODO== ==TODO==\n", []Flag{todo}},
		{"text and review", "---\nfolio:\n  review: true\n---\n==TODO==\n", []Flag{todo, review}},
		{"lowercase text", "==todo==\n", nil},
		{"comment", "# A\n\n<!-- << Check this -->the passage<!-- >> -->\n", []Flag{comment}},
		{"comment with attrs", "<!--<<{#c1 author=\"Ann\"} Hm -->x<!-- >> {#c1} -->\n", []Flag{comment}},
		{"comment over lines", "<!--\n<< Hm\n-->x<!-- >> -->\n", []Flag{comment}},
		{"comments twice", "<!-- << A -->x<!-- >> --> <!-- << B -->y<!-- >> -->\n", []Flag{comment}},
		{"all three", "---\nfolio:\n  review: true\n---\n==TODO== <!-- << A -->x<!-- >> -->\n", []Flag{todo, review, comment}},
		{"plain html comment", "<!-- a note << not a comment -->\n", nil},
		{"end marker alone", "<!-- >> -->\n", nil},
		{"comment in code span", "Write `<!-- << A -->` to comment.\n", nil},
		{"comment in double code span", "Write ``<!-- << ` -->`` to comment.\n", nil},
		{"comment in code block", "```\n<!-- << A -->x<!-- >> -->\n```\n", nil},
		{"comment in tilde block", "~~~ markdown\n<!-- << A -->\n~~~\n", nil},
		{"comment after code block", "```\ncode\n```\n<!-- << A -->x<!-- >> -->\n", []Flag{comment}},
		{"comment after code span", "`code` <!-- << A -->x<!-- >> -->\n", []Flag{comment}},
		{"unclosed backtick", "a ` b <!-- << A -->x<!-- >> -->\n", []Flag{comment}},
	}
	for _, c := range cases {
		got := PageFlags([]byte(c.src))
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: PageFlags = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestLoadFlags(t *testing.T) {
	root := t.TempDir()
	src := "---\ntitle: Flagged\norder: 1\nfolio:\n  review: true\n---\n# Flagged\n<!-- ==TODO== -->\n"
	if err := os.WriteFile(filepath.Join(root, "flagged.qmd"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	tree, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	pg := tree.Find("flagged.qmd")
	if pg == nil {
		t.Fatal("Find(flagged.qmd) = nil")
	}
	if pg.Title != "Flagged" || pg.Order == nil || *pg.Order != 1 {
		t.Errorf("title/order = %q/%v, want Flagged/1", pg.Title, pg.Order)
	}
	if want := []Flag{{"\U0001F6A7", "Contains todos"}, {"\U0001F440", "Needs review"}}; !slices.Equal(pg.Flags, want) {
		t.Errorf("Flags = %q, want %q", pg.Flags, want)
	}
}

func TestLoadHidden(t *testing.T) {
	root := t.TempDir()
	pages := map[string]string{
		"shown.qmd":           "---\ntitle: Shown\norder: 1\nfolio:\n  hide: false\n---\n",
		"secret.qmd":          "---\ntitle: Secret\norder: 2\nfolio:\n  hide: true\n---\n",
		"old.qmd":             "---\ntitle: Old\norder: 5\nqm:\n  hide: true\n---\n",
		"sec/index.qmd":       "---\ntitle: Sec\norder: 3\n---\n",
		"sec/gone.qmd":        "---\ntitle: Gone\nfolio:\n  hide: true\n---\n",
		"loose/only.qmd":      "---\ntitle: Only\nfolio:\n  hide: true\n---\n",
		"mixed/hidden.qmd":    "---\ntitle: Hidden\nfolio:\n  hide: true\n---\n",
		"mixed/visible.qmd":   "---\ntitle: Visible\n---\n",
		"plain/no-option.qmd": "---\ntitle: Plain\n---\n",
	}
	for name, content := range pages {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tree, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]bool{
		"shown.qmd":           false,
		"secret.qmd":          true,
		"old.qmd":             true,
		"sec/index.qmd":       false,
		"sec/gone.qmd":        true,
		"mixed/visible.qmd":   false,
		"plain/no-option.qmd": false,
	} {
		if got := tree.Find(p).Hidden; got != want {
			t.Errorf("%s: Hidden = %v, want %v", p, got, want)
		}
	}
	if tree.Find("sec/index.qmd").HasVisibleChildren() {
		t.Error("sec/index.qmd has visible children, want none")
	}
	// A synthetic directory node goes with its pages when they are all
	// hidden, and stays while one of them is shown.
	dirs := map[string]bool{}
	for _, p := range tree.Pages {
		if p.Path == "" {
			dirs[p.Dir] = p.Hidden
		}
	}
	if !dirs["loose"] {
		t.Error("loose/ shown although its only page is hidden")
	}
	if dirs["mixed"] {
		t.Error("mixed/ hidden although one of its pages is shown")
	}
}
