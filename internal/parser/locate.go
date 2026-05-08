package parser

import (
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/token"

	"github.com/psycofdj/yaml-lsp/internal/path"
)

// Locate returns the structural address of the YAML node at (line, col), the
// kind of position (key/value/none), and the 0-based index of the document
// containing the cursor. Coordinates are 1-based line and 1-based byte column,
// matching the goccy parser's token positions. The LSP boundary is responsible
// for converting from LSP UTF-16 positions; this function knows nothing about
// LSP.
//
// The boolean is false only when there are no documents at all. A cursor in
// whitespace returns an empty path with NodeKindNone but ok=true.
func Locate(docs []Document, line, col int) (path.Path, path.NodeKind, int, bool) {
	if len(docs) == 0 {
		return nil, path.NodeKindNone, 0, false
	}
	docIdx := pickDoc(docs, line)
	doc := docs[docIdx]
	// Cursor past the document's content (e.g. trailing blank lines before
	// the next `---` or EOF) resolves to NodeKindNone instead of being
	// absorbed by the last entry.
	if doc.Body == nil || line > doc.EndLine || line < doc.StartLine {
		return nil, path.NodeKindNone, docIdx, true
	}
	p, kind := locateInNode(doc.Body, line, col, nil)
	if kind == "" {
		kind = path.NodeKindNone
	}
	return p, kind, docIdx, true
}

func pickDoc(docs []Document, line int) int {
	idx := 0
	for i := range docs {
		if docs[i].StartLine <= line {
			idx = i
		}
	}
	return idx
}

func locateInNode(node ast.Node, line, col int, base path.Path) (path.Path, path.NodeKind) {
	if node == nil {
		return base, path.NodeKindNone
	}
	switch n := node.(type) {
	case *ast.MappingNode:
		return locateInMapping(n.Values, line, col, base)
	case *ast.MappingValueNode:
		return locateInMapping([]*ast.MappingValueNode{n}, line, col, base)
	case *ast.SequenceNode:
		return locateInSequence(n.Values, line, col, base)
	case *ast.AnchorNode:
		return locateInNode(n.Value, line, col, base)
	case *ast.TagNode:
		return locateInNode(n.Value, line, col, base)
	default:
		// Scalar leaf — caller already extended base for the property holding
		// this scalar, so we just classify the cursor as on the value.
		return base, path.NodeKindValue
	}
}

func locateInMapping(entries []*ast.MappingValueNode, line, col int, base path.Path) (path.Path, path.NodeKind) {
	if len(entries) == 0 {
		return base, path.NodeKindNone
	}
	matchIdx := -1
	for i, e := range entries {
		if e == nil || e.Key == nil {
			continue
		}
		startLine := keyLine(e)
		if startLine <= 0 {
			continue
		}
		endLine := -1
		if i+1 < len(entries) {
			endLine = keyLine(entries[i+1]) - 1
		}
		if line >= startLine && (endLine < 0 || line <= endLine) {
			matchIdx = i
		}
	}
	if matchIdx < 0 {
		return base, path.NodeKindNone
	}
	e := entries[matchIdx]
	keyName := keyString(e)
	extended := append(append(path.Path{}, base...), path.Segment{Key: keyName})

	if onTokenColumn(e.Key.GetToken(), line, col) {
		return extended, path.NodeKindKey
	}
	p, kind := locateInNode(e.Value, line, col, extended)
	if kind == path.NodeKindNone {
		return extended, path.NodeKindValue
	}
	return p, kind
}

func locateInSequence(values []ast.Node, line, col int, base path.Path) (path.Path, path.NodeKind) {
	if len(values) == 0 {
		return base, path.NodeKindNone
	}
	matchIdx := -1
	for i, v := range values {
		startLine := nodeStartLine(v)
		endLine := -1
		if i+1 < len(values) {
			endLine = nodeStartLine(values[i+1]) - 1
		}
		if line >= startLine && (endLine < 0 || line <= endLine) {
			matchIdx = i
		}
	}
	if matchIdx < 0 {
		return base, path.NodeKindNone
	}
	v := values[matchIdx]
	seg := path.Segment{IsIndex: true, Index: matchIdx}
	if name, ok := extractName(v); ok {
		seg.NameKey = name
	}
	extended := append(append(path.Path{}, base...), seg)
	p, kind := locateInNode(v, line, col, extended)
	if kind == path.NodeKindNone {
		return extended, path.NodeKindValue
	}
	return p, kind
}

func keyLine(e *ast.MappingValueNode) int {
	if e == nil || e.Key == nil {
		return 0
	}
	if tok := e.Key.GetToken(); tok != nil {
		return tok.Position.Line
	}
	return 0
}

func keyString(e *ast.MappingValueNode) string {
	if e == nil || e.Key == nil {
		return ""
	}
	if tok := e.Key.GetToken(); tok != nil {
		return tok.Value
	}
	return e.Key.String()
}

// onTokenColumn returns true when (line, col) falls within the token's
// lexical extent on a single line. For multi-line tokens we only match the
// first line — multi-line scalars are addressed by their property's key.
func onTokenColumn(tok *token.Token, line, col int) bool {
	if tok == nil || tok.Position == nil {
		return false
	}
	if tok.Position.Line != line {
		return false
	}
	// tok.Origin can include leading whitespace/newlines for block-style
	// keys (e.g. "\n  name"), so it's unsafe as a width measure. tok.Value
	// carries the parsed key text without surrounding whitespace; use it,
	// then add 2 for quoted tokens so the column range covers both quote
	// characters too. This makes cursor-on-quote resolve to the key for
	// both empty (`""`) and non-empty (`"foo"`) quoted forms.
	width := len(tok.Value)
	if tok.Type == token.DoubleQuoteType || tok.Type == token.SingleQuoteType {
		width += 2
	}
	if width == 0 {
		width = 1
	}
	start := tok.Position.Column
	end := start + width - 1
	return col >= start && col <= end
}

// extractName returns the value of a "name" property when v is a mapping
// whose "name" entry holds a scalar. Non-scalar values (`name: [a, b]`,
// `name: |\n  …`, etc.) are rejected because their first token is
// punctuation or a block indicator and would produce a nonsensical
// name=<x> selector. The bosh-ops encoder uses this for the name-keyed
// sequence form (`/seq/name=foo/...`); other encoders ignore it.
func extractName(v ast.Node) (string, bool) {
	var entries []*ast.MappingValueNode
	switch m := v.(type) {
	case *ast.MappingNode:
		entries = m.Values
	case *ast.MappingValueNode:
		entries = []*ast.MappingValueNode{m}
	default:
		return "", false
	}
	for _, e := range entries {
		if keyString(e) != "name" {
			continue
		}
		if e.Value == nil {
			return "", false
		}
		switch n := e.Value.(type) {
		case *ast.StringNode:
			return n.Value, true
		case *ast.IntegerNode, *ast.FloatNode, *ast.BoolNode, *ast.NullNode:
			if tok := e.Value.GetToken(); tok != nil {
				return tok.Value, true
			}
		}
		return "", false
	}
	return "", false
}
