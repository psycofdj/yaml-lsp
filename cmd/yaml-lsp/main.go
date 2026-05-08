// Command yaml-lsp runs the YAML language server (default) or the address
// debug subcommand.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/psycofdj/yaml-lsp/internal/buildinfo"
	"github.com/psycofdj/yaml-lsp/internal/cli"
	"github.com/psycofdj/yaml-lsp/internal/server"
)

func main() {
	root := &cobra.Command{
		Use:           "yaml-lsp",
		Short:         "YAML language server with structural address-at-point",
		Version:       buildinfo.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return server.New().Run()
		},
	}
	root.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Run the LSP server over stdio",
		RunE: func(cmd *cobra.Command, args []string) error {
			return server.New().Run()
		},
	})
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the yaml-lsp version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), buildinfo.Version)
			return err
		},
	})
	root.AddCommand(cli.NewAddressCommand())

	if err := root.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "yaml-lsp:", err)
		os.Exit(1)
	}
}
