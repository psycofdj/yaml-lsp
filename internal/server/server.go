// Package server wires the glsp protocol handler with our document store and
// custom request dispatcher. The exported entry point is Run.
package server

import (
	"encoding/json"
	"errors"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	glspserver "github.com/tliron/glsp/server"

	"github.com/psycofdj/yaml-lsp/internal/buildinfo"
	"github.com/psycofdj/yaml-lsp/internal/format"
)

// ServerName is the LSP server identifier reported during initialize and used
// as the log basename for commonlog.
const ServerName = "yaml-lsp"

// Server holds the in-memory document store and the protocol handler. The
// embedded protocol.Handler dispatches the standard LSP methods; we wrap it
// to also serve our `yaml/addressAtPoint` custom request.
type Server struct {
	documents *documentStore
	handler   protocol.Handler
	config    Config
}

// Config holds the resolved server settings parsed from the client's
// `initializationOptions` at startup.
type Config struct {
	Format FormatConfig
}

// FormatConfig controls the document formatter's behavior. Indentation
// accepts the literal "detect" (or omitted) for source-derived indent, or a
// positive integer for a fixed indent. NormalizeStrings, when true, strips
// quotes from string scalars that resolve back to themselves as plain.
type FormatConfig struct {
	Indentation      int  // 0 = detect from source; >0 = fixed indent
	NormalizeStrings bool
}

// formatInitOptions mirrors the JSON shape clients send under
// initializationOptions.format. Kept as a private type so the Config above
// can present a clean go-flavored API to the rest of the server.
type formatInitOptions struct {
	Indentation      any  `json:"indentation,omitempty"`
	NormalizeStrings bool `json:"normalizeStrings,omitempty"`
}

type initOptions struct {
	Format formatInitOptions `json:"format"`
}

// New constructs a Server with all standard handlers wired up.
func New() *Server {
	s := &Server{documents: newDocumentStore()}
	s.handler = protocol.Handler{
		Initialize:                  s.initialize,
		Initialized:                 s.initialized,
		Shutdown:                    s.shutdown,
		SetTrace:                    s.setTrace,
		TextDocumentDidOpen:         s.didOpen,
		TextDocumentDidChange:       s.didChange,
		TextDocumentDidClose:        s.didClose,
		TextDocumentHover:           s.hover,
		TextDocumentDocumentSymbol:  s.documentSymbol,
		TextDocumentFoldingRange:    s.foldingRange,
		TextDocumentCompletion:      s.completion,
		TextDocumentDefinition:      s.definition,
		TextDocumentReferences:      s.references,
		TextDocumentFormatting:      s.formatting,
		TextDocumentRangeFormatting: s.rangeFormatting,
		TextDocumentPrepareRename:   s.prepareRename,
		TextDocumentRename:          s.rename,
	}
	return s
}

// Run starts the LSP server on stdio. The glsp logging goes to stderr by
// default (commonlog's default sink), keeping stdout reserved for JSON-RPC
// traffic.
func (s *Server) Run() error {
	d := &dispatcher{server: s}
	return glspserver.NewServer(d, ServerName, false).RunStdio()
}

// dispatcher implements glsp.Handler. It serves yaml/addressAtPoint inline
// and delegates everything else to the standard protocol handler.
type dispatcher struct {
	server *Server
}

func (d *dispatcher) Handle(ctx *glsp.Context) (any, bool, bool, error) {
	if ctx.Method == "yaml/addressAtPoint" {
		return d.handleAddressAtPoint(ctx)
	}
	return d.server.handler.Handle(ctx)
}

func (d *dispatcher) handleAddressAtPoint(ctx *glsp.Context) (any, bool, bool, error) {
	if !d.server.handler.IsInitialized() {
		return nil, true, true, errors.New("server not initialized")
	}
	var p AddressAtPointParams
	if err := json.Unmarshal(ctx.Params, &p); err != nil {
		return nil, true, false, err
	}
	text, ok := d.server.documents.Get(p.TextDocument.URI)
	if !ok {
		return nil, true, true, errors.New("document not synced")
	}
	res, err := AddressAtPoint(text, int(p.Position.Line), int(p.Position.Character), p.Format)
	if err != nil && isInvalidParamsErr(err) {
		// Map InvalidParams-class errors to validParams=false so glsp emits
		// JSON-RPC code -32602 (CodeInvalidParams) per the spec's I/O Matrix.
		return nil, true, false, err
	}
	return res, true, true, err
}

func isInvalidParamsErr(err error) bool {
	var unsupported *format.ErrUnsupportedFormat
	if errors.As(err, &unsupported) {
		return true
	}
	var badPos *ErrInvalidPosition
	return errors.As(err, &badPos)
}

func (s *Server) initialize(_ *glsp.Context, params *protocol.InitializeParams) (any, error) {
	s.config = parseInitOptions(params.InitializationOptions)
	syncKind := protocol.TextDocumentSyncKindFull
	version := buildinfo.Version
	return protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: syncKind,
			// triggerCharacters: ["*"] makes the client request
			// completion the moment a `*` is typed, so alias-name
			// suggestions appear without an explicit keystroke.
			CompletionProvider: &protocol.CompletionOptions{
				TriggerCharacters: []string{"*"},
			},
			HoverProvider: true,
			DocumentSymbolProvider:          true,
			FoldingRangeProvider:            true,
			DefinitionProvider:              true,
			ReferencesProvider:              true,
			DocumentFormattingProvider:      true,
			DocumentRangeFormattingProvider: true,
			// prepareProvider=true tells the client to issue
			// `textDocument/prepareRename` before opening the rename
			// prompt, so it can pre-fill the placeholder with the
			// current symbol name and bail out cleanly off-symbol.
			RenameProvider: protocol.RenameOptions{
				PrepareProvider: boolPtr(true),
			},
		},
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    ServerName,
			Version: &version,
		},
	}, nil
}

func (s *Server) initialized(_ *glsp.Context, _ *protocol.InitializedParams) error { return nil }
func (s *Server) shutdown(_ *glsp.Context) error                                    { return nil }
func (s *Server) setTrace(_ *glsp.Context, _ *protocol.SetTraceParams) error        { return nil }

func boolPtr(b bool) *bool { return &b }

// parseInitOptions extracts the server-side Config from the raw
// `initializationOptions` payload sent by the client. The glsp protocol
// delivers it as `any` (typically map[string]any after JSON decoding); we
// re-marshal/unmarshal so a typed struct does the conversion.
//
// Unknown fields are ignored; malformed payloads degrade silently to defaults
// rather than aborting initialize — a misconfigured editor should not break
// the whole LSP session over a formatting preference.
func parseInitOptions(raw any) Config {
	cfg := Config{}
	if raw == nil {
		return cfg
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return cfg
	}
	var opts initOptions
	if err := json.Unmarshal(b, &opts); err != nil {
		return cfg
	}
	cfg.Format.NormalizeStrings = opts.Format.NormalizeStrings
	cfg.Format.Indentation = resolveIndentation(opts.Format.Indentation)
	return cfg
}

// resolveIndentation maps the polymorphic `indentation` field — string
// "detect", numeric value, or absent — to the integer the formatter expects.
// Returns 0 for "detect"/absent (formatter will infer from source) and the
// rounded integer for any positive numeric. Negative or zero numerics are
// treated as "detect" so a misconfigured client doesn't produce broken
// no-indent output.
func resolveIndentation(v any) int {
	switch x := v.(type) {
	case nil:
		return 0
	case string:
		// "detect" or anything else string-valued → infer.
		return 0
	case float64:
		// JSON numbers decode to float64 through encoding/json.
		if x >= 1 {
			return int(x)
		}
		return 0
	case int:
		if x >= 1 {
			return x
		}
		return 0
	}
	return 0
}
