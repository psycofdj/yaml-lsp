package format

import (
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Yaml3Options drives the yaml.v3-based reformatter.
//
// Indent of 0 means "detect from source"; any positive integer is fed to
// `yaml.Encoder.SetIndent`. NormalizeStrings, when true, clears quote styles
// on string scalars whose value can be safely emitted as a YAML plain scalar.
type Yaml3Options struct {
	Indent             int
	NormalizeStrings   bool
	InsertFinalNewline bool
}

// DefaultYaml3Options returns the recommended baseline: detect indent, keep
// existing quote styles, ensure a final newline.
func DefaultYaml3Options() Yaml3Options {
	return Yaml3Options{
		Indent:             0,
		NormalizeStrings:   false,
		InsertFinalNewline: true,
	}
}

// Yaml3 reformats src by roundtripping it through gopkg.in/yaml.v3, preserving
// comments via the yaml.Node tree. Folded scalar bodies (`>` blocks) are
// re-spliced from the original source after re-encoding so that yaml.v3's
// re-wrap doesn't reflow them; everything else is the encoder's output.
//
// On any decode error the function returns the error and the caller is
// expected to fall back to the conservative formatter so the user is not
// stuck without any formatting on a transiently invalid buffer.
func Yaml3(src string, opts Yaml3Options) (string, error) {
	if strings.TrimSpace(src) == "" {
		return src, nil
	}

	srcFolded := scanFoldedBlocks(src)

	dec := yaml.NewDecoder(strings.NewReader(src))
	var docs []*yaml.Node
	for {
		var n yaml.Node
		if err := dec.Decode(&n); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", err
		}
		docs = append(docs, &n)
	}
	if len(docs) == 0 {
		return src, nil
	}

	if opts.NormalizeStrings {
		for _, d := range docs {
			normalizeStringStyles(d)
		}
	}

	indent := opts.Indent
	if indent <= 0 {
		indent = detectIndent(src)
	}

	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(indent)
	for _, d := range docs {
		if err := enc.Encode(d); err != nil {
			_ = enc.Close()
			return "", err
		}
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	out := buf.String()

	if len(srcFolded) > 0 {
		outFolded := scanFoldedBlocks(out)
		if len(outFolded) == len(srcFolded) {
			out = spliceFolded(out, outFolded, srcFolded)
		}
	}

	if !opts.InsertFinalNewline {
		out = strings.TrimRight(out, "\n")
	}
	return out, nil
}

// foldedBlock describes one `>` block scalar in some YAML text: its header
// line, the line range of its body, and the indent of the body's first
// non-blank line. Used to pair source and output blocks for splice-back.
type foldedBlock struct {
	headerLine  int      // 0-based line index of the `>`-bearing line
	bodyStart   int      // 0-based inclusive
	bodyEnd     int      // 0-based exclusive
	headerCol   int      // leading-whitespace count on the header line
	bodyIndent  int      // leading-whitespace count of first non-blank body line
	bodyLines   []string // raw body lines (verbatim from input), in order
}

// foldedHeaderRE matches lines that look like a block-mapping value whose
// scalar is a folded block: `key: >`, `key: >-`, `key: >+2  # comment`, etc.
// Anchored at both ends with optional inline comment. Lines starting with `#`
// or `-` (sequence indicator) are not matched; sequence-item folded scalars
// (`- >`) are intentionally out of scope for v1 — they're vanishingly rare in
// the Helm/Kustomize/Ansible corpus we target.
var foldedHeaderRE = regexp.MustCompile(`^(\s*)[^#\s-][^:#\n]*:[^#\n]*>[-+]?\d*\s*(#.*)?$`)

// scanFoldedBlocks returns one foldedBlock per `>` block scalar found in text,
// in document order. Block bodies extend from the line after the header to the
// first non-blank line whose indent is at or below the header indent (standard
// YAML block-scalar termination).
func scanFoldedBlocks(text string) []foldedBlock {
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		// Drop the empty element strings.Split produces from a trailing
		// newline so we don't index past the document end.
		lines = lines[:len(lines)-1]
	}
	var out []foldedBlock
	for i := 0; i < len(lines); i++ {
		m := foldedHeaderRE.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		headerCol := len(m[1])
		// Walk body lines until we hit a non-blank line at or above the
		// header's indent (block terminates) or EOF.
		j := i + 1
		bodyIndent := -1
		for j < len(lines) {
			line := lines[j]
			if strings.TrimSpace(line) == "" {
				j++
				continue
			}
			lead := leadingSpaces(line)
			if lead <= headerCol {
				break
			}
			if bodyIndent < 0 {
				bodyIndent = lead
			}
			j++
		}
		// Strip any pure-blank lines at the tail back into "not part of
		// this block" so consecutive blocks stay paired correctly. The
		// block "ends" at the last non-blank body line; trailing blanks
		// remain in the surrounding document.
		end := j
		for end > i+1 && strings.TrimSpace(lines[end-1]) == "" {
			end--
		}
		body := append([]string(nil), lines[i+1:end]...)
		out = append(out, foldedBlock{
			headerLine: i,
			bodyStart:  i + 1,
			bodyEnd:    end,
			headerCol:  headerCol,
			bodyIndent: bodyIndent,
			bodyLines:  body,
		})
		i = end - 1
	}
	return out
}

// spliceFolded rewrites out by replacing the body of each folded block in
// outBlocks with the corresponding source body in srcBlocks, re-indented to
// match outBlocks' indent. Caller must ensure len(outBlocks)==len(srcBlocks).
func spliceFolded(out string, outBlocks, srcBlocks []foldedBlock) string {
	lines := strings.Split(out, "\n")
	hadTrailingNL := false
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		hadTrailingNL = true
		lines = lines[:len(lines)-1]
	}
	// Apply splices back-to-front so earlier indices stay valid.
	for k := len(outBlocks) - 1; k >= 0; k-- {
		ob := outBlocks[k]
		sb := srcBlocks[k]
		newBody := reindentBody(sb.bodyLines, sb.bodyIndent, ob.bodyIndent)
		// Replace lines[ob.bodyStart:ob.bodyEnd] with newBody.
		before := lines[:ob.bodyStart]
		after := lines[ob.bodyEnd:]
		next := make([]string, 0, len(before)+len(newBody)+len(after))
		next = append(next, before...)
		next = append(next, newBody...)
		next = append(next, after...)
		lines = next
	}
	res := strings.Join(lines, "\n")
	if hadTrailingNL {
		res += "\n"
	}
	return res
}

// reindentBody shifts each body line from srcIndent leading spaces to
// dstIndent leading spaces. Blank lines are emitted as-is (no indent applied
// to a line that has no content). Lines whose actual indent is less than
// srcIndent are passed through verbatim — defensive for malformed input.
func reindentBody(body []string, srcIndent, dstIndent int) []string {
	if srcIndent < 0 {
		srcIndent = 0
	}
	if dstIndent < 0 {
		dstIndent = 0
	}
	pad := strings.Repeat(" ", dstIndent)
	out := make([]string, len(body))
	for i, line := range body {
		if strings.TrimSpace(line) == "" {
			out[i] = ""
			continue
		}
		lead := leadingSpaces(line)
		if lead < srcIndent {
			out[i] = line
			continue
		}
		out[i] = pad + line[srcIndent:]
	}
	return out
}

// detectIndent infers the dominant indentation width by scanning for the first
// non-trivial nesting (a mapping key followed by a more-indented line, or a
// `- ` sequence item whose content is indented). Falls back to 2 — the
// yamllint default and the overwhelmingly common choice for Kubernetes/Helm
// YAML, which is the project's target corpus.
func detectIndent(src string) int {
	lines := strings.Split(src, "\n")
	prevIndent := -1
	prevIsKey := false
	for _, raw := range lines {
		trimmed := strings.TrimLeft(raw, " ")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "---") || strings.HasPrefix(trimmed, "...") {
			continue
		}
		indent := len(raw) - len(trimmed)
		if prevIsKey && indent > prevIndent {
			delta := indent - prevIndent
			if delta > 0 {
				return delta
			}
		}
		prevIsKey = isKeyOnlyLine(trimmed)
		prevIndent = indent
	}
	return 2
}

// isKeyOnlyLine reports whether trimmed (no leading whitespace) looks like a
// mapping key with no inline value — i.e. its content is followed by `:`
// optionally followed by whitespace and an inline comment. Lines like
// "foo: bar" return false because the value sits on the same line.
func isKeyOnlyLine(trimmed string) bool {
	colon := indexOfTopLevelColon(trimmed)
	if colon < 0 {
		return false
	}
	rest := strings.TrimLeft(trimmed[colon+1:], " ")
	return rest == "" || strings.HasPrefix(rest, "#")
}

// indexOfTopLevelColon returns the index of the first `:` in s that is not
// inside single or double quotes. Returns -1 when none is found. Sufficient
// for the line-level heuristics here; it does not handle escaped quotes
// because YAML's flow scalars don't escape quotes the way C-strings do.
func indexOfTopLevelColon(s string) int {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote == 0 && (c == '\'' || c == '"'):
			quote = c
		case quote != 0 && c == quote:
			quote = 0
		case quote == 0 && c == ':':
			return i
		}
	}
	return -1
}

// leadingSpaces returns the count of leading space characters on s. Tabs are
// not counted as indentation because they're disallowed for indentation in
// YAML 1.2 (and yamllint rejects them).
func leadingSpaces(s string) int {
	n := 0
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}

// normalizeStringStyles walks n and clears the quote style on every string
// scalar whose value can be safely emitted as a YAML plain scalar. Scalars
// whose plain rendering would resolve to a non-string type (number, bool,
// null, timestamp) keep their quotes so semantics are preserved.
func normalizeStringStyles(n *yaml.Node) {
	if n == nil {
		return
	}
	if n.Kind == yaml.ScalarNode &&
		(n.Tag == "!!str" || n.Tag == "") &&
		n.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle) != 0 &&
		canBePlainString(n.Value) {
		n.Style = 0
	}
	for _, c := range n.Content {
		normalizeStringStyles(c)
	}
	if n.Alias != nil {
		normalizeStringStyles(n.Alias)
	}
}

// canBePlainString reports whether s can be emitted as a YAML 1.2 plain
// scalar that still resolves back to a string. The check is intentionally
// conservative: it rejects anything that could be ambiguous (numbers,
// booleans, null literals, special leading characters, embedded `: ` or
// ` #` that would change parsing). Strings rejected here keep their quotes,
// which is always safe.
func canBePlainString(s string) bool {
	if s == "" {
		return false
	}
	if strings.ContainsAny(s, "\n\r\t") {
		return false
	}
	// Leading-character restrictions per YAML 1.2 spec for plain scalars.
	switch s[0] {
	case ' ', '\t', '[', ']', '{', '}', ',', '#', '&', '*', '!', '|', '>',
		'\'', '"', '%', '@', '`':
		return false
	case '-', '?', ':':
		// Allowed only when not followed by space — but we conservatively
		// reject all such leads to avoid sequence/mapping ambiguity.
		return false
	}
	if s[len(s)-1] == ' ' || s[len(s)-1] == '\t' {
		return false
	}
	// `: ` mid-string opens an implicit nested mapping when re-parsed.
	if strings.Contains(s, ": ") || strings.HasSuffix(s, ":") {
		return false
	}
	// ` #` mid-string opens a comment when re-parsed.
	if strings.Contains(s, " #") {
		return false
	}
	// Reject reserved tokens that resolve to non-string types under the
	// YAML 1.2 Core schema.
	switch strings.ToLower(s) {
	case "true", "false", "yes", "no", "on", "off", "null", "~":
		return false
	}
	// Reject numeric-looking values; they'd resolve to !!int / !!float.
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return false
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return false
	}
	return true
}
