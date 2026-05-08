package server

import (
	"errors"
	"fmt"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/psycofdj/yaml-lsp/internal/parser"
)

// ErrInvalidAnchorName is returned (as the rename request's error) when
// the requested new anchor name is empty or contains characters that
// YAML 1.2 disallows in a `c-anchor-char`. lsp-mode surfaces it in the
// minibuffer; the document is left unchanged.
type ErrInvalidAnchorName struct {
	Got    string
	Reason string
}

func (e *ErrInvalidAnchorName) Error() string {
	return fmt.Sprintf("invalid anchor name %q: %s", e.Got, e.Reason)
}

// prepareRename handles `textDocument/prepareRename`. It returns the
// range covering just the name part of the anchor symbol under the
// cursor (so the editor highlights the right text and pre-fills the
// minibuffer), or nil when the cursor is not on a renameable symbol.
//
// We deliberately return Range, not DefaultBehavior — the LSP "default"
// behavior is to use the word at point, which for `&anchor`/`*anchor`
// would include the leading sigil. Returning an explicit range pins the
// rename target to the name characters only.
func (s *Server) prepareRename(_ *glsp.Context, params *protocol.PrepareRenameParams) (any, error) {
	text, ok := s.documents.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	line, col, posErr := parserPosFromLSP(text,
		int(params.Position.Line), int(params.Position.Character))
	if posErr != nil {
		return nil, nil
	}
	docs, perr := parser.ParseStream([]byte(text))
	if perr != nil || len(docs) == 0 {
		return nil, nil
	}
	name, idx, ok := parser.FindAnchorSymbol(docs, line, col)
	if !ok {
		return nil, nil
	}
	// Locate the AnchorRef at the cursor (anchor or alias) so we can
	// report its name-only range as the rename target.
	for _, r := range parser.CollectAnchorOccurrences(docs[idx], name) {
		if r.Line != line {
			continue
		}
		if col >= r.StartCol && col <= r.EndCol {
			return protocol.RangeWithPlaceholder{
				Range:       anchorRefNameRange(text, r),
				Placeholder: name,
			}, nil
		}
	}
	return nil, nil
}

// rename handles `textDocument/rename`. Produces a WorkspaceEdit that
// atomically replaces the name portion of every `&name` and `*name`
// occurrence in the same document with NewName. The leading `&`/`*`
// sigils are preserved.
//
// Document scope: the rename is intentionally bounded to the document
// containing the cursor (YAML anchors are document-local per spec).
// Other documents in the same stream are not touched.
//
// Invalid names are rejected up front so a typo doesn't quietly mangle
// the buffer.
func (s *Server) rename(_ *glsp.Context, params *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	if err := validateAnchorName(params.NewName); err != nil {
		return nil, err
	}
	text, ok := s.documents.Get(params.TextDocument.URI)
	if !ok {
		return nil, errors.New("document not synced")
	}
	line, col, posErr := parserPosFromLSP(text,
		int(params.Position.Line), int(params.Position.Character))
	if posErr != nil {
		return nil, posErr
	}
	docs, perr := parser.ParseStream([]byte(text))
	if perr != nil || len(docs) == 0 {
		return nil, nil
	}
	name, idx, ok := parser.FindAnchorSymbol(docs, line, col)
	if !ok {
		return nil, nil
	}
	refs := parser.CollectAnchorOccurrences(docs[idx], name)
	if len(refs) == 0 {
		return nil, nil
	}
	edits := make([]protocol.TextEdit, 0, len(refs))
	for _, r := range refs {
		edits = append(edits, protocol.TextEdit{
			Range:   anchorRefNameRange(text, r),
			NewText: params.NewName,
		})
	}
	return &protocol.WorkspaceEdit{
		Changes: map[protocol.DocumentUri][]protocol.TextEdit{
			params.TextDocument.URI: edits,
		},
	}, nil
}

// validateAnchorName checks that name is a legal YAML 1.2 anchor name:
// non-empty, no whitespace, no flow indicators (`,`, `[`, `]`, `{`,
// `}`), and no anchor/alias/tag sigils (`&`, `*`, `!`). The YAML spec
// defines `c-anchor-char` as any non-whitespace non-flow-indicator;
// the additional sigil exclusions are pragmatic — a name starting with
// `*` would round-trip back as an alias.
func validateAnchorName(name string) error {
	if name == "" {
		return &ErrInvalidAnchorName{Got: name, Reason: "empty"}
	}
	for _, r := range name {
		switch r {
		case ' ', '\t', '\n', '\r':
			return &ErrInvalidAnchorName{Got: name, Reason: "contains whitespace"}
		case ',', '[', ']', '{', '}':
			return &ErrInvalidAnchorName{Got: name, Reason: fmt.Sprintf("contains flow indicator %q", r)}
		case '&', '*', '!':
			return &ErrInvalidAnchorName{Got: name, Reason: fmt.Sprintf("contains YAML sigil %q", r)}
		}
	}
	return nil
}
