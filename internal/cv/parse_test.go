package cv

import (
	"reflect"
	"testing"
)

const src = `# Ada Lovelace

Mathematician · Writer
<ada@example.com>

## Experience

### Analytical Engine | 1842 -- 1843

*Translator*
Notes on the engine.

- **Notes:** wrote [Note G](https://example.com/g).
  - nested

<!-- pagebreak -->

## Skills

Languages
: English, French
`

func TestParse(t *testing.T) {
	doc := Parse([]byte(src), true)

	if doc.Name != "Ada Lovelace" {
		t.Errorf("name = %q", doc.Name)
	}
	if got := len(doc.Header); got != 1 {
		t.Fatalf("header blocks = %d, want 1", got)
	}
	header := doc.Header[0].(Paragraph).Inlines
	if got := lines(header); !reflect.DeepEqual(got, []string{"Mathematician · Writer", "ada@example.com"}) {
		t.Errorf("header lines = %q", got)
	}
	if last := header[len(header)-1]; last.Style.Link != "mailto:ada@example.com" {
		t.Errorf("email not linked: %+v", last)
	}

	if len(doc.Sections) != 2 {
		t.Fatalf("sections = %d, want 2", len(doc.Sections))
	}
	exp := doc.Sections[0].Blocks
	entry := exp[0].(Entry)
	if plain(entry.Title) != "Analytical Engine" || plain(entry.Meta) != "1842 – 1843" {
		t.Errorf("entry = %q | %q", plain(entry.Title), plain(entry.Meta))
	}
	para := exp[1].(Paragraph).Inlines
	if got := lines(para); !para[0].Style.Italic || !reflect.DeepEqual(got, []string{"Translator", "Notes on the engine."}) {
		t.Errorf("hard wraps: %q", got)
	}
	list := exp[2].(List)
	if list.Items[0].Sub == nil || !list.Items[0].Inlines[0].Style.Bold {
		t.Errorf("list = %+v", list)
	}
	if _, ok := exp[3].(PageBreak); !ok {
		t.Errorf("block 3 = %T, want PageBreak", exp[3])
	}

	def := doc.Sections[1].Blocks[0].(DefList)
	if plain(def.Items[0].Term) != "Languages" || plain(def.Items[0].Desc) != "English, French" {
		t.Errorf("deflist = %+v", def)
	}
}

func TestSoftWraps(t *testing.T) {
	doc := Parse([]byte("## S\n\none\ntwo\n"), false)
	if got := lines(doc.Sections[0].Blocks[0].(Paragraph).Inlines); !reflect.DeepEqual(got, []string{"one two"}) {
		t.Errorf("got %q", got)
	}
}

// lines returns the text of in split at forced breaks.
func lines(in []Inline) []string {
	out := []string{""}
	for _, i := range in {
		if i.Break {
			out = append(out, "")
			continue
		}
		out[len(out)-1] += i.Text
	}
	return out
}

func TestSplitMetaIgnoresLinks(t *testing.T) {
	title, meta := splitMeta([]Inline{{Text: "a|b", Style: Style{Link: "x"}}, {Text: " | 2020"}})
	if plain(title) != "a|b" || plain(meta) != "2020" {
		t.Errorf("got %q | %q", plain(title), plain(meta))
	}
}
