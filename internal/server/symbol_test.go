package server

import (
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestDocumentSymbolFlatJSONPathSingleDoc(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///x.yaml"
	srv.documents.Set(uri, singleDoc, 1)

	res, err := srv.documentSymbol(nil, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatal(err)
	}
	syms, ok := res.([]protocol.DocumentSymbol)
	if !ok {
		t.Fatalf("got %T, want []DocumentSymbol", res)
	}
	want := []string{
		"$.apiVersion",
		"$.kind",
		"$.metadata",
		"$.metadata.name",
		"$.metadata.namespace",
		"$.data",
		"$.data.greeting",
	}
	if got := namesOf(syms); !equalStrings(got, want) {
		t.Fatalf("names:\n got:  %v\n want: %v", got, want)
	}
	// All entries are flat — no Children populated.
	for _, s := range syms {
		if len(s.Children) != 0 {
			t.Errorf("%q has %d children; flat list should have none", s.Name, len(s.Children))
		}
	}
}

func TestDocumentSymbolKindReflectsValueType(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///x.yaml"
	srv.documents.Set(uri, singleDoc, 1)

	res, _ := srv.documentSymbol(nil, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	syms := res.([]protocol.DocumentSymbol)
	want := map[string]protocol.SymbolKind{
		"$.apiVersion":         protocol.SymbolKindString,
		"$.kind":               protocol.SymbolKindString,
		"$.metadata":           protocol.SymbolKindObject,
		"$.metadata.name":      protocol.SymbolKindString,
		"$.metadata.namespace": protocol.SymbolKindString,
		"$.data":               protocol.SymbolKindObject,
		"$.data.greeting":      protocol.SymbolKindString,
	}
	for _, s := range syms {
		if got := s.Kind; got != want[s.Name] {
			t.Errorf("%s Kind=%v want %v", s.Name, got, want[s.Name])
		}
	}
}

func TestDocumentSymbolSequencesEmitIndexedPaths(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///x.yaml"
	srv.documents.Set(uri, sequencesDoc, 1)

	res, _ := srv.documentSymbol(nil, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	syms := res.([]protocol.DocumentSymbol)
	want := []string{
		"$.containers",
		"$.containers[0]",
		"$.containers[0].name",
		"$.containers[0].image",
		"$.containers[1]",
		"$.containers[1].name",
		"$.containers[1].image",
	}
	if got := namesOf(syms); !equalStrings(got, want) {
		t.Fatalf("names:\n got:  %v\n want: %v", got, want)
	}
	if syms[0].Kind != protocol.SymbolKindArray {
		t.Errorf("$.containers Kind=%v want Array", syms[0].Kind)
	}
	if syms[1].Kind != protocol.SymbolKindObject {
		t.Errorf("$.containers[0] Kind=%v want Object", syms[1].Kind)
	}
}

func TestDocumentSymbolMultiDocPrefixesDocIndex(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///x.yaml"
	srv.documents.Set(uri, multiDoc, 1)

	res, _ := srv.documentSymbol(nil, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	syms := res.([]protocol.DocumentSymbol)
	want := []string{
		"[0] $.apiVersion",
		"[0] $.kind",
		"[0] $.metadata",
		"[0] $.metadata.name",
		"[1] $.apiVersion",
		"[1] $.kind",
		"[1] $.metadata",
		"[1] $.metadata.name",
	}
	if got := namesOf(syms); !equalStrings(got, want) {
		t.Fatalf("names:\n got:  %v\n want: %v", got, want)
	}
}

func TestDocumentSymbolEmptyOnUnsynced(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	res, err := srv.documentSymbol(nil, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///nope.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res != nil {
		t.Errorf("expected nil for unsynced document, got %+v", res)
	}
}

func TestDocumentSymbolRangesAreLSPCoords(t *testing.T) {
	// Verify that emitted positions are 0-based (LSP) and not 1-based
	// (parser). The first key in singleDoc is `apiVersion` on parser line 1
	// col 1; in LSP that's (0, 0). The SelectionRange covers exactly that
	// key.
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///x.yaml"
	srv.documents.Set(uri, singleDoc, 1)
	res, _ := srv.documentSymbol(nil, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	syms := res.([]protocol.DocumentSymbol)
	first := syms[0]
	if first.SelectionRange.Start.Line != 0 || first.SelectionRange.Start.Character != 0 {
		t.Errorf("first.SelectionRange.Start: got (%d,%d), want (0,0)",
			first.SelectionRange.Start.Line, first.SelectionRange.Start.Character)
	}
	if first.SelectionRange.End.Character != protocol.UInteger(len("apiVersion")) {
		t.Errorf("first.SelectionRange.End.Character: got %d, want %d",
			first.SelectionRange.End.Character, len("apiVersion"))
	}
}

func TestInitializeAdvertisesDocumentSymbolProvider(t *testing.T) {
	srv := New()
	res, err := srv.initialize(nil, &protocol.InitializeParams{})
	if err != nil {
		t.Fatal(err)
	}
	caps := res.(protocol.InitializeResult).Capabilities
	if got, ok := caps.DocumentSymbolProvider.(bool); !ok || !got {
		t.Errorf("DocumentSymbolProvider: got %v, want true", caps.DocumentSymbolProvider)
	}
}

// helpers shared by symbol tests

func namesOf(syms []protocol.DocumentSymbol) []string {
	out := make([]string, len(syms))
	for i, s := range syms {
		out[i] = s.Name
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
