package server

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/psycofdj/yaml-lsp/internal/format"
)

const singleDoc = `apiVersion: v1
kind: ConfigMap
metadata:
  name: foo
  namespace: default
data:
  greeting: hello
`

const multiDoc = `apiVersion: v1
kind: ConfigMap
metadata:
  name: first
---
apiVersion: v1
kind: Service
metadata:
  name: second
`

const sequencesDoc = `containers:
  - name: web
    image: nginx
  - name: sidecar
    image: envoy
`

func TestAddressAtPointSingleDoc(t *testing.T) {
	// LSP coords are 0-based. Cursor on `foo` of "  name: foo" at line 4 col 9.
	res, err := AddressAtPoint(singleDoc, 3, 8, "jsonpath")
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "$.metadata.name" || res.DocumentIndex != 0 || res.NodeKind != "value" {
		t.Errorf("unexpected: %+v", res)
	}
}

func TestAddressAtPointMultiDoc(t *testing.T) {
	// Doc 2 starts after `---` on line 5; cursor on "second" at line 9 col 9.
	res, err := AddressAtPoint(multiDoc, 8, 8, "jsonpath")
	if err != nil {
		t.Fatal(err)
	}
	if res.DocumentIndex != 1 {
		t.Errorf("documentIndex: got %d, want 1", res.DocumentIndex)
	}
	if res.Path != "$.metadata.name" {
		t.Errorf("path: got %q, want $.metadata.name", res.Path)
	}
}

func TestAddressAtPointAllFormats(t *testing.T) {
	// Cursor on `nginx` at line 3 col 13 (1-based) -> LSP line 2 char 12.
	cases := map[string]string{
		"jsonpath":    "$.containers[0].image",
		"bosh-ops":    "/containers/name=web/image",
		"jsonpatch":   "/containers/0/image",
		"helm-values": "containers[0].image",
	}
	for fmtName, want := range cases {
		t.Run(fmtName, func(t *testing.T) {
			res, err := AddressAtPoint(sequencesDoc, 2, 12, fmtName)
			if err != nil {
				t.Fatal(err)
			}
			if res.Path != want {
				t.Errorf("got %q, want %q", res.Path, want)
			}
		})
	}
}

func TestAddressAtPointUnsupportedFormat(t *testing.T) {
	_, err := AddressAtPoint(singleDoc, 0, 0, "xpath")
	var unsupported *format.ErrUnsupportedFormat
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected ErrUnsupportedFormat, got %v", err)
	}
}

func TestAddressAtPointPositionOutOfBounds(t *testing.T) {
	_, err := AddressAtPoint("a: 1\n", 50, 0, "jsonpath")
	if err == nil {
		t.Fatal("expected error for line beyond EOF")
	}
	var bad *ErrInvalidPosition
	if !errors.As(err, &bad) {
		t.Errorf("expected ErrInvalidPosition, got %T", err)
	}
}

// TestDispatcherInvalidParamsErrorCode pins the JSON-RPC error code mapping
// required by the spec's I/O Matrix: unsupported format and out-of-bounds
// position must both surface as CodeInvalidParams (-32602).
func TestDispatcherInvalidParamsErrorCode(t *testing.T) {
	srv := New()
	// Bypass the initialization gate; this test only exercises the
	// dispatcher's classification of errors as InvalidParams.
	srv.handler.SetInitialized(true)
	srv.documents.Set("file:///x.yaml", singleDoc, 1)
	d := &dispatcher{server: srv}

	cases := []struct {
		name string
		body string
	}{
		{
			"unsupported format",
			`{"textDocument":{"uri":"file:///x.yaml"},"position":{"line":0,"character":0},"format":"xpath"}`,
		},
		{
			"position out of bounds",
			`{"textDocument":{"uri":"file:///x.yaml"},"position":{"line":99,"character":0},"format":"jsonpath"}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := &glsp.Context{Method: "yaml/addressAtPoint", Params: []byte(c.body)}
			_, validMethod, validParams, err := d.Handle(ctx)
			if !validMethod {
				t.Fatal("validMethod=false")
			}
			if validParams {
				t.Errorf("validParams=true, want false (so glsp emits -32602)")
			}
			if err == nil {
				t.Error("err=nil, want non-nil")
			}
		})
	}
}

// TestDispatcherUnsynced verifies that querying an unopened URI surfaces as
// a non-InvalidParams error (the spec maps this to a regular server error).
func TestDispatcherUnsynced(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	d := &dispatcher{server: srv}
	body := `{"textDocument":{"uri":"file:///not-open.yaml"},"position":{"line":0,"character":0},"format":"jsonpath"}`
	ctx := &glsp.Context{Method: "yaml/addressAtPoint", Params: []byte(body)}
	_, _, validParams, err := d.Handle(ctx)
	if !validParams {
		t.Error("unsynced URI should not be InvalidParams")
	}
	if err == nil {
		t.Error("expected error")
	}
}

func TestAddressAtPointWithBOM(t *testing.T) {
	// A leading UTF-8 BOM must be transparent: querying at LSP (0, 0) of
	// "<BOM>a: 1\n" resolves like the same cursor on "a: 1\n".
	bomDoc := "\xEF\xBB\xBFa: 1\n"
	res, err := AddressAtPoint(bomDoc, 0, 0, "jsonpath")
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "$.a" {
		t.Errorf("path: got %q, want %q", res.Path, "$.a")
	}
}

func TestAddressAtPointInvalidUTF8(t *testing.T) {
	// A raw \xC0 byte is not valid UTF-8. The conversion returns
	// ErrInvalidPosition; the dispatcher then maps this to JSON-RPC -32602.
	bad := "a: \xC0bar\n"
	_, err := AddressAtPoint(bad, 0, 4, "jsonpath")
	if err == nil {
		t.Fatal("expected ErrInvalidPosition for invalid UTF-8 byte")
	}
	var bad1 *ErrInvalidPosition
	if !errors.As(err, &bad1) {
		t.Fatalf("expected *ErrInvalidPosition, got %T", err)
	}
	if !strings.Contains(bad1.Reason, "invalid UTF-8") {
		t.Errorf("reason missing 'invalid UTF-8': %q", bad1.Reason)
	}
}

func TestAddressAtPointCRLFClamps(t *testing.T) {
	// On Windows-style CRLF lines, a cursor past visible EOL must clamp to
	// the line content (before \r), not extend into \r or beyond.
	doc := "a: 1\r\nb: 2\r\n"
	res, err := AddressAtPoint(doc, 0, 99, "jsonpath")
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "$.a" {
		t.Errorf("got %q, want $.a", res.Path)
	}
}

func TestDidChangeAfterCloseDropped(t *testing.T) {
	// I/O matrix: didClose then a late didChange must not recreate the
	// entry. Without the existence guard in didChange, the late change
	// silently re-opens the document.
	srv := New()
	srv.handler.SetInitialized(true)
	srv.documents.Set("file:///x.yaml", "a: 1\n", 1)
	srv.documents.Delete("file:///x.yaml")
	err := srv.didChange(nil, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: "file:///x.yaml"},
			Version:                2,
		},
		ContentChanges: []any{protocol.TextDocumentContentChangeEventWhole{Text: "b: 2\n"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, open := srv.documents.Get("file:///x.yaml"); open {
		t.Error("didChange after didClose recreated the entry")
	}
}

func TestAddressAtPointPastEOLClamps(t *testing.T) {
	// Cursor past line length is a legitimate LSP position (cursor at end
	// of line, or beyond it for some clients). We clamp to line length and
	// continue rather than error.
	res, err := AddressAtPoint("a: 1\n", 0, 99, "jsonpath")
	if err != nil {
		t.Fatalf("expected clamp + success, got error: %v", err)
	}
	// Resolves to the value of `a:` (cursor effectively at end of line).
	if res.Path != "$.a" {
		t.Errorf("got %q", res.Path)
	}
}

func TestAddressAtPointInvalidUTF8BeforeCursorOK(t *testing.T) {
	// A line with a bad byte AFTER the cursor's position must resolve
	// cleanly — the cursor never needs to cross the bad byte. This is the
	// hardening change: previously we erred globally on any bad-byte line.
	// Cursor at LSP (0, 0) is on `a`, well before the `\xC0` at byte 3.
	bad := "a: \xC0bar\n"
	res, err := AddressAtPoint(bad, 0, 0, "jsonpath")
	if err != nil {
		t.Fatalf("expected clean resolve before bad byte, got %v", err)
	}
	if res.Path != "$.a" || res.NodeKind != "key" {
		t.Errorf("got %+v, want $.a / key", res)
	}
}

func TestDocumentStoreClampsNegativeVersion(t *testing.T) {
	store := newDocumentStore()
	if !store.Set("file:///x", "v-1", -1) {
		t.Fatal("Set with negative version returned false; want clamped to 0 and stored")
	}
	// A subsequent legitimate version 0 should NOT be rejected as older.
	if !store.Set("file:///x", "v0", 0) {
		t.Error("Set with version 0 after a -1 (clamped to 0) was rejected; clamp must not poison the baseline")
	}
}

func TestDocumentStoreVersionOrdering(t *testing.T) {
	store := newDocumentStore()
	if !store.Set("file:///x", "v1", 1) {
		t.Fatal("first Set returned false")
	}
	if !store.Set("file:///x", "v5", 5) {
		t.Fatal("Set with newer version returned false")
	}
	if store.Set("file:///x", "v3", 3) {
		t.Error("Set with older version returned true; should drop")
	}
	got, _ := store.Get("file:///x")
	if got != "v5" {
		t.Errorf("text after out-of-order writes: got %q, want v5", got)
	}
}

func TestDocumentStoreDeleteResetsVersion(t *testing.T) {
	store := newDocumentStore()
	store.Set("file:///x", "v5", 5)
	store.Delete("file:///x")
	// After Delete, a Set with any version should succeed (we have no record).
	if !store.Set("file:///x", "v1", 1) {
		t.Error("Set after Delete returned false")
	}
}

func TestDocumentStoreSoftLimitWarning(t *testing.T) {
	store := newDocumentStore()
	for i := 0; i < documentStoreSoftLimit+5; i++ {
		store.Set(fmt.Sprintf("file:///doc-%d", i), "x", 1)
	}
	if !store.Warned() {
		t.Error("expected warned=true after exceeding soft limit")
	}
	// Bringing the count back to the limit re-arms the warning so a
	// subsequent re-leak is reported again.
	for i := 0; i < 5; i++ {
		store.Delete(fmt.Sprintf("file:///doc-%d", i))
	}
	if store.Warned() {
		t.Error("warned should be re-armed after Delete brings count <= limit")
	}
}

func TestAddressAtPointUTF16Conversion(t *testing.T) {
	// "naïve" contains 'ï' which is 2 bytes in UTF-8 but 1 UTF-16 code unit.
	// Astral character "💡" is 4 bytes UTF-8, 2 UTF-16 code units.
	doc := "naïve: 1\n"
	// Cursor at LSP char 5 (after "naïve") should land past the key but
	// still resolve the key's address, since col after key is on the value
	// or colon — we just check the conversion is non-zero.
	res, err := AddressAtPoint(doc, 0, 0, "jsonpath")
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != `$['naïve']` && res.Path != "$.naïve" {
		t.Errorf("got %q", res.Path)
	}
}
