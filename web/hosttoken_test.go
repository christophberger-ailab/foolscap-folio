package web

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/gitrepo"
)

const azdo = "https://org@dev.azure.com/org/proj/_git/book"

// tokenServer is a server whose settings live in a folder of the test's,
// so that what it keeps beside them can be looked at.
func tokenServer(t *testing.T) (*server, string) {
	t.Helper()
	dir := t.TempDir()
	srv, err := newServer(filepath.Join(dir, "settings.cue"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { gitrepo.SetToken("https://dev.azure.com/", "") })
	return srv, dir
}

// failClone makes the clone fail with git's words out.
func failClone(t *testing.T, out string) {
	prev := cloneRepo
	cloneRepo = func(string, string, string, string) (string, error) { return out, errors.New(out) }
	t.Cleanup(func() { cloneRepo = prev })
}

// WEB.9-2-7: the form takes an access token, which it never shows again.
func TestCloneFormTakesAToken(t *testing.T) {
	body := get(t, bareServer(t), "/clone").Body.String()
	if !strings.Contains(body, `type="password" name="token"`) {
		t.Errorf("no token field:\n%s", body)
	}
}

// WEB.9-2-7-1: a clone that a host refuses for want of a sign-in says to
// give it a token, and how to make one on Azure DevOps.
func TestCloneWithoutSignInAsksForAToken(t *testing.T) {
	failClone(t, "fatal: could not read Username for 'https://dev.azure.com': terminal prompts disabled")
	body := post(t, bareServer(t), "/clone", url.Values{"url": {azdo}, "dir": {t.TempDir()}}).Body.String()
	for _, want := range []string{"dev.azure.com wants a sign-in", "personal access token", "Code (Read &amp; write)", "terminal prompts disabled"} {
		if !strings.Contains(body, want) {
			t.Errorf("refusal lacks %q:\n%s", want, body)
		}
	}
}

// WEB.9-2-7-2: a token that worked is kept beside the settings, for its
// host alone and readable by its owner alone, and used again on the next
// start; it is not shown in the page.
func TestCloneKeepsAToken(t *testing.T) {
	bareRemote(t, map[string]string{"folio.yml": testConfig, "index.qmd": "# Home\n"})
	srv, dir := tokenServer(t)
	rec := post(t, srv, "/clone", url.Values{"url": {azdo}, "dir": {t.TempDir()}, "token": {" s3cret "}})
	if rec.Header().Get("HX-Retarget") != "#main" {
		t.Fatalf("clone did not open: %s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), "s3cret") {
		t.Error("token shown in the page")
	}
	file := filepath.Join(dir, hostTokensFile)
	fi, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		t.Errorf("token file mode %v, want owner only", fi.Mode().Perm())
	}
	var kept map[string]string
	b, _ := os.ReadFile(file)
	if json.Unmarshal(b, &kept) != nil || len(kept) != 1 || kept["https://dev.azure.com/"] != "s3cret" {
		t.Errorf("kept %s", b)
	}
	again, err := newServer(filepath.Join(dir, "settings.cue"))
	if err != nil {
		t.Fatal(err)
	}
	if again.hostTokens.m["https://dev.azure.com/"] != "s3cret" {
		t.Errorf("next start has %v", again.hostTokens.m)
	}
}

// WEB.9-2-7-2: a token the host refused is not kept, and the one kept
// before it stays in use; the form says the token was not taken.
func TestCloneDropsARefusedToken(t *testing.T) {
	srv, dir := tokenServer(t)
	os.WriteFile(filepath.Join(dir, hostTokensFile), []byte(`{"https://dev.azure.com/":"old"}`), 0o600)
	srv, _ = newServer(filepath.Join(dir, "settings.cue"))
	failClone(t, "fatal: Authentication failed for 'https://dev.azure.com/org/proj/_git/book/'")
	body := post(t, srv, "/clone", url.Values{"url": {azdo}, "dir": {t.TempDir()}, "token": {"wrong"}}).Body.String()
	if !strings.Contains(body, "did not take the access token") {
		t.Errorf("refusal not explained:\n%s", body)
	}
	if strings.Contains(body, "wrong") {
		t.Error("token shown in the page")
	}
	if got := srv.hostTokens.m["https://dev.azure.com/"]; got != "old" {
		t.Errorf("token in use = %q, want the old one", got)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, hostTokensFile)); !strings.Contains(string(b), "old") {
		t.Errorf("kept %s", b)
	}
}

// WEB.9-2-7-3: a token goes over https alone, and not to GitHub while the
// GitHub sign-in is there to use.
func TestCloneRefusesATokenItCannotSendSafely(t *testing.T) {
	srv, _ := tokenServer(t)
	for name, u := range map[string]string{"http": "http://example.com/o/book.git", "ssh": "git@example.com:o/book.git"} {
		body := post(t, srv, "/clone", url.Values{"url": {u}, "dir": {t.TempDir()}, "token": {"tok"}}).Body.String()
		if !strings.Contains(body, "only over https://") {
			t.Errorf("%s: token not refused:\n%s", name, body)
		}
	}
	srv.github.token, srv.github.login = "gho", "octocat"
	body := post(t, srv, "/clone", url.Values{"url": {"https://github.com/o/book"}, "dir": {t.TempDir()}, "token": {"tok"}}).Body.String()
	if !strings.Contains(body, "signed in to GitHub") {
		t.Errorf("GitHub token not refused:\n%s", body)
	}
}
