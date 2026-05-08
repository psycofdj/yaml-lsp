package server

import (
	"github.com/goccy/go-yaml/ast"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/psycofdj/yaml-lsp/internal/parser"
)

// foldingRange handles `textDocument/foldingRange` requests by emitting one
// folding region per multi-line YAML container. Each mapping entry whose
// value spans more than one line yields a region from the entry's key line
// through the last line of its value subtree, so editors can collapse a
// nested mapping or sequence behind its key. Sequence elements that are
// themselves multi-line containers also fold.
//
// Folding regions are NOT emitted for the document body itself (no parent
// key to fold under) or for flow-style containers that already fit on one
// line.
func (s *Server) foldingRange(_ *glsp.Context, params *protocol.FoldingRangeParams) ([]protocol.FoldingRange, error) {
	text, ok := s.documents.Get(params.TextDocument.URI)
	if !ok {
		return []protocol.FoldingRange{}, nil
	}
	docs, err := parser.ParseStream([]byte(text))
	if err != nil {
		return []protocol.FoldingRange{}, nil
	}
	var out []protocol.FoldingRange
	for _, d := range docs {
		if d.Body != nil {
			collectFolds(d.Body, &out)
		}
	}
	if out == nil {
		out = []protocol.FoldingRange{}
	}
	return out, nil
}

// collectFolds appends a FoldingRange for each multi-line mapping entry
// or sequence element under n.
func collectFolds(n ast.Node, out *[]protocol.FoldingRange) {
	if n == nil {
		return
	}
	switch v := n.(type) {
	case *ast.MappingNode:
		for _, e := range v.Values {
			emitMappingValueFold(e, out)
		}
	case *ast.MappingValueNode:
		emitMappingValueFold(v, out)
	case *ast.SequenceNode:
		for _, child := range v.Values {
			emitSequenceElementFold(child, out)
		}
	case *ast.AnchorNode:
		collectFolds(v.Value, out)
	case *ast.TagNode:
		collectFolds(v.Value, out)
	}
}

func emitMappingValueFold(e *ast.MappingValueNode, out *[]protocol.FoldingRange) {
	if e == nil || e.Key == nil || e.Value == nil {
		return
	}
	keyTok := e.Key.GetToken()
	if keyTok == nil || keyTok.Position == nil {
		return
	}
	keyLine := keyTok.Position.Line
	endLine := maxTokenLine(e.Value, keyLine)
	if endLine > keyLine {
		*out = append(*out, protocol.FoldingRange{
			StartLine: protocol.UInteger(keyLine - 1),
			EndLine:   protocol.UInteger(endLine - 1),
		})
	}
	// Recurse into the value so nested containers also get fold regions.
	collectFolds(e.Value, out)
}

func emitSequenceElementFold(child ast.Node, out *[]protocol.FoldingRange) {
	if child == nil {
		return
	}
	startLine, _ := nodeStart(child)
	endLine := maxTokenLine(child, startLine)
	if endLine > startLine {
		*out = append(*out, protocol.FoldingRange{
			StartLine: protocol.UInteger(startLine - 1),
			EndLine:   protocol.UInteger(endLine - 1),
		})
	}
	collectFolds(child, out)
}
