//
// Blackfriday Markdown Processor
// Available at http://github.com/russross/blackfriday
//
// Copyright © 2011 Russ Ross <russ@russross.com>.
// Distributed under the Simplified BSD License.
// See README.md for details.
//

//
// Functions to parse block-level elements.
//

package blackfriday

import "bytes"


// Parse block-level data.
// Note: this function and many that it calls assume that
// the input buffer ends with a newline.
func (p *Markdown) block(data []byte) {
	// this is called recursively: enforce a maximum depth
	if p.nesting >= p.maxNesting {
		return
	}
	p.nesting++



	// parse out one block-level construct at a time
	for len(data) > 0 {

		// blank lines.  note: returns the # of bytes to skip
		if i := p.isEmpty(data); i > 0 {
			data = data[i:]
			continue
		}

		// horizontal rule:
		//
		// ------
		// or
		// ******
		// or
		// ______
		if p.isHRule(data) {
			p.addBlock(HorizontalRule, nil)
			var i int
			for i = 0; i < len(data) && data[i] != '\n'; i++ {
			}
			data = data[i:]
			continue
		}

		// block quote:
		//
		// > A big quote I found somewhere
		// > on the web
		if p.quotePrefix(data) > 0 {
			data = data[p.quote(data):]
			continue
		}

		// table:
		//
		// Name  | Age | Phone
		// ------|-----|---------
		// Bob   | 31  | 555-1234
		// Alice | 27  | 555-4321
		if p.extensions&Tables != 0 {
			if i := p.table(data); i > 0 {
				data = data[i:]
				continue
			}
		}

		// an itemized/unordered list:
		//
		// * Item 1
		// * Item 2
		//
		// also works with + or -
		if p.uliPrefix(data) > 0 {
			data = data[p.list(data, 0):]
			continue
		}

		// a numbered/ordered list:
		//
		// 1. Item 1
		// 2. Item 2
		if p.oliPrefix(data) > 0 {
			data = data[p.list(data, ListTypeOrdered):]
			continue
		}

		// definition lists:
		//
		// Term 1
		// :   Definition a
		// :   Definition b
		//
		// Term 2
		// :   Definition c
		if p.extensions&DefinitionLists != 0 {
			if p.dliPrefix(data) > 0 {
				data = data[p.list(data, ListTypeDefinition):]
				continue
			}
		}

		// anything else must look like a normal paragraph
		// note: this finds underlined headings, too
		data = data[p.paragraph(data):]
	}

	p.nesting--
}

func (p *Markdown) addBlock(typ NodeType, content []byte) *Node {
	p.closeUnmatchedBlocks()
	container := p.addChild(typ, 0)
	container.content = content
	return container
}

func (*Markdown) isEmpty(data []byte) int {
	// it is okay to call isEmpty on an empty buffer
	if len(data) == 0 {
		return 0
	}

	var i int
	for i = 0; i < len(data) && data[i] != '\n'; i++ {
		if data[i] != ' ' && data[i] != '\t' {
			return 0
		}
	}
	if i < len(data) && data[i] == '\n' {
		i++
	}
	return i
}

func (*Markdown) isHRule(data []byte) bool {
	i := 0

	// skip up to three spaces
	for i < 3 && data[i] == ' ' {
		i++
	}

	// look at the hrule char
	if data[i] != '*' && data[i] != '-' && data[i] != '_' {
		return false
	}
	c := data[i]

	// the whole line must be the char or whitespace
	n := 0
	for i < len(data) && data[i] != '\n' {
		switch {
		case data[i] == c:
			n++
		case data[i] != ' ':
			return false
		}
		i++
	}

	return n >= 3
}

func (p *Markdown) table(data []byte) int {
	table := p.addBlock(Table, nil)
	i, columns := p.tableHeader(data)
	if i == 0 {
		p.tip = table.Parent
		table.Unlink()
		return 0
	}

	p.addBlock(TableBody, nil)

	for i < len(data) {
		pipes, rowStart := 0, i
		for ; i < len(data) && data[i] != '\n'; i++ {
			if data[i] == '|' {
				pipes++
			}
		}

		if pipes == 0 {
			i = rowStart
			break
		}

		// include the newline in data sent to tableRow
		if i < len(data) && data[i] == '\n' {
			i++
		}
		p.tableRow(data[rowStart:i], columns, false)
	}

	return i
}

// check if the specified position is preceded by an odd number of backslashes
func isBackslashEscaped(data []byte, i int) bool {
	backslashes := 0
	for i-backslashes-1 >= 0 && data[i-backslashes-1] == '\\' {
		backslashes++
	}
	return backslashes&1 == 1
}

func (p *Markdown) tableHeader(data []byte) (size int, columns []CellAlignFlags) {
	i := 0
	colCount := 1
	for i = 0; i < len(data) && data[i] != '\n'; i++ {
		if data[i] == '|' && !isBackslashEscaped(data, i) {
			colCount++
		}
	}

	// doesn't look like a table header
	if colCount == 1 {
		return
	}

	// include the newline in the data sent to tableRow
	j := i
	if j < len(data) && data[j] == '\n' {
		j++
	}
	header := data[:j]

	// column count ignores pipes at beginning or end of line
	if data[0] == '|' {
		colCount--
	}
	if i > 2 && data[i-1] == '|' && !isBackslashEscaped(data, i-1) {
		colCount--
	}

	columns = make([]CellAlignFlags, colCount)

	// move on to the header underline
	i++
	if i >= len(data) {
		return
	}

	if data[i] == '|' && !isBackslashEscaped(data, i) {
		i++
	}
	i = skipChar(data, i, ' ')

	// each column header is of form: / *:?-+:? *|/ with # dashes + # colons >= 3
	// and trailing | optional on last column
	col := 0
	for i < len(data) && data[i] != '\n' {
		dashes := 0

		if data[i] == ':' {
			i++
			columns[col] |= TableAlignmentLeft
			dashes++
		}
		for i < len(data) && data[i] == '-' {
			i++
			dashes++
		}
		if i < len(data) && data[i] == ':' {
			i++
			columns[col] |= TableAlignmentRight
			dashes++
		}
		for i < len(data) && data[i] == ' ' {
			i++
		}
		if i == len(data) {
			return
		}
		// end of column test is messy
		switch {
		case dashes < 3:
			// not a valid column
			return

		case data[i] == '|' && !isBackslashEscaped(data, i):
			// marker found, now skip past trailing whitespace
			col++
			i++
			for i < len(data) && data[i] == ' ' {
				i++
			}

			// trailing junk found after last column
			if col >= colCount && i < len(data) && data[i] != '\n' {
				return
			}

		case (data[i] != '|' || isBackslashEscaped(data, i)) && col+1 < colCount:
			// something else found where marker was required
			return

		case data[i] == '\n':
			// marker is optional for the last column
			col++

		default:
			// trailing junk found after last column
			return
		}
	}
	if col != colCount {
		return
	}

	p.addBlock(TableHead, nil)
	p.tableRow(header, columns, true)
	size = i
	if size < len(data) && data[size] == '\n' {
		size++
	}
	return
}

func (p *Markdown) tableRow(data []byte, columns []CellAlignFlags, header bool) {
	p.addBlock(TableRow, nil)
	i, col := 0, 0

	if data[i] == '|' && !isBackslashEscaped(data, i) {
		i++
	}

	for col = 0; col < len(columns) && i < len(data); col++ {
		for i < len(data) && data[i] == ' ' {
			i++
		}

		cellStart := i

		for i < len(data) && (data[i] != '|' || isBackslashEscaped(data, i)) && data[i] != '\n' {
			i++
		}

		cellEnd := i

		// skip the end-of-cell marker, possibly taking us past end of buffer
		i++

		for cellEnd > cellStart && cellEnd-1 < len(data) && data[cellEnd-1] == ' ' {
			cellEnd--
		}

		cell := p.addBlock(TableCell, data[cellStart:cellEnd])
		cell.IsHeader = header
		cell.Align = columns[col]
	}

	// pad it out with empty columns to get the right number
	for ; col < len(columns); col++ {
		cell := p.addBlock(TableCell, nil)
		cell.IsHeader = header
		cell.Align = columns[col]
	}

	// silently ignore rows with too many cells
}

// returns blockquote prefix length
func (p *Markdown) quotePrefix(data []byte) int {
	i := 0
	for i < 3 && i < len(data) && data[i] == ' ' {
		i++
	}
	if i < len(data) && data[i] == '>' {
		if i+1 < len(data) && data[i+1] == ' ' {
			return i + 2
		}
		return i + 1
	}
	return 0
}

// blockquote ends with at least one blank line
// followed by something without a blockquote prefix
func (p *Markdown) terminateBlockquote(data []byte, beg, end int) bool {
	if p.isEmpty(data[beg:]) <= 0 {
		return false
	}
	if end >= len(data) {
		return true
	}
	return p.quotePrefix(data[end:]) == 0 && p.isEmpty(data[end:]) == 0
}

// parse a blockquote fragment
func (p *Markdown) quote(data []byte) int {
	block := p.addBlock(BlockQuote, nil)
	var raw bytes.Buffer
	beg, end := 0, 0
	for beg < len(data) {
		end = beg
		// Step over whole lines, collecting them.
		for end < len(data) && data[end] != '\n' {
			end++
		}
		if end < len(data) && data[end] == '\n' {
			end++
		}
		if pre := p.quotePrefix(data[beg:]); pre > 0 {
			// skip the prefix
			beg += pre
		} else if p.terminateBlockquote(data, beg, end) {
			break
		}
		// this line is part of the blockquote
		raw.Write(data[beg:end])
		beg = end
	}
	p.block(raw.Bytes())
	p.finalize(block)
	return end
}

// returns unordered list item prefix
func (p *Markdown) uliPrefix(data []byte) int {
	i := 0
	// start with up to 3 spaces
	for i < len(data) && i < 3 && data[i] == ' ' {
		i++
	}
	if i >= len(data)-1 {
		return 0
	}
	// need one of {'*', '+', '-'} followed by a space or a tab
	if (data[i] != '*' && data[i] != '+' && data[i] != '-') ||
		(data[i+1] != ' ' && data[i+1] != '\t') {
		return 0
	}
	return i + 2
}

// returns ordered list item prefix
func (p *Markdown) oliPrefix(data []byte) int {
	i := 0

	// start with up to 3 spaces
	for i < 3 && i < len(data) && data[i] == ' ' {
		i++
	}

	// count the digits
	start := i
	for i < len(data) && data[i] >= '0' && data[i] <= '9' {
		i++
	}
	if start == i || i >= len(data)-1 {
		return 0
	}

	// we need >= 1 digits followed by a dot and a space or a tab
	if data[i] != '.' || !(data[i+1] == ' ' || data[i+1] == '\t') {
		return 0
	}
	return i + 2
}

// returns definition list item prefix
func (p *Markdown) dliPrefix(data []byte) int {
	if len(data) < 2 {
		return 0
	}
	i := 0
	// need a ':' followed by a space or a tab
	if data[i] != ':' || !(data[i+1] == ' ' || data[i+1] == '\t') {
		return 0
	}
	for i < len(data) && data[i] == ' ' {
		i++
	}
	return i + 2
}

// parse ordered or unordered list block
func (p *Markdown) list(data []byte, flags ListType) int {
	i := 0
	flags |= ListItemBeginningOfList
	block := p.addBlock(List, nil)
	block.ListFlags = flags
	block.Tight = true

	for i < len(data) {
		skip := p.listItem(data[i:], &flags)
		if flags&ListItemContainsBlock != 0 {
			block.ListData.Tight = false
		}
		i += skip
		if skip == 0 || flags&ListItemEndOfList != 0 {
			break
		}
		flags &= ^ListItemBeginningOfList
	}

	above := block.Parent
	finalizeList(block)
	p.tip = above
	return i
}

// Returns true if the list item is not the same type as its parent list
func (p *Markdown) listTypeChanged(data []byte, flags *ListType) bool {
	if p.dliPrefix(data) > 0 && *flags&ListTypeDefinition == 0 {
		return true
	} else if p.oliPrefix(data) > 0 && *flags&ListTypeOrdered == 0 {
		return true
	} else if p.uliPrefix(data) > 0 && (*flags&ListTypeOrdered != 0 || *flags&ListTypeDefinition != 0) {
		return true
	}
	return false
}

// Returns true if block ends with a blank line, descending if needed
// into lists and sublists.
func endsWithBlankLine(block *Node) bool {
	// TODO: figure this out. Always false now.
	for block != nil {
		//if block.lastLineBlank {
		//return true
		//}
		t := block.Type
		if t == List || t == Item {
			block = block.LastChild
		} else {
			break
		}
	}
	return false
}

func finalizeList(block *Node) {
	block.open = false
	item := block.FirstChild
	for item != nil {
		// check for non-final list item ending with blank line:
		if endsWithBlankLine(item) && item.Next != nil {
			block.ListData.Tight = false
			break
		}
		// recurse into children of list item, to see if there are spaces
		// between any of them:
		subItem := item.FirstChild
		for subItem != nil {
			if endsWithBlankLine(subItem) && (item.Next != nil || subItem.Next != nil) {
				block.ListData.Tight = false
				break
			}
			subItem = subItem.Next
		}
		item = item.Next
	}
}

// Parse a single list item.
// Assumes initial prefix is already removed if this is a sublist.
func (p *Markdown) listItem(data []byte, flags *ListType) int {
	// keep track of the indentation of the first line
	itemIndent := 0
	if data[0] == '\t' {
		itemIndent += 4
	} else {
		for itemIndent < 3 && data[itemIndent] == ' ' {
			itemIndent++
		}
	}

	var bulletChar byte = '*'
	i := p.uliPrefix(data)
	if i == 0 {
		i = p.oliPrefix(data)
	} else {
		bulletChar = data[i-2]
	}
	if i == 0 {
		i = p.dliPrefix(data)
		// reset definition term flag
		if i > 0 {
			*flags &= ^ListTypeTerm
		}
	}
	if i == 0 {
		// if in definition list, set term flag and continue
		if *flags&ListTypeDefinition != 0 {
			*flags |= ListTypeTerm
		} else {
			return 0
		}
	}

	// skip leading whitespace on first line
	for i < len(data) && data[i] == ' ' {
		i++
	}

	// find the end of the line
	line := i
	for i > 0 && i < len(data) && data[i-1] != '\n' {
		i++
	}

	// get working buffer
	var raw bytes.Buffer

	// put the first line into the working buffer
	raw.Write(data[line:i])
	line = i

	// process the following lines
	containsBlankLine := false
	sublist := 0

gatherlines:
	for line < len(data) {
		i++

		// find the end of this line
		for i < len(data) && data[i-1] != '\n' {
			i++
		}

		// if it is an empty line, guess that it is part of this item
		// and move on to the next line
		if p.isEmpty(data[line:i]) > 0 {
			containsBlankLine = true
			line = i
			continue
		}

		// calculate the indentation
		indent := 0
		indentIndex := 0
		if data[line] == '\t' {
			indentIndex++
			indent += 4
		} else {
			for indent < 4 && line+indent < i && data[line+indent] == ' ' {
				indent++
				indentIndex++
			}
		}

		chunk := data[line+indentIndex : i]

		// evaluate how this line fits in
		switch {
		// is this a nested list item?
		case (p.uliPrefix(chunk) > 0 && !p.isHRule(chunk)) ||
			p.oliPrefix(chunk) > 0 ||
			p.dliPrefix(chunk) > 0:

			// to be a nested list, it must be indented more
			// if not, it is either a different kind of list
			// or the next item in the same list
			if indent <= itemIndent {
				if p.listTypeChanged(chunk, flags) {
					*flags |= ListItemEndOfList
				} else if containsBlankLine {
					*flags |= ListItemContainsBlock
				}

				break gatherlines
			}

			if containsBlankLine {
				*flags |= ListItemContainsBlock
			}

			// is this the first item in the nested list?
			if sublist == 0 {
				sublist = raw.Len()
			}

		// anything following an empty line is only part
		// of this item if it is indented 4 spaces
		// (regardless of the indentation of the beginning of the item)
		case containsBlankLine && indent < 4:
			if *flags&ListTypeDefinition != 0 && i < len(data)-1 {
				// is the next item still a part of this list?
				next := i
				for next < len(data) && data[next] != '\n' {
					next++
				}
				for next < len(data)-1 && data[next] == '\n' {
					next++
				}
				if i < len(data)-1 && data[i] != ':' && data[next] != ':' {
					*flags |= ListItemEndOfList
				}
			} else {
				*flags |= ListItemEndOfList
			}
			break gatherlines

		// a blank line means this should be parsed as a block
		case containsBlankLine:
			raw.WriteByte('\n')
			*flags |= ListItemContainsBlock
		}

		// if this line was preceded by one or more blanks,
		// re-introduce the blank into the buffer
		if containsBlankLine {
			containsBlankLine = false
			raw.WriteByte('\n')
		}

		// add the line into the working buffer without prefix
		raw.Write(data[line+indentIndex : i])

		line = i
	}

	rawBytes := raw.Bytes()

	block := p.addBlock(Item, nil)
	block.ListFlags = *flags
	block.Tight = false
	block.BulletChar = bulletChar
	block.Delimiter = '.' // Only '.' is possible in Markdown, but ')' will also be possible in CommonMark

	// render the contents of the list item
	if *flags&ListItemContainsBlock != 0 && *flags&ListTypeTerm == 0 {
		// intermediate render of block item, except for definition term
		if sublist > 0 {
			p.block(rawBytes[:sublist])
			p.block(rawBytes[sublist:])
		} else {
			p.block(rawBytes)
		}
	} else {
		// intermediate render of inline item
		if sublist > 0 {
			child := p.addChild(Paragraph, 0)
			child.content = rawBytes[:sublist]
			p.block(rawBytes[sublist:])
		} else {
			child := p.addChild(Paragraph, 0)
			child.content = rawBytes
		}
	}
	return line
}

// render a single paragraph that has already been parsed out
func (p *Markdown) renderParagraph(data []byte) {
	if len(data) == 0 {
		return
	}

	// trim leading spaces
	beg := 0
	for data[beg] == ' ' {
		beg++
	}

	end := len(data)
	// trim trailing newline
	if data[len(data)-1] == '\n' {
		end--
	}

	// trim trailing spaces
	for end > beg && data[end-1] == ' ' {
		end--
	}

	p.addBlock(Paragraph, data[beg:end])
}

func (p *Markdown) paragraph(data []byte) int {
	// prev: index of 1st char of previous line
	// line: index of 1st char of current line
	// i: index of cursor/end of current line
	var prev, line, i int
	// keep going until we find something to mark the end of the paragraph
	for i < len(data) {
		// mark the beginning of the current line
		prev = line
		current := data[i:]
		line = i

		// did we find a blank line marking the end of the paragraph?
		if n := p.isEmpty(current); n > 0 {
			// did this blank line followed by a definition list item?
			if p.extensions&DefinitionLists != 0 {
				if i < len(data)-1 && data[i+1] == ':' {
					return p.list(data[prev:], ListTypeDefinition)
				}
			}

			p.renderParagraph(data[:i])
			return i + n
		}

		// if there's a horizontal rule after this, paragraph is over
		if p.isHRule(current) {
			p.renderParagraph(data[:i])
			return i
		}

		// if there's a definition list item, prev line is a definition term
		if p.extensions&DefinitionLists != 0 {
			if p.dliPrefix(current) != 0 {
				ret := p.list(data[prev:], ListTypeDefinition)
				return ret
			}
		}

		// if there's a list after this, paragraph is over
		if p.extensions&NoEmptyLineBeforeBlock != 0 {
			if p.uliPrefix(current) != 0 ||
				p.oliPrefix(current) != 0 ||
				p.quotePrefix(current) != 0 {
				p.renderParagraph(data[:i])
				return i
			}
		}

		// otherwise, scan to the beginning of the next line
		nl := bytes.IndexByte(data[i:], '\n')
		if nl >= 0 {
			i += nl + 1
		} else {
			i += len(data[i:])
		}
	}

	p.renderParagraph(data[:i])
	return i
}

func skipChar(data []byte, start int, char byte) int {
	i := start
	for i < len(data) && data[i] == char {
		i++
	}
	return i
}
