package server

import (
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

const anchorsDoc = "defaults: &d\n" +
	"  timeout: 30\n" +
	"server:\n" +
	"  <<: *d\n"

func TestDefinitionResolvesAliasToAnchor(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///anchors.yaml"
	srv.documents.Set(uri, anchorsDoc, 1)

	// Line 4 (LSP line 3), column 7 (LSP char 6) — the `*` in `*d`.
	res, err := srv.definition(nil, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 3, Character: 6},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	loc, ok := res.(protocol.Location)
	if !ok {
		t.Fatalf("res=%T want protocol.Location", res)
	}
	if loc.URI != uri {
		t.Errorf("URI=%q want %q", loc.URI, uri)
	}
	// Anchor `&d` is on line 1, columns 11-12 → LSP line 0, chars 10-12 (end exclusive).
	if loc.Range.Start.Line != 0 || loc.Range.Start.Character != 10 {
		t.Errorf("Start=%+v want {0,10}", loc.Range.Start)
	}
	if loc.Range.End.Line != 0 || loc.Range.End.Character != 12 {
		t.Errorf("End=%+v want {0,12}", loc.Range.End)
	}
}

func TestDefinitionFromAnchorReturnsItself(t *testing.T) {
	// Symmetric definition lookup: cursor on `&anchor` returns the
	// anchor's own location, matching gopls/clangd behavior for
	// identifiers, so M-. is useful from either side of the relation.
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///anchors.yaml"
	srv.documents.Set(uri, anchorsDoc, 1)

	// Cursor on `&` of `&d` (line 1 col 11 → LSP 0/10).
	res, err := srv.definition(nil, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 10},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	loc, ok := res.(protocol.Location)
	if !ok {
		t.Fatalf("res=%T want protocol.Location", res)
	}
	if loc.Range.Start.Line != 0 || loc.Range.Start.Character != 10 {
		t.Errorf("Start=%+v want {0,10}", loc.Range.Start)
	}
	if loc.Range.End.Line != 0 || loc.Range.End.Character != 12 {
		t.Errorf("End=%+v want {0,12}", loc.Range.End)
	}
}

func TestDefinitionNilForPlainScalar(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///plain.yaml"
	srv.documents.Set(uri, "foo: bar\n", 1)

	res, err := srv.definition(nil, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 6}, // on `bar`
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res != nil {
		t.Errorf("res=%v want nil (not on an alias)", res)
	}
}

func TestDefinitionNilForUnknownDocument(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	res, err := srv.definition(nil, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///nope.yaml"},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res != nil {
		t.Errorf("res=%v want nil", res)
	}
}

func TestDefinitionNilWhenAnchorMissing(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///missing.yaml"
	srv.documents.Set(uri, "server:\n  ref: *missing\n", 1)

	res, err := srv.definition(nil, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 1, Character: 9}, // on `*`
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res != nil {
		t.Errorf("res=%v want nil (no matching anchor)", res)
	}
}
