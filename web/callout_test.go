package web

import (
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The settings file says how callouts are titled per language, ships with
// the English and German titles folio knows, and checks what is written
// there like any other setting.
func TestCalloutTitlesAreSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), configFileName)
	if err := saveConfigFile(path, defaultConfig()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	text := string(b)
	for _, want := range []string{"#Callouts: {", "callouts: {[string]: #Callouts}", `note:      "Hinweis"`} {
		if !strings.Contains(text, want) {
			t.Errorf("the settings file is missing %q:\n%s", want, text)
		}
	}

	schema := text[:strings.Index(text, "config: #Config &")]
	write := func(settings string) error {
		if err := os.WriteFile(path, []byte(schema+"config: #Config & "+settings+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := loadConfigFile(path)
		return err
	}
	if err := write(`{callouts: fr: {note: "Remarque", tip: "Astuce"}}`); err != nil {
		t.Fatalf("French titles were refused: %v", err)
	}
	got, err := calloutTitlesIn(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["fr"]["note"] != "Remarque" || got["fr"]["tip"] != "Astuce" {
		t.Errorf("callouts = %v, want the French titles that were written", got)
	}
	if err := write(`{callouts: fr: {notice: "Remarque"}}`); err == nil {
		t.Error("a callout type that does not exist was accepted")
	}

	// Without a settings file there is nothing of the user's to add.
	if got, err := calloutTitlesIn(filepath.Join(t.TempDir(), "none.cue")); err != nil || got != nil {
		t.Errorf("no file: got %v, %v; want nil, nil", got, err)
	}
}

// The page's preview titles callouts in the project's language, from the
// user's own titles: the page carries them, resolved for that language, for
// preview.js to read.
func TestThePreviewGetsTheCalloutTitlesOfTheProjectsLanguage(t *testing.T) {
	srv, root := testServer(t)
	writeConfig(t, root, "lang: fr\n"+testConfig)
	srv.mu.Lock()
	srv.cfg.Callouts = map[string]map[string]string{"fr": {"note": "Remarque"}}
	srv.mu.Unlock()

	body := html.UnescapeString(get(t, srv, "/content?path=chapter2/second.qmd").Body.String())
	for _, want := range []string{`data-callout-titles=`, `"note":"Remarque"`, `"tip":"Tip"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not carry %q:\n%s", want, body)
		}
	}

	// A render from the panel titles them alike.
	srv.mu.Lock()
	sels, err := srv.selections(projectPrefs{Topics: []string{"chapter2"}, Formats: []string{"pdf"}})
	srv.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(sels) == 0 || sels[0].Config.CalloutTitles["fr"]["note"] != "Remarque" {
		t.Errorf("the render does not get the user's callout titles: %+v", sels)
	}
}
