package web

import (
	"path/filepath"
	"testing"
)

// The user's brand file lies beside the settings file.
func TestUserBrandIsBesideTheSettings(t *testing.T) {
	dir := t.TempDir()
	if got := userBrandIn(filepath.Join(dir, "config.cue")); got != filepath.Join(dir, "_brand.yml") {
		t.Errorf("user brand = %q", got)
	}
	if got := userBrandIn(""); got != "" {
		t.Errorf("without a settings file: %q", got)
	}
}

// A render from the panel knows the user's brand file.
func TestThePanelsRendersGetTheUsersBrand(t *testing.T) {
	srv, _ := testServer(t)
	srv.mu.Lock()
	want := userBrandIn(srv.configFile)
	sels, err := srv.selections(projectPrefs{Topics: []string{"chapter2"}, Formats: []string{"pdf"}})
	srv.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(sels) == 0 || sels[0].Config.UserBrand != want {
		t.Errorf("user brand: %+v, want %q", sels, want)
	}
}
