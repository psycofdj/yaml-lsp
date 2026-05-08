// BOSH ops files address sequence elements either by numeric index or by a
// "name" field on the element. When the locate stage captured a NameKey on a
// sequence segment (i.e. the element is a mapping with a "name" property),
// this encoder uses the name-keyed form `/seq/name=<value>/...`. Otherwise it
// falls back to the numeric form `/seq/<index>/...`. Keys containing '/' are
// percent-encoded to keep the resulting path parseable by BOSH ops tooling.
package format

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/psycofdj/yaml-lsp/internal/path"
)

func init() { Register("bosh-ops", encodeBOSH) }

func encodeBOSH(p path.Path) (string, error) {
	var b strings.Builder
	for _, s := range p {
		b.WriteByte('/')
		switch {
		case s.IsIndex && s.NameKey != "":
			b.WriteString("name=")
			b.WriteString(escapeBOSHValue(s.NameKey))
		case s.IsIndex:
			b.WriteString(strconv.Itoa(s.Index))
		default:
			b.WriteString(escapeBOSHValue(s.Key))
		}
	}
	return b.String(), nil
}

// escapeBOSHValue percent-encodes characters that would break BOSH ops path
// parsing: the segment separator '/', the name=value selector character '=',
// the URL-fragment indicators '?' and '#', and whitespace. Common identifier
// characters pass through unchanged.
func escapeBOSHValue(s string) string {
	if !strings.ContainsAny(s, "/?#= \t") {
		return s
	}
	// QueryEscape encodes spaces as '+'; restore them as %20 since BOSH path
	// segments are not query strings.
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}
