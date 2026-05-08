package server

import (
	"errors"

	yaml "github.com/goccy/go-yaml"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/psycofdj/yaml-lsp/internal/parser"
)

// publishDiagnostics parses `text` and pushes a
// `textDocument/publishDiagnostics` notification for `uri`. On a clean
// parse the published list is empty, which the LSP protocol interprets as
// "clear any diagnostics previously reported for this URI". On a parse
// failure the list contains a single error-severity diagnostic at the
// position goccy/go-yaml reported.
//
// Called from didOpen and didChange after the document store has been
// updated, so client and server agree on the text being analyzed.
func (s *Server) publishDiagnostics(ctx *glsp.Context, uri string, text string) {
	if ctx == nil {
		return
	}
	ctx.Notify(protocol.ServerTextDocumentPublishDiagnostics,
		&protocol.PublishDiagnosticsParams{
			URI:         uri,
			Diagnostics: computeDiagnostics(text),
		})
}

// computeDiagnostics is the pure side: parse text, return diagnostics.
// Exposed for tests so we don't need a full LSP connection to verify the
// shape and content of what gets published.
func computeDiagnostics(text string) []protocol.Diagnostic {
	if _, err := parser.ParseStream([]byte(text)); err == nil {
		// Clean parse — emit empty list to clear any prior diagnostics.
		return []protocol.Diagnostic{}
	} else {
		return diagnosticsFromError(text, err)
	}
}

func diagnosticsFromError(text string, parseErr error) []protocol.Diagnostic {
	severity := protocol.DiagnosticSeverityError
	source := "yaml-lsp"
	src := string(parser.StripBOM([]byte(text)))

	// goccy returns *yaml.SyntaxError for syntax issues with a Token
	// pointing at the offending content. When that's available we
	// produce a precise diagnostic.
	var synErr *yaml.SyntaxError
	if errors.As(parseErr, &synErr) && synErr.Token != nil && synErr.Token.Position != nil {
		line := synErr.Token.Position.Line
		col := synErr.Token.Position.Column
		start := lspPosFromParser(src, line, col)
		// Span the token's parsed value width if non-empty; otherwise mark
		// a single column so the diagnostic has a visible extent.
		width := len(synErr.Token.Value)
		if width <= 0 {
			width = 1
		}
		end := lspPosFromParser(src, line, col+width)
		return []protocol.Diagnostic{{
			Range:    protocol.Range{Start: start, End: end},
			Severity: &severity,
			Source:   &source,
			Message:  synErr.Message,
		}}
	}

	// Fallback for errors goccy didn't tag with a Token: anchor the
	// diagnostic to the first line so the user still sees a squiggly.
	return []protocol.Diagnostic{{
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   lspPosLineEnd(src, 1),
		},
		Severity: &severity,
		Source:   &source,
		Message:  parseErr.Error(),
	}}
}
