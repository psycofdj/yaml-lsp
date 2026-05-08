package format

import "testing"

func TestConservative(t *testing.T) {
	defaults := DefaultConservativeOptions()
	tests := []struct {
		name string
		opts ConservativeOptions
		in   string
		want string
	}{
		{
			name: "trims trailing spaces and tabs",
			opts: defaults,
			in:   "foo: bar   \nbaz:\tqux\t \n",
			want: "foo: bar\nbaz:\tqux\n",
		},
		{
			name: "keeps inner whitespace",
			opts: defaults,
			in:   "  foo:    bar\n",
			want: "  foo:    bar\n",
		},
		{
			name: "preserves comments and anchors",
			opts: defaults,
			in:   "# top\ndefaults: &d   \n  timeout: 30\nref: *d\n",
			want: "# top\ndefaults: &d\n  timeout: 30\nref: *d\n",
		},
		{
			name: "removes trailing blank lines",
			opts: defaults,
			in:   "foo\n\n\n",
			want: "foo\n",
		},
		{
			name: "inserts final newline when absent",
			opts: defaults,
			in:   "foo",
			want: "foo\n",
		},
		{
			name: "preserves CRLF line endings",
			opts: defaults,
			in:   "foo  \r\nbar\r\n",
			want: "foo\r\nbar\r\n",
		},
		{
			name: "no-op on already-clean input",
			opts: defaults,
			in:   "foo: bar\nbaz: qux\n",
			want: "foo: bar\nbaz: qux\n",
		},
		{
			name: "trimTrailingWhitespace=false keeps trailing spaces",
			opts: ConservativeOptions{TrimTrailingWhitespace: false, InsertFinalNewline: true, TrimFinalNewlines: true},
			in:   "foo: bar   \n",
			want: "foo: bar   \n",
		},
		{
			name: "trimFinalNewlines=false keeps trailing blank lines",
			opts: ConservativeOptions{TrimTrailingWhitespace: true, InsertFinalNewline: true, TrimFinalNewlines: false},
			in:   "foo\n\n\n",
			want: "foo\n\n\n",
		},
		{
			name: "insertFinalNewline=false preserves missing trailing newline",
			opts: ConservativeOptions{TrimTrailingWhitespace: true, InsertFinalNewline: false, TrimFinalNewlines: false},
			in:   "foo",
			want: "foo",
		},
		{
			name: "empty input stays empty",
			opts: defaults,
			in:   "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Conservative(tt.in, tt.opts)
			if got != tt.want {
				t.Errorf("Conservative(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
