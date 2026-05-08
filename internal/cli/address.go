// Package cli wires the `yaml-lsp address` debug subcommand. It reuses the
// parser/locate/format pipeline directly without the LSP layer.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/psycofdj/yaml-lsp/internal/format"
	"github.com/psycofdj/yaml-lsp/internal/parser"
	"github.com/psycofdj/yaml-lsp/internal/path"
)

type addressFlags struct {
	line   int
	column int
	format string
	jsonOut bool
}

// NewAddressCommand returns a cobra command suitable for attaching to the
// `yaml-lsp` root.
func NewAddressCommand() *cobra.Command {
	var f addressFlags
	cmd := &cobra.Command{
		Use:   "address [flags] FILE",
		Short: "Print the structural address of the YAML node at FILE:LINE:COLUMN",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAddress(cmd.OutOrStdout(), args[0], f)
		},
	}
	cmd.Flags().IntVar(&f.line, "line", 1, "1-based line number")
	cmd.Flags().IntVar(&f.column, "column", 1, "1-based byte column")
	cmd.Flags().StringVar(&f.format, "format", "jsonpath",
		"output format: jsonpath | bosh-ops | jsonpatch | helm-values")
	cmd.Flags().BoolVar(&f.jsonOut, "json", false,
		"emit a JSON object with path, format, documentIndex, nodeKind")
	return cmd
}

type cliResult struct {
	Path          string `json:"path"`
	Format        string `json:"format"`
	DocumentIndex int    `json:"documentIndex"`
	NodeKind      string `json:"nodeKind"`
}

func runAddress(stdout io.Writer, filename string, f addressFlags) error {
	if !format.IsSupported(f.format) {
		return &format.ErrUnsupportedFormat{Got: f.format, Supported: format.SupportedFormats()}
	}
	src, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	// ParseStream already strips a leading BOM; the explicit strip here is
	// a small defensive guard so that any future code added between read
	// and parse (e.g. logging the byte length, hashing, etc.) also sees
	// the post-strip content. CLI columns are user-provided; the BOM does
	// not affect column counting.
	src = parser.StripBOM(src)
	docs, err := parser.ParseStream(src)
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}
	if len(docs) == 0 {
		return fmt.Errorf("no documents in %s", filename)
	}
	p, kind, idx, _ := parser.Locate(docs, f.line, f.column)
	encoded := ""
	if kind != path.NodeKindNone && len(p) > 0 {
		encoded, err = format.Encode(p, f.format)
		if err != nil {
			return err
		}
	}
	if f.jsonOut {
		enc := json.NewEncoder(stdout)
		return enc.Encode(cliResult{
			Path:          encoded,
			Format:        f.format,
			DocumentIndex: idx,
			NodeKind:      string(kind),
		})
	}
	_, err = fmt.Fprintln(stdout, encoded)
	return err
}
