package web

// hosttoken.go — access tokens for hosts other than GitHub, such as Azure
// DevOps. Git asks for a sign-in on the terminal or through a credential
// helper; folio web runs git with neither to hand, so a host that wants
// one refuses the clone. The clone form takes a personal access token
// instead, which git sends to that host alone (see gitrepo.SetToken). One
// that worked is kept, as the GitHub sign-in is, in a file of its own
// beside the settings that only its owner can read, so that push and sync
// work too, also after a restart.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/christophberger-ailab/foolscap-folio/internal/gitrepo"
)

// hostTokensFile is the file beside the settings that keeps the tokens.
const hostTokensFile = "host-tokens.json"

// hostTokens are the tokens by the origin each goes to. They have a lock
// of their own: a clone runs without the server's.
type hostTokens struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *server) hostTokensPath() string {
	if s.configFile == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(s.configFile), hostTokensFile)
}

// loadHostTokens picks up the tokens kept from an earlier run. A file that
// cannot be read holds none: the user gives a token again.
func (s *server) loadHostTokens() {
	p := s.hostTokensPath()
	if p == "" {
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	var m map[string]string
	if json.Unmarshal(b, &m) != nil {
		return
	}
	for origin, t := range m {
		s.setHostToken(origin, t)
	}
}

// setHostToken makes t the token for origin, "" none, in memory and in
// git, and returns the one it replaced.
func (s *server) setHostToken(origin, t string) string {
	h := &s.hostTokens
	h.mu.Lock()
	defer h.mu.Unlock()
	prev := h.m[origin]
	if t == "" {
		delete(h.m, origin)
	} else {
		if h.m == nil {
			h.m = map[string]string{}
		}
		h.m[origin] = t
	}
	gitrepo.SetToken(origin, t)
	return prev
}

// saveHostTokens writes the tokens to their file, readable by its owner
// alone.
func (s *server) saveHostTokens() error {
	p := s.hostTokensPath()
	if p == "" {
		return nil
	}
	h := &s.hostTokens
	h.mu.Lock()
	b, err := json.Marshal(h.m)
	h.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

// signInHint is what to say when git's output out says the host at origin
// wanted a sign-in git could not give, "" when it says something else.
func signInHint(out, origin string, gaveToken bool) string {
	if !gitrepo.AuthFailed(out) {
		return ""
	}
	host := strings.TrimSuffix(strings.TrimPrefix(origin, "https://"), "/")
	switch {
	case host == "":
		return "The repository wants a sign-in that git cannot ask for here: set up an SSH key for it, or clone its https:// address with a personal access token."
	case gaveToken:
		return host + " did not take the access token: check that it has not expired and may read the repository."
	}
	hint := host + " wants a sign-in, and git cannot ask for one here. Enter a personal access token for it and clone again."
	if name, _, _ := strings.Cut(host, ":"); name == "dev.azure.com" || strings.HasSuffix(name, ".visualstudio.com") {
		hint += " On Azure DevOps, make one under User settings › Personal access tokens, with the scope Code (Read & write)."
	}
	return hint
}
