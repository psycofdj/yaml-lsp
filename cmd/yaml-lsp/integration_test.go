package main_test

// End-to-end integration test: build the actual yaml-lsp binary and drive
// it via stdio JSON-RPC, verifying that the full LSP wire protocol works
// for initialize, didOpen, yaml/addressAtPoint, textDocument/hover,
// textDocument/documentSymbol, and shutdown/exit.
//
// This complements the in-process unit tests in internal/server/* by
// exercising the JSON-RPC framing layer, the dispatcher, the capability
// advertisement, and the actual binary the user runs.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const sampleYAML = `apiVersion: v1
kind: ConfigMap
metadata:
  name: foo
  namespace: default
data:
  greeting: hello
`

func TestLSPEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test builds the binary; skipped under -short")
	}
	binary := buildBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	cmd := exec.CommandContext(ctx, binary, "serve")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		// Best-effort kill; the test sends shutdown/exit cleanly first.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() && stderr.Len() > 0 {
			t.Logf("server stderr:\n%s", stderr.String())
		}
	})

	c := &lspClient{stdin: stdin, stdout: bufio.NewReader(stdout)}

	// 1. initialize
	initRes := c.request(t, "initialize", map[string]any{
		"processId":    nil,
		"rootUri":      nil,
		"capabilities": map[string]any{},
	})
	caps, ok := initRes["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("initialize: capabilities missing or wrong type: %T", initRes["capabilities"])
	}
	if got := caps["hoverProvider"]; got != true {
		t.Errorf("hoverProvider: got %v, want true", got)
	}
	if got := caps["documentSymbolProvider"]; got != true {
		t.Errorf("documentSymbolProvider: got %v, want true", got)
	}
	// textDocumentSync should be the Full kind (1).
	if got := caps["textDocumentSync"]; got != float64(1) {
		t.Errorf("textDocumentSync: got %v (%T), want 1", got, got)
	}

	// 2. initialized notification
	c.notify(t, "initialized", map[string]any{})

	// 3. didOpen
	const uri = "file:///e2e.yaml"
	c.notify(t, "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": "yaml",
			"version":    1,
			"text":       sampleYAML,
		},
	})

	// 4. yaml/addressAtPoint at cursor on `foo` (line 4 col 9 -> LSP 3,8)
	addrRes := c.request(t, "yaml/addressAtPoint", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": 3, "character": 8},
		"format":       "jsonpath",
	})
	if got := addrRes["path"]; got != "$.metadata.name" {
		t.Errorf("addressAtPoint path: got %v, want $.metadata.name", got)
	}
	if got := addrRes["nodeKind"]; got != "value" {
		t.Errorf("addressAtPoint nodeKind: got %v, want value", got)
	}

	// 5. textDocument/hover at same position; expect all four formats in the markdown
	hoverRes := c.request(t, "textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": 3, "character": 8},
	})
	contents, ok := hoverRes["contents"].(map[string]any)
	if !ok {
		t.Fatalf("hover.contents missing or wrong type: %T", hoverRes["contents"])
	}
	value, _ := contents["value"].(string)
	for _, want := range []string{"jsonpath", "bosh-ops", "jsonpatch", "helm-values", "$.metadata.name"} {
		if !strings.Contains(value, want) {
			t.Errorf("hover content missing %q\n--- full ---\n%s", want, value)
		}
	}

	// 6. textDocument/documentSymbol — flat JSONPath list (one entry
	// per node, paths instead of bare keys). See server/symbol.go for
	// the rationale.
	symRes := c.requestRaw(t, "textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]any{"uri": uri},
	})
	syms, ok := symRes.([]any)
	if !ok {
		t.Fatalf("documentSymbol result is %T, want array", symRes)
	}
	wantNames := []string{
		"$.apiVersion",
		"$.kind",
		"$.metadata",
		"$.metadata.name",
		"$.metadata.namespace",
		"$.data",
		"$.data.greeting",
	}
	if len(syms) != len(wantNames) {
		t.Errorf("documentSymbol entries: got %d, want %d", len(syms), len(wantNames))
	}
	for i, want := range wantNames {
		if i >= len(syms) {
			break
		}
		entry := syms[i].(map[string]any)
		if entry["name"] != want {
			t.Errorf("symbol[%d].name: got %v, want %s", i, entry["name"], want)
		}
	}

	// 7. shutdown + exit
	c.request(t, "shutdown", nil)
	c.notify(t, "exit", nil)

	// Server should exit on its own; give it a moment.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		// Some platforms don't deliver clean exit on `exit` notification
		// because glsp's stdio loop closes on read EOF instead. Either is
		// acceptable; we rely on the t.Cleanup kill if the process lingers.
	case <-time.After(2 * time.Second):
		// Fall through; cleanup will kill it.
	}
}

// buildBinary compiles cmd/yaml-lsp to a temporary location and returns
// the absolute path. Built once per test invocation.
func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "yaml-lsp")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	// Find the project root: this test file is at cmd/yaml-lsp/, so the
	// module root is two levels up. `go build ./cmd/yaml-lsp` from there
	// produces our binary.
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/yaml-lsp")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return bin
}

// lspClient is a minimal LSP client speaking the JSON-RPC framed protocol
// over an existing stdin/stdout pair. It serializes requests and matches
// responses by id.
type lspClient struct {
	stdin  io.Writer
	stdout *bufio.Reader

	mu     sync.Mutex
	nextID int
}

// request sends a JSON-RPC request and waits for the matching response.
// The returned map is the `result` field; the test asserts on its shape.
func (c *lspClient) request(t *testing.T, method string, params any) map[string]any {
	t.Helper()
	res := c.requestRaw(t, method, params)
	if res == nil {
		return nil
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("request %s: result is %T, want object", method, res)
	}
	return m
}

// requestRaw is like request but tolerates non-object results (e.g.
// documentSymbol returns an array).
func (c *lspClient) requestRaw(t *testing.T, method string, params any) any {
	t.Helper()
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	if err := writeMessage(c.stdin, req); err != nil {
		t.Fatalf("write %s: %v", method, err)
	}
	for {
		var msg map[string]any
		if err := readMessage(c.stdout, &msg); err != nil {
			t.Fatalf("read response to %s: %v", method, err)
		}
		// Ignore server-sent notifications/window/log-message etc.
		if _, hasMethod := msg["method"]; hasMethod {
			continue
		}
		gotID, ok := msg["id"]
		if !ok {
			continue
		}
		// JSON unmarshals numbers as float64.
		if int(gotID.(float64)) != id {
			continue
		}
		if errVal, ok := msg["error"]; ok && errVal != nil {
			t.Fatalf("server error for %s: %v", method, errVal)
		}
		return msg["result"]
	}
}

func (c *lspClient) notify(t *testing.T, method string, params any) {
	t.Helper()
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	if err := writeMessage(c.stdin, msg); err != nil {
		t.Fatalf("write notify %s: %v", method, err)
	}
}

func writeMessage(w io.Writer, msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

func readMessage(r *bufio.Reader, dst any) error {
	contentLength := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			n := 0
			if _, err := fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:")), "%d", &n); err != nil {
				return fmt.Errorf("bad Content-Length header: %q", line)
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return fmt.Errorf("missing Content-Length header")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}
