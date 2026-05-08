package server

import (
	"strings"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestFormattingTrimsAndAddsFinalNewline(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///fmt.yaml"
	srv.documents.Set(uri, "foo: bar   \nbaz", 1)

	edits, err := srv.formatting(nil, &protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 {
		t.Fatalf("edits=%d want 1", len(edits))
	}
	if edits[0].NewText != "foo: bar\nbaz\n" {
		t.Errorf("NewText=%q want %q", edits[0].NewText, "foo: bar\nbaz\n")
	}
	// The range should cover the whole document: start at (0,0), end at
	// the position past the last char on the final line ("baz" → col 3).
	if edits[0].Range.Start.Line != 0 || edits[0].Range.Start.Character != 0 {
		t.Errorf("Start=%+v want {0,0}", edits[0].Range.Start)
	}
	if edits[0].Range.End.Line != 1 || edits[0].Range.End.Character != 3 {
		t.Errorf("End=%+v want {1,3}", edits[0].Range.End)
	}
}

func TestFormattingNoEditWhenClean(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///clean.yaml"
	srv.documents.Set(uri, "foo: bar\n", 1)

	edits, err := srv.formatting(nil, &protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatal(err)
	}
	if edits != nil {
		t.Errorf("edits=%v want nil (already clean)", edits)
	}
}

func TestFormattingPreservesComments(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///commented.yaml"
	src := "# top comment\nkey: value   \n# trailing comment\n"
	srv.documents.Set(uri, src, 1)

	edits, err := srv.formatting(nil, &protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 {
		t.Fatalf("edits=%d want 1", len(edits))
	}
	want := "# top comment\nkey: value\n# trailing comment\n"
	if edits[0].NewText != want {
		t.Errorf("NewText=%q want %q", edits[0].NewText, want)
	}
}

func TestFormattingForcedIndentationFromConfig(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	srv.config.Format.Indentation = 4
	const uri = "file:///indent.yaml"
	srv.documents.Set(uri, "outer:\n  inner:\n    leaf: 1\n", 1)

	edits, err := srv.formatting(nil, &protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 {
		t.Fatalf("edits=%d want 1", len(edits))
	}
	if !strings.Contains(edits[0].NewText, "    inner:") {
		t.Errorf("expected 4-space indent, got: %q", edits[0].NewText)
	}
}

func TestFormattingNormalizeStringsFromConfig(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	srv.config.Format.NormalizeStrings = true
	const uri = "file:///quotes.yaml"
	srv.documents.Set(uri, `key: "hello"`+"\n", 1)

	edits, err := srv.formatting(nil, &protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 {
		t.Fatalf("edits=%d want 1", len(edits))
	}
	if strings.Contains(edits[0].NewText, `"hello"`) {
		t.Errorf("expected quotes stripped, got: %q", edits[0].NewText)
	}
}

func TestFormattingFallsBackOnInvalidYaml(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///broken.yaml"
	// yaml.v3 cannot parse this; the conservative fallback should still
	// trim trailing whitespace and ensure a final newline.
	srv.documents.Set(uri, "foo: bar   \nbaz", 1)

	edits, err := srv.formatting(nil, &protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 {
		t.Fatalf("edits=%d want 1", len(edits))
	}
	if edits[0].NewText != "foo: bar\nbaz\n" {
		t.Errorf("NewText=%q want %q", edits[0].NewText, "foo: bar\nbaz\n")
	}
}

func TestFormattingUnknownDocument(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	edits, err := srv.formatting(nil, &protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///nope.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if edits != nil {
		t.Errorf("edits=%v want nil", edits)
	}
}

func TestRangeFormattingTrimsOnlyLinesInRange(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///rangefmt.yaml"
	// Lines (1-based): 1=`foo: bar   `, 2=`baz: qux   `, 3=`zap: zog   \n`
	srv.documents.Set(uri, "foo: bar   \nbaz: qux   \nzap: zog   \n", 1)

	// Range covers line 2 only (LSP lines 1..2 exclusive).
	edits, err := srv.rangeFormatting(nil, &protocol.DocumentRangeFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Range: protocol.Range{
			Start: protocol.Position{Line: 1, Character: 0},
			End:   protocol.Position{Line: 2, Character: 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 {
		t.Fatalf("edits=%d want 1", len(edits))
	}
	// We expect just the snapped slice (line 2 incl. its newline) to be
	// rewritten, leaving lines 1 and 3 untouched (still trailing-spaced).
	if edits[0].NewText != "baz: qux\n" {
		t.Errorf("NewText=%q want %q", edits[0].NewText, "baz: qux\n")
	}
	if edits[0].Range.Start.Line != 1 || edits[0].Range.Start.Character != 0 {
		t.Errorf("Start=%+v want {1,0}", edits[0].Range.Start)
	}
	if edits[0].Range.End.Line != 2 || edits[0].Range.End.Character != 0 {
		t.Errorf("End=%+v want {2,0}", edits[0].Range.End)
	}
}

func TestRangeFormattingMidLineSnapsToLines(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///rangefmt.yaml"
	srv.documents.Set(uri, "foo: bar   \nbaz: qux   \n", 1)

	// A range that starts mid-line on 1 and ends mid-line on 2 should
	// expand to whole-line coverage: lines 1..2 both rewritten.
	edits, err := srv.rangeFormatting(nil, &protocol.DocumentRangeFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 4},
			End:   protocol.Position{Line: 1, Character: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 {
		t.Fatalf("edits=%d want 1", len(edits))
	}
	if edits[0].NewText != "foo: bar\nbaz: qux\n" {
		t.Errorf("NewText=%q want %q", edits[0].NewText, "foo: bar\nbaz: qux\n")
	}
}

func TestRangeFormattingDoesNotAddTrailingNewlineMidDocument(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///rangefmt.yaml"
	// Three mapping entries (distinct keys, not a multi-line plain scalar).
	// Format only the middle line; the range does not cover EOF.
	srv.documents.Set(uri, "a: 1   \nb: 2   \nc: 3\n", 1)

	edits, err := srv.rangeFormatting(nil, &protocol.DocumentRangeFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Range: protocol.Range{
			Start: protocol.Position{Line: 1, Character: 0},
			End:   protocol.Position{Line: 2, Character: 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 {
		t.Fatalf("edits=%d want 1", len(edits))
	}
	// Line 2 changes (trailing space stripped); lines 1 and 3 also
	// changed but are outside the range so they are filtered out.
	if edits[0].NewText != "b: 2\n" {
		t.Errorf("NewText=%q want %q", edits[0].NewText, "b: 2\n")
	}
}
