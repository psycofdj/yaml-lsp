package server

import (
	"unicode/utf8"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// lspPosFromParser converts a 1-based parser line and 1-based byte column
// to an LSP Position (0-based line + 0-based UTF-16 char offset). The
// `src` argument is the post-BOM-strip text so byte columns line up with
// what goccy/go-yaml reports.
func lspPosFromParser(src string, line, byteCol int) protocol.Position {
	if line < 1 {
		line = 1
	}
	if byteCol < 1 {
		byteCol = 1
	}
	lineText := lineTextOf(src, line)
	return protocol.Position{
		Line:      protocol.UInteger(line - 1),
		Character: protocol.UInteger(byteColToUTF16(lineText, byteCol)),
	}
}

// lspPosLineEnd returns the LSP position just past the last character on
// the given 1-based line. Used as `Range.End`, which LSP treats as
// exclusive.
func lspPosLineEnd(src string, line int) protocol.Position {
	if line < 1 {
		return protocol.Position{}
	}
	text := lineTextOf(src, line)
	return protocol.Position{
		Line:      protocol.UInteger(line - 1),
		Character: protocol.UInteger(byteColToUTF16(text, len(text)+1)),
	}
}

// lineTextOf returns the content of the given 1-based line in `src`,
// without the trailing newline. Lines beyond the source return "".
func lineTextOf(src string, line int) string {
	start, cur := 0, 1
	for i := 0; i < len(src) && cur < line; i++ {
		if src[i] == '\n' {
			cur++
			start = i + 1
		}
	}
	if cur < line {
		return ""
	}
	end := start
	for end < len(src) && src[end] != '\n' {
		end++
	}
	return src[start:end]
}

// byteColToUTF16 converts a 1-based byte column on a single line to the
// 0-based UTF-16 character offset that LSP expects. Invalid UTF-8 bytes
// are counted as one unit each (defensive).
func byteColToUTF16(line string, byteCol int) int {
	target := byteCol - 1
	if target > len(line) {
		target = len(line)
	}
	units, pos := 0, 0
	for pos < target {
		r, size := utf8.DecodeRuneInString(line[pos:])
		if r == utf8.RuneError && size == 1 {
			units++
			pos++
			continue
		}
		if r > 0xFFFF {
			units += 2
		} else {
			units++
		}
		pos += size
	}
	return units
}
