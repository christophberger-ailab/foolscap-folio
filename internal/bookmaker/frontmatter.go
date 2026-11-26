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
	// Meta is the whole front matter, which gives the page's variables.
	Meta map[string]any
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

// ParseFrontMatter extracts the fields the bookmaker needs from a page's
// front matter. Files without front matter yield a zero FrontMatter.
func ParseFrontMatter(src string) (FrontMatter, string, error) {
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
	if err := yaml.Unmarshal([]byte(yamlText), &fm.Meta); err != nil {
		return FrontMatter{}, body, fmt.Errorf("parsing front matter: %w", err)
	}
	if raw.Title != nil {
		fm.Title = strings.TrimSpace(*raw.Title)
	}
	if raw.Order != nil {
		fm.Order = *raw.Order
		fm.HasOrder = true
	}
	return fm, body, nil
}

// value returns key's top-level value, else its value under the
// `folio:` or `qm:` mapping such keys sat in before (issue #94).
func (fm FrontMatter) value(key string) any {
	if v, ok := fm.Meta[key]; ok {
		return v
	}
	for _, m := range []string{"folio", "qm"} {
		if sub, ok := fm.Meta[m].(map[string]any); ok {
			if v, ok := sub[key]; ok {
				return v
			}
		}
	}
	return nil
}

// IsDraft reports whether the page says `draft: true`, which keeps it out
// of a render other than a draft.
// PageID is the page's `id:`, the name references give the page
// (`#install:overview`); "" for none.
func (fm FrontMatter) PageID() string {
	if s, ok := fm.Meta["id"].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func (fm FrontMatter) IsDraft() bool { return fm.Meta["draft"] == true }

// IsNew reports whether the page is new: `new: true`, or `status: new` as
// the editor's tree reads it.
func (fm FrontMatter) IsNew() bool { return fm.value("new") == true || fm.value("status") == "new" }

// IsBookmarked reports whether the page says `bookmarked: true`.
func (fm FrontMatter) IsBookmarked() bool { return fm.value("bookmarked") == true }

// Review returns what to review on the page, its whitespace collapsed,
// and whether it is to be reviewed: `review:` holds a text or is true.
func (fm FrontMatter) Review() (string, bool) {
	switch v := fm.value("review").(type) {
	case string:
		s := strings.Join(strings.Fields(v), " ")
		return s, s != ""
	case bool:
		return "", v
	}
	return "", false
}
