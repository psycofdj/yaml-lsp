package format

import (
	"errors"
	"reflect"
	"testing"

	"github.com/psycofdj/yaml-lsp/internal/path"
)

func TestSupportedFormats(t *testing.T) {
	got := SupportedFormats()
	want := []string{"bosh-ops", "helm-values", "jsonpatch", "jsonpath"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestUnsupportedFormat(t *testing.T) {
	_, err := Encode(path.Path{{Key: "x"}}, "xpath")
	var unsupported *ErrUnsupportedFormat
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected ErrUnsupportedFormat, got %T: %v", err, err)
	}
	if unsupported.Got != "xpath" {
		t.Errorf("Got=%q, want %q", unsupported.Got, "xpath")
	}
	if len(unsupported.Supported) != 4 {
		t.Errorf("expected 4 supported formats, got %d", len(unsupported.Supported))
	}
}

func TestIsSupported(t *testing.T) {
	for _, name := range []string{"jsonpath", "bosh-ops", "jsonpatch", "helm-values"} {
		if !IsSupported(name) {
			t.Errorf("IsSupported(%q) = false", name)
		}
	}
	if IsSupported("xpath") {
		t.Error("IsSupported(\"xpath\") = true")
	}
}
