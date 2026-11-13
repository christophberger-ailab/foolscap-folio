package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/christophberger-ailab/foolscap-folio/internal/selfupdate"
)

// releaseServer answers like GitHub's "latest release" endpoint.
func releaseServer(t *testing.T, version string) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"tag_name": version, "assets": []any{}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// updatingServer is a server running release current that looks for
// releases at src.
func updatingServer(t *testing.T, current string, src *httptest.Server) *server {
	t.Helper()
	s, err := newServer("")
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.Update.Source = src.URL
	s.update.current = current
	s.update.client = src.Client()
	return s
}

func TestUpdateButtonAppearsForANewerRelease(t *testing.T) {
	s := updatingServer(t, "v0.3.1", releaseServer(t, "v0.4.0"))
	if body := get(t, s, "/update").Body.String(); strings.Contains(body, "button") {
		t.Fatalf("an update is offered before any check:\n%s", body)
	}
	s.checkRelease(context.Background())
	body := get(t, s, "/update").Body.String()
	for _, want := range []string{
		`title="Update to v0.4.0. Your current release is v0.3.1."`,
		`hx-post="/update"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("update button lacks %q:\n%s", want, body)
		}
	}
}

func TestNoUpdateButtonWhenUpToDate(t *testing.T) {
	for _, latest := range []string{"v0.3.1", "v0.3.0"} {
		s := updatingServer(t, "v0.3.1", releaseServer(t, latest))
		s.checkRelease(context.Background())
		if body := get(t, s, "/update").Body.String(); strings.Contains(body, "button") {
			t.Errorf("latest %s: an update is offered:\n%s", latest, body)
		}
	}
}

func TestTheTopBarLoadsTheUpdateSlot(t *testing.T) {
	srv, _ := testServer(t)
	body := get(t, srv, "/").Body.String()
	if !strings.Contains(body, `id="update-slot"`) || !strings.Contains(body, `hx-get="/update"`) {
		t.Errorf("the top bar has no update slot:\n%s", body)
	}
}

func TestAFailedUpdateKeepsTheBinaryAndSaysWhy(t *testing.T) {
	s := updatingServer(t, "v0.3.1", releaseServer(t, "v0.4.0"))
	s.update.exe = filepath.Join(t.TempDir(), "folio")
	os.WriteFile(s.update.exe, []byte("old"), 0o755)
	s.checkRelease(context.Background())
	body := post(t, s, "/update", nil).Body.String()
	if !strings.Contains(body, "has no build for") {
		t.Errorf("the failure is not reported:\n%s", body)
	}
	if b, _ := os.ReadFile(s.update.exe); string(b) != "old" {
		t.Errorf("binary changed: %q", b)
	}
	if !strings.Contains(body, `hx-post="/update"`) {
		t.Errorf("the button is gone after a failure, so it cannot be retried:\n%s", body)
	}
}

func TestAnUpdateWithNothingToInstallIsRefused(t *testing.T) {
	s := updatingServer(t, "v0.3.1", releaseServer(t, "v0.3.1"))
	if rec := post(t, s, "/update", nil); rec.Code != http.StatusConflict {
		t.Errorf("status %d, want 409", rec.Code)
	}
}

func TestAnInstalledUpdateAsksForARestart(t *testing.T) {
	s := updatingServer(t, "v0.3.1", releaseServer(t, "v0.4.0"))
	s.update.installed = "v0.4.0"
	body := get(t, s, "/update").Body.String()
	if !strings.Contains(body, "Restart folio web") || strings.Contains(body, "button") {
		t.Errorf("no restart notice:\n%s", body)
	}
}

func TestCheckInterval(t *testing.T) {
	for check, want := range map[string]time.Duration{
		"daily": 24 * time.Hour, "weekly": 7 * 24 * time.Hour, "never": 0,
	} {
		if got := (updateConfig{Check: check}).interval(); got != want {
			t.Errorf("%s: %v, want %v", check, got, want)
		}
	}
}

func TestUpdateSettingsDefaultAndValidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), configFileName)
	if err := saveConfigFile(path, defaultConfig()); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Update.Source != selfupdate.DefaultSource || cfg.Update.Check != "daily" {
		t.Errorf("update defaults = %+v", cfg.Update)
	}
	good, _ := os.ReadFile(path)
	for edit, want := range map[string]string{
		`config: update: source: "http://example.com/latest"`: "source",
		`config: update: check: "hourly"`:                     "check",
	} {
		broken := filepath.Join(t.TempDir(), configFileName)
		os.WriteFile(broken, append(append([]byte{}, good...), "\n"+edit+"\n"...), 0o600)
		if _, err := loadConfigFile(broken); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", edit, err)
		}
	}
}

func TestUpdateSourceReadsTheSettingsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)
	if got, err := updateSourceIn(path); err != nil || got != selfupdate.DefaultSource {
		t.Errorf("without a file: %q, %v", got, err)
	}
	cfg := defaultConfig()
	cfg.Update.Source = "https://git.example.com/api/v1/repos/me/folio/releases/latest"
	if err := saveConfigFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	if got, err := updateSourceIn(path); err != nil || got != cfg.Update.Source {
		t.Errorf("from the file: %q, %v", got, err)
	}
}

func TestConfigPageShowsReleaseInfoFirst(t *testing.T) {
	s := updatingServer(t, "v0.3.1", releaseServer(t, "v0.4.0"))
	body := get(t, s, "/config").Body.String()
	box := strings.Index(body, `class="release-info"`)
	first := strings.Index(body, `class="config-list"`)
	if box < 0 || first < 0 || box > first {
		t.Fatalf("release info is missing or not above the settings:\n%s", body)
	}
	for _, want := range []string{"v0.3.1", `action="/config/check-update"`, ">Check for updates</button>"} {
		if !strings.Contains(body, want) {
			t.Errorf("release info lacks %q:\n%s", want, body)
		}
	}
}

func TestCheckForUpdatesFromTheConfigPage(t *testing.T) {
	s := updatingServer(t, "v0.3.1", releaseServer(t, "v0.4.0"))
	body := post(t, s, "/config/check-update", nil).Body.String()
	if !strings.Contains(body, "v0.4.0 is available") {
		t.Errorf("the newer release is not reported:\n%s", body)
	}
	if !strings.Contains(get(t, s, "/update").Body.String(), `hx-post="/update"`) {
		t.Error("the top bar does not offer the release the check found")
	}

	s = updatingServer(t, "v0.3.1", releaseServer(t, "v0.3.1"))
	if body := post(t, s, "/config/check-update", nil).Body.String(); !strings.Contains(body, "up to date") {
		t.Errorf("no up-to-date notice:\n%s", body)
	}
}

func TestCheckForUpdatesReportsAFailure(t *testing.T) {
	src := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(src.Close)
	s := updatingServer(t, "v0.3.1", src)
	if body := post(t, s, "/config/check-update", nil).Body.String(); !strings.Contains(body, "Could not check for updates") {
		t.Errorf("the failure is not reported:\n%s", body)
	}
}

// A development build has no version to compare, so whatever release the
// source names is offered, in the top bar and on the Config page alike.
func TestADevelopmentBuildIsOfferedTheNewestRelease(t *testing.T) {
	s := updatingServer(t, "dev", releaseServer(t, "v0.4.0"))
	body := post(t, s, "/config/check-update", nil).Body.String()
	if !strings.Contains(body, "v0.4.0 is available") || strings.Contains(body, "disabled") {
		t.Errorf("the Config page does not offer the release to a dev build:\n%s", body)
	}
	if !strings.Contains(get(t, s, "/update").Body.String(), `hx-post="/update"`) {
		t.Error("the top bar does not offer the release to a dev build")
	}
}

func TestTheConfigPageOffersTheUpdateItself(t *testing.T) {
	s := updatingServer(t, "v0.3.1", releaseServer(t, "v0.4.0"))
	s.checkRelease(context.Background())
	body := get(t, s, "/config").Body.String()
	for _, want := range []string{`action="/config/install-update"`, ">Update to v0.4.0</button>", `data-confirm="Update folio from v0.3.1 to v0.4.0? folio web restarts with it."`, "/static/config.js"} {
		if !strings.Contains(body, want) {
			t.Errorf("the release box lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "button in the app's top bar") {
		t.Errorf("the release box still sends the user to the top bar:\n%s", body)
	}
}

func TestAnUpdateFromTheConfigPageThatFailsSaysWhy(t *testing.T) {
	s := updatingServer(t, "v0.3.1", releaseServer(t, "v0.4.0"))
	s.update.exe = filepath.Join(t.TempDir(), "folio")
	os.WriteFile(s.update.exe, []byte("old"), 0o755)
	s.checkRelease(context.Background())
	body := post(t, s, "/config/install-update", nil).Body.String()
	if !strings.Contains(body, "has no build for") || !strings.Contains(body, `action="/config/install-update"`) {
		t.Errorf("the failure is not reported, or the button is gone:\n%s", body)
	}
	if b, _ := os.ReadFile(s.update.exe); string(b) != "old" {
		t.Errorf("binary changed: %q", b)
	}
}

func TestTheConfigPageAsksForARestartOnceInstalled(t *testing.T) {
	s := updatingServer(t, "v0.3.1", releaseServer(t, "v0.4.0"))
	s.update.installed = "v0.4.0"
	body := get(t, s, "/config").Body.String()
	if !strings.Contains(body, "Restart folio web to use it") || strings.Contains(body, `action="/config/install-update"`) {
		t.Errorf("no restart notice, or still an update button:\n%s", body)
	}
}

func TestVersionAnswersTheRunningRelease(t *testing.T) {
	s := updatingServer(t, "v0.3.1", releaseServer(t, "v0.4.0"))
	if body := get(t, s, "/version").Body.String(); body != "v0.3.1" {
		t.Errorf("/version = %q", body)
	}
}

// A server that can restart itself does so after an install, and the page
// waits for the new release to answer, then reloads.
func TestAnInstalledUpdateRestartsWhenItCan(t *testing.T) {
	s := updatingServer(t, "v0.3.1", releaseServer(t, "v0.4.0"))
	s.restart = make(chan struct{}, 1)
	s.update.installed = "v0.4.0"
	for _, page := range []string{"/update", "/config"} {
		body := get(t, s, page).Body.String()
		if !strings.Contains(body, `data-restart-to="v0.4.0"`) || !strings.Contains(body, "Restarting folio web") {
			t.Errorf("%s does not wait for the restart:\n%s", page, body)
		}
	}
	for _, page := range []string{"/", "/config"} {
		if body := get(t, s, page).Body.String(); !strings.Contains(body, "/static/restart.js") {
			t.Errorf("%s does not load restart.js", page)
		}
	}
}

func TestARestartIsAskedForOnce(t *testing.T) {
	s, err := newServer("")
	if err != nil {
		t.Fatal(err)
	}
	s.requestRestart() // no channel: nothing to do, and no panic
	s.restart = make(chan struct{}, 1)
	s.requestRestart()
	s.requestRestart() // must not block
	if len(s.restart) != 1 {
		t.Errorf("%d restarts pending, want 1", len(s.restart))
	}
}

func TestARestartKeepsTheAddressAndProject(t *testing.T) {
	t.Setenv(restartAddrEnv, "localhost:8250")
	t.Setenv(restartRootEnv, "/books/two")
	addr, root := takeRestartEnv()
	if addr != "localhost:8250" || root != "/books/two" {
		t.Errorf("takeRestartEnv = %q, %q", addr, root)
	}
	if os.Getenv(restartAddrEnv) != "" || os.Getenv(restartRootEnv) != "" {
		t.Error("the restart variables are left for the next process")
	}
}
