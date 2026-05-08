package server

import (
	"errors"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestPrepareRenameReturnsNameRange(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///rename.yaml"
	srv.documents.Set(uri, referencesDoc, 1)

	res, err := srv.prepareRename(nil, &protocol.PrepareRenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 10}, // on `&` of `&d`
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rwp, ok := res.(protocol.RangeWithPlaceholder)
	if !ok {
		t.Fatalf("res=%T want RangeWithPlaceholder", res)
	}
	if rwp.Placeholder != "d" {
		t.Errorf("Placeholder=%q want %q", rwp.Placeholder, "d")
	}
	// Name spans just the `d` at LSP (0, 11) → (0, 12). Sigil at col 10 excluded.
	if rwp.Range.Start.Line != 0 || rwp.Range.Start.Character != 11 {
		t.Errorf("Start=%+v want {0,11}", rwp.Range.Start)
	}
	if rwp.Range.End.Line != 0 || rwp.Range.End.Character != 12 {
		t.Errorf("End=%+v want {0,12}", rwp.Range.End)
	}
}

func TestPrepareRenameNilOffSymbol(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///rename.yaml"
	srv.documents.Set(uri, "foo: bar\n", 1)

	res, err := srv.prepareRename(nil, &protocol.PrepareRenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 6},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res != nil {
		t.Errorf("res=%v want nil", res)
	}
}

func TestRenameEditsAllOccurrencesNameOnly(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///rename.yaml"
	srv.documents.Set(uri, referencesDoc, 1)

	we, err := srv.rename(nil, &protocol.RenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 3, Character: 6}, // on `*` of `*d` line 4
		},
		NewName: "defaults_v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if we == nil {
		t.Fatal("WorkspaceEdit=nil")
	}
	edits := we.Changes[uri]
	if len(edits) != 3 {
		t.Fatalf("edits=%d want 3 (anchor + 2 aliases) got %+v", len(edits), edits)
	}
	for i, e := range edits {
		if e.NewText != "defaults_v2" {
			t.Errorf("edits[%d].NewText=%q want %q", i, e.NewText, "defaults_v2")
		}
	}
	// Anchor edit: name `d` at LSP (0, 11) → (0, 12). Sigil at col 10 preserved.
	if edits[0].Range.Start.Character != 11 || edits[0].Range.End.Character != 12 {
		t.Errorf("anchor edit range=%+v want chars 11..12", edits[0].Range)
	}
}

func TestRenameSymmetricFromAnchor(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///rename.yaml"
	srv.documents.Set(uri, referencesDoc, 1)

	we, err := srv.rename(nil, &protocol.RenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 10}, // on `&d` itself
		},
		NewName: "d2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if we == nil || len(we.Changes[uri]) != 3 {
		t.Fatalf("expected 3 edits regardless of cursor side, got %+v", we)
	}
}

func TestRenameRejectsInvalidNames(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///rename.yaml"
	srv.documents.Set(uri, referencesDoc, 1)

	cases := []struct{ name string }{
		{""},
		{"has space"},
		{"comma,inside"},
		{"flow[bracket"},
		{"flow]bracket"},
		{"flow{brace"},
		{"flow}brace"},
		{"&sigil"},
		{"*sigil"},
		{"!sigil"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := srv.rename(nil, &protocol.RenameParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: uri},
					Position:     protocol.Position{Line: 0, Character: 10},
				},
				NewName: c.name,
			})
			var bad *ErrInvalidAnchorName
			if !errors.As(err, &bad) {
				t.Errorf("err=%v (%T) want *ErrInvalidAnchorName", err, err)
			}
		})
	}
}

func TestRenameNilWhenCursorOffSymbol(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///rename.yaml"
	srv.documents.Set(uri, "foo: bar\n", 1)

	we, err := srv.rename(nil, &protocol.RenameParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 6},
		},
		NewName: "renamed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if we != nil {
		t.Errorf("we=%v want nil", we)
	}
}
