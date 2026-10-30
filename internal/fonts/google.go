package fonts

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// cssAPI is the Google Fonts CSS endpoint. Asked by a client it does not
// recognise as a browser, it answers with links to static .ttf files, which
// is exactly the format fpdf embeds.
var cssAPI = "https://fonts.googleapis.com/css2"

var client = &http.Client{Timeout: 30 * time.Second}

// google returns cached TrueType files for family in the given weights,
// downloading them from Google Fonts on first use. Styles the family lacks
// fall back to the nearest one it has (an upright face for italics, the
// closest weight for bold).
func google(family string, weight, boldWeight int) (map[string]string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("font: %w", err)
	}
	dir := filepath.Join(cache, "cvmaker", "fonts", fmt.Sprintf("%s-%d-%d", key(family), weight, boldWeight))
	files := map[string]string{}
	for _, style := range Styles {
		files[style] = filepath.Join(dir, fileNames[style])
	}
	if cached(files) {
		return files, nil
	}

	faces, err := fetchFaces(family, weight, boldWeight)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("font: %w", err)
	}
	want := map[string]face{
		"":   {italic: false, weight: weight},
		"B":  {italic: false, weight: boldWeight},
		"I":  {italic: true, weight: weight},
		"BI": {italic: true, weight: boldWeight},
	}
	for _, style := range Styles {
		if err := download(nearest(faces, want[style]).url, files[style]); err != nil {
			return nil, fmt.Errorf("font: downloading %s: %w", family, err)
		}
	}
	return files, nil
}

var fileNames = map[string]string{"": "regular.ttf", "B": "bold.ttf", "I": "italic.ttf", "BI": "bold_italic.ttf"}

func cached(files map[string]string) bool {
	for _, f := range files {
		if st, err := os.Stat(f); err != nil || st.Size() == 0 {
			return false
		}
	}
	return true
}

type face struct {
	italic bool
	weight int
	url    string
}

var (
	faceRe   = regexp.MustCompile(`(?s)@font-face\s*\{(.*?)\}`)
	styleRe  = regexp.MustCompile(`font-style:\s*(\w+)`)
	weightRe = regexp.MustCompile(`font-weight:\s*(\d+)`)
	urlRe    = regexp.MustCompile(`url\((\S+?\.ttf)\)`)
)

// fetchFaces asks for the four styles and parses the @font-face rules of the
// answer. Families without italics reject a request that names them, so a
// second request asks for upright faces only.
func fetchFaces(family string, weight, boldWeight int) ([]face, error) {
	queries := []string{
		fmt.Sprintf("%s:ital,wght@0,%d;0,%d;1,%d;1,%d", family, min(weight, boldWeight), max(weight, boldWeight), min(weight, boldWeight), max(weight, boldWeight)),
		fmt.Sprintf("%s:wght@%d;%d", family, min(weight, boldWeight), max(weight, boldWeight)),
		family,
	}
	var lastErr error
	for _, q := range queries {
		css, status, err := get(cssAPI + "?family=" + url.QueryEscape(q))
		if err != nil {
			return nil, fmt.Errorf("font: Google Fonts: %w", err)
		}
		if status != http.StatusOK {
			lastErr = fmt.Errorf("font: %q was not found on Google Fonts (HTTP %d); check the spelling at https://fonts.google.com", family, status)
			continue
		}
		var faces []face
		for _, m := range faceRe.FindAllStringSubmatch(string(css), -1) {
			u := urlRe.FindStringSubmatch(m[1])
			if u == nil {
				continue
			}
			f := face{url: u[1]}
			if s := styleRe.FindStringSubmatch(m[1]); s != nil {
				f.italic = s[1] == "italic"
			}
			if w := weightRe.FindStringSubmatch(m[1]); w != nil {
				f.weight, _ = strconv.Atoi(w[1])
			}
			faces = append(faces, f)
		}
		if len(faces) > 0 {
			return faces, nil
		}
		lastErr = fmt.Errorf("font: Google Fonts returned no TrueType files for %q", family)
	}
	return nil, lastErr
}

// nearest prefers the requested slant, then the closest weight.
func nearest(faces []face, want face) face {
	best, bestScore := faces[0], -1
	for _, f := range faces {
		score := abs(f.weight - want.weight)
		if f.italic != want.italic {
			score += 10000
		}
		if bestScore < 0 || score < bestScore {
			best, bestScore = f, score
		}
	}
	return best
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func get(u string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "cvmaker")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	return body, resp.StatusCode, err
}

func download(u, path string) error {
	if !strings.HasPrefix(u, "https://") {
		return fmt.Errorf("refusing non-https font URL %q", u)
	}
	data, status, err := get(u)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", status, u)
	}
	if len(data) < 4 || string(data[:4]) != "\x00\x01\x00\x00" && string(data[:4]) != "true" {
		return errors.New("downloaded file is not a TrueType font")
	}
	tmp := path + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
