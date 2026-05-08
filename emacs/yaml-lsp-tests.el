;;; yaml-lsp-tests.el --- Tests for yaml-lsp.el  -*- lexical-binding: t; -*-

;; Copyright (C) 2026  Xavier MARCELET
;; SPDX-License-Identifier: GPL-3.0-or-later

;;; Commentary:

;; ert smoke tests for yaml-lsp.el.  Run with `make test'.

;;; Code:

(require 'cl-lib)
(require 'ert)
(require 'yaml-lsp)

;; Tests assume `yaml-lsp-reload' has placed exactly one `yaml-lsp'
;; entry into `lsp-clients'.  Calling it explicitly here makes test
;; execution deterministic across batch and interactive Emacs.
(yaml-lsp-reload)

(ert-deftest yaml-lsp-test-loads ()
  "yaml-lsp provides the `yaml-lsp' feature."
  (should (featurep 'yaml-lsp)))

(ert-deftest yaml-lsp-test-server-executable-default ()
  "`yaml-lsp-server-executable' defaults to the string \"yaml-lsp\"."
  (should (stringp yaml-lsp-server-executable))
  (should (equal (default-value 'yaml-lsp-server-executable) "yaml-lsp")))

(ert-deftest yaml-lsp-test-defcustom-format-choices ()
  "`yaml-lsp-address-format' declares exactly the four supported formats."
  (let* ((spec (get 'yaml-lsp-address-format 'custom-type))
         (choices (mapcar (lambda (c) (nth 1 c)) (cdr spec))))
    (should (consp spec))
    (should (eq (car spec) 'choice))
    (should (equal (sort (copy-sequence choices) #'string<)
                   (list "bosh-ops" "helm-values" "jsonpatch" "jsonpath")))))

(ert-deftest yaml-lsp-test-default-format-is-jsonpath ()
  "Default address format is jsonpath."
  (should (equal (default-value 'yaml-lsp-address-format) "jsonpath")))

(ert-deftest yaml-lsp-test-command-is-interactive ()
  "`yaml-lsp-copy-address-at-point' is a command."
  (should (commandp 'yaml-lsp-copy-address-at-point)))

(ert-deftest yaml-lsp-test-additional-major-modes-defcustom ()
  "`yaml-lsp-additional-major-modes' is a list of symbols defaulting to yaml/yaml-ts."
  (let ((spec (get 'yaml-lsp-additional-major-modes 'custom-type)))
    (should (equal spec '(repeat symbol))))
  (should (equal (default-value 'yaml-lsp-additional-major-modes)
                 '(yaml-mode yaml-ts-mode))))

(ert-deftest yaml-lsp-test-reload-command-is-interactive ()
  "`yaml-lsp-reload' is a command."
  (should (commandp 'yaml-lsp-reload)))

(ert-deftest yaml-lsp-test-server-id-registered ()
  "`yaml-lsp' registers as a known LSP server-id."
  (should (and (boundp 'lsp-clients)
               (hash-table-p lsp-clients)
               (gethash 'yaml-lsp lsp-clients))))

(ert-deftest yaml-lsp-test-reload-is-idempotent ()
  "`yaml-lsp-reload' leaves exactly one `yaml-lsp' entry, no matter how often invoked."
  (yaml-lsp-reload)
  (yaml-lsp-reload)
  (yaml-lsp-reload)
  (should (gethash 'yaml-lsp lsp-clients))
  ;; Iterate values; only one client struct should carry the yaml-lsp id.
  (let ((count 0))
    (maphash (lambda (_k v)
               (when (eq (lsp--client-server-id v) 'yaml-lsp)
                 (cl-incf count)))
             lsp-clients)
    (should (= count 1))))

(ert-deftest yaml-lsp-test-reload-picks-up-new-major-modes ()
  "`yaml-lsp-reload' re-registers using the current `yaml-lsp-additional-major-modes'."
  (unwind-protect
      (let ((yaml-lsp-additional-major-modes '(yaml-mode yaml-ts-mode k8s-mode)))
        (yaml-lsp-reload)
        (let ((client (gethash 'yaml-lsp lsp-clients)))
          (should client)
          (should (member 'k8s-mode (lsp--client-major-modes client)))))
    ;; Always restore defaults so subsequent tests aren't affected, even if
    ;; an assertion above signaled.
    (yaml-lsp-reload)))

(ert-deftest yaml-lsp-test-set-executable-mid-session-warns ()
  "Changing `yaml-lsp-server-executable' while a workspace is active prints the restart hint."
  (let ((messages nil))
    (cl-letf (((symbol-function 'lsp-find-workspace)
               (lambda (&rest _) 'fake-workspace))
              ((symbol-function 'message)
               (lambda (fmt &rest args) (push (apply #'format fmt args) messages))))
      (let ((orig (default-value 'yaml-lsp-server-executable)))
        (unwind-protect
            (customize-set-variable 'yaml-lsp-server-executable "/some/other/yaml-lsp")
          ;; Restore via the same path so `:set' fires once.  Restoring under
          ;; the mocked `lsp-find-workspace' would log a second hint, so we
          ;; reset directly.
          (setq-default yaml-lsp-server-executable orig)))
      (should (cl-some (lambda (m) (string-match-p "lsp-restart-workspace" m))
                       messages)))))

(ert-deftest yaml-lsp-test-set-executable-no-session-silent ()
  "Changing `yaml-lsp-server-executable' with no workspace does not print the hint."
  (let ((messages nil))
    (cl-letf (((symbol-function 'lsp-find-workspace)
               (lambda (&rest _) nil))
              ((symbol-function 'message)
               (lambda (fmt &rest args) (push (apply #'format fmt args) messages))))
      (let ((orig (default-value 'yaml-lsp-server-executable)))
        (unwind-protect
            (customize-set-variable 'yaml-lsp-server-executable "/some/other/yaml-lsp")
          (setq-default yaml-lsp-server-executable orig)))
      (should-not (cl-some (lambda (m) (string-match-p "lsp-restart-workspace" m))
                           messages)))))

(ert-deftest yaml-lsp-test-release-url-shape ()
  "`yaml-lsp--release-url' produces URLs that match the goreleaser asset
naming scheme.  Keep this aligned with `.goreleaser.yaml''s
`name_template' and `format_overrides'."
  (should (equal (yaml-lsp--release-url "1.2.3" "linux" "amd64")
                 "https://github.com/psycofdj/yaml-lsp/releases/download/v1.2.3/yaml-lsp_1.2.3_linux_amd64.tar.gz"))
  (should (equal (yaml-lsp--release-url "1.2.3" "darwin" "arm64")
                 "https://github.com/psycofdj/yaml-lsp/releases/download/v1.2.3/yaml-lsp_1.2.3_darwin_arm64.tar.gz"))
  (should (equal (yaml-lsp--release-url "1.2.3" "windows" "amd64")
                 "https://github.com/psycofdj/yaml-lsp/releases/download/v1.2.3/yaml-lsp_1.2.3_windows_amd64.zip")))

(ert-deftest yaml-lsp-test-platform-detection ()
  "`yaml-lsp--platform' maps host strings to goreleaser OS/Arch names."
  (cl-letf (((symbol-value 'system-type) 'gnu/linux)
            ((symbol-value 'system-configuration) "x86_64-pc-linux-gnu"))
    (should (equal (yaml-lsp--platform) '("linux" . "amd64"))))
  (cl-letf (((symbol-value 'system-type) 'darwin)
            ((symbol-value 'system-configuration) "aarch64-apple-darwin22.4.0"))
    (should (equal (yaml-lsp--platform) '("darwin" . "arm64"))))
  (cl-letf (((symbol-value 'system-type) 'windows-nt)
            ((symbol-value 'system-configuration) "x86_64-w64-mingw32"))
    (should (equal (yaml-lsp--platform) '("windows" . "amd64"))))
  ;; Unsupported arch returns nil so callers can show a fallback message.
  (cl-letf (((symbol-value 'system-type) 'gnu/linux)
            ((symbol-value 'system-configuration) "i686-pc-linux-gnu"))
    (should-not (yaml-lsp--platform))))

(ert-deftest yaml-lsp-test-install-server-calls-lsp-download-install ()
  "`yaml-lsp--install-server' delegates to `lsp-download-install' with the
URL and decompression kind built from the host platform."
  (let (captured)
    (cl-letf (((symbol-value 'system-type) 'gnu/linux)
              ((symbol-value 'system-configuration) "x86_64-pc-linux-gnu")
              ((symbol-function 'yaml-lsp--url-reachable-p) (lambda (_url) t))
              ((symbol-function 'lsp-download-install)
               (lambda (_cb _err &rest args) (setq captured args))))
      (let ((yaml-lsp-server-version "9.9.9")
            (yaml-lsp-server-store-path "/tmp/yaml-lsp/yaml-lsp"))
        (yaml-lsp--install-server nil #'ignore #'ignore))
      (should (equal (plist-get captured :url)
                     "https://github.com/psycofdj/yaml-lsp/releases/download/v9.9.9/yaml-lsp_9.9.9_linux_amd64.tar.gz"))
      (should (eq (plist-get captured :decompress) :targz))
      (should (equal (plist-get captured :store-path) "/tmp/yaml-lsp/yaml-lsp")))))

(ert-deftest yaml-lsp-test-install-server-windows-uses-zip-and-exe ()
  "On Windows, the install function picks the .zip artifact and appends
.exe to the store path so `lsp-download-install' lands the binary at
the path the connection lambda probes."
  (let (captured)
    (cl-letf (((symbol-value 'system-type) 'windows-nt)
              ((symbol-value 'system-configuration) "x86_64-w64-mingw32")
              ((symbol-function 'yaml-lsp--url-reachable-p) (lambda (_url) t))
              ((symbol-function 'lsp-download-install)
               (lambda (_cb _err &rest args) (setq captured args))))
      (let ((yaml-lsp-server-version "9.9.9")
            (yaml-lsp-server-store-path "C:/tmp/yaml-lsp/yaml-lsp"))
        (yaml-lsp--install-server nil #'ignore #'ignore))
      (should (string-suffix-p ".zip" (plist-get captured :url)))
      (should (eq (plist-get captured :decompress) :zip))
      (should (equal (plist-get captured :store-path) "C:/tmp/yaml-lsp/yaml-lsp.exe")))))

(ert-deftest yaml-lsp-test-install-server-unsupported-platform ()
  "On a platform without a published release, the install function
invokes ERROR-CALLBACK with a message pointing to the `go install'
fallback rather than calling `lsp-download-install'."
  (let (error-msg downloaded)
    (cl-letf (((symbol-value 'system-type) 'gnu/linux)
              ((symbol-value 'system-configuration) "i686-pc-linux-gnu")
              ((symbol-function 'lsp-download-install)
               (lambda (&rest _) (setq downloaded t))))
      (yaml-lsp--install-server nil #'ignore (lambda (m) (setq error-msg m))))
    (should-not downloaded)
    (should (stringp error-msg))
    (should (string-match-p "go install" error-msg))))

(ert-deftest yaml-lsp-test-server-command-prefers-path ()
  "When `yaml-lsp-server-executable' resolves on PATH, the connection
lambda uses it (and ignores any downloaded binary)."
  (cl-letf (((symbol-function 'executable-find)
             (lambda (_) "/usr/local/bin/yaml-lsp")))
    (should (equal (yaml-lsp--server-command) "/usr/local/bin/yaml-lsp"))))

(ert-deftest yaml-lsp-test-server-command-falls-back-to-store-path ()
  "When PATH lookup fails but the downloaded binary exists, the
connection lambda returns the store path."
  (let ((tmp (make-temp-file "yaml-lsp-store" nil nil "")))
    (unwind-protect
        (cl-letf (((symbol-function 'executable-find) (lambda (_) nil))
                  ((symbol-value 'yaml-lsp-server-store-path) tmp))
          (set-file-modes tmp #o755)
          (should (equal (yaml-lsp--server-command) tmp)))
      (delete-file tmp))))

(ert-deftest yaml-lsp-test-server-command-falls-back-to-name ()
  "When neither PATH nor the store path resolves, the connection lambda
returns the configured name so lsp-mode can surface a clear error."
  (cl-letf (((symbol-function 'executable-find) (lambda (_) nil))
            ((symbol-function 'file-executable-p) (lambda (_) nil)))
    (let ((yaml-lsp-server-executable "yaml-lsp"))
      (should (equal (yaml-lsp--server-command) "yaml-lsp")))))

(ert-deftest yaml-lsp-test-address-command-builds-correct-request ()
  "`yaml-lsp-copy-address-at-point' constructs a request with the
expected shape: method `yaml/addressAtPoint', params containing
:textDocument, :position, and :format from the defcustom. The
success callback messages the path field of the response."
  (let ((captured-method nil)
        (captured-params nil)
        (captured-mode nil)
        (captured-error-handler nil)
        (captured-callback nil)
        (messages nil))
    (cl-letf* (((symbol-function 'lsp-find-workspace)
                (lambda (&rest _) 'fake-workspace))
               ((symbol-function 'lsp--text-document-identifier)
                (lambda () '(:uri "file:///fake.yaml")))
               ((symbol-function 'lsp--cur-position)
                (lambda () '(:line 3 :character 8)))
               ((symbol-function 'lsp-request-async)
                (lambda (method params callback &rest plist)
                  (setq captured-method method
                        captured-params params
                        captured-callback callback
                        captured-mode (plist-get plist :mode)
                        captured-error-handler (plist-get plist :error-handler))))
               ((symbol-function 'message)
                (lambda (fmt &rest args) (push (apply #'format fmt args) messages))))
      ;; Need a buffer-file-name for the pre-flight to pass.
      (with-temp-buffer
        (setq buffer-file-name "/tmp/fake.yaml")
        (let ((yaml-lsp-address-format "jsonpatch"))
          (yaml-lsp-copy-address-at-point))))

    (should (equal captured-method "yaml/addressAtPoint"))
    (should captured-params)
    (should (equal (plist-get captured-params :format) "jsonpatch"))
    (should (equal (plist-get captured-params :textDocument) '(:uri "file:///fake.yaml")))
    (should (equal (plist-get captured-params :position) '(:line 3 :character 8)))
    (should (eq captured-mode 'alive))
    (should captured-error-handler)
    (should captured-callback)

    ;; Drive the success callback with a synthetic response and verify
    ;; (a) the minibuffer message is the path field, (b) the path was
    ;; pushed onto the kill ring. lsp-mode's default deserialization
    ;; produces hash-tables (lsp-use-plists is nil by default in
    ;; lsp-mode 9.x); lsp-get picks gethash in that mode, so we test
    ;; the hash-table shape.
    (let ((response (make-hash-table :test 'equal))
          (messages nil)
          (kill-ring nil)
          (kill-ring-yank-pointer nil)
          (interprogram-cut-function nil))
      (puthash "path" "/spec/replicas" response)
      (puthash "nodeKind" "value" response)
      (cl-letf (((symbol-function 'message)
                 (lambda (fmt &rest args) (push (apply #'format fmt args) messages))))
        (funcall captured-callback response))
      (should (cl-some (lambda (m) (string-match-p "/spec/replicas" m)) messages))
      (should (equal "/spec/replicas" (car kill-ring))))

    ;; Drive with a no-node response: fallback message AND kill ring
    ;; must NOT be touched (cursor on whitespace shouldn't clobber the
    ;; user's clipboard).
    (let ((response (make-hash-table :test 'equal))
          (messages nil)
          (kill-ring '("preexisting-entry"))
          (kill-ring-yank-pointer nil)
          (interprogram-cut-function nil))
      (puthash "path" "" response)
      (puthash "nodeKind" "none" response)
      (cl-letf (((symbol-function 'message)
                 (lambda (fmt &rest args) (push (apply #'format fmt args) messages))))
        (funcall captured-callback response))
      (should (cl-some (lambda (m) (string-match-p "no node at point" m)) messages))
      (should (equal '("preexisting-entry") kill-ring)))

    ;; Drive the error handler with a synthetic LSP error (plist form, which
    ;; matches what lsp-mode passes to error handlers historically).
    (let ((messages nil))
      (cl-letf (((symbol-function 'message)
                 (lambda (fmt &rest args) (push (apply #'format fmt args) messages))))
        (funcall captured-error-handler '(:code -32602 :message "bad position")))
      (should (cl-some (lambda (m) (string-match-p "bad position" m)) messages)))

    ;; Drive the error handler with a hash-table payload (lsp-mode's
    ;; default deserialization mode).
    (let ((response (make-hash-table :test 'equal))
          (messages nil))
      (puthash "code" -32603 response)
      (puthash "message" "internal error" response)
      (cl-letf (((symbol-function 'message)
                 (lambda (fmt &rest args) (push (apply #'format fmt args) messages))))
        (funcall captured-error-handler response))
      (should (cl-some (lambda (m) (string-match-p "internal error" m)) messages)))

    ;; Drive the error handler with an unrecognized payload — should still
    ;; produce a message instead of erroring out.
    (let ((messages nil))
      (cl-letf (((symbol-function 'message)
                 (lambda (fmt &rest args) (push (apply #'format fmt args) messages))))
        (funcall captured-error-handler 42))
      (should (cl-some (lambda (m) (string-match-p "yaml-lsp: server error" m)) messages)))))

(ert-deftest yaml-lsp-test-format-error-helper ()
  "`yaml-lsp--format-error' extracts a message from each known shape."
  (should (equal "plain string" (yaml-lsp--format-error "plain string")))
  (should (equal "from plist" (yaml-lsp--format-error '(:code -1 :message "from plist"))))
  (let ((h (make-hash-table :test 'equal)))
    (puthash "message" "from hash" h)
    (should (equal "from hash" (yaml-lsp--format-error h))))
  ;; Fallback for arbitrary values: a non-empty string is produced.
  (should (stringp (yaml-lsp--format-error 42)))
  (should (stringp (yaml-lsp--format-error nil))))

(ert-deftest yaml-lsp-test-which-func-mode-toggles-hook ()
  "Enabling the mode adds the function to `which-func-functions'; disabling removes it."
  (with-temp-buffer
    ;; Hook variables only exist when which-func is loaded; require it so
    ;; the local hook variable is bound.
    (require 'which-func)
    (yaml-lsp-which-func-mode 1)
    (should (memq #'yaml-lsp--which-func which-func-functions))
    (yaml-lsp-which-func-mode -1)
    (should-not (memq #'yaml-lsp--which-func which-func-functions))))

(ert-deftest yaml-lsp-test-which-func-returns-nil-without-workspace ()
  "With no `yaml-lsp' workspace active, the hook returns nil quietly."
  (with-temp-buffer
    (cl-letf (((symbol-function 'lsp-find-workspace)
               (lambda (&rest _) nil)))
      (should (null (yaml-lsp--which-func))))))

(ert-deftest yaml-lsp-test-which-func-caches-async-result ()
  "An async response updates the buffer-local cache and is returned by the hook."
  (let ((captured-callback nil))
    (cl-letf (((symbol-function 'lsp-find-workspace)
               (lambda (&rest _) 'fake-workspace))
              ((symbol-function 'lsp--text-document-identifier)
               (lambda () '(:uri "file:///fake.yaml")))
              ((symbol-function 'lsp--cur-position)
               (lambda () '(:line 0 :character 0)))
              ((symbol-function 'lsp-request-async)
               (lambda (_method _params callback &rest _)
                 (setq captured-callback callback)))
              ((symbol-function 'force-mode-line-update)
               (lambda (&rest _) nil)))
      (with-temp-buffer
        (setq buffer-file-name "/tmp/fake.yaml")
        ;; The hook checks `bound-and-true-p lsp-mode`; set it locally.
        (setq-local lsp-mode t)
        ;; First call kicks off the async fetch and returns the (empty) cache.
        (should (null (yaml-lsp--which-func)))
        (should captured-callback)
        ;; Drive the captured callback with a synthetic hash-table response.
        (let ((response (make-hash-table :test 'equal)))
          (puthash "path" "$.metadata.name" response)
          (puthash "nodeKind" "value" response)
          (funcall captured-callback response))
        ;; Now the hook returns the cached value.
        (should (equal "$.metadata.name" (yaml-lsp--which-func)))))))

(ert-deftest yaml-lsp-test-which-func-no-node-cached-as-nil ()
  "A response with nodeKind=none is cached as nil so the modeline reflects 'no node'."
  (let ((captured-callback nil))
    (cl-letf (((symbol-function 'lsp-find-workspace)
               (lambda (&rest _) 'fake-workspace))
              ((symbol-function 'lsp--text-document-identifier)
               (lambda () '(:uri "file:///fake.yaml")))
              ((symbol-function 'lsp--cur-position)
               (lambda () '(:line 0 :character 0)))
              ((symbol-function 'lsp-request-async)
               (lambda (_method _params callback &rest _)
                 (setq captured-callback callback)))
              ((symbol-function 'force-mode-line-update)
               (lambda (&rest _) nil)))
      (with-temp-buffer
        (setq buffer-file-name "/tmp/fake.yaml")
        (setq-local lsp-mode t)
        (yaml-lsp--which-func) ; kicks off
        (should captured-callback)
        (let ((response (make-hash-table :test 'equal)))
          (puthash "path" "" response)
          (puthash "nodeKind" "none" response)
          (funcall captured-callback response))
        (should (null (yaml-lsp--which-func)))))))

(ert-deftest yaml-lsp-test-format-indentation-default ()
  "`yaml-lsp-format-indentation' defaults to the string \"detect\"."
  (should (equal (default-value 'yaml-lsp-format-indentation) "detect")))

(ert-deftest yaml-lsp-test-format-indentation-defcustom-type ()
  "`yaml-lsp-format-indentation' accepts the literal \"detect\" or an integer."
  (let ((spec (get 'yaml-lsp-format-indentation 'custom-type)))
    (should (consp spec))
    (should (eq (car spec) 'choice))))

(ert-deftest yaml-lsp-test-format-normalize-strings-default ()
  "`yaml-lsp-format-normalize-strings' defaults to nil."
  (should (null (default-value 'yaml-lsp-format-normalize-strings)))
  (should (eq (get 'yaml-lsp-format-normalize-strings 'custom-type) 'boolean)))

(ert-deftest yaml-lsp-test-initialization-options-default ()
  "With defaults, the payload sends \"detect\" and :json-false."
  (let ((opts (yaml-lsp--initialization-options)))
    (let ((format-opts (plist-get opts :format)))
      (should (equal (plist-get format-opts :indentation) "detect"))
      (should (eq (plist-get format-opts :normalizeStrings) :json-false)))))

(ert-deftest yaml-lsp-test-initialization-options-fixed-indent ()
  "An integer `yaml-lsp-format-indentation' is sent verbatim as a number."
  (let ((yaml-lsp-format-indentation 4))
    (let* ((opts (yaml-lsp--initialization-options))
           (format-opts (plist-get opts :format)))
      (should (eq (plist-get format-opts :indentation) 4)))))

(ert-deftest yaml-lsp-test-initialization-options-normalize-on ()
  "Enabling `yaml-lsp-format-normalize-strings' surfaces as JSON true (t)."
  (let ((yaml-lsp-format-normalize-strings t))
    (let* ((opts (yaml-lsp--initialization-options))
           (format-opts (plist-get opts :format)))
      (should (eq (plist-get format-opts :normalizeStrings) t)))))

(ert-deftest yaml-lsp-test-initialization-options-non-integer-fallback ()
  "A non-integer, non-\"detect\" value (e.g. a typo) falls back to \"detect\"."
  (let ((yaml-lsp-format-indentation "ohnoes"))
    (let* ((opts (yaml-lsp--initialization-options))
           (format-opts (plist-get opts :format)))
      (should (equal (plist-get format-opts :indentation) "detect")))))

(ert-deftest yaml-lsp-test-format-setting-changed-warns-with-workspace ()
  "Changing a format setting while a workspace is active prints the restart hint."
  (let ((messages nil))
    (cl-letf (((symbol-function 'lsp-find-workspace)
               (lambda (&rest _) 'fake-workspace))
              ((symbol-function 'message)
               (lambda (fmt &rest args) (push (apply #'format fmt args) messages))))
      (let ((orig (default-value 'yaml-lsp-format-indentation)))
        (unwind-protect
            (customize-set-variable 'yaml-lsp-format-indentation 4)
          (setq-default yaml-lsp-format-indentation orig)))
      (should (cl-some (lambda (m) (string-match-p "lsp-restart-workspace" m))
                       messages)))))

(ert-deftest yaml-lsp-test-format-setting-changed-silent-without-workspace ()
  "Changing a format setting with no workspace does not print the hint."
  (let ((messages nil))
    (cl-letf (((symbol-function 'lsp-find-workspace)
               (lambda (&rest _) nil))
              ((symbol-function 'message)
               (lambda (fmt &rest args) (push (apply #'format fmt args) messages))))
      (let ((orig (default-value 'yaml-lsp-format-normalize-strings)))
        (unwind-protect
            (customize-set-variable 'yaml-lsp-format-normalize-strings t)
          (setq-default yaml-lsp-format-normalize-strings orig)))
      (should-not (cl-some (lambda (m) (string-match-p "lsp-restart-workspace" m))
                           messages)))))

(provide 'yaml-lsp-tests)
;;; yaml-lsp-tests.el ends here
