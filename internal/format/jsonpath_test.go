package format

import (
	"testing"

	"github.com/psycofdj/yaml-lsp/internal/path"
)

func TestEncodeJSONPath(t *testing.T) {
	cases := []struct {
		name string
		in   path.Path
		want string
	}{
		{"empty", nil, "$"},
		{"single key", path.Path{{Key: "metadata"}}, "$.metadata"},
		{"nested keys", path.Path{{Key: "metadata"}, {Key: "name"}}, "$.metadata.name"},
		{"index", path.Path{{Key: "items"}, {IsIndex: true, Index: 2}}, "$.items[2]"},
		{"index then key", path.Path{{Key: "containers"}, {IsIndex: true, Index: 0}, {Key: "image"}}, "$.containers[0].image"},
		{"non-ident key bracketed", path.Path{{Key: "weird.key/v1"}}, "$['weird.key/v1']"},
		{"apostrophe RFC9535 backslash", path.Path{{Key: "it's"}}, `$['it\'s']`},
		{"backslash RFC9535 doubled", path.Path{{Key: `a\b`}}, `$['a\\b']`},
		{"backslash and quote", path.Path{{Key: `a\'b`}}, `$['a\\\'b']`},
		{"closing bracket passes through", path.Path{{Key: "foo]"}}, `$['foo]']`},
		{"leading digit bracketed", path.Path{{Key: "1stKey"}}, "$['1stKey']"},
		{"underscore-only ident plain", path.Path{{Key: "_foo"}}, "$._foo"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Encode(c.in, "jsonpath")
			if err != nil {
				t.Fatalf("Encode error: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
