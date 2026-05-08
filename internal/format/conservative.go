package format

import "strings"

// ConservativeOptions tunes Conservative.  Defaults mirror the LSP 3.15+
// FormattingOptions of the same name: trimTrailingWhitespace,
// insertFinalNewline, trimFinalNewlines all default to true (matches
// what `lsp-format-buffer` users expect from a YAML formatter).
type ConservativeOptions struct {
	TrimTrailingWhitespace bool
	InsertFinalNewline     bool
	TrimFinalNewlines      bool
}

// DefaultConservativeOptions returns the recommended baseline: all three
// normalizations enabled.
func DefaultConservativeOptions() ConservativeOptions {
	return ConservativeOptions{
		TrimTrailingWhitespace: true,
		InsertFinalNewline:     true,
		TrimFinalNewlines:      true,
	}
}

// Conservative reshapes src with whitespace-only normalizations:
//
//   - per-line trailing spaces and tabs stripped (preserving any \r so
//     CRLF line endings survive intact);
//   - trailing blank lines after the last content line removed;
//   - a single trailing newline ensured (using the dominant line ending
//     present in src — \r\n if the document uses CRLF anywhere,
//     otherwise \n).
//
// Comments, anchors, quote styles, key ordering, and indentation are all
// preserved unchanged — this is intentionally NOT a YAML reformatter,
// which would have to roundtrip through the AST and lose comments. Use
// `yamlfmt` or `prettier` externally for that.
func Conservative(src string, opts ConservativeOptions) string {
	if src == "" {
		return src
	}
	eol := dominantLineEnding(src)
	lines := splitLines(src)
	// hadFinalNewline tracks whether the original input ended in a line
	// terminator. We use it to decide whether `InsertFinalNewline=false`
	// means "leave it as it was" (the LSP spec's intent) vs "force no
	// trailing newline" (a meaning the option does not carry).
	hadFinalNewline := len(lines) > 0 && lines[len(lines)-1] == ""
	if hadFinalNewline {
		lines = lines[:len(lines)-1]
	}

	if opts.TrimTrailingWhitespace {
		for i, line := range lines {
			lines[i] = trimTrailingHorizontalWS(line)
		}
	}
	if opts.TrimFinalNewlines {
		// Drop trailing all-whitespace lines (after trim they're empty).
		for len(lines) > 0 && trimTrailingHorizontalWS(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
	}

	out := strings.Join(lines, eol)
	switch {
	case opts.InsertFinalNewline:
		out += eol
	case hadFinalNewline:
		// Original had a trailing newline and the user did not ask us to
		// trim it — keep it. We only remove the original's trailing
		// terminator above when assembling `lines`.
		out += eol
	}
	return out
}

// dominantLineEnding picks the line ending to use for output. CRLF wins
// if any \r\n appears in src; otherwise \n. Mixed-ending files thus
// normalize to CRLF on the assumption that a Windows editor wrote them.
func dominantLineEnding(src string) string {
	if strings.Contains(src, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// splitLines splits on \n and preserves any \r left on the line, so
// CRLF line endings survive even though we split on the LF only.
// Returns one extra empty trailing element when src ends with \n —
// mirrors strings.Split's behavior, which we rely on for tracking
// whether the original had a final newline.
func splitLines(src string) []string {
	return strings.Split(src, "\n")
}

// trimTrailingHorizontalWS removes trailing spaces and tabs (and any \r
// immediately preceding the EOL) from a line. Inner whitespace is
// untouched.
func trimTrailingHorizontalWS(line string) string {
	end := len(line)
	for end > 0 {
		c := line[end-1]
		if c == ' ' || c == '\t' || c == '\r' {
			end--
			continue
		}
		break
	}
	return line[:end]
}
