package build

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/christophberger-ailab/foolscap-folio/internal/docrender"
	"github.com/christophberger-ailab/foolscap-folio/internal/docx"
	"github.com/christophberger-ailab/foolscap-folio/internal/foliocore"
)

// BrandFile is the Quarto brand file (https://posit-dev.github.io/brand-yml/)
// at the project root. The qm renderer sets the PDF in its typography
// fonts; without it, or for any face it does not provide, the PDF keeps
// the Go fonts.
const BrandFile = "_brand.yml"

type brand struct {
	Typography struct {
		Fonts     []brandFont `yaml:"fonts"`
		Base      brandStyle  `yaml:"base"`
		Headings  brandStyle  `yaml:"headings"`
		Monospace brandStyle  `yaml:"monospace"`
	} `yaml:"typography"`
	// Meta holds what folio reads beyond the brand.yml standard: the fonts
	// and sizes of the cover's title and subtitle and the headings by
	// level (title, subtitle, h1 to h6), and the cover page template.
	Meta struct {
		Fonts     map[string]metaFont `yaml:"fonts"`
		Coverpage string              `yaml:"coverpage"`
	} `yaml:"meta"`
}

// metaFont is a font family and a size, such as "36pt", in meta.fonts.
type metaFont struct {
	Font string `yaml:"font"`
	Size string `yaml:"size"`
}

// metaRoles are the entries of meta.fonts, which are font roles of the PDF
// as well.
var metaRoles = []string{"title", "subtitle", "h1", "h2", "h3", "h4", "h5", "h6"}

// loadBrand reads the project's _brand.yml, or returns nil if it has none.
func loadBrand(root string) (*brand, error) {
	data, err := os.ReadFile(filepath.Join(root, BrandFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var b brand
	if err = yaml.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("%s: %w", BrandFile, err)
	}
	for role, f := range b.Meta.Fonts {
		if !slices.Contains(metaRoles, role) {
			return nil, fmt.Errorf("%s: meta: fonts: unknown entry %q, want one of %s", BrandFile, role, strings.Join(metaRoles, ", "))
		}
		if _, err := parseSize(f.Size); err != nil {
			return nil, fmt.Errorf("%s: meta: fonts: %s: %w", BrandFile, role, err)
		}
	}
	return &b, nil
}

// parseSize reads a font size in pt: a number, in pt by default, or with
// the unit pt or px. "" is 0.
func parseSize(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	num, scale := s, 1.0
	switch {
	case strings.HasSuffix(s, "pt"):
		num = strings.TrimSuffix(s, "pt")
	case strings.HasSuffix(s, "px"):
		num, scale = strings.TrimSuffix(s, "px"), 0.75
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(num), 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("invalid font size %q, want a size in pt or px", s)
	}
	return v * scale, nil
}

// size returns the size meta.fonts sets for role, or def.
func (b *brand) size(role string, def float64) float64 {
	if b != nil {
		if s, _ := parseSize(b.Meta.Fonts[role].Size); s > 0 {
			return s
		}
	}
	return def
}

// font returns the family meta.fonts sets for role, or def.
func (b *brand) font(role, def string) string {
	if b != nil && b.Meta.Fonts[role].Font != "" {
		return b.Meta.Fonts[role].Font
	}
	return def
}

// headings returns the fonts of the headings by level for the DOCX, and
// the sizes by role for the PDF.
func (b *brand) headings() ([6]docx.Font, map[string]float64) {
	var fonts [6]docx.Font
	sizes := map[string]float64{}
	for i := range fonts {
		role := fmt.Sprintf("h%d", i+1)
		fonts[i] = docx.Font{Name: b.font(role, ""), Size: b.size(role, 0)}
		if fonts[i].Size > 0 {
			sizes[role] = fonts[i].Size
		}
	}
	sizes["title"], sizes["subtitle"] = b.size("title", titleSize), b.size("subtitle", subtitleSize)
	return fonts, sizes
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

// brandStyle is a typography element, given as a family name or as family,
// weight and style.
type brandStyle struct {
	Family string `yaml:"family"`
	Weight string `yaml:"weight"`
	Style  string `yaml:"style"`
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

// pdfFonts returns the PDF's fonts from the project's _brand.yml: the
// paths of TrueType files by docrender.FontRoles, and notes on the faces
// that stay Go fonts. Without a brand file both are nil.
func (b *brand) pdfFonts(root string) (map[string]string, []string, error) {
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
			path, err := foliocore.ProjectPath(root, file.Path)
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
	type face struct {
		element, family string
		style           brandStyle
		role            string
		weight          int
		italic          bool
	}
	faces := []face{
		{"typography: base", base, t.Base, "regular", 400, false},
		{"typography: base", base, bold, "bold", 700, false},
		{"typography: base", base, brandStyle{Weight: t.Base.Weight}, "italic", 400, true},
		{"typography: base", base, brandStyle{}, "bolditalic", 700, true},
		{"typography: headings", heading, t.Headings, "heading", 700, false},
		{"typography: monospace", t.Monospace.Family, t.Monospace, "mono", 400, false},
	}
	for _, role := range metaRoles {
		weight := 700
		if role == "subtitle" {
			weight = 400
		}
		faces = append(faces, face{"meta: fonts: " + role, b.Meta.Fonts[role].Font, brandStyle{}, role, weight, false})
	}
	for _, p := range faces {
		if err := pick(p.element, p.family, p.style, p.role, p.weight, p.italic); err != nil {
			return nil, nil, err
		}
	}
	return paths, notes, nil
}
