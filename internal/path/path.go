// Package path defines the shared structural address used by the locate and
// format layers. A Path is an ordered sequence of Segments rooted at a YAML
// document. Each Segment is either a mapping key (Key) or a sequence index
// (Index). Encoders consume a Path and stringify it; they never touch the AST.
package path

import (
	"fmt"
	"strings"
)

// NodeKind classifies the cursor's relationship to the resolved node.
//
//   - Key   : the cursor is on a mapping key token
//   - Value : the cursor is on a value token (scalar, mapping, or sequence
//     held by a mapping property), or on a sequence element value.
//   - None  : the cursor is in whitespace, a comment, or otherwise outside any
//     node; the path is empty but documentIndex is still meaningful.
type NodeKind string

const (
	NodeKindKey   NodeKind = "key"
	NodeKindValue NodeKind = "value"
	NodeKindNone  NodeKind = "none"
)

// Segment is a single step of a Path: either a mapping key or a sequence
// index. The IsIndex flag discriminates which field is meaningful.
type Segment struct {
	IsIndex bool
	Key     string
	Index   int
	// NameKey, if non-empty, holds the value of the element's "name" field
	// when the segment refers to a mapping element of a sequence whose
	// elements carry a "name" property. The bosh-ops encoder prefers this
	// form when present; other encoders ignore it. Index is still set so
	// numeric encoders work uniformly.
	NameKey string
}

// Path is an ordered sequence of segments rooted at the containing document.
type Path []Segment

// String returns a debug-friendly rendering. It is NOT a stable serialization
// format — encoders in internal/format/* are responsible for the user-facing
// path syntax of each tool ecosystem.
func (p Path) String() string {
	var b strings.Builder
	for _, s := range p {
		if s.IsIndex {
			fmt.Fprintf(&b, "[%d]", s.Index)
		} else {
			b.WriteByte('.')
			b.WriteString(s.Key)
		}
	}
	return b.String()
}
