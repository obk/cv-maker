// Package render lays out a parsed CV on PDF pages.
package render

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/go-pdf/fpdf"

	"github.com/obk/cv-maker/internal/config"
	"github.com/obk/cv-maker/internal/cv"
	"github.com/obk/cv-maker/internal/fonts"
)

const (
	mm       = 72 / 25.4 // points per millimetre
	family   = "body"
	photoGap = 14.0 // between the photo and the header text
)

type rgb = config.RGB

type renderer struct {
	pdf    *fpdf.Fpdf
	cfg    config.Config
	colors struct{ text, muted, rule, link rgb }

	left, right, top, bottom float64 // content box
	y                        float64 // top of the next line
	upper                    func(string) string
}

// PDF writes doc to w as a PDF document.
func PDF(w io.Writer, doc *cv.Document, cfg config.Config, set fonts.Set) error {
	size := config.PageSizes[strings.ToLower(cfg.Page)]
	pdf := fpdf.NewCustom(&fpdf.InitType{
		OrientationStr: "P",
		UnitStr:        "pt",
		Size:           fpdf.SizeType{Wd: size[0], Ht: size[1]},
	})
	r := &renderer{
		pdf:    pdf,
		cfg:    cfg,
		left:   cfg.Margins.Left * mm,
		right:  size[0] - cfg.Margins.Right*mm,
		top:    cfg.Margins.Top * mm,
		bottom: size[1] - cfg.Margins.Bottom*mm,
		upper:  strings.ToUpper,
	}
	if l := strings.ToLower(cfg.Lang); strings.HasPrefix(l, "tr") || strings.HasPrefix(l, "az") {
		r.upper = func(s string) string { return strings.ToUpperSpecial(unicode.TurkishCase, s) }
	}
	r.colors.text, _ = config.ParseColor(cfg.Colors.Text)
	r.colors.muted, _ = config.ParseColor(cfg.Colors.Muted)
	r.colors.rule, _ = config.ParseColor(cfg.Colors.Rule)
	r.colors.link, _ = config.ParseColor(cfg.Colors.Link)

	pdf.SetMargins(r.left, r.top, size[0]-r.right)
	pdf.SetAutoPageBreak(false, 0)
	for _, style := range fonts.Styles {
		pdf.AddUTF8FontFromBytes(family, style, set[style])
	}
	if pdf.Err() {
		return fmt.Errorf("font: %w", pdf.Error())
	}

	title := cfg.Title
	if title == "" {
		title = doc.Name
	}
	author := cfg.Author
	if author == "" {
		author = doc.Name
	}
	pdf.SetTitle(title, true)
	pdf.SetAuthor(author, true)
	pdf.SetSubject(cfg.Subject, true)
	pdf.SetKeywords(strings.Join(cfg.Keywords, ", "), true)
	pdf.SetCreator("cv-maker", true)
	if cfg.Lang != "" {
		pdf.SetLang(cfg.Lang)
	}

	r.newPage()
	if err := r.header(doc); err != nil {
		return err
	}
	for _, s := range doc.Sections {
		r.section(s)
	}
	if pdf.Err() {
		return pdf.Error()
	}
	return pdf.Output(w)
}

func (r *renderer) newPage() {
	r.pdf.AddPage()
	r.y = r.top
}

// ensure starts a new page unless h more points fit on this one.
func (r *renderer) ensure(h float64) {
	if r.y+h > r.bottom+0.01 && r.y > r.top {
		r.newPage()
	}
}

// space adds vertical space, except at the top of a page.
func (r *renderer) space(h float64) {
	if r.y > r.top {
		r.y += h
	}
}

func (r *renderer) setFont(st cv.Style, size float64) {
	s := ""
	if st.Bold {
		s += "B"
	}
	if st.Italic {
		s += "I"
	}
	r.pdf.SetFont(family, s, size)
}

func (r *renderer) lineHeight() float64 { return r.cfg.Font.Size * r.cfg.Font.LineHeight }

func (r *renderer) header(doc *cv.Document) error {
	x, w := r.left, r.right-r.left
	photoTop := r.y
	ps := r.cfg.Photo.Size * mm
	if r.cfg.Photo.Path != "" {
		px := r.right - ps
		if r.cfg.Photo.Position == "left" {
			px = r.left
			x += ps + photoGap
		}
		w -= ps + photoGap
		if err := r.photo(px, photoTop, ps); err != nil {
			return err
		}
	}

	if doc.Name != "" {
		size := r.cfg.Font.NameSize
		name := doc.Name
		if r.cfg.Uppercase {
			name = r.upper(name)
		}
		lh := size * 1.15
		for _, ln := range r.breakLines([]cv.Inline{{Text: name, Style: cv.Style{Bold: true}}}, size, w) {
			r.drawLine(ln, x, r.y, r.y+size*0.85, w, size, lh, r.colors.text, false)
			r.y += lh
		}
		r.y += r.cfg.Spacing.Header
	}
	for _, b := range doc.Header {
		r.block(b, x, w, r.colors.muted)
	}
	if r.cfg.Photo.Path != "" && r.y < photoTop+ps {
		r.y = photoTop + ps
	}
	return nil
}

// photo draws the image scaled to cover an s×s box and clipped to the
// configured shape.
func (r *renderer) photo(x, y, s float64) error {
	const name = "photo"
	data, kind, err := loadPhoto(r.cfg.Photo.Path)
	if err != nil {
		return fmt.Errorf("photo: %w", err)
	}
	info := r.pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: kind}, bytes.NewReader(data))
	if r.pdf.Err() {
		return fmt.Errorf("photo: %w", r.pdf.Error())
	}
	iw, ih := info.Width(), info.Height()
	scale := max(s/iw, s/ih)
	dw, dh := iw*scale, ih*scale
	switch r.cfg.Photo.Shape {
	case "circle":
		r.pdf.ClipCircle(x+s/2, y+s/2, s/2, false)
	case "rounded":
		r.pdf.ClipRoundedRect(x, y, s, s, s*0.12, false)
	default:
		r.pdf.ClipRect(x, y, s, s, false)
	}
	r.pdf.ImageOptions(name, x+(s-dw)/2, y+(s-dh)/2, dw, dh, false, fpdf.ImageOptions{ImageType: kind}, 0, "")
	r.pdf.ClipEnd()
	return nil
}

// loadPhoto returns the image in a form fpdf handles. JPEGs pass through
// untouched; PNGs are re-encoded as 8-bit RGBA because fpdf garbles 16-bit
// and some palette images without reporting an error.
func loadPhoto(path string) (data []byte, kind string, err error) {
	data, err = os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("%s: use a JPEG or PNG image (%v)", path, err)
	}
	if format == "jpeg" {
		return data, "JPG", nil
	}
	rgba := image.NewNRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, rgba); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "PNG", nil
}

func (r *renderer) section(s cv.Section) {
	size := r.cfg.Font.SectionSize
	lh := size * 1.2
	// Keep the title together with at least two lines of its content.
	r.ensure(r.cfg.Spacing.Section + lh + 8 + 2*r.lineHeight())
	r.space(r.cfg.Spacing.Section)

	title := styled(s.Title, func(st *cv.Style) { st.Bold = true })
	if r.cfg.Uppercase {
		for i := range title {
			title[i].Text = r.upper(title[i].Text)
		}
	}
	w := r.right - r.left
	for _, ln := range r.breakLines(title, size, w) {
		r.drawLine(ln, r.left, r.y, r.baseline(r.y, size, lh), w, size, lh, r.colors.text, false)
		r.y += lh
	}
	r.y += 2
	r.pdf.SetDrawColor(r.colors.rule.R, r.colors.rule.G, r.colors.rule.B)
	r.pdf.SetLineWidth(r.cfg.RuleWidth)
	r.pdf.Line(r.left, r.y, r.right, r.y)
	r.y += r.cfg.RuleWidth + 4

	var prev cv.Block
	for _, b := range s.Blocks {
		switch b.(type) {
		case cv.PageBreak:
		case cv.Entry:
			if prev != nil {
				r.space(r.cfg.Spacing.Entry)
			}
		default:
			switch prev.(type) {
			case nil, cv.Entry, cv.Subheading:
			default:
				r.space(r.cfg.Spacing.Block)
			}
		}
		r.block(b, r.left, w, r.colors.text)
		prev = b
	}
}

func (r *renderer) block(b cv.Block, x, w float64, color rgb) {
	size := r.cfg.Font.Size
	switch b := b.(type) {
	case cv.Paragraph:
		r.drawLines(r.breakLines(b.Inlines, size, w), x, w, size, color, r.cfg.Justify && color == r.colors.text)
	case cv.Subheading:
		r.ensure(2 * r.lineHeight())
		in := styled(b.Inlines, func(st *cv.Style) { st.Bold = true })
		r.drawLines(r.breakLines(in, size, w), x, w, size, color, false)
	case cv.Entry:
		r.entry(b, x, w)
	case cv.List:
		r.list(b, x, w, 0, color)
	case cv.DefList:
		r.defList(b, x, w, color)
	case cv.Rule:
		r.ensure(10)
		r.y += 4
		r.pdf.SetDrawColor(r.colors.muted.R, r.colors.muted.G, r.colors.muted.B)
		r.pdf.SetLineWidth(0.3)
		r.pdf.Line(x, r.y, x+w, r.y)
		r.y += 5
	case cv.PageBreak:
		r.newPage()
	}
}

// entry sets "### Title | Meta": the title in bold on the left and the meta
// text, usually a date range, smaller and flush right on the same baseline.
func (r *renderer) entry(e cv.Entry, x, w float64) {
	size, metaSize := r.cfg.Font.Size, r.cfg.Font.MetaSize
	lh := r.lineHeight()

	var meta line
	mw := 0.0
	if len(e.Meta) > 0 {
		meta = r.breakLines(e.Meta, metaSize, 1e9)[0]
		mw = meta.w
	}
	titleW := w
	if mw > 0 {
		titleW -= mw + 12
	}
	title := r.breakLines(styled(e.Title, func(st *cv.Style) { st.Bold = true }), size, titleW)

	// Keep the title with the first two lines that follow it.
	r.ensure(float64(len(title)+2) * lh)
	for i, ln := range title {
		base := r.baseline(r.y, size, lh)
		r.drawLine(ln, x, r.y, base, titleW, size, lh, r.colors.text, false)
		if i == 0 && mw > 0 {
			r.drawLine(meta, x+w-mw, r.y, base, mw, metaSize, lh, r.colors.muted, false)
		}
		r.y += lh
	}
}

var bullets = []string{"•", "–", "·"}

func (r *renderer) list(l cv.List, x, w float64, level int, color rgb) {
	size := r.cfg.Font.Size
	lh := r.lineHeight()

	markers := make([]string, len(l.Items))
	indent := 0.0
	r.setFont(cv.Style{}, size)
	for i := range l.Items {
		if l.Ordered {
			markers[i] = fmt.Sprintf("%d.", l.Start+i)
		} else {
			markers[i] = bullets[min(level, len(bullets)-1)]
		}
		indent = max(indent, r.pdf.GetStringWidth(markers[i]))
	}
	indent += 5

	for i, it := range l.Items {
		if i > 0 {
			r.space(r.cfg.Spacing.Item)
		}
		for j, ln := range r.breakLines(it.Inlines, size, w-indent) {
			r.ensure(lh)
			base := r.baseline(r.y, size, lh)
			if j == 0 {
				r.setFont(cv.Style{}, size)
				r.pdf.SetTextColor(color.R, color.G, color.B)
				r.pdf.Text(x, base, markers[i])
			}
			r.drawLine(ln, x+indent, r.y, base, w-indent, size, lh, color, r.cfg.Justify && color == r.colors.text)
			r.y += lh
		}
		if it.Sub != nil {
			r.space(r.cfg.Spacing.Item)
			r.list(*it.Sub, x+indent, w-indent, level+1, color)
		}
	}
}

// defList sets term/description rows as a two column table whose first
// column is as wide as the longest term.
func (r *renderer) defList(d cv.DefList, x, w float64, color rgb) {
	size := r.cfg.Font.Size
	lh := r.lineHeight()
	const gap = 12.0

	terms := make([][]cv.Inline, len(d.Items))
	col := 0.0
	for i, it := range d.Items {
		terms[i] = styled(it.Term, func(st *cv.Style) { st.Bold = true })
		for _, ln := range r.breakLines(terms[i], size, 1e9) {
			col = max(col, ln.w)
		}
	}
	col = min(col+gap, w*0.35)

	for i, it := range d.Items {
		if i > 0 {
			r.space(r.cfg.Spacing.Item)
		}
		tl := r.breakLines(terms[i], size, col-gap)
		dl := r.breakLines(it.Desc, size, w-col)
		for j := 0; j < max(len(tl), len(dl)); j++ {
			r.ensure(lh)
			base := r.baseline(r.y, size, lh)
			if j < len(tl) {
				r.drawLine(tl[j], x, r.y, base, col-gap, size, lh, color, false)
			}
			if j < len(dl) {
				r.drawLine(dl[j], x+col, r.y, base, w-col, size, lh, color, false)
			}
			r.y += lh
		}
	}
}
