package server

import (
	"fmt"
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/psycofdj/yaml-lsp/internal/parser"
)

// completion handles `textDocument/completion` for YAML anchor aliases:
// when the cursor sits inside a `*<prefix>` context, every `&name`
// definition in the same document is offered as a candidate.
//
// Scope is deliberately narrow — only alias-name completion. Schema-
// driven key/value completion is the territory of
// yaml-language-server (RedHat) and would require pulling in JSON
// Schema infrastructure; that's not what this server is for. The
// alias slice has no external dependency and pairs naturally with the
// existing `&anchor` definition/references/rename machinery.
//
// Returns nil when the cursor is not in an alias context (whitespace,
// inside a key, after `&`, etc.) so the client falls back to its own
// generic word completion.
func (s *Server) completion(_ *glsp.Context, params *protocol.CompletionParams) (any, error) {
	text, ok := s.documents.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	line, col, posErr := parserPosFromLSP(text,
		int(params.Position.Line), int(params.Position.Character))
	if posErr != nil {
		return nil, nil
	}
	prefix, prefixStartCol, inContext := aliasPrefixAt(text, line, col)
	if !inContext {
		return nil, nil
	}
	docs, perr := parser.ParseStream([]byte(text))
	if perr != nil {
		// goccy rejects a dangling `*` (no name) or a `*<prefix>` whose
		// name isn't yet defined, which is exactly the state at the
		// instant a user invokes completion. Retry with the alias
		// span masked to whitespace so the rest of the document still
		// parses and we can enumerate anchors. The mask preserves
		// byte length, so AnchorRef columns remain valid.
		masked := maskAliasContext(text, line, prefixStartCol, prefix)
		docs, perr = parser.ParseStream([]byte(masked))
		if perr != nil || len(docs) == 0 {
			return nil, nil
		}
	} else if len(docs) == 0 {
		return nil, nil
	}
	docIdx := parser.PickDocAtLine(docs, line)
	defs := parser.CollectAnchorDefinitions(docs[docIdx])
	if len(defs) == 0 {
		return nil, nil
	}

	// LSP Range for the TextEdit: from the column just after `*` up to
	// the cursor.  The client may also send identifier characters that
	// arrive after the cursor (e.g. user typed `*fo|bar`); we leave
	// those untouched so completion never eats text the user has
	// already typed past the caret.
	editStart := lspPosFromParser(text, line, prefixStartCol)
	editEnd := params.Position

	// Dedup by name — multiple `&foo` definitions in one document are
	// legal (the parser resolves to the most recent at use), but for
	// completion one entry per name is the right UX. We keep the
	// FIRST occurrence's line for the Detail field since source order
	// matches reading order.
	seen := make(map[string]struct{}, len(defs))
	items := make([]protocol.CompletionItem, 0, len(defs))
	kind := protocol.CompletionItemKindReference
	plain := protocol.InsertTextFormatPlainText
	for _, d := range defs {
		name := text[byteOffsetOf(text, d.Line, d.NameStartCol()):byteOffsetOf(text, d.Line, d.EndCol+1)]
		if _, dup := seen[name]; dup {
			continue
		}
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			// Pre-filter by prefix. The client filters too, but this
			// keeps the response small for documents with many anchors.
			continue
		}
		seen[name] = struct{}{}
		detail := anchorDetail(d)
		items = append(items, protocol.CompletionItem{
			Label:            name,
			Kind:             &kind,
			Detail:           &detail,
			InsertTextFormat: &plain,
			TextEdit: protocol.TextEdit{
				Range:   protocol.Range{Start: editStart, End: editEnd},
				NewText: name,
			},
		})
	}
	if len(items) == 0 {
		return nil, nil
	}
	// IsIncomplete=false: the result is exhaustive for the current
	// document. The client can filter on further typing without
	// re-requesting.
	return protocol.CompletionList{IsIncomplete: false, Items: items}, nil
}

// aliasPrefixAt scans backwards from (line, col) on the same line and
// reports the partial anchor name typed in a `*<prefix>` context. When
// the cursor is not preceded by `*<name-chars>*`, ok=false.
//
// `col` is a 1-based byte column (post-LSP-conversion). prefixStartCol
// is 1-based — the column of the first name character after the `*`.
func aliasPrefixAt(text string, line, col int) (prefix string, prefixStartCol int, ok bool) {
	lineText := lineTextOf(text, line)
	end := col - 1
	if end < 0 {
		end = 0
	}
	if end > len(lineText) {
		end = len(lineText)
	}
	i := end
	for i > 0 && isAnchorNameByte(lineText[i-1]) {
		i--
	}
	if i == 0 || lineText[i-1] != '*' {
		return "", 0, false
	}
	return lineText[i:end], i + 1, true
}

// maskAliasContext returns text with the `*<prefix>` span on `line`
// (parser coordinates: 1-based line, 1-based byte column of the first
// name char) replaced by spaces of the same length. The byte length is
// preserved so positions computed against the original text remain
// valid against the masked copy.
func maskAliasContext(text string, line, nameStartCol int, prefix string) string {
	// The `*` sits at column nameStartCol-1. We replace the `*` itself
	// plus every byte of the typed prefix.
	lineStart := byteOffsetOf(text, line, 1)
	start := lineStart + nameStartCol - 2
	end := start + 1 + len(prefix)
	if start < 0 || start >= len(text) {
		return text
	}
	if end > len(text) {
		end = len(text)
	}
	b := []byte(text)
	for i := start; i < end; i++ {
		if b[i] == '\n' {
			break
		}
		b[i] = ' '
	}
	return string(b)
}

// isAnchorNameByte is the inverse of `validateAnchorName`'s reject
// list, byte-level (anchor names are ASCII-ish in practice; we err on
// the permissive side for non-ASCII since YAML allows them).
func isAnchorNameByte(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n', ',', '[', ']', '{', '}', '&', '*', '!':
		return false
	}
	return true
}

// anchorDetail produces the right-hand "where this anchor lives" text
// that lsp-mode shows beside the candidate.
func anchorDetail(r parser.AnchorRef) string {
	return fmt.Sprintf("anchor at line %d", r.Line)
}

// byteOffsetOf converts a 1-based line and 1-based byte column to a
// 0-based byte offset into text. Used so we can read the anchor name
// back out of the source by AnchorRef coordinates, without an extra
// trip through the AST.
func byteOffsetOf(text string, line, col int) int {
	if line < 1 {
		line = 1
	}
	if col < 1 {
		col = 1
	}
	cur := 1
	off := 0
	for off < len(text) && cur < line {
		if text[off] == '\n' {
			cur++
		}
		off++
	}
	off += col - 1
	if off > len(text) {
		off = len(text)
	}
	return off
}
