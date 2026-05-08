package parser

import (
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/token"
)

// AnchorKind discriminates an anchor definition (`&name`) from an alias
// reference (`*name`).
type AnchorKind int

const (
	// AnchorKindAnchor is a `&name` definition.
	AnchorKindAnchor AnchorKind = iota
	// AnchorKindAlias is a `*name` reference.
	AnchorKindAlias
)

// AnchorRef is one occurrence of an anchor in a document — either the
// `&name` definition or a `*name` alias. Columns are 1-based byte
// columns matching goccy's token positions. StartCol points at the
// leading `&` or `*`; EndCol is the inclusive column of the last
// character of the name.
type AnchorRef struct {
	Line     int
	StartCol int
	EndCol   int
	Kind     AnchorKind
}

// NameStartCol returns the 1-based byte column of the first character
// of the name (i.e. just past the leading `&` or `*`). Used by rename,
// where the edit must cover only the name and leave the sigil intact.
func (r AnchorRef) NameStartCol() int { return r.StartCol + 1 }

// FindAnchorSymbol returns the anchor name at (line, col) when the
// cursor sits on either a `&name` definition or a `*name` alias, along
// with the 0-based index of the document containing it. Returns
// ok=false when the cursor is on whitespace, a plain scalar, a key, or
// otherwise not on an anchor symbol.
//
// Coordinates are 1-based line and 1-based byte column. Document scope
// matches the YAML spec: anchors are document-local, so a symbol found
// here is only meaningful within docs[docIdx].
func FindAnchorSymbol(docs []Document, line, col int) (string, int, bool) {
	if len(docs) == 0 {
		return "", 0, false
	}
	docIdx := pickDoc(docs, line)
	doc := docs[docIdx]
	if doc.Body == nil {
		return "", docIdx, false
	}
	if name, ok := findAnchorAt(doc.Body, line, col); ok {
		return name, docIdx, true
	}
	if name, ok := findAliasAt(doc.Body, line, col); ok {
		return name, docIdx, true
	}
	return "", docIdx, false
}

// CollectAnchorOccurrences walks doc.Body and returns every `&name` and
// `*name` whose name matches the argument, in source order. Empty doc
// or empty name yields nil. Multiple `&name` definitions in one
// document are all included (YAML allows redefining; the parser
// resolves to the most recent, but rename/references must touch every
// site).
func CollectAnchorOccurrences(doc Document, name string) []AnchorRef {
	if doc.Body == nil || name == "" {
		return nil
	}
	var refs []AnchorRef
	collectOccurrences(doc.Body, name, &refs)
	return refs
}

// CollectAnchorDefinitions returns every `&name` anchor definition in
// doc.Body, in source order. Used by completion to enumerate the
// candidate names available for a `*<prefix>` alias context.
//
// Duplicate names are NOT deduplicated here — callers that want one
// entry per name (e.g. completion) collapse them. Preserving every
// site keeps the helper useful for rename/diagnostics that need to
// touch all definitions.
func CollectAnchorDefinitions(doc Document) []AnchorRef {
	if doc.Body == nil {
		return nil
	}
	var refs []AnchorRef
	walkAnchorDefs(doc.Body, &refs)
	return refs
}

// PickDocAtLine returns the 0-based index of the document containing
// the 1-based line `line`. Used by features that take a cursor
// position and need to scope their work to one document of a multi-doc
// stream (anchors are document-local; completion, references, and
// rename all need this).
func PickDocAtLine(docs []Document, line int) int {
	if len(docs) == 0 {
		return 0
	}
	return pickDoc(docs, line)
}

// FindAnchorDefinition returns the source span of the `&name`
// definition for the symbol under the cursor. When the cursor is on a
// `*name` alias, the result is the matching `&name`. When the cursor
// is on a `&name` itself, the result is that same anchor's own span
// (symmetric definition lookup, matching gopls/clangd for identifiers).
//
// Returns ok=false when the cursor is not on an anchor symbol, or when
// the symbol's alias has no matching anchor in the same document.
func FindAnchorDefinition(docs []Document, line, col int) (AnchorRef, bool) {
	name, idx, ok := FindAnchorSymbol(docs, line, col)
	if !ok {
		return AnchorRef{}, false
	}
	for _, r := range CollectAnchorOccurrences(docs[idx], name) {
		if r.Kind == AnchorKindAnchor {
			return r, true
		}
	}
	return AnchorRef{}, false
}

// findAliasAt walks node looking for an AliasNode whose `*name` token
// span covers (line, col), returning the alias name on hit.
func findAliasAt(node ast.Node, line, col int) (string, bool) {
	if node == nil {
		return "", false
	}
	switch n := node.(type) {
	case *ast.MappingNode:
		for _, e := range n.Values {
			if name, ok := findAliasAt(e, line, col); ok {
				return name, true
			}
		}
	case *ast.MappingValueNode:
		if name, ok := findAliasAt(n.Key, line, col); ok {
			return name, true
		}
		if name, ok := findAliasAt(n.Value, line, col); ok {
			return name, true
		}
	case *ast.SequenceNode:
		for _, v := range n.Values {
			if name, ok := findAliasAt(v, line, col); ok {
				return name, true
			}
		}
	case *ast.AnchorNode:
		// Anchored values can contain aliases inside; recurse into the
		// value but do not treat the anchor sigil itself as an alias.
		return findAliasAt(n.Value, line, col)
	case *ast.TagNode:
		return findAliasAt(n.Value, line, col)
	case *ast.AliasNode:
		if onSigilSpan(n.Start, aliasName(n), line, col) {
			return aliasName(n), true
		}
	}
	return "", false
}

// findAnchorAt walks node looking for an AnchorNode whose `&name` token
// span covers (line, col), returning the anchor name on hit.
func findAnchorAt(node ast.Node, line, col int) (string, bool) {
	if node == nil {
		return "", false
	}
	switch n := node.(type) {
	case *ast.MappingNode:
		for _, e := range n.Values {
			if name, ok := findAnchorAt(e, line, col); ok {
				return name, true
			}
		}
	case *ast.MappingValueNode:
		if name, ok := findAnchorAt(n.Key, line, col); ok {
			return name, true
		}
		if name, ok := findAnchorAt(n.Value, line, col); ok {
			return name, true
		}
	case *ast.SequenceNode:
		for _, v := range n.Values {
			if name, ok := findAnchorAt(v, line, col); ok {
				return name, true
			}
		}
	case *ast.AnchorNode:
		if onSigilSpan(n.Start, anchorName(n), line, col) {
			return anchorName(n), true
		}
		// Anchors can nest (anchored mapping/sequence containing
		// another anchor inside); recurse into the value.
		return findAnchorAt(n.Value, line, col)
	case *ast.TagNode:
		return findAnchorAt(n.Value, line, col)
	}
	return "", false
}

// walkAnchorDefs appends every `&name` definition found under node to
// refs, walking in source order.
func walkAnchorDefs(node ast.Node, refs *[]AnchorRef) {
	if node == nil {
		return
	}
	switch n := node.(type) {
	case *ast.MappingNode:
		for _, e := range n.Values {
			walkAnchorDefs(e, refs)
		}
	case *ast.MappingValueNode:
		walkAnchorDefs(n.Key, refs)
		walkAnchorDefs(n.Value, refs)
	case *ast.SequenceNode:
		for _, v := range n.Values {
			walkAnchorDefs(v, refs)
		}
	case *ast.AnchorNode:
		if r, ok := sigilRef(n.Start, anchorName(n), AnchorKindAnchor); ok {
			*refs = append(*refs, r)
		}
		walkAnchorDefs(n.Value, refs)
	case *ast.TagNode:
		walkAnchorDefs(n.Value, refs)
	}
}

// collectOccurrences appends every `&name` and `*name` matching name to
// refs, walking node in source order.
func collectOccurrences(node ast.Node, name string, refs *[]AnchorRef) {
	if node == nil {
		return
	}
	switch n := node.(type) {
	case *ast.MappingNode:
		for _, e := range n.Values {
			collectOccurrences(e, name, refs)
		}
	case *ast.MappingValueNode:
		collectOccurrences(n.Key, name, refs)
		collectOccurrences(n.Value, name, refs)
	case *ast.SequenceNode:
		for _, v := range n.Values {
			collectOccurrences(v, name, refs)
		}
	case *ast.AnchorNode:
		if anchorName(n) == name {
			if r, ok := sigilRef(n.Start, anchorName(n), AnchorKindAnchor); ok {
				*refs = append(*refs, r)
			}
		}
		collectOccurrences(n.Value, name, refs)
	case *ast.TagNode:
		collectOccurrences(n.Value, name, refs)
	case *ast.AliasNode:
		if aliasName(n) == name {
			if r, ok := sigilRef(n.Start, aliasName(n), AnchorKindAlias); ok {
				*refs = append(*refs, r)
			}
		}
	}
}

// onSigilSpan reports whether (line, col) sits within the
// `<sigil><name>` span anchored at tok. tok is the `&` / `*` token,
// `name` is the symbol name immediately following it on the same line.
func onSigilSpan(tok *token.Token, name string, line, col int) bool {
	if tok == nil || tok.Position == nil {
		return false
	}
	if tok.Position.Line != line {
		return false
	}
	start := tok.Position.Column
	end := start + 1 + len(name) - 1
	if end < start {
		end = start
	}
	return col >= start && col <= end
}

// sigilRef builds an AnchorRef from the `&`/`*` token and the adjacent
// name. Returns ok=false when the token lacks position info.
func sigilRef(tok *token.Token, name string, kind AnchorKind) (AnchorRef, bool) {
	if tok == nil || tok.Position == nil {
		return AnchorRef{}, false
	}
	start := tok.Position.Column
	end := start + 1 + len(name) - 1
	if end < start {
		end = start
	}
	return AnchorRef{
		Line:     tok.Position.Line,
		StartCol: start,
		EndCol:   end,
		Kind:     kind,
	}, true
}

func anchorName(n *ast.AnchorNode) string {
	if n == nil || n.Name == nil {
		return ""
	}
	if tok := n.Name.GetToken(); tok != nil {
		return tok.Value
	}
	return n.Name.String()
}

func aliasName(n *ast.AliasNode) string {
	if n == nil || n.Value == nil {
		return ""
	}
	if tok := n.Value.GetToken(); tok != nil {
		return tok.Value
	}
	return n.Value.String()
}
