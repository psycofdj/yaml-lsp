package format

import (
	"testing"

	"github.com/psycofdj/yaml-lsp/internal/path"
)

func TestEncodeBOSH(t *testing.T) {
	cases := []struct {
		name string
		in   path.Path
		want string
	}{
		{"keys", path.Path{{Key: "instance_groups"}, {Key: "name"}}, "/instance_groups/name"},
		{"named seq element field", path.Path{
			{Key: "instance_groups"},
			{IsIndex: true, Index: 0, NameKey: "web"},
			{Key: "image"},
		}, "/instance_groups/name=web/image"},
		{"unnamed seq fallback to index", path.Path{
			{Key: "items"},
			{IsIndex: true, Index: 2},
		}, "/items/2"},
		{"named seq leaf", path.Path{
			{Key: "containers"},
			{IsIndex: true, Index: 1, NameKey: "sidecar"},
		}, "/containers/name=sidecar"},
		{"key with slash percent-encoded", path.Path{{Key: "weird/key"}}, "/weird%2Fkey"},
		{"name value with slash percent-encoded", path.Path{
			{Key: "items"},
			{IsIndex: true, Index: 0, NameKey: "a/b"},
		}, "/items/name=a%2Fb"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Encode(c.in, "bosh-ops")
			if err != nil {
				t.Fatalf("Encode error: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
