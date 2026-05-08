package server

import (
	"sync"

	"github.com/tliron/commonlog"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// documentStoreSoftLimit is the entry count at which we log a warning the
// first time we cross it. Real lsp clients send didClose, so a sustained
// count above this threshold strongly suggests a misbehaving client (and
// memory growth) — but we don't evict, so the request never fails on a
// still-open document.
const documentStoreSoftLimit = 1024

// documentStore holds the latest in-memory text for every URI the client
// has opened, plus the most recently applied LSP version per URI. We
// declared full text sync in the server capabilities, so each didChange
// replaces the stored text wholesale; the version stamp lets us drop
// out-of-order writes (e.g. a stale didChange arriving after a newer one
// or after a didClose).
type documentStore struct {
	mu       sync.RWMutex
	docs     map[string]string
	versions map[string]int32
	warned   bool
	log      commonlog.Logger
}

func newDocumentStore() *documentStore {
	return &documentStore{
		docs:     make(map[string]string),
		versions: make(map[string]int32),
		log:      commonlog.GetLogger(ServerName),
	}
}

// Set stores text for uri only when version is at least the previously-
// stored version (writes go forward in time, never back). Returns true if
// the store was actually written to. Negative versions (which LSP forbids
// but `int32` doesn't constrain) are clamped to 0 so a misbehaving client
// can't poison a fresh URI with `version: -1` and then have a legitimate
// `version: 1` write succeed against a poisoned baseline.
func (s *documentStore) Set(uri string, text string, version int32) bool {
	if version < 0 {
		version = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, seen := s.versions[uri]; seen && version < existing {
		return false
	}
	_, existed := s.docs[uri]
	s.docs[uri] = text
	s.versions[uri] = version
	if !existed {
		s.maybeWarn()
	}
	return true
}

func (s *documentStore) Get(uri string) (string, bool) {
	s.mu.RLock()
	t, ok := s.docs[uri]
	s.mu.RUnlock()
	return t, ok
}

// Delete removes both the text and the version stamp for uri. After a
// Delete, a subsequent Set will succeed regardless of its version (we have
// no record of what was there before). Crossing back under the soft limit
// re-arms the warning so a subsequent re-leak is reported again.
func (s *documentStore) Delete(uri string) {
	s.mu.Lock()
	delete(s.docs, uri)
	delete(s.versions, uri)
	if s.warned && len(s.docs) <= documentStoreSoftLimit {
		s.warned = false
	}
	s.mu.Unlock()
}

// maybeWarn emits a warning when the store first crosses the soft-limit
// threshold upward. Once armed, it stays armed until Delete brings the
// count back at or below the threshold (see Delete). Caller holds the
// write lock.
func (s *documentStore) maybeWarn() {
	if !s.warned && len(s.docs) > documentStoreSoftLimit {
		s.log.Warningf("yaml-lsp: document store now holds %d entries (soft limit %d). The server does not evict; this likely indicates a client that opens documents without closing them.", len(s.docs), documentStoreSoftLimit)
		s.warned = true
	}
}

// Warned reports whether the threshold-crossing warning has fired and is
// still armed. Read under the store's lock so concurrent Set/Delete calls
// don't race the test.
func (s *documentStore) Warned() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.warned
}

func (s *Server) didOpen(ctx *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	s.documents.Set(params.TextDocument.URI, params.TextDocument.Text, params.TextDocument.Version)
	s.publishDiagnostics(ctx, params.TextDocument.URI, params.TextDocument.Text)
	return nil
}

func (s *Server) didChange(ctx *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	if len(params.ContentChanges) == 0 {
		return nil
	}
	// didChange is only valid for an already-open document. A change
	// arriving after didClose (out-of-order delivery, or a misbehaving
	// client) must NOT recreate the entry — that would silently undo the
	// close and leak memory. We drop changes for URIs we don't currently
	// have synced.
	if _, open := s.documents.Get(params.TextDocument.URI); !open {
		return nil
	}
	// glsp's UnmarshalJSON splits ContentChanges into typed values:
	// incremental changes carry a Range, full replacements don't. We
	// declared full sync, so we ONLY accept whole-text events. A
	// range-bearing event under full sync is a client bug; applying it as
	// full text would corrupt the store, so we silently ignore it.
	last := params.ContentChanges[len(params.ContentChanges)-1]
	if c, ok := last.(protocol.TextDocumentContentChangeEventWhole); ok {
		if s.documents.Set(params.TextDocument.URI, c.Text, params.TextDocument.Version) {
			s.publishDiagnostics(ctx, params.TextDocument.URI, c.Text)
		}
	}
	return nil
}

func (s *Server) didClose(_ *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	s.documents.Delete(params.TextDocument.URI)
	return nil
}
