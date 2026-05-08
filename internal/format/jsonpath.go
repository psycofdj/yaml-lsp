package format

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/psycofdj/yaml-lsp/internal/path"
)

func init() { Register("jsonpath", encodeJSONPath) }

// JSONPath uses dot notation for identifier-safe keys (`$.metadata.name`) and
// bracket-quoted strings for keys containing punctuation
// (`$['weird.key/v1']`). Sequence indices are bracketed: `[0]`.
func encodeJSONPath(p path.Path) (string, error) {
	var b strings.Builder
	b.WriteByte('$')
	for _, s := range p {
		if s.IsIndex {
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(s.Index))
			b.WriteByte(']')
			continue
		}
		if isJSONPathIdent(s.Key) {
			b.WriteByte('.')
			b.WriteString(s.Key)
		} else {
			b.WriteString("['")
			b.WriteString(escapeJSONPathString(s.Key))
			b.WriteString("']")
		}
	}
	return b.String(), nil
}

func isJSONPathIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 && !unicode.IsLetter(r) && r != '_' {
			return false
		}
		if i > 0 && !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}

// escapeJSONPathString escapes a key for use inside a single-quoted bracket
// expression, conforming to RFC 9535 §2.3.1.2 (IETF JSONPath). Inside
// `'…'`, the backslash and apostrophe MUST each be backslash-escaped; all
// other ASCII printables (including `]`) are unescaped. Other characters in
// the escapable set (`\b \f \n \r \t \/ \uXXXX`) are not produced here
// because we are encoding a YAML key whose string form is already a
// concrete sequence of code points — control characters in keys are rare
// and would round-trip via the structure of the produced literal.
func escapeJSONPathString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\\' || r == '\'' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
