package docx

import (
	"cmp"
	"encoding/xml"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// styleSet looks styles up by their display name, so that a template's
// "Heading 1" is found whatever its style ID, and adds a plain fallback
// definition for the ones the template lacks.
type styleSet struct {
	byName map[string]string // "type|lower name" -> style ID
	ids    map[string]bool
	memo   map[string]string
	added  []string
	look   Look // the brand's fonts and colors for the styles added
}

// Look is what a brand gives the styles folio defines: the names of the
// fonts of body text, headings and code, and the colors of text, headings
// and links as RRGGBB. Each is "" where the brand has none.
type Look struct {
	Font, HeadingFont, MonoFont    string
	Color, HeadingColor, LinkColor string
	// Fonts and Sizes (in points) are those of single heading levels, by
	// h1 to h6; a level without one has the headings' font and its own size.
	Fonts map[string]string
	Sizes map[string]float64
}

// rFonts sets a run's font in every script, or nothing for "".
func rFonts(name string) string {
	if name == "" {
		return ""
	}
	n := esc(name)
	return `<w:rFonts w:ascii="` + n + `" w:hAnsi="` + n + `" w:eastAsia="` + n + `" w:cs="` + n + `"/>`
}

// rColor sets a run's color, or nothing for "".
func rColor(rgb string) string {
	if rgb == "" {
		return ""
	}
	return `<w:color w:val="` + esc(rgb) + `"/>`
}

// halfPoints is a font size in points as Word counts it, in half-points.
func halfPoints(pt float64) int { return int(math.Round(2 * pt)) }

// mono is the font of code.
func (s *styleSet) mono() string {
	if s.look.MonoFont != "" {
		return rFonts(s.look.MonoFont)
	}
	return `<w:rFonts w:ascii="Consolas" w:hAnsi="Consolas" w:eastAsia="Consolas" w:cs="Courier New"/>`
}

func parseStyles(data []byte) *styleSet {
	s := &styleSet{byName: map[string]string{}, ids: map[string]bool{}, memo: map[string]string{}}
	var doc struct {
		Styles []struct {
			Type string `xml:"type,attr"`
			ID   string `xml:"styleId,attr"`
			Name struct {
				Val string `xml:"val,attr"`
			} `xml:"name"`
		} `xml:"style"`
	}
	_ = xml.Unmarshal(data, &doc) // a damaged styles part just means every style is added
	for _, st := range doc.Styles {
		s.ids[st.ID] = true
		key := st.Type + "|" + strings.ToLower(st.Name.Val)
		if _, dup := s.byName[key]; !dup {
			s.byName[key] = st.ID
		}
	}
	return s
}

// find returns the ID of the first style (of the type) named like one of
// names, or "".
func (s *styleSet) find(typ string, names ...string) string {
	for _, n := range names {
		if id, ok := s.byName[typ+"|"+strings.ToLower(n)]; ok {
			return id
		}
	}
	return ""
}

// need returns the ID of the style named like one of names; failing that
// it adds the fallback definition under id and returns that.
func (s *styleSet) need(typ, id, body string, names ...string) string {
	key := typ + "|" + id
	if v, ok := s.memo[key]; ok {
		return v
	}
	found := s.find(typ, names...)
	if found == "" {
		for s.ids[id] {
			id += "Qm"
		}
		s.ids[id] = true
		s.byName[typ+"|"+strings.ToLower(names[0])] = id
		s.added = append(s.added, fmt.Sprintf(`<w:style w:type="%s" w:customStyle="1" w:styleId="%s"><w:name w:val="%s"/>%s</w:style>`,
			typ, id, esc(names[0]), body))
		found = id
	}
	s.memo[key] = found
	return found
}

func (s *styleSet) normal() string {
	if id := s.find("paragraph", "Normal"); id != "" {
		return id
	}
	return "Normal"
}

func (s *styleSet) heading(level int) string {
	id := fmt.Sprintf("Heading%d", level)
	role := fmt.Sprintf("h%d", level)
	size := cmp.Or(halfPoints(s.look.Sizes[role]), []int{32, 28, 26, 24, 22, 22}[level-1])
	body := fmt.Sprintf(`<w:basedOn w:val="%s"/><w:next w:val="%s"/><w:qFormat/>`+
		`<w:pPr><w:keepNext/><w:keepLines/><w:spacing w:before="240" w:after="120"/><w:outlineLvl w:val="%d"/></w:pPr>`+
		`<w:rPr>%s<w:b/><w:sz w:val="%d"/><w:szCs w:val="%d"/></w:rPr>`,
		s.normal(), s.normal(), level-1, rFonts(s.look.Fonts[role]), size, size)
	return s.need("paragraph", id, body, fmt.Sprintf("heading %d", level), fmt.Sprintf("Heading %d", level))
}

// body is the style of ordinary paragraphs: "Body Text" if the document has
// it, else none (Normal).
func (s *styleSet) body() string { return s.find("paragraph", "Body Text") }

func (s *styleSet) listPara() string { return s.find("paragraph", "List Paragraph") }

func (s *styleSet) quote() string {
	return s.need("paragraph", "Quote", `<w:basedOn w:val="`+s.normal()+`"/><w:qFormat/>`+
		`<w:pPr><w:pBdr><w:left w:val="single" w:sz="18" w:space="8" w:color="BFBFBF"/></w:pBdr><w:ind w:left="567" w:right="567"/></w:pPr>`+
		`<w:rPr><w:i/></w:rPr>`, "Block Text", "Quote")
}

func (s *styleSet) code() string {
	return s.need("paragraph", "SourceCode", `<w:basedOn w:val="`+s.normal()+`"/>`+
		`<w:pPr><w:keepLines/><w:shd w:val="clear" w:color="auto" w:fill="F5F5F5"/><w:spacing w:before="120" w:after="120" w:line="240" w:lineRule="auto"/></w:pPr>`+
		`<w:rPr>`+s.mono()+`<w:sz w:val="18"/><w:szCs w:val="18"/></w:rPr>`,
		"Source Code")
}

// imageCaption is the style of a figure's caption, Pandoc's "Image
// Caption", or the template's "Caption"; the one added is small and
// italic.
func (s *styleSet) imageCaption() string {
	return s.need("paragraph", "ImageCaption", `<w:basedOn w:val="`+s.normal()+`"/><w:qFormat/>`+
		`<w:pPr><w:spacing w:before="0" w:after="240"/></w:pPr>`+
		`<w:rPr><w:i/><w:sz w:val="20"/><w:szCs w:val="20"/></w:rPr>`,
		"Image Caption", "Caption")
}

func (s *styleSet) verbatim() string {
	return s.need("character", "VerbatimChar",
		`<w:rPr>`+s.mono()+`<w:sz w:val="20"/><w:szCs w:val="20"/></w:rPr>`,
		"Verbatim Char")
}

func (s *styleSet) hyperlink() string {
	return s.need("character", "Hyperlink",
		`<w:rPr><w:color w:val="0563C1"/><w:u w:val="single"/></w:rPr>`, "Hyperlink")
}

func (s *styleSet) footnoteText() string {
	return s.need("paragraph", "FootnoteText", `<w:basedOn w:val="`+s.normal()+`"/>`+
		`<w:pPr><w:spacing w:after="0" w:line="240" w:lineRule="auto"/></w:pPr><w:rPr><w:sz w:val="20"/><w:szCs w:val="20"/></w:rPr>`,
		"footnote text")
}

func (s *styleSet) footnoteRef() string {
	return s.need("character", "FootnoteReference", `<w:rPr><w:vertAlign w:val="superscript"/></w:rPr>`,
		"footnote reference")
}

func (s *styleSet) tocHeading() string {
	return s.need("paragraph", "TOCHeading", `<w:basedOn w:val="`+s.normal()+`"/><w:next w:val="`+s.normal()+`"/>`+
		`<w:pPr><w:keepNext/><w:spacing w:before="240" w:after="120"/></w:pPr><w:rPr>`+rFonts(cmp.Or(s.look.HeadingFont, s.look.Font))+
		`<w:b/>`+rColor(s.look.HeadingColor)+`<w:sz w:val="32"/><w:szCs w:val="32"/></w:rPr>`,
		"TOC Heading")
}

func (s *styleSet) toc(level int) string {
	return s.need("paragraph", fmt.Sprintf("TOC%d", level), fmt.Sprintf(
		`<w:basedOn w:val="%s"/><w:next w:val="%s"/><w:uiPriority w:val="39"/><w:pPr><w:spacing w:after="100"/><w:ind w:left="%d"/></w:pPr>`,
		s.normal(), s.normal(), 220*(level-1)), fmt.Sprintf("toc %d", level))
}

// appendTo adds the fallback definitions to styles.xml.
func (s *styleSet) appendTo(styles string) string {
	if len(s.added) == 0 {
		return styles
	}
	return insertBefore(styles, "</w:styles>", strings.Join(s.added, ""))
}

var (
	reRPrDefault = regexp.MustCompile(`(<w:rPrDefault>\s*<w:rPr>)`)
	reSpacingDef = regexp.MustCompile(`(?s)(<w:pPrDefault>.*?)<w:spacing[^>]*>(?:</w:spacing>)?`)
	reLangTag    = regexp.MustCompile(`<w:lang\b[^>]*?(?:/>|>\s*</w:lang>)`)
)

var (
	reStyle     = regexp.MustCompile(`(?s)<w:style\b[^>]*w:styleId="(Heading\d|Title|Hyperlink)".*?</w:style>`)
	reStyleFont = regexp.MustCompile(`<w:rFonts\b[^>]*?(?:/>|>\s*</w:rFonts>)`)
	reStyleCol  = regexp.MustCompile(`<w:color\b[^>]*?(?:/>|>\s*</w:color>)`)
	reStyleSize = regexp.MustCompile(`<w:(sz|szCs)\b[^>]*?(?:/>|>\s*</w:(?:sz|szCs)>)`)
)

// tuneBase gives the docxgo base document, whose Normal style carries no
// font at all, a readable body font and paragraph spacing, and its
// defaults, headings and links the brand's fonts and colors.
func tuneBase(styles string, l Look) string {
	if reRPrDefault.MatchString(styles) {
		styles = reRPrDefault.ReplaceAllString(styles,
			`$1`+rFonts(cmp.Or(l.Font, "Calibri"))+rColor(l.Color)+`<w:sz w:val="22"/><w:szCs w:val="22"/>`)
	} else if brand := rFonts(l.Font) + rColor(l.Color); brand != "" {
		styles = strings.Replace(styles, "<w:docDefaults>",
			"<w:docDefaults><w:rPrDefault><w:rPr>"+brand+"</w:rPr></w:rPrDefault>", 1)
	}
	styles = reStyle.ReplaceAllStringFunc(styles, func(st string) string {
		// A heading level is the role h1 to h6; the other styles are none.
		role := strings.Replace(reStyle.FindStringSubmatch(st)[1], "Heading", "h", 1)
		font, col := rFonts(cmp.Or(l.Fonts[role], l.HeadingFont, l.Font)), rColor(l.HeadingColor)
		if strings.Contains(st, `w:styleId="Hyperlink"`) {
			font, col = "", rColor(l.LinkColor)
		}
		if size := halfPoints(l.Sizes[role]); size > 0 {
			st = reStyleSize.ReplaceAllString(st, fmt.Sprintf(`<w:$1 w:val="%d"/>`, size))
		}
		if font != "" {
			st = reStyleFont.ReplaceAllLiteralString(st, font)
		}
		if col != "" {
			// The color follows the font and the bold mark in a run's properties.
			st = reStyleCol.ReplaceAllLiteralString(st, "")
			at := max(strings.Index(st, "<w:sz "), strings.Index(st, "<w:u "))
			if at < 0 {
				return st
			}
			st = st[:at] + col + st[at:]
		}
		return st
	})
	return reSpacingDef.ReplaceAllString(styles,
		`${1}<w:spacing w:before="0" w:after="120" w:line="276" w:lineRule="auto"/>`)
}

// setLang sets the proofing language in the document defaults of a
// template's styles.
func setLang(styles, lang string) string {
	if lang == "" {
		return styles
	}
	start := strings.Index(styles, "<w:docDefaults>")
	end := strings.Index(styles, "</w:docDefaults>")
	if start < 0 || end < start {
		return styles
	}
	defaults := styles[start:end]
	tag := `<w:lang w:val="` + esc(lang) + `"/>`
	if reLangTag.MatchString(defaults) {
		defaults = reLangTag.ReplaceAllLiteralString(defaults, tag)
	} else if i := strings.Index(defaults, "</w:rPr>"); i >= 0 && strings.Contains(defaults, "<w:rPrDefault>") {
		defaults = defaults[:i] + tag + defaults[i:]
	} else if i := strings.Index(defaults, "<w:rPrDefault/>"); i >= 0 {
		defaults = strings.Replace(defaults, "<w:rPrDefault/>", "<w:rPrDefault><w:rPr>"+tag+"</w:rPr></w:rPrDefault>", 1)
	} else {
		defaults = strings.Replace(defaults, "<w:docDefaults>", "<w:docDefaults><w:rPrDefault><w:rPr>"+tag+"</w:rPr></w:rPrDefault>", 1)
	}
	return styles[:start] + defaults + styles[end:]
}
