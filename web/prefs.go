package web

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
)

// projectPrefs is what the UI remembers about a project: the render
// selection and the page last opened in the editor.
//
// The render selection is a point in the project's three-axis matrix (see
// internal/foliocore): topics, formats, and — per topic, because not every
// topic has the same ones — audiences.
type projectPrefs struct {
	// Topics are the selected topic names.
	Topics []string `json:"topics"`
	// Audiences maps a topic to the audiences selected for it.
	Audiences map[string][]string `json:"audiences"`
	// Formats are the selected format names (handout, handbook, ...).
	Formats []string `json:"formats"`
	// Page is the project-relative path of the page last opened in the
	// editor.
	Page string `json:"page"`
}

// defaultPrefs is what a project gets before the user picks anything:
// nothing selected, so the Render button never kicks off a long run by
// accident.
func defaultPrefs() projectPrefs { return projectPrefs{} }

// audiencesFor returns the audiences selected for a topic, defaulting — for
// a topic the user never configured — to every audience the topic takes
// part in.
func (p projectPrefs) audiencesFor(topic string, available []string) []string {
	if sel, ok := p.Audiences[topic]; ok {
		return intersect(sel, available)
	}
	return available
}

// intersect keeps the entries of sel that still exist in available, so a
// saved selection naming a deleted or no longer matching profile neither
// shows up nor breaks the restore.
func intersect(sel, available []string) []string {
	out := make([]string, 0, len(sel))
	for _, s := range sel {
		if slices.Contains(available, s) {
			out = append(out, s)
		}
	}
	return out
}

// maxRecent is how many project paths the Open field's dropdown offers.
// Ten is what fits in a glance; older paths drop off the end.
const maxRecent = 10

// rememberRoot puts dir at the head of the recently opened projects, so
// the Open dropdown offers the most recent one first and never lists the
// same project twice. The caller must hold s.mu.
func (s *server) rememberRoot(dir string) {
	if dir == "" {
		return
	}
	s.cfg.Recent = append([]string{dir}, slices.DeleteFunc(s.cfg.Recent, func(p string) bool {
		return p == dir
	})...)
	if len(s.cfg.Recent) > maxRecent {
		s.cfg.Recent = s.cfg.Recent[:maxRecent]
	}
	s.saveConfig()
}

// saveConfig writes the settings file. Persistence is best effort for
// everything that goes through here: the in-memory settings are already
// updated, so a failed write only loses the change across restarts, and
// the UI has nowhere useful to report it from. The handlers that let the
// user type something worth keeping -- the editing tasks, the API
// connections -- call the store directly and do report what went wrong.
// The caller must hold s.mu.
func (s *server) saveConfig() error {
	if s.configFile == "" {
		return nil // no config directory: these settings last this run
	}
	return saveConfigFile(s.configFile, s.cfg)
}

// prefsFor returns the saved selection of the open project, or the default
// one. The caller must hold s.mu.
func (s *server) prefsFor(root string) projectPrefs {
	if p, ok := s.cfg.Projects[root]; ok {
		return p
	}
	return defaultPrefs()
}

// rememberPage records the page the editor now shows, so the app comes back
// to it: after a restart, and after the trip to the config pages and back,
// which leaves and reloads the app page. The caller must hold s.mu.
func (s *server) rememberPage(rel string) {
	if s.root == "" {
		return
	}
	p := s.prefsFor(s.root)
	if p.Page == rel {
		return
	}
	p.Page = rel
	s.cfg.Projects[s.root] = p
	s.saveConfig()
}

// forgetPage drops the remembered page when it is the one named by rel, so
// a deleted page does not stay on record. The caller must hold s.mu.
func (s *server) forgetPage(rel string) {
	if s.root == "" {
		return
	}
	if p := s.prefsFor(s.root); p.Page == rel {
		p.Page = ""
		s.cfg.Projects[s.root] = p
		s.saveConfig()
	}
}

// defaultCSSName is the name the built-in stylesheet goes by: the one the
// preview is shown in while the project has no stylesheets of its own, and
// the file it becomes when it is saved into the project.
const defaultCSSName = "custom.css"

// projectCSSDir is the project's folder of preview stylesheets. They
// belong to the project, not the user: what they mark are the project's
// topics and audiences.
const projectCSSDir = config.StyleDir

// defaultCustomCSS is the built-in preview stylesheet. It ships the
// conditional-block (::: slide, ::: pol, ...) preview styling, so a
// project looks right before it has a stylesheet of its own.
const defaultCustomCSS = `.folio.slide {
    background: linear-gradient(135deg, #fefefe 0%, #f0f0f3 50%, #fefefe 100%);
}
.folio.slide:before {
    content: "🖥️ SLIDE"
}
.folio.pol {
    background: lightblue;
}
.folio.pol:before {
    content: "🚔"
}
.folio.fw {
    background: lightpink;
}
.folio.fw:before {
    content: "🚒"
}
.folio.tutorial {
    background: blanchedalmond;
}
.folio.tutorial:before {
    content: "🎓 TUTORIAL"
}
.folio.howto {
    background: ghostwhite;
}
.folio.howto:before {
    content: "🔨 HOWTO"
}
.folio.reference {
    background: lemonchiffon;
}
.folio.reference:before {
    content: "📃 REFERENCE"
}
`

// cssNamePattern restricts stylesheet file names to a safe, plain subset:
// no path separators or leading dots, so a name can never escape the
// stylesheet folder.
var cssNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*\.s?css$`)

// sanitizeCSSName normalizes a user-supplied stylesheet name: it trims
// space, adds the .css suffix if missing, and rejects anything that is not
// a plain file name.
func sanitizeCSSName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("stylesheet name is empty")
	}
	if !isStylesheet(name) {
		name += ".css"
	}
	if !cssNamePattern.MatchString(name) {
		return "", errors.New("invalid stylesheet name")
	}
	return name, nil
}

// activeMarkerFile is the file the active stylesheet used to be recorded
// in, inside the css directory beside the settings. It is a setting and
// lives in the settings file now; the name is kept so that the old marker
// can be read once and retired (see migrate.go).
const activeMarkerFile = ".active"

// isStylesheet reports whether a file name is a stylesheet: .css, or
// .scss written as plain CSS.
func isStylesheet(name string) bool {
	return strings.HasSuffix(name, ".css") || strings.HasSuffix(name, ".scss")
}

// styleDir is the open project's stylesheet folder, or "" without a
// project. The caller must hold s.mu.
func (s *server) styleDir() string {
	if s.root == "" {
		return ""
	}
	return filepath.Join(s.root, filepath.FromSlash(projectCSSDir))
}

// cssFiles lists the project's stylesheets, sorted by name; none when it
// has none. The caller must hold s.mu.
func (s *server) cssFiles() []string {
	entries, err := os.ReadDir(s.styleDir())
	if s.root == "" || err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() && cssNamePattern.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// loadCSS reads one of the project's stylesheets by name. While the
// project has none, it is the built-in stylesheet; a name that is not
// there, or a file that cannot be read, is an empty stylesheet, for the
// preview must still load. The caller must hold s.mu.
func (s *server) loadCSS(name string) string {
	files := s.cssFiles()
	if len(files) == 0 {
		return defaultCustomCSS
	}
	if !slices.Contains(files, name) {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(s.styleDir(), name))
	if err != nil {
		return ""
	}
	return string(b)
}

// saveCSS writes a stylesheet into the project's stylesheet folder, which
// it creates if need be. The caller must hold s.mu.
func (s *server) saveCSS(name, css string) error {
	dir := s.styleDir()
	if dir == "" {
		return errors.New("open a project first: the stylesheets belong to a project")
	}
	if !cssNamePattern.MatchString(name) {
		return errors.New("invalid stylesheet name")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeMatching(filepath.Join(dir, name), css)
}

// createCSS adds a new, empty stylesheet to the project and returns its
// final (sanitized) file name. The caller must hold s.mu.
func (s *server) createCSS(name string) (string, error) {
	clean, err := sanitizeCSSName(name)
	if err != nil {
		return "", err
	}
	if slices.Contains(s.cssFiles(), clean) {
		return "", errors.New("a stylesheet with that name already exists")
	}
	return clean, s.saveCSS(clean, "")
}

// activeCSS returns the stylesheet the live preview currently shows: the
// one last selected in the dropdown, falling back to the project's first
// when the selection names none of its stylesheets, and to the built-in
// one when it has none. The caller must hold s.mu.
func (s *server) activeCSS() string {
	files := s.cssFiles()
	switch {
	case slices.Contains(files, s.cfg.Preview.CSS):
		return s.cfg.Preview.CSS
	case len(files) > 0:
		return files[0]
	}
	return defaultCSSName
}

// setActiveCSS remembers name as the stylesheet the live preview shows.
// The caller must hold s.mu.
func (s *server) setActiveCSS(name string) error {
	if !slices.Contains(s.cssFiles(), name) {
		return errors.New("no such stylesheet")
	}
	s.cfg.Preview.CSS = name
	return s.saveConfig()
}
