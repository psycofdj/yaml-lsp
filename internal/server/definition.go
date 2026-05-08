package server

import (
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/psycofdj/yaml-lsp/internal/parser"
)

// definition handles `textDocument/definition` for YAML anchors,
// returning the `&name` site for the symbol under the cursor. Works
// from either side: cursor on a `*name` alias jumps to `&name`; cursor
// on `&name` itself returns that same anchor's location (symmetric
// behavior, matching gopls/clangd for identifiers).
//
// Returns nil when the cursor is not on an anchor symbol, when no
// matching anchor exists in the document, or when the document has not
// been synced. LSP convention is for clients to treat nil as "go-to-def
// has nothing to offer here".
//
// Scope is intentionally same-document: YAML anchors are document-local
// per the spec; a `*foo` in document 2 of a multi-doc stream cannot bind
// to a `&foo` in document 1.
func (s *Server) definition(_ *glsp.Context, params *protocol.DefinitionParams) (any, error) {
	text, ok := s.documents.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	line, col, err := parserPosFromLSP(text,
		int(params.Position.Line), int(params.Position.Character))
	if err != nil {
		return nil, nil
	}
	docs, perr := parser.ParseStream([]byte(text))
	if perr != nil || len(docs) == 0 {
		return nil, nil
	}
	r, found := parser.FindAnchorDefinition(docs, line, col)
	if !found {
		return nil, nil
	}
	return protocol.Location{
		URI:   params.TextDocument.URI,
		Range: anchorRefToLSPRange(text, r),
	}, nil
}

// anchorRefToLSPRange converts a parser AnchorRef (1-based line, 1-based
// byte columns, inclusive end) to the LSP Range that covers the same
// `<sigil><name>` span (0-based, UTF-16, exclusive end).
func anchorRefToLSPRange(text string, r parser.AnchorRef) protocol.Range {
	return protocol.Range{
		Start: lspPosFromParser(text, r.Line, r.StartCol),
		// EndCol is inclusive; LSP Range.End is exclusive, so point at
		// the column just past the last name char.
		End: lspPosFromParser(text, r.Line, r.EndCol+1),
	}
}

// anchorRefNameRange returns the LSP Range covering only the name
// characters of an AnchorRef (i.e. excluding the leading `&` or `*`),
// for use by rename / prepareRename where the sigil must stay put.
func anchorRefNameRange(text string, r parser.AnchorRef) protocol.Range {
	return protocol.Range{
		Start: lspPosFromParser(text, r.Line, r.NameStartCol()),
		End:   lspPosFromParser(text, r.Line, r.EndCol+1),
	}
}

// parserPosFromLSP converts an LSP position to parser coordinates
// (1-based line, 1-based byte column). Returns an error for negative
// coordinates or positions past EOF; callers map these to "no result"
// rather than surfacing them to the client, since go-to-def from an
// invalid position is not a useful error to display.
func parserPosFromLSP(text string, lspLine, lspChar int) (int, int, *ErrInvalidPosition) {
	if lspLine < 0 || lspChar < 0 {
		return 0, 0, &ErrInvalidPosition{Line: lspLine, Character: lspChar, Reason: "negative coordinate"}
	}
	col, err := utf16OffsetToByteCol(text, lspLine, lspChar)
	if err != nil {
		return 0, 0, err
	}
	return lspLine + 1, col, nil
}
