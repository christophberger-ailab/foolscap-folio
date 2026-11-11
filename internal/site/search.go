package site

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
)

// searchDoc is one page in the site's search index. The short JSON names
// keep the index small; it is loaded by every visitor who searches.
type searchDoc struct {
	Title   string `json:"t"`
	URL     string `json:"u"` // relative to the site root
	Section string `json:"s"` // the topic's title
	Body    string `json:"b"`
}

// blockTags end a run of text: their text is not joined to what follows.
var blockTags = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true, "br": true, "dd": true,
	"details": true, "div": true, "dl": true, "dt": true, "figcaption": true, "figure": true,
	"footer": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"header": true, "hr": true, "img": true, "li": true, "ol": true, "p": true, "pre": true,
	"section": true, "summary": true, "table": true, "td": true, "th": true, "tr": true, "ul": true,
}

// htmlText is the visible text of an HTML fragment, its whitespace
// collapsed, without the content of scripts and styles.
func htmlText(s string) string {
	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(s))
	skip := ""
	for {
		switch z.Next() {
		case html.ErrorToken:
			return strings.Join(strings.Fields(b.String()), " ")
		case html.TextToken:
			if skip == "" {
				b.Write(z.Text())
			}
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			tag := string(name)
			switch {
			case skip != "":
				if tag == skip {
					skip = ""
				}
			case tag == "script" || tag == "style":
				skip = tag
			case blockTags[tag]:
				b.WriteByte(' ')
			}
		}
	}
}

// writeSearch writes the search index as a script, not as JSON: a page
// opened from file:// may load a script but not fetch a file.
func (b *builder) writeSearch(dir string) error {
	data, err := json.Marshal(b.search)
	if err != nil {
		return err
	}
	js := append(append([]byte("window.folioSearch="), data...), ";\n"...)
	return os.WriteFile(filepath.Join(dir, "search.js"), js, 0o644)
}
