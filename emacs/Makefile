EMACS ?= emacs

# Pull in user-installed packages (lsp-mode and its transitive deps:
# dash, f, ht, spinner, markdown-mode, …) by initializing the package
# system. Set LSP_MODE_DIR to override or supplement the load-path
# when lsp-mode lives outside ELPA. Single-quoted --eval keeps the
# Lisp form intact across shell word-splitting in Make.
EMACSFLAGS = -Q --batch -L . \
  --eval='(progn (require (quote package)) (package-initialize))'

ifneq ($(strip $(LSP_MODE_DIR)),)
EMACSFLAGS += -L $(LSP_MODE_DIR)
endif

EL_FILES  = yaml-lsp.el yaml-lsp-tests.el
ELC_FILES = $(EL_FILES:.el=.elc)

.PHONY: all compile test clean check-lsp lint checkdoc package-lint

all: compile

check-lsp:
	@$(EMACS) $(EMACSFLAGS) \
	  --eval='(unless (locate-library "lsp-mode") (message "ERROR: lsp-mode not found. Install via M-x package-install RET lsp-mode RET, or set LSP_MODE_DIR=/path/to/lsp-mode") (kill-emacs 1))'

compile: check-lsp $(ELC_FILES)

# yaml-lsp-tests.el (require 'yaml-lsp) needs the library byte-compiled
# first under -j parallel builds.
yaml-lsp-tests.elc: yaml-lsp.elc

%.elc: %.el
	$(EMACS) $(EMACSFLAGS) -f batch-byte-compile $<

test: check-lsp $(ELC_FILES)
	$(EMACS) $(EMACSFLAGS) -l yaml-lsp-tests.elc -f ert-run-tests-batch-and-exit

clean:
	rm -f $(ELC_FILES)

# MELPA-readiness lints. `checkdoc` is built into Emacs; `package-lint'
# is installed from MELPA on demand via `package-install'. Both run
# against yaml-lsp.el (the publishable file) only — yaml-lsp-tests.el
# is not part of the distributed package.
lint: checkdoc package-lint

checkdoc:
	$(EMACS) $(EMACSFLAGS) --eval='(checkdoc-file "yaml-lsp.el")'

package-lint:
	$(EMACS) $(EMACSFLAGS) \
	  --eval='(progn (add-to-list (quote package-archives) (cons "melpa" "https://melpa.org/packages/")) (package-initialize) (unless (package-installed-p (quote package-lint)) (package-refresh-contents) (package-install (quote package-lint))))' \
	  -l package-lint -f package-lint-batch-and-exit yaml-lsp.el
