package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestConfigPageListsTheFileEditors(t *testing.T) {
	srv, _ := configTestServer(t)
	body := get(t, srv, "/config").Body.String()
	for _, want := range []string{
		`href="/config/project-file"`, "Project: folio.yml",
		`href="/config/settings-file"`, "folio: config.cue",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("config page missing %q:\n%s", want, body)
		}
	}
}

func TestProjectFileEditorShowsFolioYML(t *testing.T) {
	srv, _ := configTestServer(t)
	rec := get(t, srv, "/config/project-file")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<textarea name="text" class="config-file-content" data-mode="yaml"`,
		"audiences: [std]",
		`src="/static/codemirror/yaml.js"`,
		`action="/config/project-file"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("editor page missing %q:\n%s", want, body)
		}
	}
}

func TestProjectFileSaveWritesValidYAML(t *testing.T) {
	srv, root := configTestServer(t)
	text := "title: Renamed\nformats:\n  pdf: {type: pdf}\n"
	rec := post(t, srv, "/config/project-file", url.Values{"text": {text}})
	if !strings.Contains(rec.Body.String(), "Saved.") {
		t.Fatalf("save not confirmed:\n%s", rec.Body)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "folio.yml")); string(got) != text {
		t.Errorf("folio.yml = %q, want %q", got, text)
	}
}

func TestProjectFileSaveRefusesInvalidYAML(t *testing.T) {
	srv, root := configTestServer(t)
	before, _ := os.ReadFile(filepath.Join(root, "folio.yml"))
	for _, text := range []string{"nosuchkey: 1\n", "formats: [\n", "formats:\n  x: {type: exe}\n"} {
		rec := post(t, srv, "/config/project-file", url.Values{"text": {text}})
		body := rec.Body.String()
		if !strings.Contains(body, `class="error"`) || strings.Contains(body, "Saved.") {
			t.Errorf("%q: not refused:\n%s", text, body)
		}
		if !strings.Contains(body, strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])) {
			t.Errorf("%q: the editor lost the text that was refused:\n%s", text, body)
		}
		if got, _ := os.ReadFile(filepath.Join(root, "folio.yml")); string(got) != string(before) {
			t.Errorf("%q: folio.yml changed to %q", text, got)
		}
	}
}

func TestProjectFileEditsTheLegacyFile(t *testing.T) {
	srv, root := configTestServer(t)
	os.Rename(filepath.Join(root, "folio.yml"), filepath.Join(root, "qm.yml"))
	if body := get(t, srv, "/config/project-file").Body.String(); !strings.Contains(body, "qm.yml") {
		t.Errorf("editor does not name qm.yml:\n%s", body)
	}
	post(t, srv, "/config/project-file", url.Values{"text": {"title: Legacy\n"}})
	if got, _ := os.ReadFile(filepath.Join(root, "qm.yml")); string(got) != "title: Legacy\n" {
		t.Errorf("qm.yml = %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "folio.yml")); err == nil {
		t.Error("a folio.yml was written beside qm.yml")
	}
}

func TestProjectFileCreatesAMissingFile(t *testing.T) {
	srv, root := configTestServer(t)
	os.Remove(filepath.Join(root, "folio.yml"))
	post(t, srv, "/config/project-file", url.Values{"text": {"title: New\n"}})
	if got, _ := os.ReadFile(filepath.Join(root, "folio.yml")); string(got) != "title: New\n" {
		t.Errorf("folio.yml = %q", got)
	}
}

func TestProjectFileNeedsAnOpenProject(t *testing.T) {
	srv, err := newServer("")
	if err != nil {
		t.Fatal(err)
	}
	if body := get(t, srv, "/config/project-file").Body.String(); !strings.Contains(body, "Open a project") || strings.Contains(body, "<textarea") {
		t.Errorf("editor offered without a project:\n%s", body)
	}
	if body := post(t, srv, "/config/project-file", url.Values{"text": {"title: X\n"}}).Body.String(); !strings.Contains(body, "Open a project") {
		t.Errorf("save accepted without a project:\n%s", body)
	}
}

func TestProjectFileMissingFieldKeepsTheFile(t *testing.T) {
	srv, root := configTestServer(t)
	before, _ := os.ReadFile(filepath.Join(root, "folio.yml"))
	body := post(t, srv, "/config/project-file", url.Values{}).Body.String()
	if !strings.Contains(body, `class="error"`) {
		t.Errorf("missing field not reported:\n%s", body)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "folio.yml")); string(got) != string(before) {
		t.Errorf("folio.yml changed to %q", got)
	}
}

func TestSettingsFileEditorShowsConfigCUE(t *testing.T) {
	srv, _ := configTestServer(t)
	body := get(t, srv, "/config/settings-file").Body.String()
	for _, want := range []string{
		`<textarea name="text" class="config-file-content"`,
		"#Config: {",
		"config: #Config &amp; {",
		`action="/config/settings-file"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("editor page missing %q:\n%s", want, body)
		}
	}
}

// settingsWith returns the settings file as the server would write it, with
// edit applied to the settings.
func settingsWith(t *testing.T, edit func(*storedConfig)) string {
	t.Helper()
	cfg := defaultConfig()
	edit(&cfg)
	b, err := renderConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSettingsFileSaveAppliesAndWrites(t *testing.T) {
	srv, _ := configTestServer(t)
	text := settingsWith(t, func(c *storedConfig) { c.User.Name = "Ada Edited" })
	body := post(t, srv, "/config/settings-file", url.Values{"text": {text}}).Body.String()
	if !strings.Contains(body, "Saved.") {
		t.Fatalf("save not confirmed:\n%s", body)
	}
	if srv.cfg.User.Name != "Ada Edited" {
		t.Errorf("running settings not updated: user name %q", srv.cfg.User.Name)
	}
	cfg, err := loadConfigFile(srv.configFile)
	if err != nil || cfg.User.Name != "Ada Edited" {
		t.Errorf("file not updated: %v, user name %q", err, cfg.User.Name)
	}
}

// hourly finds the update check setting, to set it to a value the schema
// does not allow.
var hourly = regexp.MustCompile(`check:\s+"daily"`)

func TestSettingsFileSaveRefusesInvalidCUE(t *testing.T) {
	srv, _ := configTestServer(t)
	before, _ := os.ReadFile(srv.configFile)
	prev := srv.cfg.User.Name
	for _, text := range []string{
		"config: {",
		hourly.ReplaceAllString(settingsWith(t, func(*storedConfig) {}), `check: "hourly"`),
		// A schema loosened by hand must not let the settings past the
		// schema the binary checks against.
		hourly.ReplaceAllString(strings.Replace(settingsWith(t, func(*storedConfig) {}), `check: *"daily" | "weekly" | "never"`, `check: string`, 1), `check: "hourly"`),
	} {
		body := post(t, srv, "/config/settings-file", url.Values{"text": {text}}).Body.String()
		if !strings.Contains(body, `class="error"`) || strings.Contains(body, "Saved.") {
			t.Errorf("not refused:\n%s", body)
		}
		if got, _ := os.ReadFile(srv.configFile); string(got) != string(before) {
			t.Errorf("config.cue changed")
		}
		if srv.cfg.User.Name != prev || srv.cfg.Update.Check != checkDaily {
			t.Errorf("running settings changed")
		}
	}
}

func TestSettingsFileHidesAPIKeys(t *testing.T) {
	srv, _ := configTestServer(t)
	srv.cfg.Copyedit.Connections = []apiConnection{
		{ID: "c1", Name: "One", Kind: "anthropic", BaseURL: "https://api.example.com/v1", Model: "m", Key: "sk-secret-one"},
		{ID: "c2", Name: "Two", Kind: "openai", BaseURL: "https://api.example.com/v1", Model: "m", Key: "sk-secret-two"},
	}
	srv.saveConfig()
	text := get(t, srv, "/config/settings-file").Body.String()
	if strings.Contains(text, "sk-secret") {
		t.Fatalf("an API key is sent to the browser:\n%s", text)
	}
	if !strings.Contains(text, keyHidden) {
		t.Fatalf("hidden keys are not marked:\n%s", text)
	}

	// Saved back as shown, the keys are kept; a key typed in replaces the
	// stored one.
	edited := settingsWith(t, func(c *storedConfig) {
		c.Copyedit.Connections = []apiConnection{
			{ID: "c1", Name: "One renamed", Kind: "anthropic", BaseURL: "https://api.example.com/v1", Model: "m", Key: keyHidden},
			{ID: "c2", Name: "Two", Kind: "openai", BaseURL: "https://api.example.com/v1", Model: "m", Key: "sk-new-two"},
		}
	})
	if body := post(t, srv, "/config/settings-file", url.Values{"text": {edited}}).Body.String(); !strings.Contains(body, "Saved.") {
		t.Fatalf("save not confirmed:\n%s", body)
	}
	conns := srv.cfg.Copyedit.Connections
	if conns[0].Name != "One renamed" || conns[0].Key != "sk-secret-one" || conns[1].Key != "sk-new-two" {
		t.Errorf("connections after save: %+v", conns)
	}

	// The marker on a connection that has no key stored is refused rather
	// than written as the key.
	bogus := settingsWith(t, func(c *storedConfig) {
		c.Copyedit.Connections = []apiConnection{{ID: "c9", Name: "New", Kind: "openai", BaseURL: "https://x.example", Model: "m", Key: keyHidden}}
	})
	if body := post(t, srv, "/config/settings-file", url.Values{"text": {bogus}}).Body.String(); !strings.Contains(body, `class="error"`) {
		t.Errorf("a hidden key with nothing behind it was accepted:\n%s", body)
	}
}

// A web page open in another tab must not be able to post to the app: the
// config editors write files, and config.cue names the commands the app runs.
func TestCrossOriginPostsAreRefused(t *testing.T) {
	srv, root := configTestServer(t)
	before, _ := os.ReadFile(filepath.Join(root, "folio.yml"))
	req := httptest.NewRequest("POST", "/config/project-file", strings.NewReader(url.Values{"text": {"title: Evil\n"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-site post: status %d, want 403", rec.Code)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "folio.yml")); string(got) != string(before) {
		t.Errorf("folio.yml changed to %q", got)
	}
}
