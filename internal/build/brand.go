package build

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/christophberger-ailab/foolscap-folio/internal/docrender"
	"github.com/christophberger-ailab/foolscap-folio/internal/foliocore"
)

// BrandFile is the Quarto brand file (https://posit-dev.github.io/brand-yml/)
// at the project root, or else in the user's config directory. Its colors
// and typography style the website, the PDF and a DOCX without a template.
const BrandFile = "_brand.yml"

type brand struct {
	// dir is the directory of the file, which its paths are relative to.
	dir   string
	Color struct {
		Palette    map[string]string `yaml:"palette"`
		Foreground string            `yaml:"foreground"`
		Background string            `yaml:"background"`
		Primary    string            `yaml:"primary"`
	} `yaml:"color"`
	Typography struct {
		Fonts           []brandFont `yaml:"fonts"`
		Base            brandStyle  `yaml:"base"`
		Headings        brandStyle  `yaml:"headings"`
		Monospace       brandStyle  `yaml:"monospace"`
		MonospaceInline brandStyle  `yaml:"monospace-inline"`
		MonospaceBlock  brandStyle  `yaml:"monospace-block"`
		Link            brandStyle  `yaml:"link"`
	} `yaml:"typography"`
	// Meta is the brand file's field for what brand.yml itself does not
	// define. folio reads the fonts of a document's cover and headings and
	// its cover image from it.
	Meta struct {
		// Fonts are a font and a size by metaRoles.
		Fonts map[string]struct {
			Font string `yaml:"font"`
			Size string `yaml:"size"`
		} `yaml:"fonts"`
		// Coverpage is the SVG image behind a document's cover page, relative
		// to the project root.
		Coverpage string `yaml:"coverpage"`
	} `yaml:"meta"`
}

// metaRoles are the texts meta.fonts sets: the cover's title and subtitle
// and the six heading levels. They are font roles of the PDF as well.
var metaRoles = []string{"title", "subtitle", "h1", "h2", "h3", "h4", "h5", "h6"}

// points reads a font size as points: a number, with pt or without a
// unit, or in px. Anything else is 0.
func points(s string) float64 {
	s = strings.TrimSpace(strings.ToLower(s))
	factor := 1.0
	if v, ok := strings.CutSuffix(s, "px"); ok {
		s, factor = v, 0.75
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, "pt")), 64)
	if err != nil || v <= 0 || v > 1000 {
		return 0
	}
	return v * factor
}

// loadBrand reads the project's brand file, or else the user's, whose path
// is user ("" for none). Without either the brand is nil.
func loadBrand(root, user string) (*brand, error) {
	for _, file := range []string{filepath.Join(root, BrandFile), user} {
		if file == "" {
			continue
		}
		data, err := os.ReadFile(file)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		b := &brand{dir: filepath.Dir(file)}
		if err = yaml.Unmarshal(data, b); err != nil {
			return nil, fmt.Errorf("%s: %w", BrandFile, err)
		}
		return b, nil
	}
	return nil, nil
}

type brandFont struct {
	Family string      `yaml:"family"`
	Source string      `yaml:"source"`
	Files  []brandFile `yaml:"files"`
}

// brandFile is a font file, given as a path or as path, weight and style.
type brandFile struct {
	Path   string `yaml:"path"`
	Weight string `yaml:"weight"`
	Style  string `yaml:"style"`
}

func (f *brandFile) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		f.Path = n.Value
		return nil
	}
	type plain brandFile
	return n.Decode((*plain)(f))
}

// brandStyle is a typography element, given as a family name or as a
// mapping of its properties.
type brandStyle struct {
	Family     string `yaml:"family"`
	Weight     string `yaml:"weight"`
	Style      string `yaml:"style"`
	Size       string `yaml:"size"`
	LineHeight string `yaml:"line-height"`
	Color      string `yaml:"color"`
	Background string `yaml:"background-color"`
	Decoration string `yaml:"decoration"`
}

func (s *brandStyle) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		s.Family = n.Value
		return nil
	}
	type plain brandStyle
	return n.Decode((*plain)(s))
}

var weightNames = map[string]int{
	"thin": 100, "hairline": 100, "extra-light": 200, "ultra-light": 200,
	"light": 300, "normal": 400, "regular": 400, "medium": 500,
	"semi-bold": 600, "demi-bold": 600, "bold": 700, "extra-bold": 800,
	"ultra-bold": 800, "black": 900, "heavy": 900,
}

// parseWeight reads a CSS font weight: a number, a name, or a range such
// as "100 900" that a variable font covers. def applies to "".
func parseWeight(s string, def int) (lo, hi int, err error) {
	fields := strings.Fields(strings.ToLower(s))
	if len(fields) == 0 {
		return def, def, nil
	}
	if len(fields) > 2 {
		return 0, 0, fmt.Errorf("invalid font weight %q", s)
	}
	var w [2]int
	for i, f := range fields {
		if w[i] = weightNames[f]; w[i] == 0 {
			if w[i], err = strconv.Atoi(f); err != nil || w[i] < 1 || w[i] > 1000 {
				return 0, 0, fmt.Errorf("invalid font weight %q", s)
			}
		}
	}
	if len(fields) == 1 {
		return w[0], w[0], nil
	}
	return min(w[0], w[1]), max(w[0], w[1]), nil
}

type brandFace struct {
	path   string
	lo, hi int
	italic bool
}

// fonts reads the PDF's fonts from the brand: the paths of TrueType files
// by docrender.FontRoles, and notes on the faces that stay Go fonts.
// Without a brand both are nil.
func (b *brand) fonts() (map[string]string, []string, error) {
	if b == nil {
		return nil, nil, nil
	}
	t := b.Typography
	families := map[string][]brandFace{}
	sources := map[string]string{}
	var notes []string
	for _, f := range t.Fonts {
		key := strings.ToLower(f.Family)
		if len(f.Files) == 0 {
			sources[key] = f.Source
			continue
		}
		for _, file := range f.Files {
			path, err := foliocore.ProjectPath(b.dir, file.Path)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: font %s: %w", BrandFile, f.Family, err)
			}
			lo, hi, err := parseWeight(file.Weight, 400)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: font %s: %w", BrandFile, f.Family, err)
			}
			if err = docrender.ValidateFont(path); errors.Is(err, fs.ErrNotExist) {
				return nil, nil, fmt.Errorf("%s: font %s: %w", BrandFile, f.Family, err)
			} else if err != nil {
				notes = append(notes, fmt.Sprintf("%s: %s: %v", BrandFile, file.Path, err))
				continue
			}
			families[key] = append(families[key], brandFace{path, lo, hi, file.Style == "italic" || file.Style == "oblique"})
		}
	}

	paths := map[string]string{}
	noted := map[string]bool{}
	// pick sets role to the face of family closest to weight and style:
	// the requested style if the family has it, then the nearest weight.
	pick := func(element, family string, s brandStyle, role string, weight int, italic bool) error {
		if family == "" {
			return nil
		}
		faces := families[strings.ToLower(family)]
		if len(faces) == 0 {
			if !noted[element] {
				noted[element] = true
				why := "has no TrueType files in " + BrandFile
				if src := sources[strings.ToLower(family)]; src != "" {
					why = "comes from " + src + ", which the PDF cannot embed"
				}
				notes = append(notes, fmt.Sprintf("%s: %s: font %s %s, so the PDF does not use it",
					BrandFile, element, family, why))
			}
			return nil
		}
		if s.Weight != "" {
			var err error
			if weight, _, err = parseWeight(s.Weight, 0); err != nil {
				return fmt.Errorf("%s: %s: %w", BrandFile, element, err)
			}
		}
		if s.Style != "" {
			italic = s.Style == "italic" || s.Style == "oblique"
		}
		best, bestD := "", 0
		for _, f := range faces {
			d := max(f.lo-weight, weight-f.hi, 0)
			if f.italic != italic {
				d += 10000
			}
			if best == "" || d < bestD {
				best, bestD = f.path, d
			}
		}
		paths[role] = best
		return nil
	}
	base := t.Base.Family
	bold := t.Base // the base element's weight applies to regular text only
	bold.Weight = ""
	heading := t.Headings.Family
	if heading == "" {
		heading = base
	}
	for _, p := range []struct {
		element, family string
		style           brandStyle
		role            string
		weight          int
		italic          bool
	}{
		{"typography: base", base, t.Base, "regular", 400, false},
		{"typography: base", base, bold, "bold", 700, false},
		{"typography: base", base, brandStyle{Weight: t.Base.Weight}, "italic", 400, true},
		{"typography: base", base, brandStyle{}, "bolditalic", 700, true},
		{"typography: headings", heading, t.Headings, "heading", 700, false},
		{"typography: monospace", t.Monospace.Family, t.Monospace, "mono", 400, false},
	} {
		if err := pick(p.element, p.family, p.style, p.role, p.weight, p.italic); err != nil {
			return nil, nil, err
		}
	}
	// The title and the headings are bold, as they are without a font of
	// their own; the subtitle is not.
	for _, role := range metaRoles {
		weight := 700
		if role == "subtitle" {
			weight = 400
		}
		if err := pick("meta: fonts: "+role, b.Meta.Fonts[role].Font, brandStyle{}, role, weight, false); err != nil {
			return nil, nil, err
		}
	}
	return paths, notes, nil
}

// brandLook is what the brand gives a document: the names of its fonts and
// its colors as RRGGBB, each "" where the brand has none, and the fonts and
// the sizes in points that meta.fonts gives the metaRoles.
type brandLook struct {
	Font, HeadingFont, MonoFont    string
	Color, HeadingColor, LinkColor string
	Fonts                          map[string]string
	Sizes                          map[string]float64
}

// style is the brand's look, and notes on what of meta.fonts it leaves out.
func (b *brand) style() (brandLook, []string) {
	if b == nil {
		return brandLook{}, nil
	}
	t := b.Typography
	look := brandLook{
		Font: fontName(t.Base.Family), HeadingFont: fontName(t.Headings.Family), MonoFont: fontName(t.Monospace.Family),
		Color: b.hex(b.Color.Foreground), HeadingColor: b.hex(t.Headings.Color),
		LinkColor: b.hex(cmp.Or(t.Link.Color, b.Color.Primary)),
		Fonts:     map[string]string{}, Sizes: map[string]float64{},
	}
	var notes []string
	for _, role := range slices.Sorted(maps.Keys(b.Meta.Fonts)) {
		f := b.Meta.Fonts[role]
		if !slices.Contains(metaRoles, role) {
			notes = append(notes, fmt.Sprintf("%s: meta: fonts: %s is none of %s", BrandFile, role, strings.Join(metaRoles, ", ")))
			continue
		}
		if look.Fonts[role] = fontName(f.Font); f.Font != "" && look.Fonts[role] == "" {
			notes = append(notes, fmt.Sprintf("%s: meta: fonts: %s: %q is not a font name", BrandFile, role, f.Font))
		}
		if look.Sizes[role] = points(f.Size); f.Size != "" && look.Sizes[role] == 0 {
			notes = append(notes, fmt.Sprintf("%s: meta: fonts: %s: %q is not a size in pt or px", BrandFile, role, f.Size))
		}
	}
	return look, notes
}

// color resolves a color given by its name in the palette or as a theme
// color, to the value behind it.
func (b *brand) color(v string) string {
	for range 8 { // a palette that names itself ends here
		switch next, ok := b.Color.Palette[v]; {
		case ok:
			v = next
		case v == "foreground":
			v = b.Color.Foreground
		case v == "background":
			v = b.Color.Background
		case v == "primary":
			v = b.Color.Primary
		default:
			return v
		}
	}
	return ""
}

var (
	reHex      = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
	reCSSValue = regexp.MustCompile(`^[#\w(),.%/ +-]+$`)
	reFontName = regexp.MustCompile(`^[\w .'-]+$`)
)

// hex is a color as RRGGBB, or "" for one that is not given as #rgb or
// #rrggbb.
func (b *brand) hex(v string) string {
	v = b.color(v)
	if !reHex.MatchString(v) {
		return ""
	}
	v = strings.ToUpper(v[1:])
	if len(v) == 3 {
		v = string([]byte{v[0], v[0], v[1], v[1], v[2], v[2]})
	}
	return v
}

// fontName is a family name that is safe to write into a stylesheet or a
// document, or "".
func fontName(family string) string {
	if !reFontName.MatchString(family) {
		return ""
	}
	return family
}

// css is the stylesheet the brand adds to a website, the font files it
// refers to (name below the stylesheet's fonts/ directory to source file),
// and notes on the values left out. Colors apply to the light color
// scheme; the dark one keeps the site's own.
func (b *brand) css() (string, map[string]string, []string) {
	if b == nil {
		return "", nil, nil
	}
	var notes []string
	// val checks a value before it goes into the stylesheet.
	val := func(what, v string, re *regexp.Regexp) string {
		if v != "" && !re.MatchString(v) {
			notes = append(notes, fmt.Sprintf("%s: %s: %q is not a value the website can use", BrandFile, what, v))
			return ""
		}
		return v
	}
	color := func(what, v string) string { return val(what, b.color(v), reCSSValue) }
	// decls joins the properties that have a value.
	decls := func(kv ...string) string {
		var d []string
		for i := 0; i < len(kv); i += 2 {
			if kv[i+1] != "" {
				d = append(d, kv[i]+":"+kv[i+1])
			}
		}
		return strings.Join(d, ";")
	}
	rule := func(sel, d string) string {
		if d == "" {
			return ""
		}
		return sel + "{" + d + "}\n"
	}

	t := b.Typography
	base := val("typography: base", t.Base.Family, reFontName)
	head := val("typography: headings", t.Headings.Family, reFontName)
	mono := val("typography: monospace", t.Monospace.Family, reFontName)

	var imports, faces strings.Builder
	files := map[string]string{}
	for _, f := range t.Fonts {
		if !reFontName.MatchString(f.Family) {
			continue
		}
		// A hosted font is asked for in the weights its elements use.
		weights := map[int]bool{400: true, 700: true}
		for _, s := range []brandStyle{t.Base, t.Headings, t.Monospace} {
			if w, _, err := parseWeight(s.Weight, 0); err == nil && w > 0 && strings.EqualFold(s.Family, f.Family) {
				weights[w] = true
			}
		}
		var google, bunny []string
		for _, w := range slices.Sorted(maps.Keys(weights)) {
			google = append(google, fmt.Sprintf("%d,%ditalic", w, w))
			bunny = append(bunny, fmt.Sprintf("%d,%di", w, w))
		}
		switch f.Source {
		case "google":
			fmt.Fprintf(&imports, "@import url(\"https://fonts.googleapis.com/css?family=%s:%s&display=swap\");\n",
				strings.ReplaceAll(f.Family, " ", "+"), strings.Join(google, ","))
		case "bunny":
			fmt.Fprintf(&imports, "@import url(\"https://fonts.bunny.net/css?family=%s:%s&display=swap\");\n",
				strings.ReplaceAll(strings.ToLower(f.Family), " ", "-"), strings.Join(bunny, ","))
		}
		for _, file := range f.Files {
			path, err := foliocore.ProjectPath(b.dir, file.Path)
			lo, hi, werr := parseWeight(file.Weight, 400)
			if st, serr := os.Stat(path); err != nil || werr != nil || serr != nil || st.IsDir() {
				notes = append(notes, fmt.Sprintf("%s: font %s: %s is not a font file the website can use", BrandFile, f.Family, file.Path))
				continue
			}
			name := fmt.Sprintf("%d-%s", len(files)+1, filepath.Base(path))
			files[name] = path
			weight := strconv.Itoa(lo)
			if hi != lo {
				weight += " " + strconv.Itoa(hi)
			}
			style := "normal"
			if file.Style == "italic" || file.Style == "oblique" {
				style = file.Style
			}
			fmt.Fprintf(&faces, "@font-face{font-family:%q;src:url(%q);font-weight:%s;font-style:%s}\n",
				f.Family, "fonts/"+url.PathEscape(name), weight, style)
		}
	}
	family := func(name, fallback string) string {
		if name == "" {
			return ""
		}
		return `"` + name + `",` + fallback
	}
	weight := func(what, v string) string {
		w, _, err := parseWeight(v, 0)
		if err != nil {
			notes = append(notes, fmt.Sprintf("%s: %s: %v", BrandFile, what, err))
		}
		if w == 0 {
			return ""
		}
		return strconv.Itoa(w)
	}
	text := func(what, v string) string { return val(what, v, reCSSValue) }

	const headings = "h1,h2,h3,h4,h5,h6"
	all := rule(":root", decls("--font", family(base, "system-ui,sans-serif"), "--mono", family(mono, "ui-monospace,monospace"),
		"--lh-body", text("typography: base: line-height", t.Base.LineHeight))) +
		rule("html", decls("font-size", text("typography: base: size", t.Base.Size))) +
		rule("body", decls("font-weight", weight("typography: base: weight", t.Base.Weight))) +
		rule(headings, decls("font-family", family(head, "var(--font)"),
			"font-weight", weight("typography: headings: weight", t.Headings.Weight),
			"font-style", text("typography: headings: style", t.Headings.Style),
			"line-height", text("typography: headings: line-height", t.Headings.LineHeight))) +
		rule("a", decls("font-weight", weight("typography: link: weight", t.Link.Weight),
			"text-decoration", text("typography: link: decoration", t.Link.Decoration))) +
		rule("code,pre", decls("font-size", text("typography: monospace: size", t.Monospace.Size),
			"font-weight", weight("typography: monospace: weight", t.Monospace.Weight)))

	bg, primary := color("color: background", b.Color.Background), color("color: primary", b.Color.Primary)
	soft := ""
	if primary != "" {
		soft = "color-mix(in srgb," + primary + " 12%,var(--bg))"
	}
	inline, block := t.MonospaceInline, t.MonospaceBlock
	light := rule(":root", decls("--bg", bg, "--nav-bg", bg, "--fg", color("color: foreground", b.Color.Foreground),
		"--accent", primary, "--accent-soft", soft,
		"--link", cmp.Or(color("typography: link: color", t.Link.Color), primary))) +
		rule(headings, decls("color", color("typography: headings: color", t.Headings.Color))) +
		rule("a", decls("background-color", color("typography: link: background-color", t.Link.Background))) +
		rule(":not(pre)>code", decls("color", color("typography: monospace-inline: color", inline.Color),
			"background-color", color("typography: monospace-inline: background-color", inline.Background))) +
		rule("pre,pre code", decls("color", color("typography: monospace-block: color", block.Color),
			"background-color", color("typography: monospace-block: background-color", block.Background)))
	if light != "" {
		light = "@media (prefers-color-scheme: light){\n" + light + "}\n"
	}
	if len(files) == 0 {
		files = nil
	}
	return imports.String() + faces.String() + all + light, files, notes
}
