// Package config holds the layout settings of a CV. Settings are layered:
// built-in defaults, then config.yml, then the optional YAML front matter at
// the top of the Markdown file. Each layer only needs the keys it changes.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config describes how a CV is laid out. Lengths are in points unless the
// field documents otherwise.
type Config struct {
	// PDF metadata. Title defaults to the name in the first heading.
	Title    string   `yaml:"title"`
	Author   string   `yaml:"author"`
	Subject  string   `yaml:"subject"`
	Keywords []string `yaml:"keywords"`
	Lang     string   `yaml:"lang"`

	Page    string  `yaml:"page"` // A4, Letter or Legal
	Margins Margins `yaml:"margins"`
	Font    Font    `yaml:"font"`
	Colors  Colors  `yaml:"colors"`
	Spacing Spacing `yaml:"spacing"`
	Photo   Photo   `yaml:"photo"`

	HardWraps      bool    `yaml:"hard_wraps"` // a new source line is a new line in the PDF
	Justify        bool    `yaml:"justify"`
	Uppercase      bool    `yaml:"uppercase"` // name and section titles
	UnderlineLinks bool    `yaml:"underline_links"`
	RuleWidth      float64 `yaml:"rule_width"`
}

// Margins are in millimetres.
type Margins struct {
	Top    float64 `yaml:"top"`
	Bottom float64 `yaml:"bottom"`
	Left   float64 `yaml:"left"`
	Right  float64 `yaml:"right"`
}

// Font selects the typeface and the type scale.
type Font struct {
	// Family is "Liberation Sans" (embedded) or any other family name, which
	// is looked up according to Source.
	Family string `yaml:"family"`
	// Source says where a family other than the embedded one comes from:
	// "local" (a fonts/ folder next to the CV, then the system fonts),
	// "google" (downloaded from fonts.google.com and cached) or "auto"
	// (local first, then Google Fonts).
	Source string `yaml:"source"`
	// Weight and BoldWeight pick the weights downloaded from Google Fonts,
	// e.g. 300 and 600 for a lighter look. Local fonts use their own files.
	Weight     int `yaml:"weight"`
	BoldWeight int `yaml:"bold_weight"`
	// Files, when set, wins over Family.
	Files FontFiles `yaml:"files"`

	Size        float64 `yaml:"size"`         // body text
	LineHeight  float64 `yaml:"line_height"`  // multiple of the size
	NameSize    float64 `yaml:"name_size"`    // the "# Name" heading
	SectionSize float64 `yaml:"section_size"` // "## Section" titles
	MetaSize    float64 `yaml:"meta_size"`    // dates next to entries
}

// FontFiles are paths to TrueType (.ttf) files, one per style.
type FontFiles struct {
	Regular    string `yaml:"regular"`
	Bold       string `yaml:"bold"`
	Italic     string `yaml:"italic"`
	BoldItalic string `yaml:"bold_italic"`
}

// Colors are hex strings such as "#333333" or "333".
type Colors struct {
	Text  string `yaml:"text"`
	Muted string `yaml:"muted"` // header lines, dates
	Rule  string `yaml:"rule"`
	Link  string `yaml:"link"`
}

// Spacing is the vertical space, in points, placed before each kind of block.
type Spacing struct {
	Header  float64 `yaml:"header"`  // between the name and the lines below it
	Section float64 `yaml:"section"` // before a section title
	Entry   float64 `yaml:"entry"`   // before an entry (### heading)
	Block   float64 `yaml:"block"`   // before paragraphs and lists
	Item    float64 `yaml:"item"`    // between list items
}

// Photo places an optional picture in the header. Leaving Path empty keeps
// the CV text only, which is what most applicant tracking systems prefer.
type Photo struct {
	Path     string  `yaml:"path"`     // JPEG or PNG
	Size     float64 `yaml:"size"`     // width and height in millimetres
	Shape    string  `yaml:"shape"`    // square, rounded or circle
	Position string  `yaml:"position"` // right or left
}

// DefaultFamily is the embedded font.
const DefaultFamily = "Liberation Sans"

// Default returns the settings that reproduce the swiss-minimal-cv look.
func Default() Config {
	return Config{
		Page:    "A4",
		Margins: Margins{Top: 12.5, Bottom: 12.5, Left: 15, Right: 15},
		Font: Font{
			Family:      DefaultFamily,
			Source:      "auto",
			Weight:      400,
			BoldWeight:  700,
			Size:        10.5,
			LineHeight:  1.35,
			NameSize:    24,
			SectionSize: 12,
			MetaSize:    8.5,
		},
		Colors:    Colors{Text: "#000000", Muted: "#333333", Rule: "#000000", Link: "#000000"},
		Spacing:   Spacing{Header: 4, Section: 11, Entry: 7, Block: 2, Item: 1.5},
		Photo:     Photo{Size: 28, Shape: "square", Position: "right"},
		HardWraps: true,
		Justify:   true,
		Uppercase: true,
		RuleWidth: 0.5,
	}
}

// Load builds the configuration for a CV: defaults, then the YAML file at
// configPath (skipped when empty), then the front matter. Relative paths in
// each layer are resolved against the directory of the file they came from.
func Load(configPath string, frontMatter []byte, markdownDir string) (Config, error) {
	cfg := Default()
	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return cfg, err
		}
		if err := cfg.apply(data, filepath.Dir(configPath)); err != nil {
			return cfg, fmt.Errorf("%s: %w", configPath, err)
		}
	}
	if err := cfg.apply(frontMatter, markdownDir); err != nil {
		return cfg, fmt.Errorf("front matter: %w", err)
	}
	return cfg, cfg.Validate()
}

// apply decodes one YAML layer on top of cfg. Unknown keys are reported so
// typos do not go unnoticed.
func (c *Config) apply(data []byte, dir string) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	before := *c
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(c); err != nil {
		return err
	}
	// Explicit font files in this layer replace a family from an earlier one
	// and vice versa, so the two never mix.
	if c.Font.Files != before.Font.Files {
		c.Font.Family = ""
	} else if c.Font.Family != before.Font.Family {
		c.Font.Files = FontFiles{}
	}
	for _, p := range []*string{
		&c.Font.Files.Regular, &c.Font.Files.Bold, &c.Font.Files.Italic,
		&c.Font.Files.BoldItalic, &c.Photo.Path,
	} {
		if *p != "" && !filepath.IsAbs(*p) {
			*p = filepath.Join(dir, *p)
		}
	}
	return nil
}

// Validate checks values that would otherwise produce a broken page.
func (c Config) Validate() error {
	var errs []error
	if _, ok := PageSizes[strings.ToLower(c.Page)]; !ok {
		errs = append(errs, fmt.Errorf("page: unknown size %q (use A4, Letter or Legal)", c.Page))
	}
	for _, v := range []struct {
		name string
		val  float64
	}{
		{"font.size", c.Font.Size}, {"font.line_height", c.Font.LineHeight},
		{"font.name_size", c.Font.NameSize}, {"font.section_size", c.Font.SectionSize},
		{"font.meta_size", c.Font.MetaSize}, {"photo.size", c.Photo.Size},
	} {
		if v.val <= 0 {
			errs = append(errs, fmt.Errorf("%s: must be greater than 0", v.name))
		}
	}
	for _, v := range []struct{ name, val string }{
		{"colors.text", c.Colors.Text}, {"colors.muted", c.Colors.Muted},
		{"colors.rule", c.Colors.Rule}, {"colors.link", c.Colors.Link},
	} {
		if _, err := ParseColor(v.val); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", v.name, err))
		}
	}
	f := c.Font.Files
	if f != (FontFiles{}) && (f.Regular == "" || f.Bold == "" || f.Italic == "" || f.BoldItalic == "") {
		errs = append(errs, errors.New("font.files: set all of regular, bold, italic and bold_italic"))
	}
	if f == (FontFiles{}) && c.Font.Family == "" {
		errs = append(errs, errors.New("font: set either family or files"))
	}
	switch c.Photo.Shape {
	case "square", "rounded", "circle":
	default:
		errs = append(errs, fmt.Errorf("photo.shape: %q is not one of square, rounded, circle", c.Photo.Shape))
	}
	switch c.Photo.Position {
	case "right", "left":
	default:
		errs = append(errs, fmt.Errorf("photo.position: %q is not one of right, left", c.Photo.Position))
	}
	switch c.Font.Source {
	case "auto", "local", "google":
	default:
		errs = append(errs, fmt.Errorf("font.source: %q is not one of auto, local, google", c.Font.Source))
	}
	for _, w := range []struct {
		name string
		val  int
	}{{"font.weight", c.Font.Weight}, {"font.bold_weight", c.Font.BoldWeight}} {
		if w.val < 100 || w.val > 900 || w.val%100 != 0 {
			errs = append(errs, fmt.Errorf("%s: %d is not one of 100, 200, ... 900", w.name, w.val))
		}
	}
	return errors.Join(errs...)
}

// Split separates a leading "---" delimited front matter block from the
// Markdown body. Without front matter, meta is nil and body is src.
func Split(src []byte) (meta, body []byte) {
	src = bytes.TrimPrefix(src, []byte("\ufeff"))
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(src, []byte("---\n")) {
		return nil, src
	}
	rest := src[len("---\n"):]
	for off := 0; off < len(rest); {
		end := bytes.IndexByte(rest[off:], '\n')
		line := rest[off:]
		if end >= 0 {
			line = rest[off : off+end]
		}
		if t := string(bytes.TrimRight(line, " \t")); t == "---" || t == "..." {
			if end < 0 {
				return rest[:off], nil
			}
			return rest[:off], rest[off+end+1:]
		}
		if end < 0 {
			break
		}
		off += end + 1
	}
	return nil, src
}

// PageSizes maps lower-case page names to width and height in points.
var PageSizes = map[string][2]float64{
	"a4":     {595.28, 841.89},
	"letter": {612, 792},
	"legal":  {612, 1008},
}

// RGB is a color with 0-255 components.
type RGB struct{ R, G, B int }

// ParseColor accepts "#rrggbb", "rrggbb", "#rgb" and "rgb".
func ParseColor(s string) (RGB, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return RGB{}, fmt.Errorf("invalid color %q", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return RGB{}, fmt.Errorf("invalid color %q", s)
	}
	return RGB{int(v >> 16 & 0xff), int(v >> 8 & 0xff), int(v & 0xff)}, nil
}
