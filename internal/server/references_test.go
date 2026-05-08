package server

import (
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

const referencesDoc = "defaults: &d\n" + // line 1: &d at cols 11-12
	"  timeout: 30\n" +
	"server:\n" +
	"  <<: *d\n" + //                    line 4: *d at cols 7-8
	"backup:\n" +
	"  ref: *d\n" //                     line 6: *d at cols 8-9

func TestReferencesFromAliasReturnsAllAliases(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///refs.yaml"
	srv.documents.Set(uri, referencesDoc, 1)

	res, err := srv.references(nil, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 3, Character: 6}, // line 4 col 7 → `*`
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("len=%d want 2 (got %+v)", len(res), res)
	}
	// First alias: LSP line 3, chars 6..8 (cols 7-8 inclusive → 6..8 exclusive).
	if res[0].Range.Start.Line != 3 || res[0].Range.Start.Character != 6 {
		t.Errorf("res[0].Start=%+v want {3,6}", res[0].Range.Start)
	}
	// Second alias: LSP line 5, chars 7..9 (col 8 inclusive → 7 in LSP).
	if res[1].Range.Start.Line != 5 || res[1].Range.Start.Character != 7 {
		t.Errorf("res[1].Start=%+v want {5,7}", res[1].Range.Start)
	}
}

func TestReferencesIncludeDeclarationAddsAnchor(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///refs.yaml"
	srv.documents.Set(uri, referencesDoc, 1)

	res, err := srv.references(nil, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 3, Character: 6},
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("len=%d want 3 (got %+v)", len(res), res)
	}
	// First entry should be the anchor declaration at line 1.
	if res[0].Range.Start.Line != 0 || res[0].Range.Start.Character != 10 {
		t.Errorf("res[0].Start=%+v want {0,10} (anchor)", res[0].Range.Start)
	}
}

func TestReferencesFromAnchorIsSymmetric(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///refs.yaml"
	srv.documents.Set(uri, referencesDoc, 1)

	// Cursor on `&d` (line 1 col 11 → LSP 0/10).
	res, err := srv.references(nil, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 10},
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("len=%d want 3 (got %+v)", len(res), res)
	}
}

func TestReferencesNilForPlainScalar(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///refs.yaml"
	srv.documents.Set(uri, "foo: bar\n", 1)

	res, err := srv.references(nil, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 6}, // on `bar`
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res != nil {
		t.Errorf("res=%v want nil", res)
	}
}

func TestReferencesAcrossDocumentsAreScoped(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///refs.yaml"
	// Two documents; only doc 1 has the symbol. Cursor in doc 1
	// should only find doc-1 occurrences.
	srv.documents.Set(uri, "first: &d 1\nref: *d\n---\nsecond: *d\n", 1)

	res, err := srv.references(nil, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 7}, // `&d` in doc 1
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("len=%d want 2 (anchor + first alias only) got %+v", len(res), res)
	}
}
