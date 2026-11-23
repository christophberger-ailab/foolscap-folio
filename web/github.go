package web

// github.go — signing in to GitHub, so that folio can clone, push, and sync
// a user's repositories without the user setting up git credentials first.
//
// The sign-in is GitHub's device flow, the one the GitHub CLI uses: folio
// asks GitHub for a code, the user enters it at github.com/login/device in
// their own browser, and folio polls until GitHub hands over a token. It
// needs no client secret and no redirect back to folio, which is what a
// program on the user's machine, listening on localhost, can do with
// neither. What it does need is an OAuth app registered on GitHub with the
// device flow enabled; githubClientID names it.
//
// The token is handed to git (see gitrepo.SetGitHubToken), which sends it
// to github.com only. It is kept beside the settings file, in a file of
// its own that only its owner can read, rather than in the settings,
// which the Config page shows and the user edits.

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/christophberger-ailab/foolscap-folio/internal/gitrepo"
)

// githubClientID is the client ID of folio's OAuth app on GitHub. A client
// ID is not a secret: the device flow is designed for programs that cannot
// keep one. FOLIO_GITHUB_CLIENT_ID overrides it; with neither, folio offers
// no sign-in and git uses the user's own credentials.
var githubClientID = ""

// githubBase and githubAPI are where GitHub is; tests point them elsewhere.
var (
	githubBase = "https://github.com"
	githubAPI  = "https://api.github.com"
)

// githubTokenFile is the file beside the settings that keeps the sign-in.
const githubTokenFile = "github-token.json"

var githubHTTP = &http.Client{Timeout: 30 * time.Second}

// pollUnit is what the poll interval GitHub names is counted in; tests
// make it nothing so as not to wait.
var pollUnit = time.Second

func clientID() string {
	if id := strings.TrimSpace(os.Getenv("FOLIO_GITHUB_CLIENT_ID")); id != "" {
		return id
	}
	return githubClientID
}

// githubState is the sign-in, and the one under way. It has a lock of its
// own: a poll talks to GitHub, and the tree handlers must not wait on it.
type githubState struct {
	mu    sync.Mutex
	token string
	login string
	// device is the sign-in waiting for the user to enter its code. Its
	// device code is what the token is asked for with, and stays here:
	// the page only ever sees the code the user types.
	device *deviceCode
}

type deviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	Interval        int    `json:"interval"`
	ExpiresIn       int    `json:"expires_in"`

	expires time.Time
	next    time.Time
}

// githubView is the GitHub line of the clone form.
type githubView struct {
	// Offered says sign-in is possible at all: folio has a client ID.
	Offered bool
	Login   string
	// UserCode, VerifyURL, and Interval are the sign-in under way: the
	// code to enter, where, and how many seconds to wait between polls.
	UserCode  string
	VerifyURL string
	Interval  int
	Error     string
}

// githubView reports the sign-in as it stands.
func (s *server) githubView() githubView {
	g := &s.github
	g.mu.Lock()
	defer g.mu.Unlock()
	v := githubView{Offered: clientID() != "", Login: g.login}
	if d := g.device; d != nil && v.Login == "" {
		v.UserCode, v.VerifyURL, v.Interval = d.UserCode, d.VerificationURI, d.Interval
	}
	return v
}

func (s *server) renderGitHub(w http.ResponseWriter, errMsg string) {
	v := s.githubView()
	v.Error = errMsg
	s.render(w, "github-status", v)
}

// githubTokenPath is the file the sign-in is kept in, "" when settings are
// kept to this run.
func (s *server) githubTokenPath() string {
	if s.configFile == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(s.configFile), githubTokenFile)
}

type storedToken struct {
	Token string `json:"token"`
	Login string `json:"login"`
}

// loadGitHubToken picks up the sign-in kept from an earlier run. A file
// that cannot be read is a sign-in that is not there: the user signs in
// again, which is all there is to lose.
func (s *server) loadGitHubToken() {
	p := s.githubTokenPath()
	if p == "" {
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	var t storedToken
	if json.Unmarshal(b, &t) != nil || t.Token == "" {
		return
	}
	s.github.token, s.github.login = t.Token, t.Login
	gitrepo.SetGitHubToken(t.Token)
}

// setGitHubToken signs in (or, with "", out): in memory, in git, and on
// disk. The caller must hold s.github.mu.
func (s *server) setGitHubToken(token, login string) error {
	g := &s.github
	g.token, g.login, g.device = token, login, nil
	gitrepo.SetGitHubToken(token)
	p := s.githubTokenPath()
	if p == "" {
		return nil
	}
	if token == "" {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := json.Marshal(storedToken{token, login})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

// githubLoginHandler starts a sign-in: it asks GitHub for a code and shows
// it, with where to enter it.
func (s *server) githubLoginHandler(w http.ResponseWriter, r *http.Request) {
	id := clientID()
	if id == "" {
		s.renderGitHub(w, "GitHub sign-in is not set up in this build of folio; git uses your own credentials")
		return
	}
	var d deviceCode
	if err := githubPost("/login/device/code", url.Values{"client_id": {id}, "scope": {"repo"}}, &d); err != nil {
		s.renderGitHub(w, err.Error())
		return
	}
	if d.DeviceCode == "" || d.UserCode == "" {
		s.renderGitHub(w, "GitHub answered without a code to enter")
		return
	}
	d.Interval = max(d.Interval, 5)
	now := time.Now()
	d.expires = now.Add(time.Duration(max(d.ExpiresIn, 60)) * time.Second)
	d.next = now.Add(time.Duration(d.Interval) * pollUnit)
	s.github.mu.Lock()
	s.github.device = &d
	s.github.mu.Unlock()
	s.renderGitHub(w, "")
}

// githubPollHandler asks GitHub whether the user has entered the code yet.
// The page asks every few seconds while it shows the code; a poll that
// comes sooner than GitHub allows is answered "still waiting" without
// asking, since asking too often is what GitHub slows a client down for.
func (s *server) githubPollHandler(w http.ResponseWriter, r *http.Request) {
	g := &s.github
	now := time.Now()
	g.mu.Lock()
	d := g.device
	var early, expired bool
	if d != nil {
		early, expired = now.Before(d.next), now.After(d.expires)
	}
	g.mu.Unlock()
	switch {
	case d == nil || early:
		s.renderGitHub(w, "")
		return
	case expired:
		s.endDevice(d)
		s.renderGitHub(w, "the code has expired; sign in again")
		return
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
		Interval    int    `json:"interval"`
	}
	err := githubPost("/login/oauth/access_token", url.Values{
		"client_id":   {clientID()},
		"device_code": {d.DeviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}, &tok)
	g.mu.Lock()
	d.next = time.Now().Add(time.Duration(d.Interval) * pollUnit)
	g.mu.Unlock()
	switch {
	case err != nil:
		s.renderGitHub(w, err.Error())
		return
	case tok.Error == "authorization_pending":
		s.renderGitHub(w, "")
		return
	case tok.Error == "slow_down":
		g.mu.Lock()
		d.Interval = max(tok.Interval, d.Interval+5)
		g.mu.Unlock()
		s.renderGitHub(w, "")
		return
	case tok.Error == "access_denied":
		s.endDevice(d)
		s.renderGitHub(w, "the sign-in was declined on GitHub")
		return
	case tok.Error != "" || tok.AccessToken == "":
		s.endDevice(d)
		msg := tok.Description
		if msg == "" {
			msg = "GitHub gave no token: " + tok.Error
		}
		s.renderGitHub(w, msg)
		return
	}
	login, err := githubLogin(tok.AccessToken)
	if err != nil {
		s.endDevice(d)
		s.renderGitHub(w, err.Error())
		return
	}
	g.mu.Lock()
	err = s.setGitHubToken(tok.AccessToken, login)
	g.mu.Unlock()
	msg := ""
	if err != nil {
		msg = "signed in, but the sign-in could not be kept for the next start: " + err.Error()
	}
	s.renderGitHub(w, msg)
}

// endDevice drops the sign-in under way, if it is still d.
func (s *server) endDevice(d *deviceCode) {
	s.github.mu.Lock()
	defer s.github.mu.Unlock()
	if s.github.device == d {
		s.github.device = nil
	}
}

// githubLogoutHandler signs out, or cancels the sign-in under way. The
// token stays valid on GitHub until the user revokes it there; folio
// forgets it.
func (s *server) githubLogoutHandler(w http.ResponseWriter, r *http.Request) {
	s.github.mu.Lock()
	err := s.setGitHubToken("", "")
	s.github.mu.Unlock()
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	s.renderGitHub(w, msg)
}

// githubPost posts a form to github.com and decodes the JSON answer.
func githubPost(path string, form url.Values, into any) error {
	req, err := http.NewRequest(http.MethodPost, githubBase+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	return githubDo(req, into)
}

// githubLogin is the account a token belongs to.
func githubLogin(token string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, githubAPI+"/user", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	var u struct {
		Login string `json:"login"`
	}
	if err := githubDo(req, &u); err != nil {
		return "", err
	}
	return u.Login, nil
}

func githubDo(req *http.Request, into any) error {
	resp, err := githubHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("reaching GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// GitHub says why in the body — "device_flow_disabled" on an app
		// that has not turned the device flow on, for one — and that is
		// worth more to the user than the status alone.
		var why struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
			Message     string `json:"message"`
		}
		json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&why)
		if msg := cmp.Or(why.Description, why.Message, why.Error); msg != "" {
			return fmt.Errorf("GitHub answered %s: %s", resp.Status, msg)
		}
		return fmt.Errorf("GitHub answered %s", resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(into); err != nil {
		return fmt.Errorf("reading GitHub's answer: %w", err)
	}
	return nil
}
