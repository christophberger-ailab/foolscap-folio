package web

import (
	"os"
	"path/filepath"
	"testing"
)

// A settings directory from before the rename moves to its new name, with
// everything in it; one already under the new name is left as it is.
func TestConfigFileInMovesLegacyDir(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, legacyConfigDirName)
	os.MkdirAll(filepath.Join(legacy, "custom-css"), 0o755)
	os.WriteFile(filepath.Join(legacy, configFileName), []byte("old"), 0o644)
	os.WriteFile(filepath.Join(legacy, "custom-css", "a.css"), []byte("x"), 0o644)

	got := configFileIn(dir)
	want := filepath.Join(dir, configDirName, configFileName)
	if got != want {
		t.Fatalf("configFileIn = %q, want %q", got, want)
	}
	if b, err := os.ReadFile(want); err != nil || string(b) != "old" {
		t.Errorf("config.cue not moved: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, configDirName, "custom-css", "a.css")); err != nil {
		t.Errorf("custom-css not moved: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("legacy dir still there: %v", err)
	}

	// Both present: the new one is used and the old one is not touched.
	os.MkdirAll(legacy, 0o755)
	if got := configFileIn(dir); got != want {
		t.Errorf("with both: %q", got)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Errorf("legacy dir removed although folio/ existed: %v", err)
	}
}
