package format

import (
	"strconv"
	"strings"

	"github.com/psycofdj/yaml-lsp/internal/path"
)

func init() { Register("helm-values", encodeHelm) }

// Helm `--set` syntax: keys are dot-joined, sequences use `[N]` indices.
// Backslash-escape commas, dots, equals signs, and backslashes inside a key
// so the receiver can split on the unescaped delimiters.
func encodeHelm(p path.Path) (string, error) {
	var b strings.Builder
	for i, s := range p {
		if s.IsIndex {
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(s.Index))
			b.WriteByte(']')
			continue
		}
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(escapeHelmKey(s.Key))
	}
	return b.String(), nil
}

func escapeHelmKey(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case ',', '.', '=', '\\', '[', ']':
			// '[' and ']' are Helm's index syntax delimiters; without
			// escaping, a key like "foo[0]" would be misread as an index.
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
