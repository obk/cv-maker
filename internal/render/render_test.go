package render

import (
	"bytes"
	"testing"

	"github.com/go-pdf/fpdf"

	"github.com/obk/cv-maker/internal/config"
	"github.com/obk/cv-maker/internal/cv"
	"github.com/obk/cv-maker/internal/fonts"
)

func TestPDF(t *testing.T) {
	cfg := config.Default()
	set, err := fonts.Load(cfg.Font, ".")
	if err != nil {
		t.Fatal(err)
	}
	src := "# Şule Işık\n\nİstanbul\n\n## Deneyim\n\n### Firma | 2024\n\n- madde\n\n<!-- pagebreak -->\n\n## İkinci\n\nmetin\n"
	var buf bytes.Buffer
	if err := PDF(&buf, cv.Parse([]byte(src), true), cfg, set); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatal("output is not a PDF")
	}
	if n := bytes.Count(out, []byte("/Type /Page\n")); n != 2 {
		t.Errorf("pages = %d, want 2", n)
	}
}

func TestBreakLines(t *testing.T) {
	cfg := config.Default()
	set, _ := fonts.Load(cfg.Font, ".")
	pdf := fpdf.New("P", "pt", "A4", "")
	for _, style := range fonts.Styles {
		pdf.AddUTF8FontFromBytes(family, style, set[style])
	}
	r := &renderer{cfg: cfg, pdf: pdf}

	in := []cv.Inline{{Text: "one two three four five six seven eight nine ten"}}
	lines := r.breakLines(in, 10, 60)
	if len(lines) < 3 {
		t.Fatalf("expected wrapping, got %d lines", len(lines))
	}
	for i, ln := range lines {
		if ln.w > 60.01 && len(ln.words) > 1 {
			t.Errorf("line %d is %.1fpt wide", i, ln.w)
		}
		if ln.last != (i == len(lines)-1) {
			t.Errorf("line %d: last = %v", i, ln.last)
		}
	}

	// "**Go**," is one word: no break may happen between the bold and the comma.
	lines = r.breakLines([]cv.Inline{{Text: "Go", Style: cv.Style{Bold: true}}, {Text: ", Rust"}}, 10, 1e9)
	if len(lines[0].words) != 2 || len(lines[0].words[0].spans) != 2 {
		t.Errorf("words = %+v", lines[0].words)
	}
}
