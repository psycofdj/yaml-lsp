package format

import (
	"strconv"
	"strings"

	"github.com/psycofdj/yaml-lsp/internal/path"
)

func init() { Register("jsonpatch", encodeJSONPatch) }

// JSON Patch (RFC 6902) targets use RFC 6901 JSON Pointer paths: a sequence
// of '/'-separated reference tokens. Mapping keys are escaped: '~' -> '~0',
// '/' -> '~1'. Sequence indices are written as decimal. Used by kustomize's
// `patches:` (json6902) field and any direct JSON Patch consumer.
func encodeJSONPatch(p path.Path) (string, error) {
	var b strings.Builder
	for _, s := range p {
		b.WriteByte('/')
		if s.IsIndex {
			b.WriteString(strconv.Itoa(s.Index))
		} else {
			b.WriteString(escapeJSONPointer(s.Key))
		}
	}
	return b.String(), nil
}

func escapeJSONPointer(s string) string {
	// '~' must be escaped before '/' so that any literal '/' produced by the
	// '/' substitution is not double-escaped.
	s = strings.ReplaceAll(s, "~", "~0")
	s = strings.ReplaceAll(s, "/", "~1")
	return s
}
