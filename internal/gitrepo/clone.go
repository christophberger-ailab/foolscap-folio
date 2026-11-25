package gitrepo

import (
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// CloneTimeout bounds a clone. It is longer than Timeout because a clone
// carries a repository's whole history, which for a book with its images
// can be a while in coming.
var CloneTimeout = 10 * time.Minute

// ValidRemote reports why url is not something folio offers to clone. What
// it accepts are the network forms a user copies off a hosting site —
// https://, http://, ssh://, and the scp-like user@host:path — and nothing
// else: a URL that starts with a dash would be read by git as an option, and
// git's other transports (ext::, file://, a local path) run commands or read
// the local disk, which a form in a browser has no business asking for.
func ValidRemote(url string) error {
	if url == "" {
		return errors.New("no repository URL given")
	}
	if strings.IndexFunc(url, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return errors.New("a repository URL has no spaces or line breaks in it")
	}
	if strings.HasPrefix(url, "-") || strings.Contains(url, "::") {
		return fmt.Errorf("%q is not a repository URL", url)
	}
	for _, scheme := range []string{"https://", "http://", "ssh://"} {
		if rest, ok := strings.CutPrefix(url, scheme); ok {
			host, repo, _ := strings.Cut(rest, "/")
			if host == "" || strings.Trim(repo, "/") == "" {
				return fmt.Errorf("%q names no repository", url)
			}
			return nil
		}
	}
	// user@host:path, the form GitHub offers for SSH. The user is what
	// tells it apart from a Windows drive letter.
	if at := strings.Index(url, "@"); at > 0 {
		if host, repo, ok := strings.Cut(url[at+1:], ":"); ok && host != "" && !strings.Contains(host, "/") {
			if repo == "" {
				return fmt.Errorf("%q names no repository", url)
			}
			return nil
		}
	}
	return fmt.Errorf("%q is not a repository URL: use the https:// or SSH address the hosting site shows", url)
}

// RepoName is the folder a clone of url goes into, as git itself would
// name it: the last part of the path without ".git". It is "" when that
// part would not be a plain folder name.
func RepoName(url string) string {
	if i := strings.LastIndex(url, ":"); i >= 0 && !strings.Contains(url, "://") {
		url = url[i+1:]
	}
	name := strings.TrimSuffix(path.Base(strings.TrimRight(url, "/")), ".git")
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") ||
		strings.ContainsAny(name, `/\:?*"<>|`) {
		return ""
	}
	return name
}

// Clone makes a working tree of the repository at url in dest, which must
// not exist yet. The remote is called origin and the default branch is
// checked out tracking it, so push and sync work on it at once.
func Clone(url, dest string) (string, error) {
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("%s is already there; choose another folder", dest)
	}
	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	// The "--" keeps a URL from being read as an option, which ValidRemote
	// checks too; this is the second lock.
	return runFor(CloneTimeout, parent, 0, "clone", "--", url, dest)
}

// GitHubOrigin is where the GitHub sign-in's token goes.
const GitHubOrigin = "https://github.com/"

// tokens are the access tokens git authenticates with, by the https
// origin each is sent to: the GitHub sign-in's, and those the user gave
// for other hosts, such as Azure DevOps, which git could not ask for
// itself with no terminal to prompt on.
var tokens struct {
	sync.Mutex
	m map[string]string
}

// SetToken sets the token every git command run from here on sends to
// origin, as Origin names it; "" sends none, and git falls back on the
// user's own credential helpers.
func SetToken(origin, t string) {
	tokens.Lock()
	defer tokens.Unlock()
	if t == "" {
		delete(tokens.m, origin)
		return
	}
	if tokens.m == nil {
		tokens.m = map[string]string{}
	}
	tokens.m[origin] = t
}

// SetGitHubToken sets the token git sends to github.com.
func SetGitHubToken(t string) { SetToken(GitHubOrigin, t) }

// Origin is the origin of an https URL as git matches http.<url>.*
// settings -- "https://host[:port]/", without the user -- or "" for any
// other URL: a token goes over https only.
func Origin(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	return "https://" + strings.ToLower(u.Host) + "/"
}

// AuthFailed reports whether git's output says the remote wanted a
// sign-in that git had no way to give: no token, no credential helper
// that answered, and no terminal to ask on. Azure DevOps, asked without
// one, may answer with a redirect to its sign-in page rather than a 401.
func AuthFailed(out string) bool {
	out = strings.ToLower(out)
	for _, s := range []string{
		"could not read username", "could not read password", "terminal prompts disabled",
		"authentication failed", "unable to update url base from redirection",
		"returned error: 401", "returned error: 403",
	} {
		if strings.Contains(out, s) {
			return true
		}
	}
	return false
}

// authEnv is environ with each token added as an HTTP header for its
// origin and nothing else.
//
// They go in as configuration through the environment (GIT_CONFIG_COUNT
// and its pairs) rather than as -c options: the command line of a
// process is readable by every user of the machine, its environment only
// by its owner. Pairs the user set the same way are kept, and the tokens'
// are numbered after them, in the order of their origins.
func authEnv(environ []string) []string {
	tokens.Lock()
	origins := slices.Sorted(maps.Keys(tokens.m))
	ts := make([]string, len(origins))
	for i, o := range origins {
		ts[i] = tokens.m[o]
	}
	tokens.Unlock()
	if len(origins) == 0 {
		return environ
	}
	n := 0
	out := make([]string, 0, len(environ)+1+2*len(origins))
	for _, kv := range environ {
		if v, ok := strings.CutPrefix(kv, "GIT_CONFIG_COUNT="); ok {
			n, _ = strconv.Atoi(v)
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "GIT_CONFIG_COUNT="+strconv.Itoa(n+len(origins)))
	for i, o := range origins {
		k := strconv.Itoa(n + i)
		header := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+ts[i]))
		out = append(out, "GIT_CONFIG_KEY_"+k+"=http."+o+".extraheader", "GIT_CONFIG_VALUE_"+k+"="+header)
	}
	return out
}
