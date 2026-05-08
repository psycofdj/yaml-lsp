package server

import (
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestFoldingRangeMappingEntries(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///x.yaml"
	srv.documents.Set(uri, singleDoc, 1)
	// singleDoc shape (1-based parser lines):
	//   1: apiVersion: v1
	//   2: kind: ConfigMap
	//   3: metadata:
	//   4:   name: foo
	//   5:   namespace: default
	//   6: data:
	//   7:   greeting: hello
	// Multi-line containers: `metadata:` (3-5), `data:` (6-7).

	res, err := srv.foldingRange(nil, &protocol.FoldingRangeParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("got %d folds, want 2 (%v)", len(res), res)
	}
	if res[0].StartLine != 2 || res[0].EndLine != 4 { // metadata: lines 3-5 → 2-4 LSP
		t.Errorf("metadata fold: got [%d,%d], want [2,4]", res[0].StartLine, res[0].EndLine)
	}
	if res[1].StartLine != 5 || res[1].EndLine != 6 { // data: lines 6-7 → 5-6 LSP
		t.Errorf("data fold: got [%d,%d], want [5,6]", res[1].StartLine, res[1].EndLine)
	}
}

func TestFoldingRangeSequences(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///x.yaml"
	srv.documents.Set(uri, sequencesDoc, 1)
	// sequencesDoc:
	//   1: containers:
	//   2:   - name: web
	//   3:     image: nginx
	//   4:   - name: sidecar
	//   5:     image: envoy
	// Expected folds:
	//   containers entry: lines 1-5 → LSP [0,4]
	//   element [0]: lines 2-3 → LSP [1,2]
	//   element [1]: lines 4-5 → LSP [3,4]

	res, _ := srv.foldingRange(nil, &protocol.FoldingRangeParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	if len(res) != 3 {
		t.Fatalf("got %d folds, want 3 (%v)", len(res), res)
	}
	expected := []struct{ start, end protocol.UInteger }{
		{0, 4}, {1, 2}, {3, 4},
	}
	for i, want := range expected {
		if res[i].StartLine != want.start || res[i].EndLine != want.end {
			t.Errorf("fold[%d]: got [%d,%d], want [%d,%d]",
				i, res[i].StartLine, res[i].EndLine, want.start, want.end)
		}
	}
}

func TestFoldingRangeSingleLineNoFold(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///x.yaml"
	srv.documents.Set(uri, "a: 1\nb: 2\n", 1)
	res, _ := srv.foldingRange(nil, &protocol.FoldingRangeParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	if len(res) != 0 {
		t.Errorf("expected no folds for single-line entries, got %v", res)
	}
}

func TestFoldingRangeUnsynced(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	res, err := srv.foldingRange(nil, &protocol.FoldingRangeParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///nope.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Errorf("expected no folds for unsynced doc, got %v", res)
	}
}

func TestInitializeAdvertisesFoldingRangeProvider(t *testing.T) {
	srv := New()
	res, _ := srv.initialize(nil, &protocol.InitializeParams{})
	caps := res.(protocol.InitializeResult).Capabilities
	if got, ok := caps.FoldingRangeProvider.(bool); !ok || !got {
		t.Errorf("FoldingRangeProvider: got %v, want true", caps.FoldingRangeProvider)
	}
}
