;;; yaml-lsp.el --- LSP client for the yaml-lsp server  -*- lexical-binding: t; -*-

;; Copyright (C) 2026  Xavier MARCELET

;; Author: Xavier MARCELET <xavier@marcelet.com>
;; URL: https://github.com/psycofdj/yaml-lsp
;; Version: 0.1.0
;; Package-Requires: ((emacs "27.1") (lsp-mode "8.0.0"))
;; Keywords: languages, tools

;; This program is free software; you can redistribute it and/or modify
;; it under the terms of the GNU General Public License as published by
;; the Free Software Foundation, either version 3 of the License, or
;; (at your option) any later version.
;;
;; This program is distributed in the hope that it will be useful,
;; but WITHOUT ANY WARRANTY; without even the implied warranty of
;; MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
;; GNU General Public License for more details.
;;
;; You should have received a copy of the GNU General Public License
;; along with this program.  If not, see <https://www.gnu.org/licenses/>.

;;; Commentary:

;; lsp-mode client for the yaml-lsp server
;; (https://github.com/psycofdj/yaml-lsp).
;;
;; Provides one interactive command, `yaml-lsp-copy-address-at-point',
;; which echoes the structural address of the YAML node at point in
;; one of four tool-specific formats (jsonpath, bosh-ops, jsonpatch,
;; helm-values).  The command is asynchronous and only runs when the
;; buffer's active workspace is `yaml-lsp'.
;;
;; The set of major modes that activate this client is configurable
;; via `yaml-lsp-additional-major-modes'.  After changing the value,
;; run `M-x yaml-lsp-reload' to re-register with lsp-mode.
;;
;; Formatting behavior is controlled by two defcustoms:
;; `yaml-lsp-format-indentation' ("detect" or a positive integer) and
;; `yaml-lsp-format-normalize-strings' (boolean).  Both are sent to
;; the server via `initializationOptions' at LSP startup; changing
;; them after the workspace is up requires `M-x lsp-restart-workspace'.
;;
;; Note on lsp-mode internals: this client uses
;; `lsp--text-document-identifier' and `lsp--cur-position', which are
;; lsp-mode private API (double-dash prefix).  No public equivalent
;; exists at the time of writing.  If lsp-mode renames or removes
;; them, the address command will fail with a clear `void-function'
;; error rather than silently producing wrong output.

;;; Code:

(require 'cl-lib)
(require 'lsp-mode)

(defconst yaml-lsp--supported-formats
  '("jsonpath" "bosh-ops" "jsonpatch" "helm-values")
  "Format strings the server accepts in `yaml/addressAtPoint' requests.")

(defgroup yaml-lsp nil
  "LSP client for the yaml-lsp server."
  :group 'lsp
  :prefix "yaml-lsp-")

(defun yaml-lsp--server-executable-changed (sym new)
  "Apply SYM=NEW and warn when changed during an active session."
  (set-default sym new)
  (when (lsp-find-workspace 'yaml-lsp nil)
    (message "yaml-lsp: server executable changed; M-x lsp-restart-workspace to apply")))

(defcustom yaml-lsp-server-executable "yaml-lsp"
  "Name of, or absolute path to, the yaml-lsp server binary.
The default \"yaml-lsp\" is resolved by `lsp-mode' via PATH.

Changing this value while a `yaml-lsp' workspace is active prints
a one-shot hint recommending `M-x lsp-restart-workspace'.  This
function does NOT auto-restart the workspace.

The hint fires via this defcustom's `:set' function, which Emacs
runs only when the value is changed through `customize-set-variable',
`M-x customize', or a Custom-style assignment in `custom-set-variables'.
A direct `setq' or `setq-default' bypasses `:set' and the hint does
not appear; in that case run `M-x lsp-restart-workspace' yourself."
  :type 'string
  :group 'yaml-lsp
  :set #'yaml-lsp--server-executable-changed)

(defcustom yaml-lsp-address-format "jsonpath"
  "Address format requested by `yaml-lsp-copy-address-at-point'.
Sent verbatim as the `format' field of the `yaml/addressAtPoint'
LSP custom request."
  :type '(choice (const "jsonpath")
                 (const "bosh-ops")
                 (const "jsonpatch")
                 (const "helm-values"))
  :group 'yaml-lsp)

(defcustom yaml-lsp-additional-major-modes '(yaml-mode yaml-ts-mode)
  "Major modes for which `yaml-lsp' should activate.
Read at registration time; changes take effect after running
`M-x yaml-lsp-reload'.  Common derived modes that work with the
server include `k8s-mode', `ansible', and `yaml-pro-mode'."
  :type '(repeat symbol)
  :group 'yaml-lsp)

(defcustom yaml-lsp-server-version "latest"
  "Version of the yaml-lsp server to fetch via `:download-server-fn'.
Either the literal string \"latest\" (resolved at install time by
following the GitHub `/releases/latest' redirect) or a tag without
the leading \"v\" published at
https://github.com/psycofdj/yaml-lsp/releases.  Release artifacts
are produced by goreleaser using
\"yaml-lsp_<version>_<os>_<arch>.<ext>\" naming; the URL builder
in `yaml-lsp--release-url' depends on this scheme."
  :type 'string
  :group 'yaml-lsp)

(defcustom yaml-lsp-server-store-path
  (expand-file-name "yaml-lsp/yaml-lsp"
                    (or (bound-and-true-p lsp-server-install-dir)
                        (expand-file-name ".cache/lsp/" user-emacs-directory)))
  "Filesystem location where the downloaded server binary is stored.
The Emacs client appends \".exe\" on Windows.  `lsp-download-install'
decompresses the release archive into the parent directory of this
path; the archive must contain the binary at its root so it lands
exactly here."
  :type 'string
  :group 'yaml-lsp)

(defun yaml-lsp--format-setting-changed (sym new)
  "Apply SYM=NEW and warn if a `yaml-lsp' workspace is currently active.
Formatting settings are read from `initializationOptions' at startup,
so a running server keeps its previous values until restarted.  The
hint suggests `M-x lsp-restart-workspace' as the way to apply changes
without restarting Emacs."
  (set-default sym new)
  (when (lsp-find-workspace 'yaml-lsp nil)
    (message "yaml-lsp: format setting changed; M-x lsp-restart-workspace to apply")))

(defcustom yaml-lsp-format-indentation "detect"
  "Indentation policy the server uses when formatting buffers.

Either the string \"detect\" (the default — the server infers the
indent width from the source buffer) or a positive integer naming
the exact number of spaces to use per nesting level.

This value is sent in `initializationOptions' at LSP startup; the
server treats anything other than a positive integer as \"detect\".
Changing this value while a `yaml-lsp' workspace is active prints a
one-shot hint recommending `M-x lsp-restart-workspace'.  The hint
only fires when the value is changed via `customize-set-variable',
`M-x customize', or a Custom-style assignment.  A direct `setq' or
`setq-default' bypasses `:set' and the hint does not appear; in
that case run `M-x lsp-restart-workspace' yourself."
  :type '(choice (const :tag "Detect from source" "detect")
                 (integer :tag "Fixed number of spaces"))
  :group 'yaml-lsp
  :set #'yaml-lsp--format-setting-changed)

(defcustom yaml-lsp-format-normalize-strings nil
  "When non-nil, the server removes unnecessary quotes from string scalars.
For example, `key: \"hello\"' becomes `key: hello' on format.  Strings
whose unquoted form would resolve to a non-string type (numbers,
booleans, null, timestamps) keep their quotes regardless.

This value is sent in `initializationOptions' at LSP startup.
Changing it while a `yaml-lsp' workspace is active prints a hint
recommending `M-x lsp-restart-workspace'; see
`yaml-lsp-format-indentation' for the same `:set' caveats."
  :type 'boolean
  :group 'yaml-lsp
  :set #'yaml-lsp--format-setting-changed)

(defun yaml-lsp--initialization-options ()
  "Build the `initializationOptions' payload for the LSP `initialize' request.
Return a plist consumed by `lsp-mode' and serialized as JSON for the
server.  Numeric `yaml-lsp-format-indentation' is sent as a JSON
number; the string \"detect\" is sent verbatim.  See the server's
`parseInitOptions' for the consumer side."
  (list :format
        (list :indentation
              (cond
               ((integerp yaml-lsp-format-indentation) yaml-lsp-format-indentation)
               (t "detect"))
              :normalizeStrings (if yaml-lsp-format-normalize-strings t :json-false))))

(defun yaml-lsp--platform ()
  "Return (OS . ARCH) for the current host, matching goreleaser names.
OS is one of \"linux\", \"darwin\", \"windows\"; ARCH is \"amd64\"
or \"arm64\".  Return nil when the host platform is not one of
the published targets."
  (let ((os (pcase system-type
              ('gnu/linux "linux")
              ('darwin "darwin")
              ('windows-nt "windows")))
        (arch (let ((c (downcase (or system-configuration ""))))
                (cond
                 ((string-match-p "\\b\\(aarch64\\|arm64\\)" c) "arm64")
                 ((string-match-p "\\b\\(x86_64\\|amd64\\)" c) "amd64")))))
    (and os arch (cons os arch))))

(defun yaml-lsp--release-url (version os arch)
  "Return the goreleaser-produced asset URL for VERSION on OS/ARCH.
Format must stay in lockstep with `.goreleaser.yaml''s
`name_template' and `format_overrides'."
  (format "https://github.com/psycofdj/yaml-lsp/releases/download/v%s/yaml-lsp_%s_%s_%s.%s"
          version version os arch
          (if (string= os "windows") "zip" "tar.gz")))

(defun yaml-lsp--url-reachable-p (url)
  "Return non-nil when an HTTP HEAD on URL gives a 2xx status.
Uses curl with `--fail' so HTTP errors (e.g. 404 from a missing
release asset) surface as a non-zero exit, unlike
`lsp--download-file' which passes `--silent' without `--fail' and
silently writes the error body to the destination path."
  (zerop (call-process "curl" nil nil nil
                       "--silent" "--location" "--fail" "--head"
                       "--max-time" "10" url)))

(defconst yaml-lsp--releases-latest-url
  "https://github.com/psycofdj/yaml-lsp/releases/latest"
  "GitHub `/releases/latest' endpoint that 302-redirects to the current tag.")

(defun yaml-lsp--resolve-latest-version ()
  "Resolve the tag of the most recent yaml-lsp release to a bare version.
Follows the GitHub `/releases/latest' redirect with curl and reads
the effective URL (which points to `/releases/tag/v<version>').
Return the version string without the leading \"v\", or nil on
failure (network error, curl missing, unexpected redirect target)."
  (with-temp-buffer
    (let ((status (call-process
                   "curl" nil t nil
                   "--silent" "--location" "--fail" "--head"
                   "--max-time" "10"
                   "--write-out" "\nEFFECTIVE:%{url_effective}\n"
                   yaml-lsp--releases-latest-url)))
      (when (zerop status)
        (goto-char (point-min))
        (when (re-search-forward "^EFFECTIVE:\\(.*\\)$" nil t)
          (let ((effective (match-string 1)))
            (and (string-match "/releases/tag/v\\([^/\r\n]+\\)" effective)
                 (match-string 1 effective))))))))

(defun yaml-lsp--install-server (_client callback error-callback &rest _)
  "Download a prebuilt yaml-lsp server binary from GitHub releases.
Calls `lsp-download-install' with the goreleaser-produced archive
for the host platform (linux/darwin/windows × amd64/arm64).  The
archive is decompressed into the parent of
`yaml-lsp-server-store-path' — the binary ends up at that path,
which the connection lambda then prefers over PATH.  CALLBACK is
invoked on success.

ERROR-CALLBACK is invoked with an explicit message when:
- the host platform has no published release (e.g. 32-bit, BSD),
- `curl' is not on PATH,
- `yaml-lsp-server-version' is \"latest\" and the GitHub redirect
  cannot be resolved (offline, rate-limited, no releases),
- a HEAD probe of the release URL fails (typically because
  `yaml-lsp-server-version' does not match a published release;
  `lsp-download-install' alone would otherwise treat the GitHub
  404 body as a successful download and silently produce a broken
  tarball)."
  (let ((platform (yaml-lsp--platform)))
    (cond
     ((not platform)
      (funcall error-callback
               (format "yaml-lsp: no prebuilt binary for system-type=%s system-configuration=%s; install with `go install github.com/psycofdj/yaml-lsp/cmd/yaml-lsp@latest'"
                       system-type system-configuration)))
     ((not (executable-find "curl"))
      (funcall error-callback "yaml-lsp: `curl' is required to download the server binary"))
     (t
      (let* ((os (car platform))
             (arch (cdr platform))
             (windowsp (string= os "windows"))
             (store-path (concat yaml-lsp-server-store-path (if windowsp ".exe" "")))
             (latestp (string= yaml-lsp-server-version "latest"))
             (version (if latestp
                          (yaml-lsp--resolve-latest-version)
                        yaml-lsp-server-version)))
        (cond
         ((and latestp (not version))
          (funcall error-callback
                   (format "yaml-lsp: could not resolve latest release tag via %s — set `yaml-lsp-server-version' to an explicit tag from https://github.com/psycofdj/yaml-lsp/releases"
                           yaml-lsp--releases-latest-url)))
         (t
          (let ((url (yaml-lsp--release-url version os arch)))
            (if (not (yaml-lsp--url-reachable-p url))
                (funcall error-callback
                         (format "yaml-lsp: release asset not reachable at %s — check that `yaml-lsp-server-version' (%S, resolved to %S) matches a published release at https://github.com/psycofdj/yaml-lsp/releases"
                                 url yaml-lsp-server-version version))
              (lsp--info "yaml-lsp: resolved server version to %s" version)
              (lsp-download-install
               callback error-callback
               :url url
               :decompress (if windowsp :zip :targz)
               :store-path store-path))))))))))

(defun yaml-lsp--server-command ()
  "Return the path to use when launching the yaml-lsp server.
Prefers `yaml-lsp-server-executable' when it resolves on PATH;
otherwise falls back to `yaml-lsp-server-store-path' (where
`yaml-lsp--install-server' places the downloaded binary) when
present.  If neither exists, returns the configured executable
name so `lsp-mode' surfaces a clear \"not found\" error."
  (or (executable-find yaml-lsp-server-executable)
      (let ((p (concat yaml-lsp-server-store-path
                       (if (eq system-type 'windows-nt) ".exe" ""))))
        (and (file-executable-p p) p))
      yaml-lsp-server-executable))

(defun yaml-lsp--register ()
  "Register (or re-register) the yaml-lsp client with `lsp-mode'.
Idempotent: clears any prior `yaml-lsp' entry from `lsp-clients'
before re-registering, so re-loading the file (`eval-buffer',
`package-reinstall') does not leave stale entries pointing at
previous closures."
  (when (hash-table-p lsp-clients)
    (remhash 'yaml-lsp lsp-clients))
  (lsp-register-client
   (make-lsp-client
    :new-connection (lsp-stdio-connection #'yaml-lsp--server-command)
    :major-modes yaml-lsp-additional-major-modes
    :server-id 'yaml-lsp
    ;; Negative priority so a user's existing generic YAML server
    ;; (e.g. RedHat's yaml-language-server) wins by default.  Users
    ;; who want this server can disable the other via
    ;; `lsp-disabled-clients'.
    :priority 2
    ;; Function (not literal value) so the payload re-reads the
    ;; defcustoms at every registration — i.e. `yaml-lsp-reload'
    ;; picks up new settings without a separate restart step.
    :initialization-options #'yaml-lsp--initialization-options
    :download-server-fn #'yaml-lsp--install-server)))

;;;###autoload
(defun yaml-lsp-reload ()
  "Re-register the yaml-lsp client with `lsp-mode'.
Run after customizing `yaml-lsp-additional-major-modes' or
`yaml-lsp-server-executable' so the change takes effect."
  (interactive)
  (yaml-lsp--register))

(yaml-lsp--register)

;;;###autoload
(defun yaml-lsp-copy-address-at-point ()
  "Copy the structural address of the YAML node at point.

Sends a `yaml/addressAtPoint' LSP custom request, asynchronously,
to the active `yaml-lsp' workspace, using the format configured
in `yaml-lsp-address-format'.  When the response arrives, the
resulting address is pushed onto the kill ring (so a subsequent
`yank' / \\[yank] will paste it) and echoed in the minibuffer.

Shows \"yaml-lsp: no node at point\" without altering the kill
ring when the cursor is not on a node.

The command does NOT block Emacs — slow servers do not freeze
the UI.  The response is bound to the originating buffer at
request time; the cursor may have moved by the time the kill
ring is updated, but the address is the one that was at point
when the command was invoked.

Note on lsp-mode internals: this command uses
`lsp--text-document-identifier' and `lsp--cur-position' (lsp-mode
private API).  Breakage on lsp-mode upgrades surfaces as a
`void-function' error rather than silent miscompute."
  (interactive)
  (unless buffer-file-name
    (user-error "Yaml-lsp: buffer is not visiting a file"))
  (unless (lsp-find-workspace 'yaml-lsp buffer-file-name)
    (user-error "Yaml-lsp: workspace not active in this buffer"))
  (unless (member yaml-lsp-address-format yaml-lsp--supported-formats)
    (user-error "Yaml-lsp: invalid address format %S" yaml-lsp-address-format))
  (let ((params (save-restriction
                  (widen)
                  (list :textDocument (lsp--text-document-identifier)
                        :position (lsp--cur-position)
                        :format yaml-lsp-address-format))))
    (lsp-request-async
     "yaml/addressAtPoint"
     params
     (lambda (response)
       (let ((path (and response (lsp-get response :path)))
             (kind (and response (lsp-get response :nodeKind))))
         (if (or (not (stringp path))
                 (string-empty-p path)
                 (equal kind "none"))
             (message "yaml-lsp: no node at point")
           (kill-new path)
           (message "%s" path))))
     ;; 'alive lets the response reach the originating buffer even if the
     ;; user has typed or moved the cursor since invocation. The address is
     ;; the one that was at point at request time, which is still the
     ;; useful answer. 'tick would silently drop the response on any edit
     ;; with no user feedback (poor UX for an interactive query).
     :mode 'alive
     :error-handler (lambda (err)
                      (message "yaml-lsp: server error: %s"
                               (yaml-lsp--format-error err))))))

(defun yaml-lsp--format-error (err)
  "Best-effort message extraction from an LSP error payload ERR.
Handles plist (`(:code N :message \"...\")`), hash-table
\(`{\"message\": \"...\"}`), record / cl-struct (jsonrpc-error
or similar), and falls back to printing the raw form."
  (cond
   ((stringp err) err)
   ((and (listp err) (plist-member err :message))
    (plist-get err :message))
   ((and (hash-table-p err) (gethash "message" err)))
   ((recordp err)
    (condition-case nil
        ;; jsonrpc-error stores the message in slot named `message`.
        (or (and (fboundp 'jsonrpc-error-message) (jsonrpc-error-message err))
            (format "%S" err))
      (error (format "%S" err))))
   (t (format "%S" err))))

;;;; ---------------------------------------------------------------------
;;;; which-func integration
;;;; ---------------------------------------------------------------------

;; `which-function-mode' wants a synchronous function on
;; `which-func-functions' that returns a string for the current point.
;; Our address query is asynchronous, so we keep a per-buffer cache:
;; the hook function returns the cached value; if `point' has moved
;; since the cache was filled and no request is in flight, the hook
;; kicks off a fresh async fetch. When the response arrives, the cache
;; is updated and the mode line is forced to re-render.

(defvar-local yaml-lsp--which-func-cache nil
  "Cons cell (POSITION . PATH) holding the most recent cached address.
PATH is a string or nil (no node at point).  POSITION is the buffer
`point' the cache was filled for.")

(defvar-local yaml-lsp--which-func-pending-pos nil
  "Buffer `point' for which a request is currently in flight, or nil.")

(defun yaml-lsp--which-func-refresh-modeline (path)
  "Propagate PATH to `which-function-mode' state for the current buffer.

`which-function-mode' is timer-driven: it calls `which-function'
once and stashes the result in `which-func-table' keyed by window;
the mode-line `:eval' reads from that table.  When our hook
returns nil on its first synchronous call (request still in
flight), the table caches nil for the window — a later
`force-mode-line-update' re-evaluates the same stale entry and
shows nothing.  Once the async response lands, we overwrite the
per-window entries ourselves so the next redisplay shows PATH
without waiting for the idle timer to fire again.

Our hook is added with depth nil (front of the local hook), so it
wins the hook chain; writing the path directly into the table is
consistent with what `which-function' would return next time it
runs."
  (when (and (bound-and-true-p which-function-mode)
             (boundp 'which-func-table))
    (dolist (win (get-buffer-window-list (current-buffer) nil t))
      (puthash win path which-func-table)))
  (force-mode-line-update))

(defun yaml-lsp--which-func-fetch ()
  "Issue an async `yaml/addressAtPoint' request and cache the result.
Captures the buffer and point at request time so the response
updates the right cache entry even if the user has since moved on."
  (let ((buf (current-buffer))
        (pos (point)))
    (setq yaml-lsp--which-func-pending-pos pos)
    (let ((params (save-restriction
                    (widen)
                    (list :textDocument (lsp--text-document-identifier)
                          :position (lsp--cur-position)
                          :format yaml-lsp-address-format))))
      (lsp-request-async
       "yaml/addressAtPoint" params
       (lambda (response)
         (when (buffer-live-p buf)
           (with-current-buffer buf
             (setq yaml-lsp--which-func-pending-pos nil)
             (let* ((path (and response (lsp-get response :path)))
                    (kind (and response (lsp-get response :nodeKind)))
                    (final (and (stringp path)
                                (not (string-empty-p path))
                                (not (equal kind "none"))
                                path)))
               (setq yaml-lsp--which-func-cache (cons pos final))
               (yaml-lsp--which-func-refresh-modeline final)))))
       :mode 'alive
       :error-handler (lambda (_err)
                        (when (buffer-live-p buf)
                          (with-current-buffer buf
                            (setq yaml-lsp--which-func-pending-pos nil))))))))

(defun yaml-lsp--which-func ()
  "Function for `which-func-functions': return the YAML address at point.
Synchronous: returns a (possibly stale) cached value.  When the
cursor has moved to a position not yet cached and no request is
in flight, kicks off an async fetch in the background; the
modeline refreshes when the response lands."
  (when (and (bound-and-true-p lsp-mode)
             buffer-file-name
             (lsp-find-workspace 'yaml-lsp buffer-file-name))
    (let ((cached-pos (car-safe yaml-lsp--which-func-cache)))
      (unless (or (and cached-pos (= (point) cached-pos))
                  (and yaml-lsp--which-func-pending-pos
                       (= (point) yaml-lsp--which-func-pending-pos)))
        (yaml-lsp--which-func-fetch)))
    (cdr-safe yaml-lsp--which-func-cache)))

;;;###autoload
(define-minor-mode yaml-lsp-which-func-mode
  "Show the YAML address-at-point in the `which-function-mode' mode line.

When enabled in a YAML buffer with an active `yaml-lsp' workspace,
this mode adds a function to `which-func-functions' that returns
the structural address of the node at point.  The format follows
`yaml-lsp-address-format'.  Address queries are asynchronous and
cached by buffer position; the mode line refreshes when responses
arrive.  No effect on buffers without an active `yaml-lsp'
workspace — the hook returns nil and other `which-func-functions'
entries take over.

Enable per buffer, e.g.:

  (add-hook \\='yaml-mode-hook \\='yaml-lsp-which-func-mode)
  (add-hook \\='yaml-ts-mode-hook \\='yaml-lsp-which-func-mode)
  (which-function-mode 1)

`which-function-mode' is gated on the buffer-local `which-func-mode'
flag, which `which-func-ff-hook' clears whenever the major-mode's
`imenu' setup is missing or errors out — the standard case for
`yaml-mode' / `yaml-ts-mode'.  With `which-func-mode' off, the
idle-timer pipeline skips the buffer entirely and the mode-line
shows nothing (or `which-func-unknown', for third-party mode-lines
that display `which-func-current' unconditionally).  Enabling this
mode therefore also sets `which-func-mode' buffer-locally so the
pipeline actually runs here."
  :lighter ""
  (if yaml-lsp-which-func-mode
      (progn
        (add-hook 'which-func-functions #'yaml-lsp--which-func nil t)
        (setq-local which-func-mode t)
        ;; When this mode is enabled via `yaml-mode-hook' (the common
        ;; case), our body runs *inside* `run-mode-hooks', which then
        ;; goes on to fire `after-change-major-mode-hook' — and
        ;; `which-func-ff-hook' on that hook resets `which-func-mode'
        ;; back to nil because `yaml-mode' / `yaml-ts-mode' don't ship
        ;; a working imenu.  Defer a second `setq-local' to the next
        ;; event-loop tick so it lands after that reset.  Idempotent
        ;; when called outside the mode-init context (`run-at-time'
        ;; just queues a no-op refresh).
        (let ((buf (current-buffer)))
          (run-at-time 0 nil
                       (lambda ()
                         (when (and (buffer-live-p buf)
                                    (buffer-local-value
                                     'yaml-lsp-which-func-mode buf))
                           (with-current-buffer buf
                             (setq-local which-func-mode t))))))
        ;; The mode-line `:eval' is gated on `which-func-mode'; mark
        ;; the line for refresh so the bracketed slot appears as soon
        ;; as the deferred `setq-local' lands and the next async
        ;; response fills the table.
        (force-mode-line-update))
    (remove-hook 'which-func-functions #'yaml-lsp--which-func t)
    (kill-local-variable 'which-func-mode)
    (setq yaml-lsp--which-func-cache nil
          yaml-lsp--which-func-pending-pos nil)))

;;;; ---------------------------------------------------------------------
;;;; Folding
;;;; ---------------------------------------------------------------------

;; Interactive folding commands backed by the server's
;; `textDocument/foldingRange' response.  lsp-mode does not expose
;; user-facing fold/unfold commands; it only registers the server's
;; ranges as a thing-at-point.  We reach into the same internals
;; (`lsp--get-folding-ranges', `lsp--get-current-innermost-folding-range',
;; `lsp--folding-range-beg', `lsp--folding-range-end') and hide the
;; ranges with overlays tagged `yaml-lsp-fold' — no dependency on
;; `hs-minor-mode' or other folding minor modes.
;;
;; Like the address-at-point command, these lean on lsp-mode private
;; API; breakage on lsp-mode upgrades surfaces as a `void-function'
;; error rather than silent miscompute.

(defun yaml-lsp--fold-overlay-at (pos)
  "Return the yaml-lsp fold overlay starting at POS, or nil."
  (cl-find-if (lambda (ov)
                (and (overlay-get ov 'yaml-lsp-fold)
                     (= (overlay-start ov) pos)))
              (overlays-in pos (1+ pos))))

(defun yaml-lsp--make-fold-overlay (beg end)
  "Hide [end-of-BEG-line, END] with a fold overlay; return the overlay.
The header line containing BEG stays visible — matches hideshow /
outline conventions."
  (let* ((line-end (save-excursion (goto-char beg) (line-end-position)))
         (ov (make-overlay line-end end)))
    (overlay-put ov 'yaml-lsp-fold t)
    (overlay-put ov 'evaporate t)
    (overlay-put ov 'display (propertize " ..." 'face 'shadow))
    ov))

(defun yaml-lsp--fold-range (range)
  "Fold RANGE if not already folded.  Return the overlay or nil."
  (let* ((beg (lsp--folding-range-beg range))
         (line-end (save-excursion (goto-char beg) (line-end-position))))
    (unless (yaml-lsp--fold-overlay-at line-end)
      (yaml-lsp--make-fold-overlay beg (lsp--folding-range-end range)))))

(defun yaml-lsp--unfold-range (range)
  "Remove the yaml-lsp fold overlay at RANGE's header line, if any.
Return t when an overlay was deleted, nil otherwise."
  (let* ((beg (lsp--folding-range-beg range))
         (line-end (save-excursion (goto-char beg) (line-end-position)))
         (ov (yaml-lsp--fold-overlay-at line-end)))
    (when ov (delete-overlay ov) t)))

(defun yaml-lsp--ranges-in-region (beg end)
  "Return server-reported folding ranges fully inside [BEG, END]."
  (seq-filter (lambda (r)
                (and (>= (lsp--folding-range-beg r) beg)
                     (<= (lsp--folding-range-end r) end)))
              (lsp--get-folding-ranges)))

(defun yaml-lsp--element-range-at-point ()
  "Return the folding range to act on at point, or nil.

Prefers the broadest range whose header line is the current line,
so placing the cursor on a key folds *that key's* children rather
than the container the key sits inside of.  This is necessary
because the server reports `lineFoldingOnly' ranges starting at
end-of-key-line; a strict point-inside test would miss every range
whose key the cursor is on.

When no range starts on the current line (cursor on a value-only
line inside a block), falls back to the innermost range containing
point — that gives the expected behavior for cursor positions where
the line-anchored intent does not apply."
  (let* ((bol (line-beginning-position))
         (eol (line-end-position))
         (header-line-ranges
          (seq-filter (lambda (r)
                        (let ((b (lsp--folding-range-beg r)))
                          (and (>= b bol) (<= b eol))))
                      (lsp--get-folding-ranges))))
    (if header-line-ranges
        ;; Multiple ranges may share a header line (e.g. a sequence
        ;; element whose mapping value is itself multi-line).  Pick
        ;; the broadest: cursor-on-element should fold the *whole*
        ;; element's contents, not the innermost nested block.
        (car (sort header-line-ranges
                   (lambda (r1 r2)
                     (> (lsp--folding-range-end r1)
                        (lsp--folding-range-end r2)))))
      (lsp--get-current-innermost-folding-range))))

(defun yaml-lsp--require-workspace ()
  "Signal a `user-error' unless yaml-lsp is active in the current buffer."
  (unless (and buffer-file-name
               (lsp-find-workspace 'yaml-lsp buffer-file-name))
    (user-error "Yaml-lsp: workspace not active in this buffer")))

;;;###autoload
(defun yaml-lsp-element-fold ()
  "Fold the children of the YAML element at point.
When point is on a key line, folds that key's value subtree
\(closing every contained mapping / sequence).  When point is on a
value line, folds the innermost block containing it."
  (interactive)
  (yaml-lsp--require-workspace)
  (let ((range (yaml-lsp--element-range-at-point)))
    (unless range (user-error "Yaml-lsp: no folding range at point"))
    (or (yaml-lsp--fold-range range)
        (message "yaml-lsp: already folded"))))

;;;###autoload
(defun yaml-lsp-element-unfold ()
  "Unfold the YAML element at point.
Symmetric with `yaml-lsp-element-fold': operates on the same range
the fold command would choose."
  (interactive)
  (yaml-lsp--require-workspace)
  (let ((range (yaml-lsp--element-range-at-point)))
    (unless range (user-error "Yaml-lsp: no folding range at point"))
    (or (yaml-lsp--unfold-range range)
        (message "yaml-lsp: not folded"))))

;;;###autoload
(defun yaml-lsp-element-toggle ()
  "Toggle the fold state of the YAML element at point.
Symmetric with `yaml-lsp-element-fold': operates on the same range
the fold command would choose."
  (interactive)
  (yaml-lsp--require-workspace)
  (let ((range (yaml-lsp--element-range-at-point)))
    (unless range (user-error "Yaml-lsp: no folding range at point"))
    (or (yaml-lsp--unfold-range range)
        (yaml-lsp--fold-range range))))

;;;###autoload
(defun yaml-lsp-region-fold (beg end)
  "Fold every server-reported folding range fully inside [BEG, END]."
  (interactive "r")
  (yaml-lsp--require-workspace)
  (let ((count 0))
    (dolist (r (yaml-lsp--ranges-in-region beg end))
      (when (yaml-lsp--fold-range r) (cl-incf count)))
    (message "yaml-lsp: folded %d range(s)" count)))

;;;###autoload
(defun yaml-lsp-region-unfold (beg end)
  "Remove every yaml-lsp fold overlay overlapping [BEG, END]."
  (interactive "r")
  (yaml-lsp--require-workspace)
  (let ((count 0))
    (dolist (ov (overlays-in beg end))
      (when (overlay-get ov 'yaml-lsp-fold)
        (delete-overlay ov)
        (cl-incf count)))
    (message "yaml-lsp: unfolded %d range(s)" count)))

;;;###autoload
(defun yaml-lsp-region-toggle (beg end)
  "Toggle folds in [BEG, END].
If any yaml-lsp fold overlay overlaps the region, unfold everything
in it; otherwise fold every server-reported range fully contained
in the region."
  (interactive "r")
  (yaml-lsp--require-workspace)
  (if (cl-some (lambda (ov) (overlay-get ov 'yaml-lsp-fold))
               (overlays-in beg end))
      (yaml-lsp-region-unfold beg end)
    (yaml-lsp-region-fold beg end)))

(provide 'yaml-lsp)

;;; yaml-lsp.el ends here
