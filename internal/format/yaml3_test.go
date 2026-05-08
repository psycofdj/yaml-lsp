package format

import (
	"strings"
	"testing"
)

func TestYaml3PreservesComments(t *testing.T) {
	src := "# top comment\nkey: value # inline\n# trailing\n"
	got, err := Yaml3(src, DefaultYaml3Options())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "# top comment") {
		t.Errorf("top comment lost: %q", got)
	}
	if !strings.Contains(got, "# inline") {
		t.Errorf("inline comment lost: %q", got)
	}
}

func TestYaml3DetectIndentTwoSpaces(t *testing.T) {
	src := "outer:\n  inner:\n    leaf: 1\n"
	got, err := Yaml3(src, DefaultYaml3Options())
	if err != nil {
		t.Fatal(err)
	}
	// Should preserve 2-space indent on roundtrip.
	if !strings.Contains(got, "  inner:") {
		t.Errorf("expected 2-space indent, got: %q", got)
	}
	if !strings.Contains(got, "    leaf:") {
		t.Errorf("expected 4-space indent at depth 2, got: %q", got)
	}
}

func TestYaml3DetectIndentFourSpaces(t *testing.T) {
	src := "outer:\n    inner:\n        leaf: 1\n"
	got, err := Yaml3(src, DefaultYaml3Options())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "    inner:") {
		t.Errorf("expected 4-space indent, got: %q", got)
	}
}

func TestYaml3ForcedIndent(t *testing.T) {
	src := "outer:\n    inner:\n        leaf: 1\n" // 4-space source
	opts := DefaultYaml3Options()
	opts.Indent = 2
	got, err := Yaml3(src, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "  inner:") {
		t.Errorf("expected forced 2-space indent, got: %q", got)
	}
	if strings.Contains(got, "    inner:") {
		t.Errorf("4-space indent leaked through: %q", got)
	}
}

func TestYaml3NormalizeStringsStripsUnneededQuotes(t *testing.T) {
	src := `key: "hello"
other: "world"
`
	opts := DefaultYaml3Options()
	opts.NormalizeStrings = true
	got, err := Yaml3(src, opts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, `"hello"`) {
		t.Errorf("quotes not stripped: %q", got)
	}
	if !strings.Contains(got, "key: hello") {
		t.Errorf("expected plain scalar, got: %q", got)
	}
}

func TestYaml3NormalizeStringsKeepsAmbiguousQuotes(t *testing.T) {
	src := `version: "1.0"
flag: "yes"
num: "42"
`
	opts := DefaultYaml3Options()
	opts.NormalizeStrings = true
	got, err := Yaml3(src, opts)
	if err != nil {
		t.Fatal(err)
	}
	// These would resolve to non-string types if unquoted — quotes stay.
	if !strings.Contains(got, `"1.0"`) {
		t.Errorf("\"1.0\" quotes stripped (would become a float): %q", got)
	}
	if !strings.Contains(got, `"yes"`) {
		t.Errorf("\"yes\" quotes stripped (would become a bool): %q", got)
	}
	if !strings.Contains(got, `"42"`) {
		t.Errorf("\"42\" quotes stripped (would become an int): %q", got)
	}
}

func TestYaml3NormalizeStringsOffPreservesQuotes(t *testing.T) {
	src := `key: "hello"
`
	got, err := Yaml3(src, DefaultYaml3Options()) // NormalizeStrings=false
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"hello"`) {
		t.Errorf("quotes stripped when NormalizeStrings=false: %q", got)
	}
}

func TestYaml3SplicesBackFoldedScalars(t *testing.T) {
	src := `description: >
  this is line one
  this is line two
  this is line three
other: value
`
	got, err := Yaml3(src, DefaultYaml3Options())
	if err != nil {
		t.Fatal(err)
	}
	// Splice-back restores the source's line breaks; without it,
	// yaml.v3 would emit "this is line one this is line two..." as one line.
	if !strings.Contains(got, "  this is line one\n") {
		t.Errorf("folded body line 1 lost or reflowed: %q", got)
	}
	if !strings.Contains(got, "  this is line two\n") {
		t.Errorf("folded body line 2 lost or reflowed: %q", got)
	}
	if !strings.Contains(got, "  this is line three") {
		t.Errorf("folded body line 3 lost or reflowed: %q", got)
	}
}

func TestYaml3SpliceReindentsFoldedBodyOnIndentChange(t *testing.T) {
	src := "outer:\n    description: >\n        body line one\n        body line two\n    next: 1\n"
	opts := DefaultYaml3Options()
	opts.Indent = 2 // force 4-space source down to 2-space output
	got, err := Yaml3(src, opts)
	if err != nil {
		t.Fatal(err)
	}
	// Output should have 2-space outer indent and 4-space body indent.
	if !strings.Contains(got, "  description: >") {
		t.Errorf("header indent not normalized: %q", got)
	}
	if !strings.Contains(got, "    body line one") {
		t.Errorf("folded body not re-indented: %q", got)
	}
	if strings.Contains(got, "        body line one") {
		t.Errorf("original 8-space body indent leaked through: %q", got)
	}
}

func TestYaml3PreservesLiteralScalars(t *testing.T) {
	src := `script: |
  echo hello
  echo world
`
	got, err := Yaml3(src, DefaultYaml3Options())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "echo hello\n") {
		t.Errorf("literal body line 1 lost: %q", got)
	}
	if !strings.Contains(got, "echo world") {
		t.Errorf("literal body line 2 lost: %q", got)
	}
}

func TestYaml3MultiDocument(t *testing.T) {
	src := "---\nfirst: 1\n---\nsecond: 2\n"
	got, err := Yaml3(src, DefaultYaml3Options())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "first: 1") || !strings.Contains(got, "second: 2") {
		t.Errorf("multi-doc content lost: %q", got)
	}
	if !strings.Contains(got, "---") {
		t.Errorf("document separator lost: %q", got)
	}
}

func TestYaml3EmptyDocument(t *testing.T) {
	got, err := Yaml3("", DefaultYaml3Options())
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("empty input changed: %q", got)
	}
}

func TestYaml3WhitespaceOnlyDocument(t *testing.T) {
	got, err := Yaml3("   \n\n  \n", DefaultYaml3Options())
	if err != nil {
		t.Fatal(err)
	}
	if got != "   \n\n  \n" {
		t.Errorf("whitespace-only input changed: %q", got)
	}
}

func TestYaml3InvalidYamlReturnsError(t *testing.T) {
	_, err := Yaml3("key: [unclosed", DefaultYaml3Options())
	if err == nil {
		t.Errorf("expected error on invalid YAML")
	}
}

func TestYaml3InsertFinalNewlineFalseStripsTrailingNewline(t *testing.T) {
	opts := DefaultYaml3Options()
	opts.InsertFinalNewline = false
	got, err := Yaml3("key: value\n", opts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(got, "\n") {
		t.Errorf("trailing newline not stripped: %q", got)
	}
}

func TestYaml3InsertFinalNewlineTrueAddsTrailingNewline(t *testing.T) {
	got, err := Yaml3("key: value", DefaultYaml3Options())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("trailing newline missing: %q", got)
	}
}

func TestDetectIndent(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{"two-space mapping", "a:\n  b: 1\n", 2},
		{"four-space mapping", "a:\n    b: 1\n", 4},
		{"three-space mapping", "a:\n   b: 1\n", 3},
		{"flat", "a: 1\nb: 2\n", 2}, // no nesting → fallback
		{"with comments", "# top\na:\n  b: 1\n", 2},
		{"with doc separator", "---\na:\n  b: 1\n", 2},
		{"empty", "", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectIndent(tt.src)
			if got != tt.want {
				t.Errorf("detectIndent(%q) = %d, want %d", tt.src, got, tt.want)
			}
		})
	}
}

func TestScanFoldedBlocks(t *testing.T) {
	src := `top: 1
folded: >
  body one
  body two
after: 2
`
	blocks := scanFoldedBlocks(src)
	if len(blocks) != 1 {
		t.Fatalf("blocks=%d want 1", len(blocks))
	}
	b := blocks[0]
	if b.headerLine != 1 {
		t.Errorf("headerLine=%d want 1", b.headerLine)
	}
	if b.bodyStart != 2 || b.bodyEnd != 4 {
		t.Errorf("body=[%d,%d) want [2,4)", b.bodyStart, b.bodyEnd)
	}
	if b.bodyIndent != 2 {
		t.Errorf("bodyIndent=%d want 2", b.bodyIndent)
	}
}

func TestScanFoldedBlocksSkipsLiterals(t *testing.T) {
	src := "literal: |\n  body\nfolded: >\n  body\n"
	blocks := scanFoldedBlocks(src)
	if len(blocks) != 1 {
		t.Fatalf("blocks=%d want 1 (literal should be skipped)", len(blocks))
	}
	if blocks[0].headerLine != 2 {
		t.Errorf("headerLine=%d want 2", blocks[0].headerLine)
	}
}

func TestCanBePlainString(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"hello", true},
		{"hello world", true},
		{"", false},
		{"true", false},
		{"False", false},
		{"yes", false},
		{"null", false},
		{"~", false},
		{"42", false},
		{"3.14", false},
		{"1.0", false},
		{"-1", false},
		{"[bracket", false},
		{"{brace", false},
		{"#hash", false},
		{"&anchor", false},
		{"*alias", false},
		{"!tag", false},
		{"|pipe", false},
		{">gt", false},
		{"'quote", false},
		{"\"dquote", false},
		{"key: value", false},  // embedded `: `
		{"trailing:", false},   // trailing colon
		{"with #hash", false},  // embedded ` #`
		{"line\nbreak", false}, // newline
		{"trail ", false},      // trailing space
		{" lead", false},       // leading space
		{"abc/def", true},
		{"http://x", true}, // colon followed by `/`, not `: `, so safe as plain
	}
	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := canBePlainString(tt.s)
			if got != tt.want {
				t.Errorf("canBePlainString(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}
