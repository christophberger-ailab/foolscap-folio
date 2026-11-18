package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The settings file holds the wording of the section and chapter prefixes
// per language, ships with the English and German ones folio knows, and
// checks what is written there like any other setting (issue #96).
func TestStringsAreSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), configFileName)
	if err := saveConfigFile(path, defaultConfig()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	text := string(b)
	for _, want := range []string{"#Strings: {", "strings: {[string]: #Strings}", `section_prefix: "Teil"`, `chapter_prefix: "Chapter"`} {
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
	if err := write(`{strings: fr: {section_prefix: "Partie", chapter_prefix: "Chapitre"}}`); err != nil {
		t.Fatalf("French strings were refused: %v", err)
	}
	got, err := stringsIn(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["fr"]["section_prefix"] != "Partie" || got["fr"]["chapter_prefix"] != "Chapitre" {
		t.Errorf("strings = %v, want the French ones that were written", got)
	}
	if err := write(`{strings: fr: {part_prefix: "Partie"}}`); err == nil {
		t.Error("a string that does not exist was accepted")
	}
	if got, err := stringsIn(filepath.Join(t.TempDir(), "none.cue")); err != nil || got != nil {
		t.Errorf("no file: got %v, %v; want nil, nil", got, err)
	}
}
