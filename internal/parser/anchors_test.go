package parser

import (
	"reflect"
	"testing"
)

func TestFindAnchorDefinition(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		line     int
		col      int
		wantOK   bool
		wantLine int
		wantSCol int
		wantECol int
	}{
		{
			name: "cursor on alias jumps to anchor",
			// 1: defaults: &d
			// 2:   timeout: 30
			// 3: server:
			// 4:   <<: *d   (cols: ..6=' ', 7='*', 8='d')
			src:      "defaults: &d\n  timeout: 30\nserver:\n  <<: *d\n",
			line:     4,
			col:      7, // on `*` of *d
			wantOK:   true,
			wantLine: 1,
			wantSCol: 11, // `&` column
			wantECol: 12, // `d`
		},
		{
			name:   "cursor on the alias name (after *) still resolves",
			src:    "defaults: &d\n  timeout: 30\nserver:\n  <<: *d\n",
			line:   4,
			col:    8, // on `d`
			wantOK: true,
		},
		{
			name:     "cursor on anchor definition resolves to itself (symmetric)",
			src:      "defaults: &d\n  timeout: 30\nserver:\n  <<: *d\n",
			line:     1,
			col:      11, // on `&`
			wantOK:   true,
			wantLine: 1,
			wantSCol: 11,
			wantECol: 12,
		},
		{
			name:   "cursor on whitespace returns no result",
			src:    "defaults: &d\n  timeout: 30\nserver:\n  <<: *d\n",
			line:   3,
			col:    8,
			wantOK: false,
		},
		{
			name:   "alias with no matching anchor returns no result",
			src:    "server:\n  ref: *missing\n",
			line:   2,
			col:    10, // on `*`
			wantOK: false,
		},
		{
			name: "alias outside merge key resolves",
			// `name: *anchor` shape, anchor defined earlier
			src:      "first: &x value1\nsecond: *x\n",
			line:     2,
			col:     10, // on `*`
			wantOK:   true,
			wantLine: 1,
			wantSCol: 8, // `&`
			wantECol: 9, // `x`
		},
		{
			name: "multi-char anchor name span",
			// `&longname` is 1 + 8 = 9 columns wide
			src:      "k: &longname v\nq: *longname\n",
			line:     2,
			col:     4, // on `*`
			wantOK:   true,
			wantLine: 1,
			wantSCol: 4,
			wantECol: 12,
		},
		{
			name: "anchor in another document is not visible",
			// Document 1 defines &d, document 2 references *d — invalid YAML
			// in spirit, but the parser still produces nodes; we must NOT
			// return a hit because anchors are document-scoped.
			src:    "first: &d v\n---\nsecond: *d\n",
			line:   3,
			col:    10, // on `*`
			wantOK: false,
		},
		{
			name:   "empty document",
			src:    "",
			line:   1,
			col:    1,
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs, err := ParseStream([]byte(tt.src))
			if err != nil {
				t.Fatalf("ParseStream: %v", err)
			}
			got, ok := FindAnchorDefinition(docs, tt.line, tt.col)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v want %v (got=%+v)", ok, tt.wantOK, got)
			}
			if !ok {
				return
			}
			if tt.wantLine != 0 && got.Line != tt.wantLine {
				t.Errorf("Line=%d want %d", got.Line, tt.wantLine)
			}
			if tt.wantSCol != 0 && got.StartCol != tt.wantSCol {
				t.Errorf("StartCol=%d want %d", got.StartCol, tt.wantSCol)
			}
			if tt.wantECol != 0 && got.EndCol != tt.wantECol {
				t.Errorf("EndCol=%d want %d", got.EndCol, tt.wantECol)
			}
		})
	}
}

func TestFindAnchorSymbol(t *testing.T) {
	src := "defaults: &d\n  timeout: 30\nserver:\n  <<: *d\n"
	docs, err := ParseStream([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		line     int
		col      int
		wantName string
		wantIdx  int
		wantOK   bool
	}{
		{"on anchor &", 1, 11, "d", 0, true},
		{"on anchor name", 1, 12, "d", 0, true},
		{"on alias *", 4, 7, "d", 0, true},
		{"on alias name", 4, 8, "d", 0, true},
		{"on plain scalar value", 2, 13, "", 0, false},
		{"on key", 2, 3, "", 0, false},
		{"on whitespace", 3, 8, "", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotName, gotIdx, ok := FindAnchorSymbol(docs, c.line, c.col)
			if ok != c.wantOK {
				t.Fatalf("ok=%v want %v (name=%q idx=%d)", ok, c.wantOK, gotName, gotIdx)
			}
			if !ok {
				return
			}
			if gotName != c.wantName {
				t.Errorf("name=%q want %q", gotName, c.wantName)
			}
			if gotIdx != c.wantIdx {
				t.Errorf("idx=%d want %d", gotIdx, c.wantIdx)
			}
		})
	}
}

func TestCollectAnchorOccurrences(t *testing.T) {
	src := "defaults: &d\n" + // line 1: &d at col 11-12
		"  timeout: 30\n" +
		"server:\n" +
		"  <<: *d\n" + //     line 4: *d at col 7-8
		"backup: *d\n" //     line 5: *d at col 9-10
	docs, err := ParseStream([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	got := CollectAnchorOccurrences(docs[0], "d")
	want := []AnchorRef{
		{Line: 1, StartCol: 11, EndCol: 12, Kind: AnchorKindAnchor},
		{Line: 4, StartCol: 7, EndCol: 8, Kind: AnchorKindAlias},
		{Line: 5, StartCol: 9, EndCol: 10, Kind: AnchorKindAlias},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestCollectAnchorOccurrencesMultipleAnchors(t *testing.T) {
	// YAML allows redefining anchors; the parser resolves to the most
	// recent at use, but every site must be addressable for rename.
	src := "first: &d 1\nsecond: &d 2\nref: *d\n"
	docs, err := ParseStream([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	got := CollectAnchorOccurrences(docs[0], "d")
	wantKinds := []AnchorKind{AnchorKindAnchor, AnchorKindAnchor, AnchorKindAlias}
	if len(got) != len(wantKinds) {
		t.Fatalf("len(got)=%d want %d (%+v)", len(got), len(wantKinds), got)
	}
	for i, r := range got {
		if r.Kind != wantKinds[i] {
			t.Errorf("[%d] Kind=%v want %v", i, r.Kind, wantKinds[i])
		}
	}
}

func TestAnchorRefNameStartCol(t *testing.T) {
	r := AnchorRef{Line: 1, StartCol: 11, EndCol: 12, Kind: AnchorKindAnchor}
	if got := r.NameStartCol(); got != 12 {
		t.Errorf("NameStartCol=%d want 12", got)
	}
}
