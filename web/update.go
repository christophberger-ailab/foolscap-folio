package web

// update.go — the top bar's Update button.
//
// While folio web runs it looks for a new release as often as the update
// section of config.cue says (daily by default), and when it finds one the
// top bar shows a button that installs it. The page asks for the button
// itself (GET /update) when it loads and every hour after, so a release
// found while the page is open still shows up.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/christophberger-ailab/foolscap-folio/internal/selfupdate"
)

// updateConfig is the update section of config.cue.
type updateConfig struct {
	// Source is the URL of the newest release's description.
	Source string `json:"source"`
	// Check is how often folio web looks for one: daily, weekly or never.
	Check string `json:"check"`
}

const checkDaily = "daily"

// interval is how long folio web waits between checks; 0 means it does not
// check at all.
func (u updateConfig) interval() time.Duration {
	switch u.Check {
	case "never":
		return 0
	case "weekly":
		return 7 * 24 * time.Hour
	}
	return 24 * time.Hour
}

// UpdateSource is where `folio upgrade` looks for releases: the source the
// settings file names, or the default when there is no file.
func UpdateSource() (string, error) { return updateSourceIn(defaultConfigFile()) }

func updateSourceIn(path string) (string, error) {
	if path == "" {
		return selfupdate.DefaultSource, nil
	}
	cfg, err := loadConfigFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return selfupdate.DefaultSource, nil
	}
	return cfg.Update.Source, err
}

// updateState is what the server knows about releases. It has its own
// lock: a check or a download must not block the page.
type updateState struct {
	mu sync.Mutex
	// current is the running release; client and exe are what a check
	// and an install use. Serve sets them.
	current string
	client  *http.Client
	exe     string
	// latest is a release newer than current, once a check found one.
	latest *selfupdate.Release
	// installed is the release put in place of the binary, which takes
	// effect when folio web is started again.
	installed string
	busy      bool
	// checked is when a check last got an answer.
	checked time.Time
}

// updateView is the top bar's update slot.
type updateView struct {
	Current, Latest, Installed, Error string
	// Restarting is an installed update the server restarts with.
	Restarting bool
}

func (u *updateState) view() updateView {
	v := updateView{Current: u.current, Installed: u.installed}
	if u.latest != nil {
		v.Latest = u.latest.Version
	}
	return v
}

// startUpdateChecks sets the server up to update the binary of release
// current, and starts looking for new releases. A development build looks
// too: it has no release to compare against, so it is offered the newest.
func (s *server) startUpdateChecks(current string) {
	s.update.mu.Lock()
	s.update.current = current
	s.update.mu.Unlock()
	exe, err := selfupdate.Executable()
	if err != nil {
		return
	}
	s.update.mu.Lock()
	s.update.client, s.update.exe = selfupdate.Client(), exe
	s.update.mu.Unlock()
	go s.watchReleases()
}

// watchReleases checks for a new release whenever the configured interval
// has passed. It looks at the setting every hour, so a change to it takes
// effect without a restart.
func (s *server) watchReleases() {
	var last time.Time
	for ; ; time.Sleep(time.Hour) {
		s.mu.Lock()
		every := s.cfg.Update.interval()
		s.mu.Unlock()
		if every > 0 && time.Since(last) >= every {
			last = time.Now()
			if err := s.checkRelease(context.Background()); err != nil {
				fmt.Fprintf(os.Stderr, "folio web: cannot check for updates: %v\n", err)
			}
		}
	}
}

// checkRelease asks the configured source for the newest release and
// remembers it when it is newer than the running one. The periodic checks
// only report a failure on the console: there is nothing the page could do
// about it, and the next check may well succeed.
func (s *server) checkRelease(ctx context.Context) error {
	s.mu.Lock()
	source := s.cfg.Update.Source
	s.mu.Unlock()
	s.update.mu.Lock()
	client, current := s.update.client, s.update.current
	s.update.mu.Unlock()
	if client == nil {
		return errors.New("folio cannot find its own binary to update")
	}

	rel, err := selfupdate.Latest(ctx, client, source)
	if err != nil {
		return err
	}
	s.update.mu.Lock()
	defer s.update.mu.Unlock()
	s.update.checked = time.Now()
	if offered(current, rel.Version) {
		s.update.latest = &rel
	}
	return nil
}

// offered reports whether latest is worth installing over current: a newer
// release, or any release over a development build, which cannot be
// compared with one.
func offered(current, latest string) bool {
	return selfupdate.IsRelease(latest) && (!selfupdate.IsRelease(current) || selfupdate.Newer(current, latest))
}

// releaseView is the release box atop the Config overview.
type releaseView struct {
	Current, Latest, LatestURL, Installed string
	// Dev is a development build, which is offered any release.
	Dev bool
	// Restarting is an installed update the server restarts with.
	Restarting bool
	// Checked is when a check last succeeded; Check how often they run.
	Checked, Check string
	Message, Error string
}

// releaseView describes the running release and what the checks found.
// The caller must hold s.mu.
func (s *server) releaseView() releaseView {
	u := &s.update
	u.mu.Lock()
	defer u.mu.Unlock()
	v := releaseView{Current: u.current, Installed: u.installed, Dev: !selfupdate.IsRelease(u.current), Check: s.cfg.Update.Check}
	if u.latest != nil {
		v.Latest, v.LatestURL = u.latest.Version, u.latest.URL
	}
	v.Restarting = v.Installed != "" && s.restart != nil
	if !u.checked.IsZero() {
		v.Checked = u.checked.Format("2006-01-02 15:04")
	}
	return v
}

// checkUpdateHandler is the release box's Check for updates button.
func (s *server) checkUpdateHandler(w http.ResponseWriter, r *http.Request) {
	err := s.checkRelease(r.Context())
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.configView()
	switch {
	case err != nil:
		v.Release.Error = "Could not check for updates: " + err.Error()
	case v.Release.Latest == "":
		v.Release.Message = "folio is up to date."
	}
	s.render(w, "config-page", v)
}

func (s *server) updateHandler(w http.ResponseWriter, r *http.Request) {
	s.render(w, "update", s.updateView())
}

// updateView is the top bar's update slot as it stands.
func (s *server) updateView() updateView {
	s.update.mu.Lock()
	defer s.update.mu.Unlock()
	v := s.update.view()
	v.Restarting = v.Installed != "" && s.restart != nil
	return v
}

// versionHandler answers the running release: after an update, the page
// asks it until the restarted server answers with the new one.
func (s *server) versionHandler(w http.ResponseWriter, r *http.Request) {
	s.update.mu.Lock()
	defer s.update.mu.Unlock()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, s.update.current)
}

// restartDelay gives the response that reports an install the time to
// reach the browser before the server goes away.
const restartDelay = 500 * time.Millisecond

// requestRestart asks Serve to restart on the new binary. A server that
// Serve did not start (tests) has no one to ask; a second request while
// one is pending is dropped.
func (s *server) requestRestart() {
	if s.restart == nil {
		return
	}
	select {
	case s.restart <- struct{}{}:
	default:
	}
}

// errNothingToInstall is an install asked for when there is no release
// waiting, one is already installed, or one is being installed.
var errNothingToInstall = errors.New("there is no update to install")

// installUpdate puts the release a check found in place of the running
// binary. The top bar's Update button and the Config page's both run it.
func (s *server) installUpdate(ctx context.Context) error {
	u := &s.update
	u.mu.Lock()
	if u.latest == nil || u.installed != "" || u.busy {
		u.mu.Unlock()
		return errNothingToInstall
	}
	u.busy = true
	rel, client, exe := *u.latest, u.client, u.exe
	u.mu.Unlock()

	err := selfupdate.Install(ctx, client, rel, exe)

	u.mu.Lock()
	defer u.mu.Unlock()
	u.busy = false
	if err == nil {
		u.installed = rel.Version
		fmt.Fprintf(os.Stderr, "folio web: updated to %s\n", rel.Version)
		time.AfterFunc(restartDelay, s.requestRestart)
	}
	return err
}

// installUpdateHandler is the top bar's Update button.
func (s *server) installUpdateHandler(w http.ResponseWriter, r *http.Request) {
	err := s.installUpdate(r.Context())
	v := s.updateView()
	switch {
	case errors.Is(err, errNothingToInstall):
		w.WriteHeader(http.StatusConflict)
	case err != nil:
		v.Error = err.Error()
	}
	s.render(w, "update", v)
}

// installUpdateConfigHandler is the Config page's Update button.
func (s *server) installUpdateConfigHandler(w http.ResponseWriter, r *http.Request) {
	err := s.installUpdate(r.Context())
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.configView()
	if err != nil {
		v.Release.Error = "Not updated: " + err.Error()
	}
	s.render(w, "config-page", v)
}

// The restarted server listens where the old one did and opens the project
// that was open, rather than the one its command line named: the page
// waiting for it asks the same address, and reloads into the same folio.
const (
	restartAddrEnv = "FOLIO_RESTART_ADDR"
	restartRootEnv = "FOLIO_RESTART_ROOT"
)

// takeRestartEnv reads what a restart handed over and clears it, so a
// process folio web starts in turn does not inherit it.
func takeRestartEnv() (addr, root string) {
	addr, root = os.Getenv(restartAddrEnv), os.Getenv(restartRootEnv)
	os.Unsetenv(restartAddrEnv)
	os.Unsetenv(restartRootEnv)
	return addr, root
}
