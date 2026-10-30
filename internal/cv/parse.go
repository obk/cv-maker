package cv

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var markdown = goldmark.New(goldmark.WithExtensions(
	extension.DefinitionList,
	extension.Linkify,
	extension.Strikethrough,
	extension.Typographer,
))

// Parse reads a CV written in Markdown. The front matter must already be
// stripped (see config.Split). With hardWraps set, every line of the source
// is a line in the output; otherwise lines of a paragraph are joined as in
// standard Markdown. The header always keeps its lines.
func Parse(src []byte, hardWraps bool) *Document {
	p := &parser{src: src, doc: &Document{}, section: -1, hardWraps: hardWraps}
	root := markdown.Parser().Parse(text.NewReader(src))
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		p.block(n)
	}
	return p.doc
}

type parser struct {
	src       []byte
	doc       *Document
	section   int // index into doc.Sections, -1 while still in the header
	hardWraps bool
}

func (p *parser) add(b Block) {
	if p.section < 0 {
		p.doc.Header = append(p.doc.Header, b)
		return
	}
	s := &p.doc.Sections[p.section]
	s.Blocks = append(s.Blocks, b)
}

func (p *parser) block(n ast.Node) {
	switch n := n.(type) {
	case *ast.Heading:
		p.heading(n)
	case *ast.Paragraph, *ast.TextBlock:
		if in := p.inlines(n, p.hardWraps || p.section < 0); len(in) > 0 {
			p.add(Paragraph{Inlines: in})
		}
	case *ast.List:
		p.add(p.list(n))
	case *extast.DefinitionList:
		p.add(p.defList(n))
	case *ast.ThematicBreak:
		p.add(Rule{})
	case *ast.HTMLBlock:
		if bytes.Contains(bytes.ToLower(p.lines(n)), []byte("pagebreak")) {
			p.add(PageBreak{})
		}
	case *ast.Blockquote:
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			p.block(c)
		}
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		var in []Inline
		for i, line := range strings.Split(strings.TrimRight(string(p.lines(n)), "\n"), "\n") {
			if i > 0 {
				in = append(in, Inline{Break: true})
			}
			in = append(in, Inline{Text: line})
		}
		p.add(Paragraph{Inlines: in})
	}
}

func (p *parser) heading(h *ast.Heading) {
	in := p.inlines(h, false)
	switch {
	case h.Level == 1 && p.doc.Name == "":
		p.doc.Name = plain(in)
	case h.Level <= 2:
		p.doc.Sections = append(p.doc.Sections, Section{Title: in})
		p.section = len(p.doc.Sections) - 1
	case h.Level == 3:
		title, meta := splitMeta(in)
		p.add(Entry{Title: title, Meta: meta})
	default:
		p.add(Subheading{Inlines: in})
	}
}

func (p *parser) list(l *ast.List) List {
	out := List{Ordered: l.IsOrdered(), Start: l.Start}
	for it := l.FirstChild(); it != nil; it = it.NextSibling() {
		var item ListItem
		for c := it.FirstChild(); c != nil; c = c.NextSibling() {
			switch c := c.(type) {
			case *ast.Paragraph, *ast.TextBlock:
				if len(item.Inlines) > 0 {
					item.Inlines = append(item.Inlines, Inline{Break: true})
				}
				item.Inlines = append(item.Inlines, p.inlines(c, p.hardWraps)...)
			case *ast.List:
				sub := p.list(c)
				item.Sub = &sub
			}
		}
		out.Items = append(out.Items, item)
	}
	return out
}

func (p *parser) defList(l *extast.DefinitionList) DefList {
	var out DefList
	for c := l.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *extast.DefinitionTerm:
			out.Items = append(out.Items, Definition{Term: p.inlines(c, false)})
		case *extast.DefinitionDescription:
			if len(out.Items) == 0 {
				out.Items = append(out.Items, Definition{})
			}
			last := &out.Items[len(out.Items)-1]
			for b := c.FirstChild(); b != nil; b = b.NextSibling() {
				if len(last.Desc) > 0 {
					last.Desc = append(last.Desc, Inline{Break: true})
				}
				last.Desc = append(last.Desc, p.inlines(b, p.hardWraps)...)
			}
		}
	}
	return out
}

// inlines flattens the inline children of n. With softBreaks set, a plain
// newline in the source becomes a line break instead of a space.
func (p *parser) inlines(n ast.Node, softBreaks bool) []Inline {
	var all []Inline
	p.walk(n, Style{}, softBreaks, &all)
	out := all[:0]
	for _, in := range all {
		if in.Text != "" || in.Break {
			out = append(out, in)
		}
	}
	return trim(out)
}

func (p *parser) walk(n ast.Node, st Style, softBreaks bool, out *[]Inline) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Text:
			v := c.Segment.Value(p.src)
			if !c.IsRaw() {
				v = unescape(v)
			}
			*out = append(*out, Inline{Text: string(v), Style: st})
			switch {
			case c.HardLineBreak() || (c.SoftLineBreak() && softBreaks):
				*out = append(*out, Inline{Break: true})
			case c.SoftLineBreak():
				*out = append(*out, Inline{Text: " ", Style: st})
			}
		case *ast.String:
			// Typographer output ("&ndash;", "&rsquo;") is flagged as code,
			// meaning ready-made HTML: only its entities need resolving.
			v := util.ResolveEntityNames(c.Value)
			if !c.IsCode() {
				v = unescape(c.Value)
			}
			*out = append(*out, Inline{Text: string(v), Style: st})
		case *ast.Emphasis:
			s := st
			if c.Level >= 2 {
				s.Bold = true
			} else {
				s.Italic = true
			}
			p.walk(c, s, softBreaks, out)
		case *ast.Link:
			s := st
			s.Link = string(c.Destination)
			p.walk(c, s, softBreaks, out)
		case *ast.AutoLink:
			s := st
			s.Link = string(c.URL(p.src))
			if c.AutoLinkType == ast.AutoLinkEmail && !strings.HasPrefix(s.Link, "mailto:") {
				s.Link = "mailto:" + s.Link
			}
			*out = append(*out, Inline{Text: string(c.Label(p.src)), Style: s})
		case *ast.RawHTML:
			var raw []byte
			for i := 0; i < c.Segments.Len(); i++ {
				seg := c.Segments.At(i)
				raw = append(raw, seg.Value(p.src)...)
			}
			if tag := strings.ToLower(strings.ReplaceAll(string(raw), " ", "")); tag == "<br>" || tag == "<br/>" {
				*out = append(*out, Inline{Break: true})
			}
		case *ast.Image:
			// Images have no place in a text-only CV; keep the alt text out too.
		default:
			p.walk(c, st, softBreaks, out)
		}
	}
}

func (p *parser) lines(n ast.Node) []byte {
	var b []byte
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b = append(b, seg.Value(p.src)...)
	}
	return b
}

func unescape(v []byte) []byte {
	return util.ResolveEntityNames(util.ResolveNumericReferences(util.UnescapePunctuations(v)))
}

// splitMeta splits "Company | Jan 2024 – Present" at the last "|" that is
// not part of a link.
func splitMeta(in []Inline) (title, meta []Inline) {
	for i := len(in) - 1; i >= 0; i-- {
		if in[i].Style.Link != "" {
			continue
		}
		j := strings.LastIndex(in[i].Text, "|")
		if j < 0 {
			continue
		}
		title = append(title, in[:i]...)
		title = append(title, Inline{Text: in[i].Text[:j], Style: in[i].Style})
		meta = append(meta, Inline{Text: in[i].Text[j+1:], Style: in[i].Style})
		meta = append(meta, in[i+1:]...)
		return trim(title), trim(meta)
	}
	return in, nil
}

// trim drops leading and trailing whitespace and breaks.
func trim(in []Inline) []Inline {
	for len(in) > 0 {
		first := &in[0]
		first.Text = strings.TrimLeft(first.Text, " \t\n")
		if first.Text != "" {
			break
		}
		in = in[1:]
	}
	for len(in) > 0 {
		last := &in[len(in)-1]
		last.Text = strings.TrimRight(last.Text, " \t\n")
		if last.Text != "" {
			break
		}
		in = in[:len(in)-1]
	}
	return in
}

func plain(in []Inline) string {
	var b strings.Builder
	for _, i := range in {
		if i.Break {
			b.WriteByte(' ')
		}
		b.WriteString(i.Text)
	}
	return b.String()
}
