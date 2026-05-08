// Package buildinfo carries the single source of truth for the
// yaml-lsp build version.
//
// `Version` defaults to "dev" for ad-hoc `go build` / `go install`
// invocations.  Released binaries override it at link time via
// goreleaser's ldflag injection (see `.goreleaser.yaml`):
//
//	-X github.com/psycofdj/yaml-lsp/internal/buildinfo.Version={{.Version}}
//
// goreleaser's `.Version` is the tag without the leading `v`, so a
// `v1.2.3` tag becomes `Version="1.2.3"`. The CLI's `version` command
// and the LSP `initialize` response both read this value.
package buildinfo

// Version is the semver of this build. Overridden at link time on
// released binaries; "dev" otherwise.
var Version = "dev"
