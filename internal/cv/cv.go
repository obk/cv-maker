// Package cv turns the Markdown source of a CV into a small document model
// that the renderer lays out.
//
// The mapping is deliberately plain:
//
//	# Name                  -> name at the top of the page
//	lines under the name    -> header lines (one per source line)
//	## Section              -> section title with a rule underneath
//	### Title | Date        -> entry title, with the date set flush right
//	paragraphs, lists       -> body text
//	Term / : Description    -> two column rows (skills, languages, ...)
//	<!-- pagebreak -->      -> forced page break
package cv

// Document is a parsed CV.
type Document struct {
	Name     string
	Header   []Block // blocks between the name and the first section
	Sections []Section
}

// Section is a "##" heading and the blocks that follow it.
type Section struct {
	Title  []Inline
	Blocks []Block
}

// Block is one of the block types below.
type Block interface{ block() }

// Paragraph is a run of inline text.
type Paragraph struct{ Inlines []Inline }

// Entry is a "###" heading, split at the last " | " into title and meta
// (usually a date range).
type Entry struct{ Title, Meta []Inline }

// Subheading is a "####" (or deeper) heading.
type Subheading struct{ Inlines []Inline }

// List is a bullet or numbered list.
type List struct {
	Ordered bool
	Start   int
	Items   []ListItem
}

// ListItem is one list item, optionally with a nested list.
type ListItem struct {
	Inlines []Inline
	Sub     *List
}

// DefList holds term/description rows rendered as two columns.
type DefList struct{ Items []Definition }

// Definition is a single row of a DefList.
type Definition struct{ Term, Desc []Inline }

// Rule is a thin horizontal line ("***").
type Rule struct{}

// PageBreak forces the following content onto a new page.
type PageBreak struct{}

func (Paragraph) block()  {}
func (Entry) block()      {}
func (Subheading) block() {}
func (List) block()       {}
func (DefList) block()    {}
func (Rule) block()       {}
func (PageBreak) block()  {}

// Style is the formatting applied to a piece of text.
type Style struct {
	Bold   bool
	Italic bool
	Link   string
}

// Inline is a piece of text in a single style, or a forced line break.
type Inline struct {
	Text  string
	Style Style
	Break bool
}
