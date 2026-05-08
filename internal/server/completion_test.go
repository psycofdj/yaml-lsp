package server

import (
	"strings"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestCompletionOffersAnchorNamesAfterStar(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///c.yaml"
	src := "first: &alpha 1\n" +
		"second: &beta 2\n" +
		"ref: *\n" //                  cursor immediately after `*`
	srv.documents.Set(uri, src, 1)

	// LSP cursor: line 2 (0-based), char 6 (just past `*` at col 7).
	res, err := srv.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 2, Character: 6},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	list, ok := res.(protocol.CompletionList)
	if !ok {
		t.Fatalf("res=%T want CompletionList", res)
	}
	if got := labels(list.Items); !sameSet(got, []string{"alpha", "beta"}) {
		t.Errorf("labels=%v want [alpha beta]", got)
	}
	// TextEdit range should be degenerate (start == end) at the
	// cursor, with NewText = the chosen name (no `*` re-inserted).
	for _, it := range list.Items {
		if it.TextEdit == nil {
			t.Fatalf("item %q missing TextEdit", it.Label)
		}
		te, _ := it.TextEdit.(protocol.TextEdit)
		if te.Range.Start != te.Range.End {
			t.Errorf("%q range=%+v want start==end (no prefix typed)", it.Label, te.Range)
		}
		if te.NewText != it.Label {
			t.Errorf("%q NewText=%q want %q", it.Label, te.NewText, it.Label)
		}
	}
}

func TestCompletionFiltersByPrefix(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///c.yaml"
	src := "first: &alpha 1\n" +
		"second: &beta 2\n" +
		"ref: *al\n" //                cursor after `al`
	srv.documents.Set(uri, src, 1)

	res, err := srv.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 2, Character: 8}, // after `al`
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	list := res.(protocol.CompletionList)
	if got := labels(list.Items); !sameSet(got, []string{"alpha"}) {
		t.Fatalf("labels=%v want [alpha]", got)
	}
	// TextEdit should replace the `al` prefix (2 chars before cursor).
	te, _ := list.Items[0].TextEdit.(protocol.TextEdit)
	if te.Range.Start.Character != 6 || te.Range.End.Character != 8 {
		t.Errorf("range chars=%d..%d want 6..8", te.Range.Start.Character, te.Range.End.Character)
	}
}

func TestCompletionNilOffAliasContext(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///c.yaml"
	srv.documents.Set(uri, "first: &alpha 1\nplain: value\n", 1)

	// Cursor in a plain value — no `*` to its left on the line.
	res, err := srv.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 1, Character: 10},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res != nil {
		t.Errorf("res=%v want nil (not in alias context)", res)
	}
}

func TestCompletionNilOnAnchorName(t *testing.T) {
	// Cursor inside a `&anchor` name — must NOT offer alias completion
	// (would suggest renaming the anchor to itself, which is nonsense).
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///c.yaml"
	srv.documents.Set(uri, "first: &alpha 1\n", 1)

	res, err := srv.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 0, Character: 10}, // inside `alpha`
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res != nil {
		t.Errorf("res=%v want nil (inside &anchor name)", res)
	}
}

func TestCompletionScopedToCursorDocument(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///c.yaml"
	// Document 1 defines &a; document 2 defines &b. Cursor in doc 2
	// should see only `b`, not `a`.
	srv.documents.Set(uri, "first: &a 1\n---\nsecond: &b 2\nref: *\n", 1)

	res, err := srv.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 3, Character: 6},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	list := res.(protocol.CompletionList)
	if got := labels(list.Items); !sameSet(got, []string{"b"}) {
		t.Errorf("labels=%v want [b] (doc-2 only)", got)
	}
}

func TestCompletionDedupesByName(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///c.yaml"
	// Two anchors with the same name in one document — the parser
	// resolves to the most recent at use, but completion should show
	// one entry, not two.
	srv.documents.Set(uri, "first: &dup 1\nsecond: &dup 2\nref: *\n", 1)

	res, err := srv.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 2, Character: 6},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	list := res.(protocol.CompletionList)
	if len(list.Items) != 1 || list.Items[0].Label != "dup" {
		t.Errorf("items=%+v want one entry labelled 'dup'", list.Items)
	}
}

func TestCompletionItemDetailReferencesAnchorLine(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///c.yaml"
	srv.documents.Set(uri, "first: &alpha 1\nref: *\n", 1)

	res, err := srv.completion(nil, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 1, Character: 6},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	list := res.(protocol.CompletionList)
	if len(list.Items) != 1 {
		t.Fatalf("items=%d want 1", len(list.Items))
	}
	if list.Items[0].Detail == nil || !strings.Contains(*list.Items[0].Detail, "line 1") {
		t.Errorf("Detail=%v want mention of line 1", list.Items[0].Detail)
	}
}

func labels(items []protocol.CompletionItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Label
	}
	return out
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]int)
	for _, s := range a {
		m[s]++
	}
	for _, s := range b {
		m[s]--
	}
	for _, v := range m {
		if v != 0 {
			return false
		}
	}
	return true
}
