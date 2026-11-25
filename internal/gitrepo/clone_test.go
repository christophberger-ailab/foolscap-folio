package gitrepo

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestValidRemote(t *testing.T) {
	ok := []string{
		"https://github.com/owner/book.git",
		"https://github.com/owner/book",
		"http://example.com/book.git",
		"ssh://git@example.com/owner/book.git",
		"git@github.com:owner/book.git",
	}
	for _, u := range ok {
		if err := ValidRemote(u); err != nil {
			t.Errorf("ValidRemote(%q) = %v, want nil", u, err)
		}
	}
	bad := []string{
		"",
		"-uhttps://github.com/owner/book",
		"--upload-pack=touch /tmp/x",
		"ext::sh -c touch% /tmp/x",
		"file:///etc",
		"/home/user/book",
		"C:\\books\\mine",
		"https://github.com/owner/book name",
		"https://github.com/owner/\nbook",
		"https://",
		"git@github.com:",
		"https://github.com/",
	}
	for _, u := range bad {
		if err := ValidRemote(u); err == nil {
			t.Errorf("ValidRemote(%q) = nil, want an error", u)
		}
	}
}

func TestRepoName(t *testing.T) {
	tests := map[string]string{
		"https://github.com/owner/book.git":   "book",
		"https://github.com/owner/book":       "book",
		"https://github.com/owner/book/":      "book",
		"git@github.com:owner/my-book.git":    "my-book",
		"ssh://git@example.com/a/b/c.git":     "c",
		"https://example.com/owner/..":        "",
		"https://example.com/owner/.git":      "",
		"git@github.com:book.git":             "book",
		"https://github.com/owner/book.git?x": "",
	}
	for in, want := range tests {
		if got := RepoName(in); got != want {
			t.Errorf("RepoName(%q) = %q, want %q", in, got, want)
		}
	}
}

// Clone makes a working tree from a remote, here a local bare repository
// standing in for one.
func TestClone(t *testing.T) {
	seed := testRepo(t)
	bare := filepath.Join(t.TempDir(), "book.git")
	git(t, seed, "clone", "--bare", seed, bare)

	dest := filepath.Join(t.TempDir(), "book")
	if _, err := Clone(bare, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "index.qmd")); err != nil {
		t.Errorf("clone has no index.qmd: %v", err)
	}
	st, err := GetStatus(dest)
	if err != nil {
		t.Fatal(err)
	}
	if st.Upstream != "origin/main" {
		t.Errorf("upstream = %q, want origin/main", st.Upstream)
	}

	// A folder that is already there is not cloned into: git would
	// refuse a non-empty one anyway, and an empty one may be the user's.
	if _, err := Clone(bare, dest); err == nil {
		t.Error("cloned into an existing folder")
	}
}

func TestAuthEnv(t *testing.T) {
	SetGitHubToken("")
	if env := authEnv([]string{"PATH=/bin"}); !slices.Equal(env, []string{"PATH=/bin"}) {
		t.Errorf("no token, env = %v", env)
	}

	SetGitHubToken("tok")
	t.Cleanup(func() { SetGitHubToken("") })
	env := authEnv([]string{"PATH=/bin"})
	header := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:tok"))
	for _, want := range []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.https://github.com/.extraheader", "GIT_CONFIG_VALUE_0=" + header} {
		if !slices.Contains(env, want) {
			t.Errorf("env lacks %q: %v", want, env)
		}
	}

	// Settings the user passes the same way are kept, and the token's
	// entry goes after them.
	env = authEnv([]string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.autocrlf", "GIT_CONFIG_VALUE_0=true"})
	for _, want := range []string{"GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=core.autocrlf", "GIT_CONFIG_KEY_1=http.https://github.com/.extraheader"} {
		if !slices.Contains(env, want) {
			t.Errorf("env lacks %q: %v", want, env)
		}
	}
	if slices.Contains(env, "GIT_CONFIG_COUNT=1") {
		t.Errorf("old count kept: %v", env)
	}
}

// A token set for another host goes to that host alone, beside the
// GitHub one; "" takes it back.
func TestAuthEnvPerHost(t *testing.T) {
	SetGitHubToken("gh")
	SetToken("https://dev.azure.com/", "pat")
	t.Cleanup(func() { SetGitHubToken(""); SetToken("https://dev.azure.com/", "") })
	basic := func(t string) string {
		return "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+t))
	}
	env := authEnv(nil)
	want := []string{
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=http.https://dev.azure.com/.extraheader", "GIT_CONFIG_VALUE_0=" + basic("pat"),
		"GIT_CONFIG_KEY_1=http.https://github.com/.extraheader", "GIT_CONFIG_VALUE_1=" + basic("gh"),
	}
	if !slices.Equal(env, want) {
		t.Errorf("env = %v\nwant %v", env, want)
	}
	SetToken("https://dev.azure.com/", "")
	if env := authEnv(nil); !slices.Contains(env, "GIT_CONFIG_COUNT=1") || slices.Contains(env, want[1]) {
		t.Errorf("token not taken back: %v", env)
	}
}

// A token is only ever sent over https, to the host the URL names, with
// whatever user the URL carries left out: git matches a header without one
// to every user.
func TestOrigin(t *testing.T) {
	for url, want := range map[string]string{
		"https://dev.azure.com/org/proj/_git/repo":     "https://dev.azure.com/",
		"https://org@dev.azure.com/org/proj/_git/repo": "https://dev.azure.com/",
		"https://u:pw@GitLab.example.com:8443/g/r.git": "https://gitlab.example.com:8443/",
		"http://example.com/r.git":                     "",
		"git@github.com:o/r.git":                       "",
		"ssh://git@example.com/r.git":                  "",
		"https:///r":                                   "",
	} {
		if got := Origin(url); got != want {
			t.Errorf("Origin(%q) = %q, want %q", url, got, want)
		}
	}
}

// What git says when a remote wants a sign-in it could not give is told
// apart from other failures.
func TestAuthFailed(t *testing.T) {
	for out, want := range map[string]bool{
		"fatal: could not read Username for 'https://dev.azure.com': terminal prompts disabled":                                                                                                                                    true,
		"remote: TF401019: ...\nfatal: Authentication failed for 'https://dev.azure.com/o/p/_git/r/'":                                                                                                                              true,
		"fatal: unable to update url base from redirection:\n  asked for: https://dev.azure.com/o/p/_git/r/info/refs?service=git-upload-pack\n   redirect: https://spsprodweu5.vssps.visualstudio.com/_signin?realm=dev.azure.com": true,
		"fatal: unable to access 'https://x/r/': The requested URL returned error: 401":                                                                                                                                            true,
		"fatal: unable to access 'https://x/r/': The requested URL returned error: 403":                                                                                                                                            true,
		"fatal: repository 'https://x/r/' not found":                                                                                                                                                                               false,
		"fatal: unable to access 'https://x/r/': Could not resolve host: x":                                                                                                                                                        false,
	} {
		if got := AuthFailed(out); got != want {
			t.Errorf("AuthFailed(%q) = %v, want %v", out, got, want)
		}
	}
}

// The token reaches git in its environment, never on its command line,
// where any process on the machine could read it.
func TestTokenStaysOffTheCommandLine(t *testing.T) {
	if !Available() {
		t.Skip("git not installed")
	}
	SetGitHubToken("s3cret")
	t.Cleanup(func() { SetGitHubToken("") })
	out, err := run(t.TempDir(), "config", "--get", "http.https://github.com/.extraheader")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "Authorization: Basic ") {
		t.Errorf("git does not see the header: %q", out)
	}
}
