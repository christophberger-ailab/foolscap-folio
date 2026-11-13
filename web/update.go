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
}

// updateView is the top bar's update slot.
type updateView struct {
	Current, Latest, Installed, Error string
}

func (u *updateState) view() updateView {
	v := updateView{Current: u.current, Installed: u.installed}
	if u.latest != nil {
		v.Latest = u.latest.Version
	}
	return v
}

// startUpdateChecks sets the server up to update the binary of release
// current, and starts looking for new releases -- unless this is a
// development build, which has no release to compare against.
func (s *server) startUpdateChecks(current string) {
	exe, err := selfupdate.Executable()
	if err != nil || !selfupdate.IsRelease(current) {
		return
	}
	s.update.mu.Lock()
	s.update.current, s.update.client, s.update.exe = current, selfupdate.Client(), exe
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
			s.checkRelease(context.Background())
		}
	}
}

// checkRelease asks the configured source for the newest release and
// remembers it when it is newer than the running one. A failed check is
// only reported on the console: there is nothing the page could do about
// it, and the next check may well succeed.
func (s *server) checkRelease(ctx context.Context) {
	s.mu.Lock()
	source := s.cfg.Update.Source
	s.mu.Unlock()
	s.update.mu.Lock()
	client, current := s.update.client, s.update.current
	s.update.mu.Unlock()

	rel, err := selfupdate.Latest(ctx, client, source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "folio web: cannot check for updates: %v\n", err)
		return
	}
	if selfupdate.Newer(current, rel.Version) {
		s.update.mu.Lock()
		s.update.latest = &rel
		s.update.mu.Unlock()
	}
}

func (s *server) updateHandler(w http.ResponseWriter, r *http.Request) {
	s.update.mu.Lock()
	v := s.update.view()
	s.update.mu.Unlock()
	s.render(w, "update", v)
}

// installUpdateHandler puts the release the button offers in place of the
// running binary.
func (s *server) installUpdateHandler(w http.ResponseWriter, r *http.Request) {
	u := &s.update
	u.mu.Lock()
	if u.latest == nil || u.installed != "" || u.busy {
		v := u.view()
		u.mu.Unlock()
		w.WriteHeader(http.StatusConflict)
		s.render(w, "update", v)
		return
	}
	u.busy = true
	rel, client, exe := *u.latest, u.client, u.exe
	u.mu.Unlock()

	err := selfupdate.Install(r.Context(), client, rel, exe)

	u.mu.Lock()
	defer u.mu.Unlock()
	u.busy = false
	if err == nil {
		u.installed = rel.Version
		fmt.Fprintf(os.Stderr, "folio web: updated to %s; restart folio web to use it\n", rel.Version)
	}
	v := u.view()
	if err != nil {
		v.Error = err.Error()
	}
	s.render(w, "update", v)
}
