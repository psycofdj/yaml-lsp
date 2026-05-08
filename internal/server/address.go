package server

import (
	"fmt"
	"strings"
	"unicode/utf8"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/psycofdj/yaml-lsp/internal/format"
	"github.com/psycofdj/yaml-lsp/internal/parser"
	"github.com/psycofdj/yaml-lsp/internal/path"
)

// utf8BOM is the byte sequence editors sometimes prepend to UTF-8 files.
// We strip it at the parser boundary; here we adjust line-0 column counting
// so the LSP→byte conversion lines up with the post-strip parser view.
var utf8BOM = "\xEF\xBB\xBF"

// AddressAtPointParams is the JSON shape clients send for the
// `yaml/addressAtPoint` custom request.
type AddressAtPointParams struct {
	TextDocument protocol.TextDocumentIdentifier `json:"textDocument"`
	Position     protocol.Position               `json:"position"`
	Format       string                          `json:"format"`
}

// AddressAtPointResult is what the server returns.
type AddressAtPointResult struct {
	Path          string `json:"path"`
	Format        string `json:"format"`
	DocumentIndex int    `json:"documentIndex"`
	NodeKind      string `json:"nodeKind"`
}

// ErrInvalidPosition is returned when the LSP coordinate cannot be mapped to
// a position within the document. The dispatcher maps this to JSON-RPC
// CodeInvalidParams (-32602), matching the spec's I/O Matrix.
type ErrInvalidPosition struct {
	Line, Character int
	Reason          string
}

func (e *ErrInvalidPosition) Error() string {
	return fmt.Sprintf("invalid position: line=%d character=%d (%s)", e.Line, e.Character, e.Reason)
}

// locateAtLSP converts an LSP-coordinate position into parser-coordinate
// (1-based line + 1-based byte column) and runs the parser's Locate. The
// returned ok==false case means there are no documents (Locate's contract);
// callers should return an empty/none response in that case. Both
// `AddressAtPoint` and `Hover` go through this helper so the LSP→byte
// conversion lives in exactly one place.
func locateAtLSP(text string, lspLine, lspChar int) (path.Path, path.NodeKind, int, bool, error) {
	if lspLine < 0 || lspChar < 0 {
		return nil, path.NodeKindNone, 0, false,
			&ErrInvalidPosition{Line: lspLine, Character: lspChar, Reason: "negative coordinate"}
	}
	totalLines := strings.Count(text, "\n") + 1
	if lspLine >= totalLines {
		return nil, path.NodeKindNone, 0, false,
			&ErrInvalidPosition{Line: lspLine, Character: lspChar, Reason: fmt.Sprintf("line beyond EOF (%d lines)", totalLines)}
	}
	line := lspLine + 1
	col, posErr := utf16OffsetToByteCol(text, lspLine, lspChar)
	if posErr != nil {
		return nil, path.NodeKindNone, 0, false, posErr
	}
	docs, err := parser.ParseStream([]byte(text))
	if err != nil {
		return nil, path.NodeKindNone, 0, false, err
	}
	if len(docs) == 0 {
		return nil, path.NodeKindNone, 0, false, nil
	}
	p, kind, idx, ok := parser.Locate(docs, line, col)
	return p, kind, idx, ok, nil
}

// AddressAtPoint runs the locate-and-encode pipeline on the given document
// text. It is exposed (and exported) so tests can call it without a running
// LSP server. Position is in LSP coordinates: 0-based line, 0-based UTF-16
// character offset on that line. Conversion to the parser's 1-based byte
// columns happens here, at the LSP boundary; parser/locate/format never see
// LSP coordinates.
func AddressAtPoint(text string, lspLine, lspChar int, fmtName string) (*AddressAtPointResult, error) {
	if !format.IsSupported(fmtName) {
		return nil, &format.ErrUnsupportedFormat{Got: fmtName, Supported: format.SupportedFormats()}
	}
	p, kind, idx, ok, err := locateAtLSP(text, lspLine, lspChar)
	if err != nil {
		return nil, err
	}
	if !ok {
		return &AddressAtPointResult{Format: fmtName, NodeKind: string(path.NodeKindNone)}, nil
	}
	encoded := ""
	if kind != path.NodeKindNone && len(p) > 0 {
		encoded, err = format.Encode(p, fmtName)
		if err != nil {
			return nil, err
		}
	}
	return &AddressAtPointResult{
		Path:          encoded,
		Format:        fmtName,
		DocumentIndex: idx,
		NodeKind:      string(kind),
	}, nil
}

// utf16OffsetToByteCol converts a 0-based UTF-16 character offset on
// lspLine (0-based) to a 1-based byte column. Characters above U+FFFF
// occupy two UTF-16 code units and one rune; we walk runes and accumulate
// both counters so the conversion is correct for surrogate pairs.
//
// On line 0, a leading UTF-8 BOM is consumed silently — the parser strips
// it from its view of the document, so the byte column we report has to
// match. On lines past EOL we clamp the byte column to the line length
// (cursor "at end of line" is a valid LSP position; we extend that to
// "past EOL" since clients vary).
//
// Invalid UTF-8 bytes (`RuneError` of size 1) are tolerated: each is
// counted as one UTF-16 unit and one byte, so a cursor positioned BEFORE
// a bad byte resolves cleanly. Only when the cursor's offset requires
// advancing past one or more bad bytes do we return *ErrInvalidPosition
// (and even then, the caller can decide whether to surface it). This
// matches the spirit of LSP's "best effort on malformed input" rather
// than poisoning every cursor on the line.
func utf16OffsetToByteCol(text string, lspLine, lspChar int) (int, *ErrInvalidPosition) {
	lineStart := 0
	if lspLine == 0 && strings.HasPrefix(text, utf8BOM) {
		lineStart = len(utf8BOM)
	}
	currentLine := 0
	for i := lineStart; i < len(text) && currentLine < lspLine; i++ {
		if text[i] == '\n' {
			currentLine++
			lineStart = i + 1
		}
	}
	units := 0
	byteOffsetInLine := 0
	sawBadByte := false
	for byteOffsetInLine < len(text)-lineStart && units < lspChar {
		r, size := utf8.DecodeRuneInString(text[lineStart+byteOffsetInLine:])
		if r == '\n' || r == '\r' {
			// Cursor past end of line: clamp to the line length. Both LF
			// and CRLF terminations are treated as the same EOL boundary.
			break
		}
		if r == utf8.RuneError && size == 1 {
			// Skip the bad byte but remember we crossed one — if the
			// cursor required this advance, the caller may want to know.
			sawBadByte = true
			units++
			byteOffsetInLine++
			continue
		}
		if r > 0xFFFF {
			units += 2
		} else {
			units++
		}
		byteOffsetInLine += size
	}
	if sawBadByte && units == lspChar {
		// We advanced past at least one invalid byte to reach the cursor.
		// The position is unreliable; surface it as invalid params.
		return byteOffsetInLine + 1, &ErrInvalidPosition{
			Line: lspLine, Character: lspChar,
			Reason: "invalid UTF-8 in document text crosses cursor position",
		}
	}
	return byteOffsetInLine + 1, nil
}
