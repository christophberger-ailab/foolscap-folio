package web

import (
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The preview shows {{name}} and {{< var name >}} resolved: the page
// carries the variables of its topic, _variables.yml's winning over
// folio.yml's, for preview.js to put in (issue #124).
func TestThePreviewGetsTheVariablesOfThePagesTopic(t *testing.T) {
	srv, root := testServer(t)
	writeConfig(t, root, "vars: {version: '1.0', owner: Me}\n"+strings.Replace(testConfig, "chapter2:\n", "chapter2: {vars: {area: Two}}\n", 1))
	if err := os.WriteFile(filepath.Join(root, "_variables.yml"), []byte("version: dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := html.UnescapeString(get(t, srv, "/content?path=chapter2/second.qmd").Body.String())
	for _, want := range []string{`data-variables=`, `"version":"dev"`, `"owner":"Me"`, `"area":"Two"`, `"topic":"chapter2"`, `"date":"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not carry %q:\n%s", want, body)
		}
	}
}

// preview.js puts the variables in before the Markdown is parsed, as the
// render does before it parses.
func TestPreviewResolvesVariables(t *testing.T) {
	for file, wants := range map[string][]string{
		"assets/static/preview.js": {"function resolveVariables(body, vars)", "resolveVariables(page.body, vars)"},
		"assets/static/app.js":     {"calloutTitles(), pageVariables());", "form.getAttribute('data-variables')"},
	} {
		src, err := assets.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range wants {
			if !assetContains(file, string(src), want) {
				t.Errorf("%s does not resolve variables, missing %q", file, want)
			}
		}
	}
}
