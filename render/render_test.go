package render

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/build"
	"github.com/christophberger-ailab/foolscap-folio/internal/config"
)

// fixture builds a small project with two topics. `chapter2` takes part in
// every format and audience; `extra` declares a narrower matrix.
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"index.qmd":           "---\ntitle: Home\norder: 1\n---\n# Home\n",
		"chapter2/index.qmd":  "---\ntitle: Chapter 2\norder: 2\n---\n# Two\n\n::: pol\nnur pol\n:::\n",
		"chapter2/second.qmd": "---\ntitle: Second\norder: 1\n---\n# Second\n",
		"extra/index.qmd":     "---\ntitle: Extra\norder: 3\n---\n# Extra\n",
		"folio.yml": `title: Test
formats:
  web: {type: html}
  handout: {type: [docx, pdf]}
  handbook: {type: docx}
audiences:
  std:
  pol:
topics:
  chapter2:
  extra: {formats: [handbook], audiences: [std]}
`,
	}
	for name, content := range files {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func selections(opts build.Options) []string {
	out := make([]string, 0, len(opts.Selections))
	for _, s := range opts.Selections {
		out = append(out, s.String())
	}
	return out
}

func TestBuildOptionsExpandsTheDeclaredMatrix(t *testing.T) {
	root := fixture(t)
	opts, err := BuildOptions(root, config.Request{}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	got := selections(opts)
	want := []string{
		"web · std (site)", "web · pol (site)",
		"chapter2 · handout · std", "chapter2 · handout · pol",
		"chapter2 · handbook · std", "chapter2 · handbook · pol",
		"extra · handbook · std",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
	// The website covers both topics; extra takes no part in it.
	if n := len(opts.Selections[0].Topics); n != 1 {
		t.Errorf("site topics = %d, want 1 (extra declares handbook only)", n)
	}
}

func TestBuildOptionsNarrowsByFlag(t *testing.T) {
	root := fixture(t)
	opts, err := BuildOptions(root, config.Request{Formats: []string{"handbook"}, Audiences: []string{"std"}}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"chapter2 · handbook · std", "extra · handbook · std"}
	if got := selections(opts); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildOptionsRejectsUnknownNames(t *testing.T) {
	root := fixture(t)
	for _, req := range []config.Request{
		{Topics: []string{"calltakr"}},
		{Formats: []string{"pptx"}},
		{Audiences: []string{"fw"}},
	} {
		if _, err := BuildOptions(root, req, false, false); err == nil {
			t.Errorf("%+v: accepted", req)
		}
	}
}

func TestBuildOptionsWithoutConfigUsesDefaults(t *testing.T) {
	root := fixture(t)
	os.Remove(filepath.Join(root, "folio.yml"))
	opts, err := BuildOptions(root, config.Request{}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"web · all (site)"}
	if got := selections(opts); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRunWritesDocuments(t *testing.T) {
	root := fixture(t)
	if err := Run(root, config.Request{Topics: []string{"chapter2"}, Formats: []string{"handout"}}, false, false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"chapter2-std.docx", "chapter2-std.pdf", "chapter2-pol.docx", "chapter2-pol.pdf"} {
		if fi, err := os.Stat(filepath.Join(root, "_output", "handout", name)); err != nil || fi.Size() == 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestRunWritesTheSite(t *testing.T) {
	root := fixture(t)
	if err := Run(root, config.Request{Formats: []string{"web"}, Audiences: []string{"std"}}, true, false); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(root, "_output", "web-std", "chapter2", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page), "nur pol") {
		t.Error("the std site shows pol content")
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	root := fixture(t)
	if err := Run(root, config.Request{}, false, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "_output")); !os.IsNotExist(err) {
		t.Errorf("dry run wrote output: %v", err)
	}
}

func TestSplitList(t *testing.T) {
	for in, want := range map[string][]string{
		"":        nil,
		"a":       {"a"},
		"a, b,,c": {"a", "b", "c"},
		" , ":     nil,
	} {
		if got := splitList(in); !slices.Equal(got, want) {
			t.Errorf("splitList(%q) = %q, want %q", in, got, want)
		}
	}
}

// A missing image does not stop a render: every format shows the
// project's placeholder in its place.
func TestRunUsesPlaceholderForMissingImages(t *testing.T) {
	root := fixture(t)
	page := filepath.Join(root, "chapter2", "second.qmd")
	os.WriteFile(page, []byte("---\ntitle: Second\norder: 1\n---\n# Second\n\n![gone](/assets/images/gone.png)\n"), 0o644)
	ph := filepath.Join(root, "assets", "static", "_placeholder.svg")
	os.MkdirAll(filepath.Dir(ph), 0o755)
	os.WriteFile(ph, []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="64" height="32"><rect width="64" height="32" fill="red"/></svg>`), 0o644)

	if err := Run(root, config.Request{Topics: []string{"chapter2"}, Audiences: []string{"std"}}, false, false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"handout/chapter2-std.docx", "handout/chapter2-std.pdf", "handbook/chapter2-std.docx"} {
		if fi, err := os.Stat(filepath.Join(root, "_output", name)); err != nil || fi.Size() == 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
	html, err := os.ReadFile(filepath.Join(root, "_output", "web-std", "chapter2", "second.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), `src="../assets/static/_placeholder.svg"`) {
		t.Errorf("the site page does not show the placeholder:\n%s", html)
	}
	if _, err := os.Stat(filepath.Join(root, "_output", "web-std", "assets", "static", "_placeholder.svg")); err != nil {
		t.Errorf("the placeholder is not copied into the site: %v", err)
	}
}

// A cover that is not there does not stop a render either: the placeholder
// takes its place, with a warning.
func TestRunUsesPlaceholderForMissingCover(t *testing.T) {
	root := fixture(t)
	cfg := filepath.Join(root, "folio.yml")
	b, _ := os.ReadFile(cfg)
	os.WriteFile(cfg, []byte(strings.Replace(string(b), "  chapter2:\n", "  chapter2: {cover: assets/templates/gone.png}\n", 1)), 0o644)

	if err := Run(root, config.Request{Topics: []string{"chapter2"}, Formats: []string{"handout"}, Audiences: []string{"std"}}, false, false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"chapter2-std.docx", "chapter2-std.pdf"} {
		if fi, err := os.Stat(filepath.Join(root, "_output", "handout", name)); err != nil || fi.Size() == 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Every selection gets the user's callout titles, from the settings file
// the command was registered with.
func TestBuildOptionsCarriesTheUsersCalloutTitles(t *testing.T) {
	root := fixture(t)
	titles := map[string]map[string]string{"fr": {"note": "Remarque"}}
	defer func(f func() (map[string]map[string]string, error)) { calloutTitles = f }(calloutTitles)
	calloutTitles = func() (map[string]map[string]string, error) { return titles, nil }

	opts, err := BuildOptions(root, config.Request{}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, sel := range opts.Selections {
		if sel.Config.CalloutTitles["fr"]["note"] != "Remarque" {
			t.Errorf("%s: callout titles = %v", sel, sel.Config.CalloutTitles)
		}
	}

	calloutTitles = func() (map[string]map[string]string, error) { return nil, errors.New("config.cue:3: bad") }
	if _, err := BuildOptions(root, config.Request{}, false, false); err == nil || !strings.Contains(err.Error(), "config.cue:3") {
		t.Errorf("a broken settings file was not reported: %v", err)
	}
}
