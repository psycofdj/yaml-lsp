package format

import (
	"testing"

	"github.com/psycofdj/yaml-lsp/internal/path"
)

func TestEncodeJSONPatch(t *testing.T) {
	cases := []struct {
		name string
		in   path.Path
		want string
	}{
		{"keys", path.Path{{Key: "spec"}, {Key: "replicas"}}, "/spec/replicas"},
		{"with index", path.Path{
			{Key: "spec"},
			{Key: "containers"},
			{IsIndex: true, Index: 0},
			{Key: "image"},
		}, "/spec/containers/0/image"},
		{"escape tilde", path.Path{{Key: "weird~key"}}, "/weird~0key"},
		{"escape slash", path.Path{{Key: "weird/key"}}, "/weird~1key"},
		{"escape both", path.Path{{Key: "a~b/c"}}, "/a~0b~1c"},
		{"name-keyed segment ignored, uses index", path.Path{
			{Key: "containers"},
			{IsIndex: true, Index: 0, NameKey: "web"},
			{Key: "image"},
		}, "/containers/0/image"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Encode(c.in, "jsonpatch")
			if err != nil {
				t.Fatalf("Encode error: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
