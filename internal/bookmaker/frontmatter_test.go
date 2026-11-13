package bookmaker

import (
	"strings"
	"testing"

	"github.com/christophberger-ailab/foolscap-folio/internal/foliocore"
)

func lines(s string) []string { return strings.Split(strings.TrimPrefix(s, "\n"), "\n") }

func TestSplitFrontMatter(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		yaml  string
		body  string
		found bool
	}{
		{
			name:  "simple",
			src:   "---\ntitle: A\n---\nbody\n",
			yaml:  "title: A",
			body:  "body\n",
			found: true,
		},
		{
			name:  "closed with dots",
			src:   "---\norder: 3\n...\nbody",
			yaml:  "order: 3",
			body:  "body",
			found: true,
		},
		{
			name:  "no front matter",
			src:   "# Heading\n\ntext",
			body:  "# Heading\n\ntext",
			found: false,
		},
		{
			name:  "unterminated block is treated as body",
			src:   "---\ntitle: A\nbody",
			body:  "---\ntitle: A\nbody",
			found: false,
		},
		{
			name:  "byte order mark",
			src:   "\uFEFF---\ntitle: A\n---\nbody",
			yaml:  "title: A",
			body:  "body",
			found: true,
		},
		{
			name:  "thematic break is not front matter",
			src:   "text\n\n---\n\nmore",
			body:  "text\n\n---\n\nmore",
			found: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			yamlText, body, found := foliocore.SplitFrontMatter(tc.src)
			if found != tc.found {
				t.Fatalf("found = %v, want %v", found, tc.found)
			}
			if yamlText != tc.yaml {
				t.Errorf("yaml = %q, want %q", yamlText, tc.yaml)
			}
			if body != tc.body {
				t.Errorf("body = %q, want %q", body, tc.body)
			}
		})
	}
}

func TestParseFrontMatter(t *testing.T) {
	fm, body, err := parseFrontMatter("---\ntitle: \"Die Wartenwand\"\norder: 7\nlisting:\n  type: table\n---\n\ntext\n")
	if err != nil {
		t.Fatal(err)
	}
	if fm.Title != "Die Wartenwand" {
		t.Errorf("Title = %q", fm.Title)
	}
	if !fm.HasOrder || fm.Order != 7 {
		t.Errorf("Order = %d, HasOrder = %v", fm.Order, fm.HasOrder)
	}
	if fm.SortKey() != 7 {
		t.Errorf("SortKey = %d", fm.SortKey())
	}
	if strings.TrimSpace(body) != "text" {
		t.Errorf("body = %q", body)
	}
}

func TestParseFrontMatterWithoutOrder(t *testing.T) {
	fm, _, err := parseFrontMatter("---\ntitle: A\n---\n")
	if err != nil {
		t.Fatal(err)
	}
	if fm.HasOrder {
		t.Fatal("HasOrder should be false")
	}
	if fm.SortKey() != noOrder {
		t.Errorf("SortKey = %d, want %d", fm.SortKey(), noOrder)
	}
}

func TestParseFrontMatterInvalidYAML(t *testing.T) {
	if _, _, err := parseFrontMatter("---\ntitle: [unclosed\n---\nbody"); err == nil {
		t.Fatal("expected an error for malformed YAML")
	}
}

// A page's title and subtitle take {name} placeholders. One without a
// variable is reported, and leaves the text as it is written.
func TestExpandTitle(t *testing.T) {
	fm, _, err := parseFrontMatter("---\ntitle: \"{product} Handbook\"\nsubtitle: For {audience-label}\n---\nbody\n")
	if err != nil {
		t.Fatal(err)
	}
	n := &Node{Name: "book", Page: &Page{FrontMatter: fm}}
	if n.Subtitle() != "For {audience-label}" {
		t.Fatalf("subtitle = %q", n.Subtitle())
	}
	if err := n.ExpandTitle(map[string]string{"product": "Folio"}); err == nil || !strings.Contains(err.Error(), "audience-label") {
		t.Errorf("err = %v, want one naming audience-label", err)
	}
	if n.Title() != "Folio Handbook" || n.Subtitle() != "For {audience-label}" {
		t.Errorf("title, subtitle = %q, %q", n.Title(), n.Subtitle())
	}
	if err := n.ExpandTitle(map[string]string{"audience-label": "developers"}); err != nil {
		t.Fatal(err)
	}
	if n.Subtitle() != "For developers" {
		t.Errorf("subtitle = %q", n.Subtitle())
	}
	if (&Node{Name: "x"}).Subtitle() != "" || (&Node{Name: "x"}).ExpandTitle(nil) != nil {
		t.Error("a node without a page has a subtitle or fails to expand")
	}
}
