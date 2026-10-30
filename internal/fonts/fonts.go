// Package fonts loads the TrueType files used to set a CV. Liberation Sans, a
// free Helvetica/Arial-metric sans serif with full Latin coverage (Turkish
// included), is embedded as the default; other families come from disk or
// from Google Fonts.
package fonts

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/obk/cv-maker/internal/config"
)

//go:embed liberation/*.ttf
var embedded embed.FS

// Styles are the fpdf style strings, in the order files are reported.
var Styles = []string{"", "B", "I", "BI"}

// Set is the TrueType data for each style in Styles.
type Set map[string][]byte

// Load returns the font data for f. Explicit files win; otherwise the family
// is taken from the embedded fonts, a "fonts" folder next to the CV
// (searchDir/fonts), the fonts installed on the system or Google Fonts,
// depending on f.Source.
func Load(f config.Font, searchDir string) (Set, error) {
	if f.Files != (config.FontFiles{}) {
		return readFiles(map[string]string{
			"": f.Files.Regular, "B": f.Files.Bold, "I": f.Files.Italic, "BI": f.Files.BoldItalic,
		}, os.ReadFile)
	}
	if key(f.Family) == key(config.DefaultFamily) {
		return readFiles(map[string]string{
			"":   "liberation/LiberationSans-Regular.ttf",
			"B":  "liberation/LiberationSans-Bold.ttf",
			"I":  "liberation/LiberationSans-Italic.ttf",
			"BI": "liberation/LiberationSans-BoldItalic.ttf",
		}, embedded.ReadFile)
	}
	var (
		files map[string]string
		err   error
	)
	if f.Source != "google" {
		files, err = find(f.Family, append([]string{filepath.Join(searchDir, "fonts")}, systemDirs()...))
	}
	if f.Source == "google" || (f.Source == "auto" && errors.Is(err, errNotFound)) {
		files, err = google(f.Family, f.Weight, f.BoldWeight)
	}
	if err != nil {
		return nil, err
	}
	return readFiles(files, os.ReadFile)
}

var errNotFound = errors.New("not installed")

func readFiles(files map[string]string, read func(string) ([]byte, error)) (Set, error) {
	set := Set{}
	for _, style := range Styles {
		data, err := read(files[style])
		if err != nil {
			return nil, fmt.Errorf("font: %w", err)
		}
		set[style] = data
	}
	return set, nil
}

// suffixes lists the file name endings accepted for each style, so that
// "Inter" matches Inter-Regular.ttf, Inter-Bold.ttf and so on.
var suffixes = map[string][]string{
	"":   {"regular", "", "book", "roman"},
	"B":  {"bold"},
	"I":  {"italic", "oblique", "regularitalic"},
	"BI": {"bolditalic", "boldoblique"},
}

func find(family string, dirs []string) (map[string]string, error) {
	want := key(family)
	byName := map[string]string{}
	for _, dir := range dirs {
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".ttf") {
				return nil
			}
			name := key(strings.TrimSuffix(d.Name(), filepath.Ext(path)))
			if _, seen := byName[name]; !seen && strings.HasPrefix(name, want) {
				byName[name] = path
			}
			return nil
		})
	}
	files := map[string]string{}
	var missing []string
	for _, style := range Styles {
		for _, s := range suffixes[style] {
			if p, ok := byName[want+s]; ok {
				files[style] = p
				break
			}
		}
		if files[style] == "" {
			missing = append(missing, map[string]string{"": "regular", "B": "bold", "I": "italic", "BI": "bold italic"}[style])
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("font: no TrueType (.ttf) files found for family %q; put them in a fonts/ folder next to the CV or list them under font.files (OpenType .otf and Type 1 fonts are not supported): %w", family, errNotFound)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("font: family %q has no %s style; list the files under font.files instead", family, strings.Join(missing, ", "))
	}
	return files, nil
}

// key normalizes a family or file name: "IBM Plex Sans" and
// "IBMPlexSans-Regular" share the prefix "ibmplexsans".
func key(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '_' {
			return -1
		}
		return r
	}, strings.ToLower(s))
}

func systemDirs() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return []string{filepath.Join(home, "Library/Fonts"), "/Library/Fonts", "/System/Library/Fonts"}
	case "windows":
		return []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), `Microsoft\Windows\Fonts`),
			filepath.Join(os.Getenv("WINDIR"), "Fonts"),
		}
	default:
		return []string{
			filepath.Join(home, ".local/share/fonts"), filepath.Join(home, ".fonts"),
			"/usr/local/share/fonts", "/usr/share/fonts",
		}
	}
}
