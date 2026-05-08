package format

import (
	"testing"

	"github.com/psycofdj/yaml-lsp/internal/path"
)

func TestEncodeHelm(t *testing.T) {
	cases := []struct {
		name string
		in   path.Path
		want string
	}{
		{"keys", path.Path{{Key: "image"}, {Key: "tag"}}, "image.tag"},
		{"with index", path.Path{
			{Key: "containers"},
			{IsIndex: true, Index: 0},
			{Key: "image"},
		}, "containers[0].image"},
		{"index then index", path.Path{
			{Key: "matrix"},
			{IsIndex: true, Index: 0},
			{IsIndex: true, Index: 1},
		}, "matrix[0][1]"},
		{"escape comma dot equals", path.Path{{Key: "a,b.c=d"}}, `a\,b\.c\=d`},
		{"escape backslash", path.Path{{Key: `a\b`}}, `a\\b`},
		{"leading index", path.Path{
			{IsIndex: true, Index: 3},
			{Key: "image"},
		}, "[3].image"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Encode(c.in, "helm-values")
			if err != nil {
				t.Fatalf("Encode error: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
