package server

import (
	"fmt"

	"github.com/goccy/go-yaml/ast"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/psycofdj/yaml-lsp/internal/format"
	"github.com/psycofdj/yaml-lsp/internal/parser"
	"github.com/psycofdj/yaml-lsp/internal/path"
)

// documentSymbol handles `textDocument/documentSymbol` by walking the
// parsed AST and emitting a FLAT list of symbols whose names are the
// JSONPath address of each node (`$.metadata.name`, `$.spec.containers[0].image`).
// The list is intentionally flat — no `Children` — so that
// `lsp-ui-imenu` (and any other client consuming this method) renders
// a one-row-per-path index rather than a collapsible tree.
//
// Trade-off: this change affects every documentSymbol consumer, not
// just lsp-ui-imenu. VS Code's outline pane and lsp-mode's
// treemacs-symbols view will also show the same flat jsonpath list.
// That is consistent with the project's "everything is a jsonpath"
// theme — `yaml/addressAtPoint`, hover, and now the outline all speak
// the same address language.
//
// Multi-document streams: when the stream contains more than one
// document, each path is prefixed with `[N] ` where N is the 0-based
// document index, so doc-2's `$.metadata.name` shows as
// `[1] $.metadata.name`. Single-doc streams omit the prefix.
//
// Both container nodes (mappings, sequences) and scalar leaves get
// entries. The Kind reflects the value's YAML type
// (Object/Array/String/Number/Boolean/Null) so editors can still pick
// the right icon. Range covers the entry key through the value's last
// token; SelectionRange pins the cursor target to the key (or the
// element's start, for sequences).
func (s *Server) documentSymbol(_ *glsp.Context, params *protocol.DocumentSymbolParams) (any, error) {
	text, ok := s.documents.Get(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	// The parser strips a leading BOM; use the same view of `text` for
	// position resolution so byte columns match goccy's.
	src := string(parser.StripBOM([]byte(text)))
	docs, err := parser.ParseStream([]byte(text))
	if err != nil {
		// Editors expect symbols on a best-effort basis; signalling an
		// error suppresses the outline entirely. Return an empty list.
		return []protocol.DocumentSymbol{}, nil
	}
	w := &symWalker{src: src, multiDoc: len(docs) > 1}
	out := []protocol.DocumentSymbol{}
	for i, d := range docs {
		if d.Body == nil {
			continue
		}
		w.docIdx = i
		w.walk(d.Body, nil, &out)
	}
	return out, nil
}

// symWalker carries the per-request state: the source text (for
// LSP-coordinate conversion) and the current document index plus a
// flag for whether we're in a multi-doc stream (controls the path
// prefix).
type symWalker struct {
	src      string
	docIdx   int
	multiDoc bool
}

// walk recursively traverses node, appending one DocumentSymbol per
// mapping entry / sequence element to out. `prefix` is the path leading
// up to (but not including) node.
func (w *symWalker) walk(node ast.Node, prefix path.Path, out *[]protocol.DocumentSymbol) {
	if node == nil {
		return
	}
	switch n := node.(type) {
	case *ast.MappingNode:
		for _, e := range n.Values {
			w.emitMappingEntry(e, prefix, out)
		}
	case *ast.MappingValueNode:
		w.emitMappingEntry(n, prefix, out)
	case *ast.SequenceNode:
		for i, v := range n.Values {
			w.emitSequenceElement(i, v, prefix, out)
		}
	case *ast.AnchorNode:
		w.walk(n.Value, prefix, out)
	case *ast.TagNode:
		w.walk(n.Value, prefix, out)
	}
}

// emitMappingEntry appends a symbol for e (whose key extends prefix)
// and recurses into the value.
func (w *symWalker) emitMappingEntry(e *ast.MappingValueNode, prefix path.Path, out *[]protocol.DocumentSymbol) {
	if e == nil || e.Key == nil {
		return
	}
	keyTok := e.Key.GetToken()
	if keyTok == nil || keyTok.Position == nil {
		return
	}
	childPath := extendPath(prefix, path.Segment{Key: keyTok.Value})
	selStart := w.lspPos(keyTok.Position.Line, keyTok.Position.Column)
	selEnd := w.lspPos(keyTok.Position.Line, keyTok.Position.Column+len(keyTok.Value))
	rngEndLine := keyTok.Position.Line
	if e.Value != nil {
		rngEndLine = maxTokenLine(e.Value, rngEndLine)
	}
	*out = append(*out, protocol.DocumentSymbol{
		Name:           w.formatName(childPath),
		Kind:           kindForValue(e.Value),
		Range:          protocol.Range{Start: selStart, End: w.lspPosLineEnd(rngEndLine)},
		SelectionRange: protocol.Range{Start: selStart, End: selEnd},
	})
	w.walk(e.Value, childPath, out)
}

// emitSequenceElement appends a symbol for the i-th element of a
// sequence and recurses into it.
func (w *symWalker) emitSequenceElement(i int, v ast.Node, prefix path.Path, out *[]protocol.DocumentSymbol) {
	childPath := extendPath(prefix, path.Segment{IsIndex: true, Index: i})
	startLine, startCol := nodeStart(v)
	endLine := maxTokenLine(v, startLine)
	startPos := w.lspPos(startLine, startCol)
	*out = append(*out, protocol.DocumentSymbol{
		Name:           w.formatName(childPath),
		Kind:           kindForValue(v),
		Range:          protocol.Range{Start: startPos, End: w.lspPosLineEnd(endLine)},
		SelectionRange: protocol.Range{Start: startPos, End: startPos},
	})
	w.walk(v, childPath, out)
}

// formatName encodes p as a JSONPath, prepending `[N] ` when the
// stream contains more than one document.
func (w *symWalker) formatName(p path.Path) string {
	jp, err := format.Encode(p, "jsonpath")
	if err != nil {
		// Encode only fails on unsupported format names — "jsonpath"
		// is registered, so this is unreachable in practice. Fall back
		// to Path.String() rather than dropping the entry.
		jp = p.String()
	}
	if w.multiDoc {
		return fmt.Sprintf("[%d] %s", w.docIdx, jp)
	}
	return jp
}

// extendPath returns prefix with seg appended, without aliasing the
// underlying slice — important because the same `prefix` is reused
// across siblings during the walk.
func extendPath(prefix path.Path, seg path.Segment) path.Path {
	out := make(path.Path, len(prefix)+1)
	copy(out, prefix)
	out[len(prefix)] = seg
	return out
}

// kindForValue maps a YAML AST node to an LSP SymbolKind icon.
func kindForValue(v ast.Node) protocol.SymbolKind {
	switch n := v.(type) {
	case *ast.MappingNode, *ast.MappingValueNode:
		return protocol.SymbolKindObject
	case *ast.SequenceNode:
		return protocol.SymbolKindArray
	case *ast.StringNode, *ast.LiteralNode:
		return protocol.SymbolKindString
	case *ast.IntegerNode, *ast.FloatNode:
		return protocol.SymbolKindNumber
	case *ast.BoolNode:
		return protocol.SymbolKindBoolean
	case *ast.NullNode:
		return protocol.SymbolKindNull
	case *ast.AnchorNode:
		return kindForValue(n.Value)
	case *ast.TagNode:
		return kindForValue(n.Value)
	default:
		return protocol.SymbolKindKey
	}
}

// Position helpers delegate to the package-level converters in
// positions.go so call sites stay readable.
func (w *symWalker) lspPos(line, byteCol int) protocol.Position {
	return lspPosFromParser(w.src, line, byteCol)
}

func (w *symWalker) lspPosLineEnd(line int) protocol.Position {
	return lspPosLineEnd(w.src, line)
}

// nodeStart returns the 1-based (line, byte column) of the first
// content token of n. For containers it descends into the first child
// so the start position points at content rather than a bookkeeping
// `:`.
func nodeStart(n ast.Node) (int, int) {
	if n == nil {
		return 1, 1
	}
	switch v := n.(type) {
	case *ast.MappingNode:
		if len(v.Values) > 0 && v.Values[0].Key != nil {
			if t := v.Values[0].Key.GetToken(); t != nil && t.Position != nil {
				return t.Position.Line, t.Position.Column
			}
		}
	case *ast.MappingValueNode:
		if v.Key != nil {
			if t := v.Key.GetToken(); t != nil && t.Position != nil {
				return t.Position.Line, t.Position.Column
			}
		}
	case *ast.SequenceNode:
		if len(v.Values) > 0 {
			return nodeStart(v.Values[0])
		}
	}
	if t := n.GetToken(); t != nil && t.Position != nil {
		return t.Position.Line, t.Position.Column
	}
	return 1, 1
}

// maxTokenLine returns the largest 1-based line reached by any token
// in n, with floor as a lower bound. Mirrors the helper in
// internal/parser; duplicated here to avoid cross-package coupling.
func maxTokenLine(n ast.Node, floor int) int {
	max := floor
	if n == nil {
		return max
	}
	if t := n.GetToken(); t != nil && t.Position != nil && t.Position.Line > max {
		max = t.Position.Line
	}
	switch v := n.(type) {
	case *ast.MappingNode:
		for _, e := range v.Values {
			if m := maxTokenLine(e, max); m > max {
				max = m
			}
		}
	case *ast.MappingValueNode:
		if m := maxTokenLine(v.Key, max); m > max {
			max = m
		}
		if m := maxTokenLine(v.Value, max); m > max {
			max = m
		}
	case *ast.SequenceNode:
		for _, e := range v.Values {
			if m := maxTokenLine(e, max); m > max {
				max = m
			}
		}
	case *ast.AnchorNode:
		if m := maxTokenLine(v.Value, max); m > max {
			max = m
		}
	case *ast.TagNode:
		if m := maxTokenLine(v.Value, max); m > max {
			max = m
		}
	}
	return max
}
