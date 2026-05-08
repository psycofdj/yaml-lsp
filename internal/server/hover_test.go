package server

import (
	"strings"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestHoverRendersAllFourFormats(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///x.yaml"
	srv.documents.Set(uri, sequencesDoc, 1)

	// Cursor on `nginx` at line 3 col 13 (1-based) -> LSP (2, 12).
	res, err := srv.hover(nil, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 2, Character: 12},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("expected non-nil hover for cursor on a node")
	}
	mc, ok := res.Contents.(protocol.MarkupContent)
	if !ok {
		t.Fatalf("Contents is %T, want MarkupContent", res.Contents)
	}
	if mc.Kind != protocol.MarkupKindMarkdown {
		t.Errorf("MarkupKind: got %q, want markdown", mc.Kind)
	}
	for _, want := range []string{
		"$.containers[0].image",
		"/containers/name=web/image",
		"/containers/0/image",
		"containers[0].image",
	} {
		if !strings.Contains(mc.Value, want) {
			t.Errorf("hover content missing %q\nfull content:\n%s", want, mc.Value)
		}
	}
}

func TestHoverNilOnNonNode(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///x.yaml"
	srv.documents.Set(uri, singleDoc, 1)

	// Past EOF: locate returns NodeKindNone via the EndLine bound.
	res, err := srv.hover(nil, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 50, Character: 0},
		},
	})
	if err == nil && res != nil {
		t.Errorf("expected nil hover for past-EOF cursor, got %+v", res)
	}
}

func TestHoverNilOnUnsyncedDocument(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	res, err := srv.hover(nil, &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///nope.yaml"},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res != nil {
		t.Errorf("expected nil hover for unsynced doc, got %+v", res)
	}
}

func TestInitializeAdvertisesHoverProvider(t *testing.T) {
	srv := New()
	res, err := srv.initialize(nil, &protocol.InitializeParams{})
	if err != nil {
		t.Fatal(err)
	}
	caps := res.(protocol.InitializeResult).Capabilities
	if got, ok := caps.HoverProvider.(bool); !ok || !got {
		t.Errorf("HoverProvider: got %v, want true", caps.HoverProvider)
	}
}
