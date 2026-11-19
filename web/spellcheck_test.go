package web

import (
	"strings"
	"testing"
)

// The editor's toolbar carries a spellcheck toggle beside the vim toggle,
// starting unpressed: spellchecking is off unless the user turns it on
// (spec-web.yaml 3-6-9, 3-6-10).
func TestContentServesSpellToggle(t *testing.T) {
	srv, _ := testServer(t)
	body := get(t, srv, "/content?path=index.qmd").Body.String()
	if !strings.Contains(body, `id="spell-toggle" aria-pressed="false"`) {
		t.Fatalf("editor pane has no unpressed spellcheck toggle:\n%s", body)
	}
	vim := strings.Index(body, `id="vim-toggle"`)
	spell := strings.Index(body, `id="spell-toggle"`)
	divider := strings.Index(body, `id="preview-pane"`)
	if !(vim < spell && spell < divider) {
		t.Errorf("spellcheck toggle does not follow the vim toggle in the editor's column:\n%s", body)
	}
}

// The editor is told the project's language, so the browser checks the
// page against that language's dictionary (spec-web.yaml 3-6-11).
func TestContentCarriesProjectLang(t *testing.T) {
	srv, root := testServer(t)
	writeConfig(t, root, "lang: fr\n"+testConfig)
	body := get(t, srv, "/content?path=index.qmd").Body.String()
	if !strings.Contains(body, `data-lang="fr"`) {
		t.Errorf("the page does not carry the project's language:\n%s", body)
	}
}

// The browser does the checking; what a Go test can check is that
// editor.js remembers the choice and hands it to CodeMirror.
func TestEditorWiresSpellcheck(t *testing.T) {
	file := "assets/static/editor.js"
	src, err := assets.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"var SPELL_KEY = 'spellcheck';",
		"var spellOn = localStorage.getItem(SPELL_KEY) === 'on';",
		"spellcheck: spellOn,",
		"function toggleSpell(",
	} {
		if !assetContains(file, string(src), want) {
			t.Errorf("%s does not wire spellchecking, missing %q", file, want)
		}
	}
	app := "assets/static/app.js"
	src, err = assets.ReadFile(app)
	if err != nil {
		t.Fatal(err)
	}
	if !assetContains(app, string(src), "toggleSpell();") {
		t.Errorf("%s does not toggle spellchecking on a click", app)
	}
}
