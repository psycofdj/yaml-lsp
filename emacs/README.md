# yaml-lsp.el

[![Emacs](https://github.com/psycofdj/yaml-lsp/actions/workflows/emacs.yml/badge.svg)](https://github.com/psycofdj/yaml-lsp/actions/workflows/emacs.yml)

Single-file [`lsp-mode`](https://emacs-lsp.github.io/lsp-mode/) client
for the [yaml-lsp server](../README.md). Registering the client is
enough to get the server's full LSP surface — hover, diagnostics,
outline, folding, formatting, completion, anchor-aware definition /
references / rename — through lsp-mode's standard keybindings.

On top of that, the client adds a handful of project-specific
commands that wrap the server's `yaml/addressAtPoint` custom request
and the standard `textDocument/foldingRange` request with
ergonomics that lsp-mode doesn't ship out of the box:

- **Structural address-at-point** — copy the node's address (jsonpath
  / bosh-ops / jsonpatch / helm-values) to the kill ring, or display
  it live in the `which-function-mode` modeline.
- **Element / region folding** — fold and unfold the
  server-reported ranges with overlays, no `hs-minor-mode` setup
  required.

The client is GPL-3.0-or-later. It is a thin elisp layer; all
intelligence lives in the Go server.

## Install

### From MELPA

(Pending acceptance — once merged.) `M-x package-install RET yaml-lsp RET`.

### From source

```elisp
(add-to-list 'load-path "/path/to/yaml-lsp/emacs")
(require 'yaml-lsp)
```

### Server binary

The client tries `yaml-lsp` on `$PATH` first and falls back to a
download cache. If neither is populated, the first `M-x lsp` in a
YAML buffer offers:

```
M-x lsp-install-server RET yaml-lsp RET
```

This downloads the goreleaser-produced archive for the host platform
(linux / darwin / windows × amd64 / arm64) from the GitHub release
matching `yaml-lsp-server-version`, and extracts the binary into
`lsp-server-install-dir`. No Go toolchain needed.

For other platforms (32-bit, BSD), build from source:

```sh
go install github.com/psycofdj/yaml-lsp/cmd/yaml-lsp@latest
```

### Activation

Open a YAML file, `M-x lsp`. The client registers against
`yaml-mode` and `yaml-ts-mode` by default; extend via
`yaml-lsp-additional-major-modes` (then `M-x yaml-lsp-reload`).

The client registers with `:priority 2` and a negative-priority
fallback intent — a user's existing generic YAML server (e.g.
RedHat's `yaml-language-server`) will be picked by lsp-mode if both
are installed. Set `lsp-disabled-clients` to exclude the other and
force `yaml-lsp` to win.

## User documentation

### Standard lsp-mode commands

Once the workspace is up, the server's capabilities are reachable
through lsp-mode's normal bindings:

| Action                       | Command                                       |
|------------------------------|-----------------------------------------------|
| Go to anchor definition      | `M-.` (`xref-find-definitions`)               |
| Find anchor references       | `M-?` (`xref-find-references`)                |
| Rename anchor                | `M-x lsp-rename`                              |
| Format buffer / region       | `M-x lsp-format-buffer` / `lsp-format-region` |
| Show address (hover)         | `M-x lsp-describe-thing-at-point`             |
| Document outline             | `M-x lsp-ui-imenu` / `lsp-treemacs-symbols`   |
| Complete `*alias` after `*`  | (automatic in `company`/`corfu`)              |

### Project-specific commands

| Command                              | Effect                                                                                                |
|--------------------------------------|-------------------------------------------------------------------------------------------------------|
| `M-x yaml-lsp-copy-address-at-point` | Push the structural address of the node at point onto the kill ring (format from `yaml-lsp-address-format`). |
| `M-x yaml-lsp-which-func-mode`       | Buffer-local minor mode: feed the address-at-point into `which-function-mode`'s modeline.             |
| `M-x yaml-lsp-element-fold`          | Fold the YAML element at point — cursor on a key folds *that key's* children; on a value line, folds the innermost containing block. |
| `M-x yaml-lsp-element-unfold`        | Unfold the element at point (symmetric with `yaml-lsp-element-fold`).                                 |
| `M-x yaml-lsp-element-toggle`        | Toggle the fold state of the element at point.                                                        |
| `M-x yaml-lsp-region-fold`           | Fold every server-reported range fully contained in the active region.                                |
| `M-x yaml-lsp-region-unfold`         | Unfold every yaml-lsp fold overlapping the active region.                                             |
| `M-x yaml-lsp-region-toggle`         | Unfold the region if any fold overlaps it; otherwise fold every range contained in it.                |

The fold commands talk to the server via `textDocument/foldingRange`
and render the result with overlays tagged `yaml-lsp-fold` — they do
not require `hs-minor-mode`.

Enabling the modeline address:

```elisp
(add-hook 'yaml-mode-hook #'yaml-lsp-which-func-mode)
(add-hook 'yaml-ts-mode-hook #'yaml-lsp-which-func-mode)
(which-function-mode 1)
```

Queries are asynchronous and cached by buffer position; the modeline
refreshes when the response arrives.

## API

### Defcustoms

`M-x customize-group RET yaml-lsp RET`:

| Variable                            | Default              | Purpose                                                                                                                                            |
|-------------------------------------|----------------------|----------------------------------------------------------------------------------------------------------------------------------------------------|
| `yaml-lsp-server-executable`        | `"yaml-lsp"`         | Name or absolute path of the server binary; bare names resolve via `$PATH`.                                                                        |
| `yaml-lsp-server-version`           | (current release)    | Tag (without leading `v`) to fetch via `lsp-install-server`. Must match a published GitHub release.                                                |
| `yaml-lsp-server-store-path`        | `<lsp-cache>/yaml-lsp/yaml-lsp` | Where `lsp-install-server` lands the downloaded binary. The Emacs client appends `.exe` on Windows.                                     |
| `yaml-lsp-additional-major-modes`   | `'(yaml-mode yaml-ts-mode)` | Modes for which `yaml-lsp` should activate. After changing, run `M-x yaml-lsp-reload`.                                                       |
| `yaml-lsp-address-format`           | `"jsonpath"`         | One of `"jsonpath"` / `"bosh-ops"` / `"jsonpatch"` / `"helm-values"`. Used by `yaml-lsp-copy-address-at-point` and the `which-func` modeline.       |
| `yaml-lsp-format-indentation`       | `"detect"`           | `"detect"` (server infers from source) or a positive integer (exact spaces per level). Sent in `initializationOptions`; restart workspace to apply. |
| `yaml-lsp-format-normalize-strings` | `nil`                | When non-nil, the server strips unnecessary quotes from string scalars (`key: "hello"` → `key: hello`) on format. Restart workspace to apply.       |

The `:set` functions on `yaml-lsp-server-executable`,
`yaml-lsp-format-indentation` and `yaml-lsp-format-normalize-strings`
print a one-shot hint when the value is changed mid-session,
recommending `M-x lsp-restart-workspace`. The hint fires through
`customize-set-variable` / `M-x customize` / `custom-set-variables`;
bare `setq` / `setq-default` bypass it.

### Interactive commands

| Command                              | Args  | Returns                                                                                       |
|--------------------------------------|-------|-----------------------------------------------------------------------------------------------|
| `yaml-lsp-reload`                    | —     | Re-register the client with `lsp-mode`. Run after changing `yaml-lsp-additional-major-modes`. |
| `yaml-lsp-copy-address-at-point`     | —     | Async; pushes the address to the kill ring and echoes it. Errors land in `*Messages*`.        |
| `yaml-lsp-which-func-mode`           | —     | Buffer-local minor mode toggling the `which-func-functions` hook entry.                       |
| `yaml-lsp-element-fold` / `-unfold` / `-toggle` | —              | Synchronous. Operate on the innermost folding range at point.                      |
| `yaml-lsp-region-fold` / `-unfold` / `-toggle`  | `beg` `end` (`"r"`) | Synchronous. Operate on every range fully contained in the region.            |

### Custom LSP request

The client uses the server's `yaml/addressAtPoint` custom request
for both `yaml-lsp-copy-address-at-point` and the `which-func`
integration. Request and response shape are documented in the
[server README](../README.md#yamladdressatpoint-custom-request).

### lsp-mode internals used

The client touches a handful of lsp-mode private (`lsp--…`) symbols
that have no public equivalent at the time of writing:

| Symbol                                          | Used by                                                                  |
|-------------------------------------------------|--------------------------------------------------------------------------|
| `lsp--text-document-identifier`, `lsp--cur-position` | `yaml-lsp-copy-address-at-point`, `yaml-lsp--which-func-fetch`     |
| `lsp--get-folding-ranges`, `lsp--get-current-innermost-folding-range`, `lsp--folding-range-beg`, `lsp--folding-range-end` | Fold commands |

If lsp-mode renames or removes any of these, the affected command
will fail with a clear `void-function` error rather than silently
producing wrong output.

## Development

```sh
make compile        # byte-compile yaml-lsp.el + yaml-lsp-tests.el
make test           # run the ert suite
make lint           # checkdoc + package-lint (MELPA readiness)
make clean
```

`lsp-mode` and its transitive deps must be installed via `package.el`
(the `Makefile` does `(package-initialize)` before compiling). Override
the load-path with `LSP_MODE_DIR=/path/to/lsp-mode make compile` when
running outside a normal Emacs configuration.

CI runs the same compile + ert targets on every push under
[`emacs.yml`](../.github/workflows/emacs.yml), with path filters so
the workflow only fires when files under `emacs/` change.

## License

GPL-3.0-or-later. See [LICENSE](../LICENSE).
