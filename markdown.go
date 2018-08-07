// Blackfriday Markdown Processor
// Available at http://github.com/russross/blackfriday
//
// Copyright © 2011 Russ Ross <russ@russross.com>.
// Distributed under the Simplified BSD License.
// See README.md for details.

package blackfriday

import (
	"bytes"
	"io"
)

//
// Markdown parsing and processing
//

// Version string of the package. Appears in the rendered document when
// CompletePage flag is on.
const Version = "2.0"

// Extensions is a bitwise or'ed collection of enabled Blackfriday's
// extensions.
type Extensions int

// These are the supported markdown parsing extensions.
// OR these values together to select multiple extensions.
const (
	NoExtensions           Extensions = 0
	NoIntraEmphasis        Extensions = 1 << iota // Ignore emphasis markers inside words
	Tables                                        // Render tables
	Autolink                                      // Detect embedded URLs that are not explicitly marked
	Strikethrough                                 // Strikethrough text using ~~test~~
	HardLineBreak                                 // Translate newlines into line breaks
	NoEmptyLineBeforeBlock                        // No need to insert an empty line to start a (code, quote, ordered list, unordered list) block
	BackslashLineBreak                            // Translate trailing backslashes into line breaks
	DefinitionLists                               // Render definition lists

	CommonHTMLFlags HTMLFlags = UseXHTML | Smartypants |
		SmartypantsFractions | SmartypantsDashes | SmartypantsLatexDashes

	MfnStandardExtensions Extensions = NoIntraEmphasis | Tables |
		Strikethrough | Autolink | NoEmptyLineBeforeBlock |
		BackslashLineBreak
)

// ListType contains bitwise or'ed flags for list and list item objects.
type ListType int

// These are the possible flag values for the ListItem renderer.
// Multiple flag values may be ORed together.
// These are mostly of interest if you are writing a new output format.
const (
	ListTypeOrdered ListType = 1 << iota
	ListTypeDefinition
	ListTypeTerm

	ListItemContainsBlock
	ListItemBeginningOfList // TODO: figure out if this is of any use now
	ListItemEndOfList
)

// CellAlignFlags holds a type of alignment in a table cell.
type CellAlignFlags int

// These are the possible flag values for the table cell renderer.
// Only a single one of these values will be used; they are not ORed together.
// These are mostly of interest if you are writing a new output format.
const (
	TableAlignmentLeft CellAlignFlags = 1 << iota
	TableAlignmentRight
	TableAlignmentCenter = (TableAlignmentLeft | TableAlignmentRight)
)

// Renderer is the rendering interface. This is mostly of interest if you are
// implementing a new rendering format.
//
// Only an HTML implementation is provided in this repository, see the README
// for external implementations.
type Renderer interface {
	// RenderNode is the main rendering method. It will be called once for
	// every leaf node and twice for every non-leaf node (first with
	// entering=true, then with entering=false). The method should write its
	// rendition of the node to the supplied writer w.
	RenderNode(w io.Writer, node *Node, entering bool) WalkStatus

	// RenderHeader is a method that allows the renderer to produce some
	// content preceding the main body of the output document. The header is
	// understood in the broad sense here. For example, the default HTML
	// renderer will write not only the HTML document preamble, but also the
	// table of contents if it was requested.
	//
	// The method will be passed an entire document tree, in case a particular
	// implementation needs to inspect it to produce output.
	//
	// The output should be written to the supplied writer w. If your
	// implementation has no header to write, supply an empty implementation.
	RenderHeader(w io.Writer, ast *Node)

	// RenderFooter is a symmetric counterpart of RenderHeader.
	RenderFooter(w io.Writer, ast *Node)
}

// Callback functions for inline parsing. One such function is defined
// for each character that triggers a response when parsing inline data.
type inlineParser func(p *Markdown, data []byte, offset int) (int, *Node)

// Markdown is a type that holds extensions and the runtime state used by
// Parse, and the renderer. You can not use it directly, construct it with New.
type Markdown struct {
	renderer          Renderer
	inlineCallback    [256]inlineParser
	extensions        Extensions
	nesting           int
	maxNesting        int
	insideLink        bool


	doc                  *Node
	tip                  *Node // = doc
	oldTip               *Node
	lastMatchedContainer *Node // = doc
	allClosed            bool
}

func (p *Markdown) finalize(block *Node) {
	above := block.Parent
	block.open = false
	p.tip = above
}

func (p *Markdown) addChild(node NodeType, offset uint32) *Node {
	return p.addExistingChild(NewNode(node), offset)
}

func (p *Markdown) addExistingChild(node *Node, offset uint32) *Node {
	for !p.tip.canContain(node.Type) {
		p.finalize(p.tip)
	}
	p.tip.AppendChild(node)
	p.tip = node
	return node
}

func (p *Markdown) closeUnmatchedBlocks() {
	if !p.allClosed {
		for p.oldTip != p.lastMatchedContainer {
			parent := p.oldTip.Parent
			p.finalize(p.oldTip)
			p.oldTip = parent
		}
		p.allClosed = true
	}
}

//
//
// Public interface
//
//


// New constructs a Markdown processor. You can use the same With* functions as
// for Run() to customize parser's behavior and the renderer.
func New(opts ...Option) *Markdown {
	var p Markdown
	for _, opt := range opts {
		opt(&p)
	}
	p.maxNesting = 16
	p.insideLink = false
	docNode := NewNode(Document)
	p.doc = docNode
	p.tip = docNode
	p.oldTip = docNode
	p.lastMatchedContainer = docNode
	p.allClosed = true
	// register inline parsers
	p.inlineCallback[' '] = maybeLineBreak
	p.inlineCallback['*'] = emphasis
	p.inlineCallback['_'] = emphasis
	if p.extensions&Strikethrough != 0 {
		p.inlineCallback['~'] = emphasis
	}
	p.inlineCallback['\n'] = lineBreak
	p.inlineCallback['['] = link
	p.inlineCallback['\\'] = escape
	p.inlineCallback['&'] = entity
	//p.inlineCallback['!'] = maybeImage
	if p.extensions&Autolink != 0 {
		p.inlineCallback['h'] = maybeAutoLink
		p.inlineCallback['m'] = maybeAutoLink
		p.inlineCallback['f'] = maybeAutoLink
		p.inlineCallback['H'] = maybeAutoLink
		p.inlineCallback['M'] = maybeAutoLink
		p.inlineCallback['F'] = maybeAutoLink
	}
	return &p
}

// Option customizes the Markdown processor's default behavior.
type Option func(*Markdown)

// WithRenderer allows you to override the default renderer.
func WithRenderer(r Renderer) Option {
	return func(p *Markdown) {
		p.renderer = r
	}
}

// WithExtensions allows you to pick some of the many extensions provided by
// Blackfriday. You can bitwise OR them.
func WithExtensions(e Extensions) Option {
	return func(p *Markdown) {
		p.extensions = e
	}
}

// WithNoExtensions turns off all extensions and custom behavior.
func WithNoExtensions() Option {
	return func(p *Markdown) {
		p.extensions = NoExtensions
		p.renderer = NewHTMLRenderer(HTMLRendererParameters{
			Flags: HTMLFlagsNone,
		})
	}
}

// Run is the main entry point to Blackfriday. It parses and renders a
// block of markdown-encoded text.
//
// The simplest invocation of Run takes one argument, input:
//     output := Run(input)
// This will parse the input with CommonExtensions enabled and render it with
// the default HTMLRenderer (with CommonHTMLFlags).
//
// Variadic arguments opts can customize the default behavior. Since Markdown
// type does not contain exported fields, you can not use it directly. Instead,
// use the With* functions. For example, this will call the most basic
// functionality, with no extensions:
//     output := Run(input, WithNoExtensions())
//
// You can use any number of With* arguments, even contradicting ones. They
// will be applied in order of appearance and the latter will override the
// former:
//     output := Run(input, WithNoExtensions(), WithExtensions(exts),
//         WithRenderer(yourRenderer))
func Run(input []byte, opts ...Option) []byte {
	r := NewHTMLRenderer(HTMLRendererParameters{
		Flags: CommonHTMLFlags,
	})
	optList := []Option{WithRenderer(r), WithExtensions(MfnStandardExtensions)}
	optList = append(optList, opts...)
	parser := New(optList...)
	ast := parser.Parse(input)
	var buf bytes.Buffer
	parser.renderer.RenderHeader(&buf, ast)
	ast.Walk(func(node *Node, entering bool) WalkStatus {
		return parser.renderer.RenderNode(&buf, node, entering)
	})
	parser.renderer.RenderFooter(&buf, ast)
	return buf.Bytes()
}

// Parse is an entry point to the parsing part of Blackfriday. It takes an
// input markdown document and produces a syntax tree for its contents. This
// tree can then be rendered with a default or custom renderer, or
// analyzed/transformed by the caller to whatever non-standard needs they have.
// The return value is the root node of the syntax tree.
func (p *Markdown) Parse(input []byte) *Node {
	p.block(input)
	// Walk the tree and finish up some of unfinished blocks
	for p.tip != nil {
		p.finalize(p.tip)
	}
	// Walk the tree again and process inline markdown in each block
	p.doc.Walk(func(node *Node, entering bool) WalkStatus {
		if node.Type == Paragraph || node.Type == TableCell {
			p.inline(node, node.content)
			node.content = nil
		}
		return GoToNext
	})
	return p.doc
}

//
//
// Miscellaneous helper functions
//
//

// Test if a character is a punctuation symbol.
// Taken from a private function in regexp in the stdlib.
func ispunct(c byte) bool {
	for _, r := range []byte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~") {
		if c == r {
			return true
		}
	}
	return false
}

// Test if a character is a whitespace character.
func isspace(c byte) bool {
	return ishorizontalspace(c) || isverticalspace(c)
}

// Test if a character is a horizontal whitespace character.
func ishorizontalspace(c byte) bool {
	return c == ' ' || c == '\t'
}

// Test if a character is a vertical character.
func isverticalspace(c byte) bool {
	return c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

// Test if a character is letter.
func isletter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// Test if a character is a letter or a digit.
// TODO: check when this is looking for ASCII alnum and when it should use unicode
func isalnum(c byte) bool {
	return (c >= '0' && c <= '9') || isletter(c)
}

// Create a url-safe slug for fragments
func slugify(in []byte) []byte {
	if len(in) == 0 {
		return in
	}
	out := make([]byte, 0, len(in))
	sym := false

	for _, ch := range in {
		if isalnum(ch) {
			sym = false
			out = append(out, ch)
		} else if sym {
			continue
		} else {
			out = append(out, '-')
			sym = true
		}
	}
	var a, b int
	var ch byte
	for a, ch = range out {
		if ch != '-' {
			break
		}
	}
	for b = len(out) - 1; b > 0; b-- {
		if out[b] != '-' {
			break
		}
	}
	return out[a : b+1]
}
