package web

// callouts.go — the titles callouts get in the user's languages, and the
// other texts folio puts into a render.
//
// A callout without a title of its own is titled with its type's name in
// the project's language (folio.yml's lang). folio knows English and
// German; the settings file holds the user's own titles for any language,
// and those win. They are the user's rather than the project's, so they
// live in config.cue, and every render and preview reads them from there.
// The strings, such as the "Part" and "Chapter" that go before section and
// chapter titles, work the same way.

import (
	"encoding/json"
	"errors"
	"io/fs"
	"maps"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/internal/markdown"
)

// CalloutTitles are the user's callout titles by language and type, from
// the settings file; nil when there is no settings file.
func CalloutTitles() (map[string]map[string]string, error) {
	return calloutTitlesIn(defaultConfigFile())
}

func calloutTitlesIn(path string) (map[string]map[string]string, error) {
	cfg, err := settingsIn(path)
	if cfg == nil {
		return nil, err
	}
	return cfg.Callouts, nil
}

// Strings are the user's strings by language and key, from the settings
// file; nil when there is no settings file.
func Strings() (map[string]map[string]string, error) {
	return stringsIn(defaultConfigFile())
}

func stringsIn(path string) (map[string]map[string]string, error) {
	cfg, err := settingsIn(path)
	if cfg == nil {
		return nil, err
	}
	return cfg.Strings, nil
}

// settingsIn reads the settings file at path; nil when there is none.
func settingsIn(path string) (*storedConfig, error) {
	if path == "" {
		return nil, nil
	}
	cfg, err := loadConfigFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// defaultCallouts are the titles a fresh settings file starts with: the
// ones folio knows itself, so that the file shows what to write for
// another language.
func defaultCallouts() map[string]map[string]string {
	out := map[string]map[string]string{}
	for lang, titles := range markdown.DefaultCalloutTitles {
		out[lang] = maps.Clone(titles)
	}
	return out
}

// defaultStrings are the strings a fresh settings file starts with: the
// ones folio knows itself.
func defaultStrings() map[string]map[string]string {
	out := map[string]map[string]string{}
	for lang, texts := range config.DefaultStrings {
		out[lang] = maps.Clone(texts)
	}
	return out
}

// calloutTitlesJSON is what the preview titles callouts by: every type's
// title in the open project's language, as JSON. The caller must hold
// s.mu.
func (s *server) calloutTitlesJSON() string {
	b, _ := json.Marshal(markdown.CalloutTitles(s.cfg.Callouts, s.projectLang()))
	return string(b)
}

// projectLang is the open project's `lang:`, or "" when it sets none. The
// caller must hold s.mu.
func (s *server) projectLang() string {
	if cfg, err := config.Load(s.root); err == nil {
		return cfg.Lang
	}
	return ""
}
