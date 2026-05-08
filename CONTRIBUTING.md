# Contributing to yaml-lsp

Thanks for considering a contribution. This project has two halves with
different toolchains:

- **Go server** in `cmd/yaml-lsp/` and `internal/`.
- **Emacs Elisp client** in `emacs/`.

## Quick checks before opening a PR

```sh
# Go
go build ./...
go vet ./...
go test ./... -race -count=1

# Emacs
cd emacs && make compile && make test
```

CI runs both on every PR (matrix: Go 1.22/1.23/1.25, Emacs 27.2/28.2/29.4).

## Project conventions

- **Format encoders are pure functions** of `internal/path.Path`. Adding a
  fifth format is a one-file change in `internal/format/`.
- **LSP boundary code lives only in `internal/server/`.** UTF-16 ↔ byte
  conversion, JSON-RPC error mapping, and document store concurrency stay
  there. The CLI never sees LSP types.
- **`internal/parser/`** wraps `goccy/go-yaml`. AST nodes never cross
  the package boundary.
- **The Elisp file is single-file** by design (lsp-pyright/lsp-rust
  convention). Keep all defcustoms prefixed `yaml-lsp-`.

## Adding a feature

1. Add the parser/format pieces if needed — pure functions, table-driven
   tests under the same package.
2. Wire the LSP handler in `internal/server/`. Update server capabilities
   in `initialize`.
3. The Emacs client picks up most LSP features automatically once
   advertised. New custom requests need a wrapper command in
   `emacs/yaml-lsp.el`.
4. Update `README.md` and add a row to the I/O matrix in any new spec.

## Bug reports

Please include:
- `yaml-lsp address` CLI invocation that reproduces (most LSP issues
  reproduce here without an editor).
- The smallest YAML fixture that triggers it.
- Output of `go version`.

## Releases

`yaml-lsp.el`'s `Version:` header is the canonical version. Tag releases
as `v<major>.<minor>.<patch>`; CI does not auto-publish.

## MELPA submission

The Elisp package is MELPA-compatible (lexical-binding, GPL-3+,
single-file, conventional headers). When ready to submit, add a recipe
to [melpa/melpa](https://github.com/melpa/melpa) under `recipes/yaml-lsp`:

```elisp
(yaml-lsp :fetcher github
          :repo "psycofdj/yaml-lsp"
          :files ("emacs/yaml-lsp.el"))
```

## License

By contributing, you agree your work is licensed under GPL-3.0-or-later
(see `LICENSE`).
