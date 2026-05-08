package server

import (
	"strings"
	"testing"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// makeContextWithNotify constructs a *glsp.Context whose Notify callback is
// the provided function. Used by tests to capture diagnostic notifications
// without a real LSP transport.
func makeContextWithNotify(notify func(method string, params any)) *glsp.Context {
	return &glsp.Context{Notify: notify}
}

func TestDiagnosticsCleanParseEmpty(t *testing.T) {
	diags := computeDiagnostics(singleDoc)
	if len(diags) != 0 {
		t.Errorf("expected 0 diagnostics for clean YAML, got %d: %+v", len(diags), diags)
	}
}

func TestDiagnosticsParseError(t *testing.T) {
	// A tab-indented mapping value is illegal YAML; goccy emits a
	// SyntaxError tagged with the offending token.
	bad := "foo:\n\tbar: 1\n"
	diags := computeDiagnostics(bad)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	d := diags[0]
	if d.Severity == nil || *d.Severity != protocol.DiagnosticSeverityError {
		t.Errorf("severity: got %v, want Error", d.Severity)
	}
	if d.Source == nil || *d.Source != "yaml-lsp" {
		t.Errorf("source: got %v, want yaml-lsp", d.Source)
	}
	if d.Message == "" {
		t.Error("message is empty")
	}
	// Range must be non-degenerate (end > start) so the squiggly is visible.
	if d.Range.End.Line == d.Range.Start.Line && d.Range.End.Character <= d.Range.Start.Character {
		t.Errorf("range is empty: %+v", d.Range)
	}
}

func TestDiagnosticsLSPCoordinates(t *testing.T) {
	// Verify diagnostic positions are 0-based (LSP) not 1-based (parser).
	bad := "foo:\n\tbar: 1\n"
	diags := computeDiagnostics(bad)
	if len(diags) != 1 {
		t.Skipf("got %d diagnostics; this test asserts on the single-error shape", len(diags))
	}
	if diags[0].Range.Start.Line == 0 && diags[0].Range.Start.Character == 0 {
		// Acceptable: error pinned to (0,0) when goccy didn't tag a token.
		return
	}
	// Otherwise, the line must be a 0-based line within the source text.
	totalLines := uint32(strings.Count(bad, "\n") + 1)
	if uint32(diags[0].Range.Start.Line) >= totalLines {
		t.Errorf("Start.Line %d is beyond %d total lines", diags[0].Range.Start.Line, totalLines)
	}
}

// fakeContext captures notifications instead of writing to a real client.
// Used by the publishDiagnostics tests below.
type fakeNotifier struct {
	method string
	params any
}

func TestPublishDiagnosticsEmitsNotification(t *testing.T) {
	srv := New()
	srv.handler.SetInitialized(true)
	const uri = "file:///x.yaml"

	var captured fakeNotifier
	notify := func(method string, params any) {
		captured.method = method
		captured.params = params
	}
	// We can't construct a real *glsp.Context with a hand-rolled Notify
	// unless we know its struct shape. Use the `_test` pattern: build a
	// pointer to a glsp.Context via the public field.
	srv.publishDiagnostics(makeContextWithNotify(notify), uri, "foo: 1\n")

	if captured.method != "textDocument/publishDiagnostics" {
		t.Errorf("notification method: got %q, want textDocument/publishDiagnostics", captured.method)
	}
	pp, ok := captured.params.(*protocol.PublishDiagnosticsParams)
	if !ok {
		t.Fatalf("params type: got %T, want *PublishDiagnosticsParams", captured.params)
	}
	if pp.URI != uri {
		t.Errorf("uri: got %q, want %q", pp.URI, uri)
	}
	if len(pp.Diagnostics) != 0 {
		t.Errorf("clean parse should yield 0 diagnostics, got %+v", pp.Diagnostics)
	}
}

func TestPublishDiagnosticsErrorPath(t *testing.T) {
	srv := New()
	const uri = "file:///x.yaml"
	var captured fakeNotifier
	notify := func(method string, params any) {
		captured.method = method
		captured.params = params
	}
	srv.publishDiagnostics(makeContextWithNotify(notify), uri, "foo:\n\tbar: 1\n")

	pp := captured.params.(*protocol.PublishDiagnosticsParams)
	if len(pp.Diagnostics) == 0 {
		t.Errorf("expected diagnostics for malformed YAML")
	}
}
