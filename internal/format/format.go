// Package format encodes a path.Path into one of several tool-specific
// addressing syntaxes. Each encoder is a pure function of the Path —
// encoders never see the YAML AST.
package format

import (
	"fmt"
	"sort"

	"github.com/psycofdj/yaml-lsp/internal/path"
)

// Encoder produces the user-facing string for a Path in a specific syntax.
type Encoder func(p path.Path) (string, error)

var registry = map[string]Encoder{}

// Register associates an encoder with a format name. Called from each
// encoder file's init().
func Register(name string, enc Encoder) {
	registry[name] = enc
}

// Encode dispatches to the named encoder. Unknown formats return
// ErrUnsupportedFormat carrying the supported list.
func Encode(p path.Path, format string) (string, error) {
	enc, ok := registry[format]
	if !ok {
		return "", &ErrUnsupportedFormat{Got: format, Supported: SupportedFormats()}
	}
	return enc(p)
}

// IsSupported reports whether format is a registered encoder. Useful at the
// LSP boundary to fail fast before parsing.
func IsSupported(format string) bool {
	_, ok := registry[format]
	return ok
}

// SupportedFormats returns the registered format names sorted alphabetically
// for stable error messages.
func SupportedFormats() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ErrUnsupportedFormat is returned by Encode when the requested format has no
// registered encoder. The message lists the supported formats so users can
// correct their input without consulting docs.
type ErrUnsupportedFormat struct {
	Got       string
	Supported []string
}

func (e *ErrUnsupportedFormat) Error() string {
	return fmt.Sprintf("unsupported format %q: supported formats are %v", e.Got, e.Supported)
}
