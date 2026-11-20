package web

import (
	"encoding/json"
	"path"
	"strings"

	"github.com/christophberger-ailab/foolscap-folio/internal/config"
	"github.com/christophberger-ailab/foolscap-folio/internal/shortcode"
)

// variablesJSON is what the preview resolves {{name}} and {{< var name >}}
// against in the page rel: the variables of folio.yml for the page's
// topic, overlaid with _variables.yml, and the date, as a render without a
// format or audience has them; as JSON. The caller must hold s.mu.
func (s *server) variablesJSON(rel string) string {
	cfg, err := config.Load(s.root)
	if err != nil {
		return "{}"
	}
	sel := config.Selection{Config: cfg}
	for _, e := range cfg.Topics {
		if t := e.Value; t.Folder != "" && strings.HasPrefix(path.Clean(rel)+"/", t.Folder+"/") {
			sel.Topic = t
		}
	}
	vars, _ := shortcode.Variables(cfg.Root, cfg.Lang, sel.Vars())
	b, _ := json.Marshal(vars)
	return string(b)
}
