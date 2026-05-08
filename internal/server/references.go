package server

import (
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/psycofdj/yaml-lsp/internal/parser"
)

// references handles `textDocument/references` for YAML anchors,
// returning every `*name` alias in the document — and, when
// `context.includeDeclaration` is true, the `&name` definition too.
// Works from either side of the relation: cursor on the anchor or on
// any alias produces the same complete result set, matching how
// `xref-find-references` and `lsp-mode`'s reference UI expect symbols
// to behave.
//
// Scope is intentionally same-document: YAML anchors are document-local
// per the spec.
func (s *Server) references(_ *glsp.Context, params *protocol.ReferenceParams) ([]protocol.Location, error) {
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
	refs := parser.CollectAnchorOccurrences(docs[idx], name)
	if len(refs) == 0 {
		return nil, nil
	}
	out := make([]protocol.Location, 0, len(refs))
	for _, r := range refs {
		if r.Kind == parser.AnchorKindAnchor && !params.Context.IncludeDeclaration {
			continue
		}
		out = append(out, protocol.Location{
			URI:   params.TextDocument.URI,
			Range: anchorRefToLSPRange(text, r),
		})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
