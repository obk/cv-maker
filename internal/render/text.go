package render

import "github.com/obk/cv-maker/internal/cv"

// Text is set with a small line breaker of its own instead of fpdf's
// MultiCell, which cannot mix bold, italic and links inside one paragraph.
//
// Inlines are cut into words at spaces. A word may consist of several spans
// in different styles ("**Go**," is one word with a bold and a plain span),
// and lines only break between words.

type span struct {
	text  string
	style cv.Style
	w     float64
}

type word struct {
	spans []span
	w     float64
	space bool // preceded by a space
}

type line struct {
	words  []word
	w      float64 // natural width including spaces
	spaceW float64
	last   bool // ends a paragraph or a forced break; never justified
}

// breakLines fills lines of at most maxW points greedily.
func (r *renderer) breakLines(in []cv.Inline, size, maxW float64) []line {
	r.setFont(cv.Style{}, size)
	spaceW := r.pdf.GetStringWidth(" ")

	var (
		lines   []line
		cur     = line{spaceW: spaceW}
		w       word
		pending bool // a space was seen since the last placed word
	)
	place := func() {
		if len(w.spans) == 0 {
			return
		}
		w.space = pending && len(cur.words) > 0
		gap := 0.0
		if w.space {
			gap = spaceW
		}
		if len(cur.words) > 0 && cur.w+gap+w.w > maxW+0.01 {
			lines = append(lines, cur)
			cur = line{spaceW: spaceW}
			w.space, gap = false, 0
		}
		cur.words = append(cur.words, w)
		cur.w += gap + w.w
		w, pending = word{}, false
	}
	add := func(s string, st cv.Style) {
		if s == "" {
			return
		}
		r.setFont(st, size)
		sw := r.pdf.GetStringWidth(s)
		w.spans = append(w.spans, span{text: s, style: st, w: sw})
		w.w += sw
	}

	for _, it := range in {
		if it.Break {
			place()
			cur.last = true
			lines = append(lines, cur)
			cur, pending = line{spaceW: spaceW}, false
			continue
		}
		start := 0
		for i, c := range it.Text {
			if c == ' ' || c == '\t' || c == '\n' {
				add(it.Text[start:i], it.Style)
				place()
				pending = true
				start = i + 1
			}
		}
		add(it.Text[start:], it.Style)
	}
	place()
	if len(cur.words) > 0 {
		cur.last = true
		lines = append(lines, cur)
	}
	return lines
}

// drawLine sets one line with its baseline at base. top and lh give the
// clickable area of links.
func (r *renderer) drawLine(ln line, x, top, base, maxW, size, lh float64, color rgb, justify bool) {
	extra := 0.0
	if justify && !ln.last {
		gaps := 0
		for _, w := range ln.words {
			if w.space {
				gaps++
			}
		}
		// A line with very few words would turn into a row of holes; leave it
		// ragged as a typesetter would.
		if gaps > 0 && (maxW-ln.w)/float64(gaps) < ln.spaceW*3 {
			extra = (maxW - ln.w) / float64(gaps)
		}
	}
	cx := x
	for _, w := range ln.words {
		if w.space {
			cx += ln.spaceW + extra
		}
		for _, s := range w.spans {
			r.setFont(s.style, size)
			c := color
			// Links only stand out in body text; in the muted header they
			// keep the surrounding color.
			if s.style.Link != "" && color == r.colors.text {
				c = r.colors.link
			}
			r.pdf.SetTextColor(c.R, c.G, c.B)
			r.pdf.Text(cx, base, s.text)
			if s.style.Link != "" {
				r.pdf.LinkString(cx, top, s.w, lh, s.style.Link)
				if r.cfg.UnderlineLinks {
					r.pdf.SetDrawColor(c.R, c.G, c.B)
					r.pdf.SetLineWidth(size * 0.05)
					r.pdf.Line(cx, base+size*0.12, cx+s.w, base+size*0.12)
				}
			}
			cx += s.w
		}
	}
}

// drawLines sets lines one below the other from the current position,
// moving to a new page when one does not fit.
func (r *renderer) drawLines(lines []line, x, maxW, size float64, color rgb, justify bool) {
	lh := size * r.cfg.Font.LineHeight
	for _, ln := range lines {
		r.ensure(lh)
		r.drawLine(ln, x, r.y, r.baseline(r.y, size, lh), maxW, size, lh, color, justify)
		r.y += lh
	}
}

// baseline centres the font's em box vertically in a line of height lh.
func (r *renderer) baseline(top, size, lh float64) float64 {
	return top + lh/2 + size*0.35
}

func styled(in []cv.Inline, f func(*cv.Style)) []cv.Inline {
	out := make([]cv.Inline, len(in))
	for i, v := range in {
		f(&v.Style)
		out[i] = v
	}
	return out
}
