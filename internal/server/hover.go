package server

import (
	"fmt"
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/psycofdj/yaml-lsp/internal/format"
	"github.com/psycofdj/yaml-lsp/internal/path"
)

// hover handles `textDocument/hover` requests by reusing the locate pipeline
// and rendering the address in all four supported formats as a Markdown
// table. When the cursor is not on a node (whitespace, blank line, between
// documents) the response is nil — lsp-mode interprets that as "no hover".
func (s *Server) hover(_ *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	text, ok := s.documents.Get(params.TextDocument.URI)
	if !ok {
		// No record of this document; quietly skip rather than error.
		return nil, nil
	}
	p, kind, _, located, err := locateAtLSP(text,
		int(params.Position.Line), int(params.Position.Character))
	if err != nil {
		return nil, err
	}
	if !located || kind == path.NodeKindNone || len(p) == 0 {
		return nil, nil
	}

	var b strings.Builder
	b.WriteString("**yaml-lsp address**\n\n")
	b.WriteString("| format | path |\n|---|---|\n")
	for _, fmtName := range format.SupportedFormats() {
		encoded, encErr := format.Encode(p, fmtName)
		if encErr != nil {
			// Should not happen — SupportedFormats only returns registered
			// names — but skip the row defensively rather than panic.
			continue
		}
		fmt.Fprintf(&b, "| `%s` | `%s` |\n", fmtName, encoded)
	}

	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: b.String(),
		},
	}, nil
}
