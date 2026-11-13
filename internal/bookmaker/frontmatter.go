package bookmaker

import (
	"fmt"
	"strings"

	"github.com/christophberger-ailab/foolscap-folio/internal/foliocore"
	"go.yaml.in/yaml/v4"
)

// FrontMatter holds the subset of Quarto front-matter fields that the
// bookmaker cares about. Everything else (listing, toc-depth, sidebar, ...)
// is page-level website metadata that is meaningless once the pages are
// flattened into a single document, and is therefore discarded.
type FrontMatter struct {
	// Title is the `title:` field. Empty when absent; in that case the page
	// heading is taken from the first ATX heading in the body.
	Title string
	// Subtitle is the `subtitle:` field. A book's root page puts it on the
	// document's cover, below the title.
	Subtitle string
	// Order is the `order:` field, used for sorting siblings.
	Order int
	// HasOrder reports whether `order:` was present. Pages without an order
	// sort after all ordered siblings.
	HasOrder bool
}

// noOrder is the sort key used for pages without an `order:` field. Quarto
// sorts such pages last, alphabetically among themselves.
const noOrder = 1 << 30

// SortKey returns the value used to order this page among its siblings.
func (fm FrontMatter) SortKey() int {
	if !fm.HasOrder {
		return noOrder
	}
	return fm.Order
}

// parseFrontMatter extracts the fields the bookmaker needs from a page's
// front matter. Files without front matter yield a zero FrontMatter.
func parseFrontMatter(src string) (FrontMatter, string, error) {
	yamlText, body, found := foliocore.SplitFrontMatter(src)
	if !found {
		return FrontMatter{}, body, nil
	}

	var raw struct {
		Title    *string `yaml:"title"`
		Subtitle string  `yaml:"subtitle"`
		Order    *int    `yaml:"order"`
	}
	if err := yaml.Unmarshal([]byte(yamlText), &raw); err != nil {
		return FrontMatter{}, body, fmt.Errorf("parsing front matter: %w", err)
	}

	fm := FrontMatter{Subtitle: strings.TrimSpace(raw.Subtitle)}
	if raw.Title != nil {
		fm.Title = strings.TrimSpace(*raw.Title)
	}
	if raw.Order != nil {
		fm.Order = *raw.Order
		fm.HasOrder = true
	}
	return fm, body, nil
}
