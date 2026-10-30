package fonts

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNearest(t *testing.T) {
	faces := []face{{weight: 400, url: "r"}, {weight: 700, url: "b"}, {italic: true, weight: 400, url: "i"}}
	for _, tc := range []struct {
		want face
		url  string
	}{
		{face{weight: 400}, "r"},
		{face{weight: 600}, "b"},
		{face{italic: true, weight: 400}, "i"},
		{face{italic: true, weight: 700}, "i"}, // slant beats weight
	} {
		if got := nearest(faces, tc.want).url; got != tc.url {
			t.Errorf("nearest(%+v) = %s, want %s", tc.want, got, tc.url)
		}
	}
}

func TestGoogleDownloadsAndCaches(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	ttf, _ := embedded.ReadFile("liberation/LiberationSans-Regular.ttf")
	requests := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if strings.HasSuffix(r.URL.Path, ".ttf") {
			w.Write(ttf)
			return
		}
		if strings.Contains(r.URL.RawQuery, "ital") {
			http.Error(w, "no italics", http.StatusBadRequest)
			return
		}
		base := "https://" + r.Host
		w.Write([]byte(`@font-face { font-style: normal; font-weight: 400; src: url(` + base + `/r.ttf) format('truetype'); }
@font-face { font-style: normal; font-weight: 700; src: url(` + base + `/b.ttf) format('truetype'); }`))
	}))
	defer srv.Close()
	oldAPI, oldClient := cssAPI, client
	cssAPI, client = srv.URL+"/css2", srv.Client()
	defer func() { cssAPI, client = oldAPI, oldClient }()

	files, err := google("Test Sans", 400, 700)
	if err != nil {
		t.Fatal(err)
	}
	for _, style := range Styles {
		if st, err := os.Stat(files[style]); err != nil || st.Size() == 0 {
			t.Errorf("style %q not cached: %v", style, err)
		}
	}
	before := requests
	if _, err := google("Test Sans", 400, 700); err != nil {
		t.Fatal(err)
	}
	if requests != before {
		t.Errorf("second call hit the network %d times", requests-before)
	}
}
