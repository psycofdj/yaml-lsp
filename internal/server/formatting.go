package server

import (
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/psycofdj/yaml-lsp/internal/format"
)

// formatting handles `textDocument/formatting`. The reformatter roundtrips
// the document through gopkg.in/yaml.v3, preserving comments via the AST and
// splicing folded scalar bodies back from the source to avoid yaml.v3's
// re-wrap. The chosen indent comes from the user's `format.indentation`
// initialization option (detect-from-source or fixed integer); quote-stripping
// is gated on `format.normalizeStrings`.
//
// If yaml.v3 cannot parse the buffer (transiently invalid YAML during typing),
// the server falls back to the conservative line-level formatter so the user
// still gets trailing-whitespace cleanup rather than an error.
func (s *Server) formatting(_ *glsp.Context, params *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	text, ok := s.documents.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	formatted := s.formatDocument(text, params.Options)
	if formatted == text {
		return nil, nil
	}
	return []protocol.TextEdit{{
		Range:   wholeDocumentRange(text),
		NewText: formatted,
	}}, nil
}

// rangeFormatting handles `textDocument/rangeFormatting`. yaml.v3 cannot
// reformat a partial document (indentation context depends on bytes outside
// the range), so we format the whole buffer and return only the per-line
// edits that fall inside the snapped range. If formatting changed the line
// count (e.g. a folded scalar was reflowed across a different number of
// lines) we fall back to a whole-document edit, since per-line filtering
// would produce nonsense.
func (s *Server) rangeFormatting(_ *glsp.Context, params *protocol.DocumentRangeFormattingParams) ([]protocol.TextEdit, error) {
	text, ok := s.documents.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	formatted := s.formatDocument(text, params.Options)
	if formatted == text {
		return nil, nil
	}
	startLine, endLineExcl := snapRangeToLines(text, params.Range)
	if startLine >= endLineExcl {
		return nil, nil
	}
	srcLines := splitKeepEmpty(text)
	dstLines := splitKeepEmpty(formatted)
	if len(srcLines) != len(dstLines) {
		// Line count diverged — we can't safely filter; fall back to
		// whole-document replacement so the user still sees the result.
		return []protocol.TextEdit{{
			Range:   wholeDocumentRange(text),
			NewText: formatted,
		}}, nil
	}
	if endLineExcl > len(srcLines) {
		endLineExcl = len(srcLines)
	}
	// Collect contiguous runs of changed lines within the range.
	var edits []protocol.TextEdit
	i := startLine
	for i < endLineExcl {
		if srcLines[i] == dstLines[i] {
			i++
			continue
		}
		j := i + 1
		for j < endLineExcl && srcLines[j] != dstLines[j] {
			j++
		}
		_, sliceRange := extractLineSlice(text, i, j)
		edits = append(edits, protocol.TextEdit{
			Range:   sliceRange,
			NewText: strings.Join(dstLines[i:j], "\n") + sliceTerminator(formatted, dstLines, j),
		})
		i = j
	}
	if len(edits) == 0 {
		return nil, nil
	}
	return edits, nil
}

// formatDocument runs the yaml.v3 reformatter with the server's configured
// options, falling back to whitespace-only conservative formatting when
// yaml.v3 can't parse the buffer (mid-edit invalid state). Honors the LSP
// per-request InsertFinalNewline option.
func (s *Server) formatDocument(text string, lspOpts protocol.FormattingOptions) string {
	opts := format.DefaultYaml3Options()
	opts.Indent = s.config.Format.Indentation
	opts.NormalizeStrings = s.config.Format.NormalizeStrings
	if v, ok := lspOpts[protocol.FormattingOptionInsertFinalNewline].(bool); ok {
		opts.InsertFinalNewline = v
	}
	if out, err := format.Yaml3(text, opts); err == nil {
		return out
	}
	// Parse failure — fall back to byte-level cleanup so the user still
	// gets something useful on a transiently malformed buffer.
	return format.Conservative(text, conservativeFallback(lspOpts))
}

// conservativeFallback builds ConservativeOptions from per-request LSP
// FormattingOptions for the fall-back path. Mirrors the historical defaults
// (trim trailing whitespace, ensure final newline, drop trailing blanks)
// while honoring any client overrides on those three keys.
func conservativeFallback(o protocol.FormattingOptions) format.ConservativeOptions {
	opts := format.DefaultConservativeOptions()
	if v, ok := o[protocol.FormattingOptionTrimTrailingWhitespace].(bool); ok {
		opts.TrimTrailingWhitespace = v
	}
	if v, ok := o[protocol.FormattingOptionInsertFinalNewline].(bool); ok {
		opts.InsertFinalNewline = v
	}
	if v, ok := o[protocol.FormattingOptionTrimFinalNewlines].(bool); ok {
		opts.TrimFinalNewlines = v
	}
	return opts
}

// splitKeepEmpty splits text on `\n`, dropping the trailing empty element
// strings.Split produces from a trailing newline. The result mirrors the
// document's visible lines so index comparisons line up between source and
// formatted output.
func splitKeepEmpty(text string) []string {
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// sliceTerminator returns the newline (if any) that the replacement text
// must carry. Mid-document slices always need `\n` between themselves and
// the next surviving line. At the document's tail, the slice's Range
// (computed by extractLineSlice) covers the trailing `\n` of the source —
// we must reproduce that `\n` in NewText whenever the formatted output
// itself ends with one, or the edit would silently strip the final newline.
func sliceTerminator(formatted string, dstLines []string, endExcl int) string {
	if endExcl < len(dstLines) {
		return "\n"
	}
	if strings.HasSuffix(formatted, "\n") {
		return "\n"
	}
	return ""
}

// wholeDocumentRange returns a Range that covers `text` end-to-end.
// End is positioned past the final character on the final line so
// LSP's exclusive Range.End semantics produce a true full replacement.
func wholeDocumentRange(text string) protocol.Range {
	lineCount := strings.Count(text, "\n") + 1
	lastLine := lineCount - 1
	lastLineText := lineTextOf(text, lineCount) // 1-based
	endChar := byteColToUTF16(lastLineText, len(lastLineText)+1)
	return protocol.Range{
		Start: protocol.Position{Line: 0, Character: 0},
		End: protocol.Position{
			Line:      protocol.UInteger(lastLine),
			Character: protocol.UInteger(endChar),
		},
	}
}

// snapRangeToLines returns [startLine, endLineExcl) in 0-based LSP line
// coordinates such that the slice always covers whole lines. If the
// range's End is at column 0 of line N, the slice stops at N (does NOT
// include line N). Otherwise it extends through line N.
func snapRangeToLines(text string, r protocol.Range) (int, int) {
	lineCount := strings.Count(text, "\n") + 1
	start := int(r.Start.Line)
	if start < 0 {
		start = 0
	}
	if start > lineCount {
		start = lineCount
	}
	end := int(r.End.Line)
	if r.End.Character > 0 {
		end++
	}
	if end < start {
		end = start
	}
	if end > lineCount {
		end = lineCount
	}
	return start, end
}

// extractLineSlice returns (text, range) for the LSP-line interval
// [startLine, endLineExcl). The range starts at (startLine, 0) and ends
// either at (endLineExcl, 0) if there is content after that line, or at
// the document's end position if the slice reaches EOF.
func extractLineSlice(text string, startLine, endLineExcl int) (string, protocol.Range) {
	byteStart := lineStartByte(text, startLine)
	byteEnd := lineStartByte(text, endLineExcl)
	slice := text[byteStart:byteEnd]
	end := protocol.Position{Line: protocol.UInteger(endLineExcl), Character: 0}
	if byteEnd == len(text) {
		// Slice reaches EOF: snap to the actual end position so a
		// formatted slice that trims the trailing newline doesn't
		// leave the rest of the file misaligned.
		end = wholeDocumentRange(text).End
	}
	return slice, protocol.Range{
		Start: protocol.Position{Line: protocol.UInteger(startLine), Character: 0},
		End:   end,
	}
}

// lineStartByte returns the 0-based byte offset of the start of the
// 0-based LSP line, clamped to len(text) for lines past EOF.
func lineStartByte(text string, lspLine int) int {
	if lspLine <= 0 {
		return 0
	}
	cur := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			cur++
			if cur == lspLine {
				return i + 1
			}
		}
	}
	return len(text)
}
