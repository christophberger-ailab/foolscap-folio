package web

// callouts.go — the titles callouts get in the user's languages.
//
// A callout without a title of its own is titled with its type's name in
// the project's language (folio.yml's lang). folio knows English and
// German; the settings file holds the user's own titles for any language,
// and those win. They are the user's rather than the project's, so they
// live in config.cue, and every render and preview reads them from there.

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
	return cfg.Callouts, nil
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

// calloutTitlesJSON is what the preview titles callouts by: every type's
// title in the open project's language, as JSON. The caller must hold
// s.mu.
func (s *server) calloutTitlesJSON() string {
	lang := ""
	if cfg, err := config.Load(s.root); err == nil {
		lang = cfg.Lang
	}
	b, _ := json.Marshal(markdown.CalloutTitles(s.cfg.Callouts, lang))
	return string(b)
}
