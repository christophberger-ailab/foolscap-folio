package web

// configfile.go — the Config area's page for the configuration files
// themselves, a tab per file: the project's folio.yml, its _brand.yml and
// its preview stylesheets, and folio's own config.cue.
//
// The editors check what they are given the way the file is checked when
// it is read, and write nothing that does not check out: a file the next
// start cannot read is worse than an edit that was refused.

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/christophberger-ailab/foolscap-folio/internal/build"
	"github.com/christophberger-ailab/foolscap-folio/internal/config"
)

// keyHidden stands in for a stored API key in the config.cue editor. The
// keys are never sent to the browser; the marker saved back keeps the key
// the connection already has.
const keyHidden = "(stored key, not shown)"

// configFileView is the page data of a configuration file editor.
type configFileView struct {
	// Kind is the tab shown, which picks the page's explanation:
	// "project", "brand", "css" or "settings".
	Kind string
	// File is the stylesheet the css tab edits, one of Files, the
	// project's stylesheets.
	File  string
	Files []string
	// Action is where the form posts; Mode the CodeMirror mode.
	Action, Mode string
	// Path is the file being edited; Text what the editor holds.
	Path, Text string
	// Unavailable, when set, says why there is nothing to edit.
	Unavailable    string
	Message, Error string
}

// formText returns the posted text field with the line breaks a browser
// sends (CRLF) turned back into the file's own.
func formText(r *http.Request) (string, error) {
	if err := r.ParseForm(); err != nil {
		return "", err
	}
	if !r.PostForm.Has("text") {
		return "", fmt.Errorf("missing text field")
	}
	return strings.ReplaceAll(r.PostForm.Get("text"), "\r\n", "\n"), nil
}

// projectFileView describes the open project's folio.yml, or its legacy
// qm.yml. The caller must hold s.mu.
func (s *server) projectFileView() configFileView {
	v := configFileView{Kind: "project", Action: "/config/files/project", Mode: "yaml"}
	if s.root == "" {
		v.Unavailable = "Open a project in the app first: folio.yml belongs to a project."
		return v
	}
	v.Path = config.Find(s.root)
	if v.Path == "" {
		v.Path = filepath.Join(s.root, config.FileName)
	}
	if b, err := os.ReadFile(v.Path); err == nil {
		v.Text = string(b)
	} else if !os.IsNotExist(err) {
		v.Error = err.Error()
	}
	return v
}

// configFilesPage shows a tab of the configuration files page; the
// project's folio.yml without one.
func (s *server) configFilesPage(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var v configFileView
	switch r.PathValue("tab") {
	case "", "project":
		v = s.projectFileView()
	case "brand":
		v = s.brandFileView()
	case "css":
		v = s.cssFileView(r.URL.Query().Get("file"))
	case "settings":
		v = s.settingsFileView()
	default:
		http.NotFound(w, r)
		return
	}
	s.render(w, "config-file-page", v)
}

// cssFileView describes the project's stylesheet named file, or the active
// one. While the project has none, it is the built-in stylesheet, which
// saving puts into the project. The caller must hold s.mu.
func (s *server) cssFileView(file string) configFileView {
	v := configFileView{Kind: "css", Action: "/config/files/css", Mode: "css"}
	if s.root == "" {
		v.Unavailable = "Open a project in the app first: the preview stylesheets belong to a project."
		return v
	}
	v.Files = s.cssFiles()
	v.File = s.cssFileParam(file)
	v.Path = filepath.Join(s.styleDir(), v.File)
	v.Text = s.loadCSS(v.File)
	return v
}

// saveCSSFileHandler writes a stylesheet of the project. A form that cannot
// be read saves nothing: the stylesheet is the user's own text, and a
// malformed form must not read as an empty file.
func (s *server) saveCSSFileHandler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	text, err := formText(r)
	v := s.cssFileView(r.PostForm.Get("file"))
	if v.Unavailable != "" {
		s.render(w, "config-file-page", v)
		return
	}
	if err == nil {
		v.Text = text
		err = s.saveCSS(v.File, text)
	}
	if err != nil {
		v.Error = err.Error()
	} else {
		v.Files, v.Message = s.cssFiles(), "Saved."
	}
	s.render(w, "config-file-page", v)
}

// newCSSFileHandler adds a stylesheet to the project, named by the form,
// and opens it in the editor.
func (s *server) newCSSFileHandler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name, err := s.createCSS(r.PostFormValue("name"))
	v := s.cssFileView(name)
	if err != nil {
		v.Error = err.Error()
	} else if v.Unavailable == "" {
		v.Message = "Created."
	}
	s.render(w, "config-file-page", v)
}

// saveProjectFileHandler writes folio.yml once it reads as a project
// configuration; otherwise the editor keeps the text and says what is wrong.
func (s *server) saveProjectFileHandler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.projectFileView()
	if v.Unavailable != "" {
		s.render(w, "config-file-page", v)
		return
	}
	text, err := formText(r)
	if err == nil {
		v.Text = text
		if _, err = config.Parse(s.root, v.Path, []byte(text)); err == nil {
			err = os.WriteFile(v.Path, []byte(text), 0o644)
		}
	}
	if err != nil {
		v.Error = err.Error()
	} else {
		v.Error, v.Message = "", "Saved."
	}
	s.render(w, "config-file-page", v)
}

// brandFileView describes the brand file in effect for the open project:
// its own _brand.yml, or else the user's beside the settings file. With
// neither, it is the project's, which saving creates. The caller must hold
// s.mu.
func (s *server) brandFileView() configFileView {
	v := configFileView{Kind: "brand", Action: "/config/files/brand", Mode: "yaml"}
	var candidates []string
	if s.root != "" {
		candidates = append(candidates, filepath.Join(s.root, build.BrandFile))
	}
	if user := userBrandIn(s.configFile); user != "" {
		candidates = append(candidates, user)
	}
	if len(candidates) == 0 {
		v.Unavailable = "Open a project in the app first: there is no brand file to edit."
		return v
	}
	v.Path = candidates[0]
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			continue
		}
		v.Path = p
		if err != nil {
			v.Error = err.Error()
		}
		v.Text = string(b)
		break
	}
	return v
}

// saveBrandFileHandler writes _brand.yml once a render can read it.
func (s *server) saveBrandFileHandler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.brandFileView()
	if v.Unavailable != "" {
		s.render(w, "config-file-page", v)
		return
	}
	text, err := formText(r)
	if err == nil {
		v.Text = text
		if err = build.CheckBrand([]byte(text)); err == nil {
			err = os.WriteFile(v.Path, []byte(text), 0o644)
		}
	}
	v.Error, v.Message = "", "Saved."
	if err != nil {
		v.Error, v.Message = err.Error(), ""
	}
	s.render(w, "config-file-page", v)
}

// settingsFileView shows the settings as they are in effect, written the way
// they are written to config.cue, with the API keys hidden. The caller must
// hold s.mu.
func (s *server) settingsFileView() configFileView {
	v := configFileView{Kind: "settings", Action: "/config/files/settings", Path: s.configFile}
	shown := s.cfg
	shown.Copyedit.Connections = make([]apiConnection, len(s.cfg.Copyedit.Connections))
	for i, c := range s.cfg.Copyedit.Connections {
		if c.Key != "" {
			c.Key = keyHidden
		}
		shown.Copyedit.Connections[i] = c
	}
	b, err := renderConfig(shown)
	if err != nil {
		v.Error = err.Error()
	}
	v.Text = string(b)
	return v
}

// saveSettingsFileHandler puts the edited settings in effect and writes
// them. They are checked against the schema the file carries and then, as
// they will be written, against the one this binary holds, so a schema
// loosened by hand lets nothing through.
func (s *server) saveSettingsFileHandler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	text, err := formText(r)
	var cfg storedConfig
	if err == nil {
		cfg, err = s.parseSettings(text)
	}
	if err == nil {
		prev := s.cfg
		s.cfg = cfg
		if err = s.saveConfig(); err != nil {
			s.cfg = prev
		}
	}
	v := s.settingsFileView()
	if err != nil {
		v.Text, v.Error = text, err.Error()
	} else {
		v.Message = "Saved."
	}
	s.render(w, "config-file-page", v)
}

// parseSettings reads edited settings, putting the stored key back where
// the editor showed keyHidden. The caller must hold s.mu.
func (s *server) parseSettings(text string) (storedConfig, error) {
	name := s.configFile
	if name == "" {
		name = configFileName
	}
	cfg, err := decodeConfig([]byte(text), name)
	if err != nil {
		return cfg, err
	}
	stored := map[string]string{}
	for _, c := range s.cfg.Copyedit.Connections {
		stored[c.ID] = c.Key
	}
	for i, c := range cfg.Copyedit.Connections {
		if c.Key != keyHidden {
			continue
		}
		key := stored[c.ID]
		if key == "" {
			return cfg, fmt.Errorf("connection %q: there is no stored key to keep; enter the key instead of %q", c.ID, keyHidden)
		}
		cfg.Copyedit.Connections[i].Key = key
	}
	b, err := renderConfig(cfg)
	if err != nil {
		return cfg, err
	}
	return decodeConfig(b, name)
}
