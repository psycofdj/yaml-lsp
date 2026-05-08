// Package parser wraps goccy/go-yaml to expose a stream of documents whose AST
// nodes carry token-level position information. Locate consumes this output;
// the LSP boundary and the CLI consume Locate's output.
package parser

import (
	"github.com/goccy/go-yaml/ast"
	goparser "github.com/goccy/go-yaml/parser"
)

// Document is one YAML document within a stream. Body is the AST root (nil
// for an empty document, e.g. between two `---` separators). StartLine is the
// 1-based line of the document's first body token (or of its `---` header for
// any document after the first). EndLine is the 1-based line of the last
// content token within the body (StartLine when Body is nil). Locate uses
// these bounds to decide which document a cursor falls into and whether the
// cursor is past the document's content.
type Document struct {
	Body      ast.Node
	StartLine int
	EndLine   int
}

// utf8BOM is the byte sequence editors sometimes prepend to UTF-8 files.
// goccy treats it as part of the first key when present, shifting columns by
// 3; we strip it at the boundary so locate sees the canonical bytes.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// StripBOM returns src with a leading UTF-8 BOM removed (if present). Used
// by ParseStream and by the CLI so the LSP server and `yaml-lsp address`
// agree on what column 1 of line 1 means.
func StripBOM(src []byte) []byte {
	if len(src) >= 3 && src[0] == utf8BOM[0] && src[1] == utf8BOM[1] && src[2] == utf8BOM[2] {
		return src[3:]
	}
	return src
}

// ParseStream parses src and returns one Document per YAML document in the
// stream, in source order. A leading UTF-8 BOM is stripped. Empty documents
// are preserved (with Body=nil) so the reported documentIndex matches the
// human's count of `---`-separated sections (yq convention).
func ParseStream(src []byte) ([]Document, error) {
	src = StripBOM(src)
	f, err := goparser.ParseBytes(src, 0)
	if err != nil {
		return nil, err
	}
	docs := make([]Document, 0, len(f.Docs))
	for _, d := range f.Docs {
		startLine := 1
		switch {
		case d.Start != nil:
			startLine = d.Start.Position.Line
		case d.Body != nil:
			// Streams whose first document has no `---` header (legal at
			// stream start) leave d.Start nil; derive from the body's first
			// content token so multi-doc routing still works.
			startLine = nodeStartLine(d.Body)
		}
		if startLine <= 0 {
			startLine = 1
		}
		endLine := startLine
		if d.Body != nil {
			endLine = maxTokenLine(d.Body, startLine)
		}
		docs = append(docs, Document{Body: d.Body, StartLine: startLine, EndLine: endLine})
	}
	return docs, nil
}

// maxTokenLine returns the largest 1-based line number reached by any token
// in n, with floor as a lower bound. Used to bound the last entry of the
// outermost mapping/sequence so cursors past the document content resolve to
// NodeKindNone instead of being absorbed by the trailing entry.
func maxTokenLine(n ast.Node, floor int) int {
	max := floor
	if n == nil {
		return max
	}
	if tok := n.GetToken(); tok != nil && tok.Position != nil && tok.Position.Line > max {
		max = tok.Position.Line
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

// nodeStartLine returns the 1-based line of the first content token in n. For
// containers, this means the first key (mapping) or the first element
// (sequence), not the container's bookkeeping start token, because goccy's
// MappingNode.Start can point at the colon of the first entry rather than the
// key itself.
func nodeStartLine(n ast.Node) int {
	if n == nil {
		return 0
	}
	switch v := n.(type) {
	case *ast.MappingNode:
		if len(v.Values) > 0 {
			return nodeStartLine(v.Values[0])
		}
		if v.Start != nil {
			return v.Start.Position.Line
		}
	case *ast.MappingValueNode:
		if v.Key != nil {
			if tok := v.Key.GetToken(); tok != nil {
				return tok.Position.Line
			}
		}
	case *ast.SequenceNode:
		if len(v.Values) > 0 {
			return nodeStartLine(v.Values[0])
		}
		if v.Start != nil {
			return v.Start.Position.Line
		}
	case *ast.AnchorNode:
		if v.Value != nil {
			return nodeStartLine(v.Value)
		}
	case *ast.TagNode:
		if v.Value != nil {
			return nodeStartLine(v.Value)
		}
	}
	if tok := n.GetToken(); tok != nil {
		return tok.Position.Line
	}
	return 0
}
