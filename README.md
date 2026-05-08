# yaml-lsp

[![Go](https://github.com/psycofdj/yaml-lsp/actions/workflows/go.yml/badge.svg)](https://github.com/psycofdj/yaml-lsp/actions/workflows/go.yml)
[![Emacs](https://github.com/psycofdj/yaml-lsp/actions/workflows/emacs.yml/badge.svg)](https://github.com/psycofdj/yaml-lsp/actions/workflows/emacs.yml)
[![License: GPL-3.0-or-later](https://img.shields.io/badge/license-GPL--3.0--or--later-blue.svg)](LICENSE)

A YAML language server in Go. Speaks LSP over stdio with the usual
capabilities (hover, diagnostics, outline, folding, formatting,
completion) and adds two features that are uncommon in the YAML LSP
landscape:

- **Anchor-as-identifier navigation** — `&anchor` / `*alias` are
  treated as a symbol pair, so go-to-definition, find-references, and
  rename all work on them like they would on a variable in any other
  language.
- **Structural address-at-point** — exposes the address of the node
  under the cursor in four tool-specific formats (jsonpath, bosh-ops,
  jsonpatch, helm-values), both as a hover info-bar and as a custom
  LSP request (`yaml/addressAtPoint`) consumed by an Emacs client.

The same parser/locate pipeline that powers the server is also
reachable as a `yaml-lsp address` CLI subcommand for scripts and
debugging.

## Install

Prebuilt binaries for Linux, macOS, and Windows (amd64 + arm64) are
attached to every GitHub release:
<https://github.com/psycofdj/yaml-lsp/releases>.

From source:

```sh
go install github.com/psycofdj/yaml-lsp/cmd/yaml-lsp@latest
```

Verify:

```sh
yaml-lsp version
```

## Run as a language server

```sh
yaml-lsp           # equivalent to `yaml-lsp serve`
yaml-lsp serve     # explicit
```

The server reads JSON-RPC on stdin / writes on stdout, logs to stderr,
and declares full text-document sync. Wire it into any LSP client —
the [Emacs client](#emacs) bundled in this repo, VS Code's generic
LSP bridge, Neovim's `nvim-lspconfig`, etc.

## LSP capabilities

| Capability | Method(s) | Notes |
|---|---|---|
| Hover | `textDocument/hover` | Markdown table of the node's address in all four formats. |
| Outline | `textDocument/documentSymbol` | **Flat list of JSONPaths** (e.g. `$.spec.containers[0].image`), one entry per node, tagged Object/Array/String/Number/Boolean/Null. Renders as a one-row-per-path index in `lsp-ui-imenu` / VS Code outline / `lsp-treemacs-symbols`. Multi-doc streams prefix `[N]`. |
| Folding | `textDocument/foldingRange` | One fold per multi-line mapping entry / sequence element. Flow style not folded. |
| Diagnostics | `textDocument/publishDiagnostics` | Pushed on every `didOpen` / `didChange`; parse errors surface at the offending token. |
| Completion | `textDocument/completion` | `*name` alias-name completion — every `&name` in the same document, triggered on `*`. Schema-aware key/value completion is out of scope. |
| Definition | `textDocument/definition` | Symmetric: from `*name` jumps to `&name`; from `&name` returns its own location. |
| References | `textDocument/references` | All `*name` aliases (plus `&name` when `includeDeclaration=true`). |
| Rename | `textDocument/rename` + `prepareRename` | Atomic rename of `&name` + every `*name` in the document. Sigils preserved; names validated. |
| Formatting | `textDocument/formatting` + `rangeFormatting` | Conservative whitespace-only: trim trailing whitespace, collapse trailing blank lines, ensure single final newline. No AST roundtrip, so comments and quote style are preserved. |
| Address (custom) | `yaml/addressAtPoint` | Single-format address-of-node query — see [below](#yamladdressatpoint-custom-request). |

**Document-local scope** for anchor features. YAML anchors are
document-local per spec, so a `*foo` in document 2 of a multi-doc
stream cannot bind to a `&foo` in document 1, and `references` /
`rename` operate within one document.

## `yaml/addressAtPoint` (custom request)

The four format encoders, given a cursor position:

| Format        | Example                          |
|---------------|----------------------------------|
| `jsonpath`    | `$.metadata.name`                |
| `bosh-ops`    | `/instance_groups/name=web/jobs` |
| `jsonpatch`   | `/spec/containers/0/image`       |
| `helm-values` | `image.repository`               |

```text
Method:  yaml/addressAtPoint
Params:  {
  textDocument: TextDocumentIdentifier,
  position:     { line, character },  # 0-based, UTF-16 (LSP convention)
  format:       "jsonpath" | "bosh-ops" | "jsonpatch" | "helm-values",
}
Result:  {
  path:          string,    # empty when nodeKind == "none"
  format:        string,    # echoes the requested format
  documentIndex: int,       # 0-based; the doc containing the cursor
  nodeKind:      "key" | "value" | "none",
}
```

### Format encoding details

| Format        | Escaping                                                                |
|---------------|--------------------------------------------------------------------------|
| `jsonpath`    | non-identifier keys bracket-quoted; `\` and `'` backslash-escaped per RFC 9535 §2.3.1.2; `]` passes through literally |
| `jsonpatch`   | RFC 6901 JSON Pointer (also used by JSON Patch / kustomize json6902): `~` → `~0`, `/` → `~1` |
| `helm-values` | backslash-escape `,` `.` `=` `\` so `--set` parses correctly             |
| `bosh-ops`    | `/` `?` `#` percent-encoded                                              |

**BOSH-ops sequence elements** prefer the name-keyed form
(`/containers/name=web/...`) when the addressed sequence element is a
mapping with a scalar `name` field; they fall back to the numeric form
(`/containers/0/...`) otherwise.

**Multi-document streams**: the returned `path` is relative to the
document containing the cursor; `documentIndex` carries the 0-based
position in the stream (matching `yq`'s convention).

**UTF-8 BOM** is stripped transparently — column 1 of line 1 is always
the first non-BOM byte, in both the LSP server and the CLI. A
`Position.character` past EOL is clamped. A non-UTF-8 byte on the
cursor's line surfaces as JSON-RPC `-32602`.

## `yaml-lsp address` (CLI)

The same address pipeline as the LSP custom request, accessible from
the shell for scripts and debugging:

```sh
yaml-lsp address [--line N] [--column N] [--format FORMAT] [--json] FILE
```

```sh
$ yaml-lsp address --line 4 --column 9 --format jsonpath testdata/single.yaml
$.metadata.name

$ yaml-lsp address --line 3 --column 13 --format bosh-ops testdata/sequences.yaml
/containers/name=web/image

$ yaml-lsp address --line 9 --column 10 --format jsonpath --json testdata/multidoc.yaml
{"path":"$.metadata.name","format":"jsonpath","documentIndex":1,"nodeKind":"value"}
```

Lines and byte columns are 1-based (matching the goccy/go-yaml
parser's convention, not LSP's). `--json` returns the same shape the
LSP custom request does.

## Emacs

A single-file lsp-mode client lives at [`emacs/yaml-lsp.el`](emacs/yaml-lsp.el).

```elisp
(add-to-list 'load-path "/path/to/yaml-lsp/emacs")
(require 'yaml-lsp)
```

Open a YAML file, `M-x lsp`. If the server binary isn't on `$PATH`,
lsp-mode offers `M-x lsp-install-server RET yaml-lsp RET` — that
downloads the release archive matching the host platform
(linux/darwin/windows × amd64/arm64) and installs it under
`lsp-server-install-dir`. No Go toolchain required.

Once the session is up, every capability above is reachable through
lsp-mode's standard keybindings:

| Action | Command |
|---|---|
| Go to anchor definition | `M-.` (`xref-find-definitions`) |
| Find anchor references | `M-?` (`xref-find-references`) |
| Rename anchor | `M-x lsp-rename` |
| Format buffer / region | `M-x lsp-format-buffer` / `lsp-format-region` |
| Show address (hover) | `M-x lsp-describe-thing-at-point` |

Plus two project-specific commands:

| Command | Effect |
|---|---|
| `M-x yaml-lsp-copy-address-at-point` | Push the structural address of the node at point onto the kill ring (format from `yaml-lsp-address-format`). |
| `M-x yaml-lsp-which-func-mode` | Show the address-at-point in the `which-function-mode` modeline. Enable via `(add-hook 'yaml-mode-hook #'yaml-lsp-which-func-mode)` + `(which-function-mode 1)`. |

### Customization

`M-x customize-group RET yaml-lsp RET`:

| Variable | Default | Purpose |
|---|---|---|
| `yaml-lsp-server-executable` | `"yaml-lsp"` | Name or absolute path of the server binary; bare names resolve via `$PATH`. |
| `yaml-lsp-address-format` | `"jsonpath"` | Address format requested by `yaml-lsp-copy-address-at-point` and the `which-func` modeline. |
| `yaml-lsp-additional-major-modes` | `'(yaml-mode yaml-ts-mode)` | Modes for which `yaml-lsp` should activate. Add `k8s-mode`, `ansible`, etc. then `M-x yaml-lsp-reload`. |
| `yaml-lsp-server-version` | (current release) | Tag to fetch via `lsp-install-server`. |

After changing the major-modes list, run `M-x yaml-lsp-reload` to
re-register the client.

## Development

```sh
go build ./...                        # build server + CLI
go test ./... -race -count=1          # full test suite
golangci-lint run ./...               # lint
goreleaser release --snapshot --clean # cross-compile all release archives locally

cd emacs && make compile && make test # elisp byte-compile + ert tests
```

CI runs the Go and Emacs sides separately
([`.github/workflows/go.yml`](.github/workflows/go.yml),
[`.github/workflows/emacs.yml`](.github/workflows/emacs.yml)) with
path filters so only the relevant suite fires per PR. Tagged pushes
(`v*`) trigger
[`release.yml`](.github/workflows/release.yml), which runs
goreleaser and publishes archives + checksums.

### Module layout

```
cmd/yaml-lsp/         entrypoint (cobra: serve / version / address)
internal/buildinfo/   single source of truth for the build version
internal/parser/      goccy/go-yaml wrapper, locate-at-position, anchor walks
internal/path/        shared Path + Segment + NodeKind types
internal/format/      jsonpath, bosh-ops, jsonpatch, helm-values encoders;
                      conservative whitespace formatter
internal/server/      glsp wiring, document store, all LSP method handlers
internal/cli/         `address` subcommand
emacs/                lsp-mode client (yaml-lsp.el)
testdata/             YAML fixtures
```

The four format encoders are pure functions of `path.Path`; the AST
is owned by `internal/parser` and never crosses the package boundary.
Adding a fifth format is a one-file change in `internal/format/`.

## License

GPL-3.0-or-later. See [LICENSE](LICENSE).
