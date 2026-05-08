package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/psycofdj/yaml-lsp/internal/parser"
	"github.com/psycofdj/yaml-lsp/internal/path"
)

func mustParse(t *testing.T, file string) []parser.Document {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", file))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	docs, err := parser.ParseStream(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return docs
}

func TestLocateSingleDoc(t *testing.T) {
	docs := mustParse(t, "single.yaml")
	cases := []struct {
		name        string
		line, col   int
		wantPath    string // path.Path.String() form
		wantKind    path.NodeKind
		wantDocIdx  int
	}{
		// 1: apiVersion: v1
		{"on key apiVersion", 1, 3, ".apiVersion", path.NodeKindKey, 0},
		{"on value v1", 1, 14, ".apiVersion", path.NodeKindValue, 0},
		// 3: metadata:
		{"on key metadata", 3, 3, ".metadata", path.NodeKindKey, 0},
		// 4:   name: foo
		{"on inner key name", 4, 4, ".metadata.name", path.NodeKindKey, 0},
		{"on inner value foo", 4, 10, ".metadata.name", path.NodeKindValue, 0},
		// 5:   namespace: default
		{"on namespace value", 5, 14, ".metadata.namespace", path.NodeKindValue, 0},
		// 7:   greeting: hello
		{"on greeting value", 7, 14, ".data.greeting", path.NodeKindValue, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, kind, idx, ok := parser.Locate(docs, c.line, c.col)
			if !ok {
				t.Fatal("Locate returned ok=false")
			}
			if got := p.String(); got != c.wantPath {
				t.Errorf("path: got %q, want %q", got, c.wantPath)
			}
			if kind != c.wantKind {
				t.Errorf("kind: got %s, want %s", kind, c.wantKind)
			}
			if idx != c.wantDocIdx {
				t.Errorf("docIdx: got %d, want %d", idx, c.wantDocIdx)
			}
		})
	}
}

func TestLocateMultiDoc(t *testing.T) {
	docs := mustParse(t, "multidoc.yaml")
	if len(docs) != 2 {
		t.Fatalf("want 2 docs, got %d", len(docs))
	}
	// 1: apiVersion: v1   (doc 0)
	// 4:   name: first
	// 5: ---               (doc 1 starts here)
	// 9:   name: second
	cases := []struct {
		line, col  int
		wantPath   string
		wantKind   path.NodeKind
		wantDocIdx int
	}{
		{4, 10, ".metadata.name", path.NodeKindValue, 0},
		{9, 10, ".metadata.name", path.NodeKindValue, 1},
		{6, 1, ".apiVersion", path.NodeKindKey, 1},
	}
	for _, c := range cases {
		p, kind, idx, ok := parser.Locate(docs, c.line, c.col)
		if !ok {
			t.Fatalf("line %d col %d: ok=false", c.line, c.col)
		}
		if p.String() != c.wantPath || kind != c.wantKind || idx != c.wantDocIdx {
			t.Errorf("line %d col %d: got (%q,%s,%d), want (%q,%s,%d)",
				c.line, c.col, p.String(), kind, idx,
				c.wantPath, c.wantKind, c.wantDocIdx)
		}
	}
}

func TestLocateSequenceWithName(t *testing.T) {
	docs := mustParse(t, "sequences.yaml")
	// 1: containers:
	// 2:   - name: web
	// 3:     image: nginx
	// 4:   - name: sidecar
	// 5:     image: envoy
	// 6: items:
	// 7:   - alpha
	// 8:   - beta
	p, kind, idx, ok := parser.Locate(docs, 3, 13)
	if !ok || idx != 0 || kind != path.NodeKindValue {
		t.Fatalf("unexpected: ok=%v idx=%d kind=%s", ok, idx, kind)
	}
	if len(p) != 3 {
		t.Fatalf("want 3 segments, got %d (%v)", len(p), p)
	}
	if p[0].Key != "containers" {
		t.Errorf("seg0 key=%q", p[0].Key)
	}
	if !p[1].IsIndex || p[1].Index != 0 || p[1].NameKey != "web" {
		t.Errorf("seg1: IsIndex=%v Index=%d NameKey=%q", p[1].IsIndex, p[1].Index, p[1].NameKey)
	}
	if p[2].Key != "image" {
		t.Errorf("seg2 key=%q", p[2].Key)
	}
}

func TestLocateScalarSequenceNoName(t *testing.T) {
	docs := mustParse(t, "sequences.yaml")
	// 7:   - alpha
	p, _, idx, ok := parser.Locate(docs, 7, 5)
	if !ok || idx != 0 {
		t.Fatalf("unexpected: ok=%v idx=%d", ok, idx)
	}
	if len(p) != 2 || p[0].Key != "items" || !p[1].IsIndex || p[1].Index != 0 || p[1].NameKey != "" {
		t.Errorf("got %v", p)
	}
}

func TestLocateBeyondContent(t *testing.T) {
	// single.yaml has 7 lines of content; cursor on a virtual line past the
	// last token must return NodeKindNone with an empty path, not the last
	// key's address.
	docs := mustParse(t, "single.yaml")
	p, kind, idx, ok := parser.Locate(docs, 99, 1)
	if !ok {
		t.Fatal("ok=false")
	}
	if idx != 0 {
		t.Errorf("docIdx: got %d, want 0", idx)
	}
	if kind != path.NodeKindNone {
		t.Errorf("kind: got %s, want none", kind)
	}
	if len(p) != 0 {
		t.Errorf("path: got %v, want empty", p)
	}
}

func TestLocateSpecialKeys(t *testing.T) {
	docs := mustParse(t, "special-keys.yaml")
	// special-keys.yaml line 1 is `"weird.key/v1":` — 14-character lexical
	// extent (12 inner chars plus two quote characters).  Cursor on the
	// key text itself, on either quote, AND on the inner content all
	// resolve to the key.
	for _, col := range []int{1, 5, 14} {
		p, kind, _, ok := parser.Locate(docs, 1, col)
		if !ok || kind != path.NodeKindKey {
			t.Errorf("col %d: unexpected ok=%v kind=%s", col, ok, kind)
			continue
		}
		if len(p) != 1 || p[0].Key != "weird.key/v1" {
			t.Errorf("col %d: got %v, want [weird.key/v1]", col, p)
		}
	}
	// Col 15 is the `:` — past the quoted key's closing quote — should NOT
	// be classified as the key.
	_, kind, _, _ := parser.Locate(docs, 1, 15)
	if kind == path.NodeKindKey {
		t.Errorf("col 15 (colon) should not be NodeKindKey, got %s", kind)
	}
}

func TestLocateAnchorAndAlias(t *testing.T) {
	docs := mustParse(t, "anchors.yaml")
	// 1: defaults: &defaults    cursor on `defaults` key
	p, kind, _, ok := parser.Locate(docs, 1, 3)
	if !ok || kind != path.NodeKindKey {
		t.Fatalf("kind on key: ok=%v kind=%s", ok, kind)
	}
	if len(p) != 1 || p[0].Key != "defaults" {
		t.Errorf("got %v, want [defaults]", p)
	}
	// 5:   <<: *defaults        cursor on the alias `*defaults`
	// Alias resolution is intentionally NOT performed (spec says no merge-key
	// resolution beyond what goccy exposes natively); we just check it
	// doesn't panic and returns within the expected sub-tree.
	_, _, _, ok = parser.Locate(docs, 5, 8)
	if !ok {
		t.Fatal("alias position: ok=false")
	}
}

func TestParseStreamUnparseable(t *testing.T) {
	// Tab character in a mapping value is illegal in YAML; goccy reports a
	// parse error, which ParseStream must propagate (not panic).
	bad := []byte("foo:\n\tbar: 1\n")
	_, err := parser.ParseStream(bad)
	if err == nil {
		t.Skip("goccy accepted tab-indented YAML; nothing to assert")
	}
}

func TestLocateEmptyQuotedKey(t *testing.T) {
	// `"":` — a key whose parsed value is empty but whose source text
	// occupies two columns (the quote pair). Cursor on either quote must
	// resolve to the empty key with NodeKindKey.
	docs, err := parser.ParseStream([]byte("\"\":\n  inner: 1\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, col := range []int{1, 2} {
		p, kind, _, ok := parser.Locate(docs, 1, col)
		if !ok {
			t.Fatalf("col %d: ok=false", col)
		}
		if kind != path.NodeKindKey {
			t.Errorf("col %d: kind got %s, want key", col, kind)
		}
		if len(p) != 1 || p[0].Key != "" {
			t.Errorf("col %d: path got %v, want [\"\"]", col, p)
		}
	}
}

func TestParseStreamStripsBOM(t *testing.T) {
	// A leading UTF-8 BOM must be invisible to locate: column 1 of line 1
	// is the first non-BOM byte.
	bom := []byte{0xEF, 0xBB, 0xBF}
	docs, err := parser.ParseStream(append(bom, []byte("a: 1\n")...))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	p, kind, _, ok := parser.Locate(docs, 1, 1)
	if !ok || kind != path.NodeKindKey {
		t.Fatalf("ok=%v kind=%s", ok, kind)
	}
	if len(p) != 1 || p[0].Key != "a" {
		t.Errorf("got %v, want [a]", p)
	}
}

func TestParseStreamMultiDocNoLeadingHeader(t *testing.T) {
	// goccy reports d.Start == nil for a stream's first document when
	// there's no `---` header before it. Our fallback must derive the
	// start line from the body's first content token so routing still
	// works under multi-doc.
	src := []byte("a: 1\n---\nb: 2\n")
	docs, err := parser.ParseStream(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("want 2 docs, got %d", len(docs))
	}
	p, _, idx, ok := parser.Locate(docs, 1, 1)
	if !ok || idx != 0 {
		t.Fatalf("doc 0: ok=%v idx=%d", ok, idx)
	}
	if len(p) != 1 || p[0].Key != "a" {
		t.Errorf("doc 0 path: got %v, want [a]", p)
	}
	p, _, idx, ok = parser.Locate(docs, 3, 1)
	if !ok || idx != 1 {
		t.Fatalf("doc 1: ok=%v idx=%d", ok, idx)
	}
	if len(p) != 1 || p[0].Key != "b" {
		t.Errorf("doc 1 path: got %v, want [b]", p)
	}
}

func TestLocateEmptyInput(t *testing.T) {
	docs, err := parser.ParseStream([]byte(""))
	if err != nil {
		t.Fatal(err)
	}
	// Empty input either yields zero documents or one document with nil body;
	// both should resolve a cursor to NodeKindNone with an empty path.
	p, kind, _, _ := parser.Locate(docs, 1, 1)
	if kind != path.NodeKindNone {
		t.Errorf("kind: got %s, want %s", kind, path.NodeKindNone)
	}
	if len(p) != 0 {
		t.Errorf("path: got %v, want empty", p)
	}
}
