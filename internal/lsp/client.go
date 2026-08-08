package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DiagnosticSeverity represents LSP diagnostic severity levels.
type DiagnosticSeverity int

const (
	SeverityError       DiagnosticSeverity = 1
	SeverityWarning     DiagnosticSeverity = 2
	SeverityInformation DiagnosticSeverity = 3
	SeverityHint        DiagnosticSeverity = 4
)

// Diagnostic represents an LSP diagnostic.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Code     string `json:"code,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

// Range represents an LSP range.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Position represents an LSP position.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Location represents an LSP location.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// SymbolInformation represents an LSP symbol.
type SymbolInformation struct {
	Name          string   `json:"name"`
	Kind          int      `json:"kind"`
	Location      Location `json:"location"`
	ContainerName string   `json:"containerName,omitempty"`
}

// WorkspaceEdit represents an LSP workspace edit.
type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes,omitempty"`
}

// TextEdit represents an LSP text edit.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// RenameCapabilities describes rename support.
type RenameCapabilities struct {
	PrepareSupport bool `json:"prepareSupport,omitempty"`
}

// LSPClient communicates with a single language server subprocess.
type LSPClient struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   *bufio.Reader
	mu       sync.Mutex
	nextID   atomic.Int64
	pending  map[interface{}]chan json.RawMessage
	closed   atomic.Bool
	language string
}

// NewLSPClient starts a language server subprocess and initializes the LSP protocol.
func NewLSPClient(ctx context.Context, lang string, cfg ServerConfig) (*LSPClient, error) {
	args := cfg.Args
	cmd := exec.CommandContext(ctx, cfg.Command, args...) // #nosec G204 -- cfg comes from trusted LSP server config (built-in defaults or user/project lsp.json), not external input
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: stdout pipe: %w", err)
	}
	if startErr := cmd.Start(); startErr != nil {
		return nil, fmt.Errorf("lsp: start %s: %w", cfg.Command, startErr)
	}

	c := &LSPClient{
		cmd:      cmd,
		stdin:    stdin,
		stdout:   bufio.NewReader(stdout),
		pending:  make(map[interface{}]chan json.RawMessage),
		language: lang,
	}

	// Start read loop
	go c.readLoop()

	// Initialize
	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err = c.call(initCtx, "initialize", map[string]interface{}{
		"processId": cmd.Process.Pid,
		"capabilities": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"definition":         map[string]interface{}{"dynamicRegistration": false},
				"references":         map[string]interface{}{"dynamicRegistration": false},
				"documentSymbol":     map[string]interface{}{"dynamicRegistration": false},
				"rename":             map[string]interface{}{"dynamicRegistration": false, "prepareSupport": true},
				"publishDiagnostics": map[string]interface{}{"relatedInformation": false},
			},
		},
	})
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("lsp: initialize %s: %w", lang, err)
	}

	// Send initialized notification
	_ = c.notify("initialized", map[string]interface{}{})

	return c, nil
}

func (c *LSPClient) readLoop() {
	// LSP uses Content-Length framed messages: "Content-Length: N\r\n\r\n<N bytes>".
	// We read line-by-line for headers, then read exactly N bytes for the body.
	// This correctly handles pretty-printed JSON (embedded newlines) that a
	// naive line-by-line parser would fragment.
	for {
		// Read the header block line by line.
		var contentLength int
		for {
			line, err := c.stdout.ReadString('\n')
			if err != nil {
				return
			}
			// Headers are terminated by an empty line.
			if line == "\r\n" || line == "\n" {
				break
			}
			header := strings.TrimSpace(line)
			if strings.HasPrefix(header, "Content-Length:") {
				n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, "Content-Length:")))
				if err == nil && n > 0 {
					contentLength = n
				}
				continue
			}
			// Any other header (e.g. Content-Type) — keep reading headers.
		}

		if contentLength == 0 {
			// No Content-Length header found — skip this frame and keep reading.
			continue
		}

		// Read exactly contentLength bytes for the JSON-RPC body.
		body := make([]byte, contentLength)
		if _, err := io.ReadFull(c.stdout, body); err != nil {
			return
		}

		var msg struct {
			ID     interface{}     `json:"id"`
			Method string          `json:"method,omitempty"`
			Result json.RawMessage `json:"result,omitempty"`
			Error  json.RawMessage `json:"error,omitempty"`
			Params json.RawMessage `json:"params,omitempty"`
		}
		if err := json.Unmarshal(body, &msg); err != nil {
			continue
		}

		// Response to a request
		if msg.ID != nil {
			c.mu.Lock()
			ch, ok := c.pending[msg.ID]
			if ok {
				delete(c.pending, msg.ID)
			}
			c.mu.Unlock()
			if ok {
				if msg.Error != nil {
					ch <- msg.Error
				} else {
					ch <- msg.Result
				}
			}
			continue
		}

		// Server notification (e.g., publishDiagnostics)
		if msg.Method != "" {
			// Diagnostics are handled by the caller; we just log them.
			if msg.Method == "textDocument/publishDiagnostics" {
				slog.Debug("lsp: diagnostics notification", "language", c.language)
			}
		}
	}
}

func (c *LSPClient) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("lsp: client closed")
	}

	id := c.nextID.Add(1)
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}

	ch := make(chan json.RawMessage, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Write with Content-Length header (LSP requires it)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	c.mu.Lock()
	_, err = c.stdin.Write([]byte(header))
	if err == nil {
		_, err = c.stdin.Write(data)
	}
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("lsp: write: %w", err)
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case result := <-ch:
		return result, nil
	}
}

func (c *LSPClient) notify(method string, params interface{}) error {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err = c.stdin.Write([]byte(header))
	if err == nil {
		_, err = c.stdin.Write(data)
	}
	return err
}

// Close shuts down the language server.
func (c *LSPClient) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	_ = c.notify("shutdown", nil)
	_ = c.notify("exit", nil)
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	return c.cmd.Wait()
}

// Language returns the language this client serves.
func (c *LSPClient) Language() string {
	return c.language
}

// Diagnostics requests diagnostics for a file.
func (c *LSPClient) Diagnostics(ctx context.Context, uri string) ([]Diagnostic, error) {
	result, err := c.call(ctx, "textDocument/diagnostic", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Items []Diagnostic `json:"items"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		// Some servers return diagnostics as a flat array
		var items []Diagnostic
		if err2 := json.Unmarshal(result, &items); err2 == nil {
			return items, nil
		}
		return nil, nil
	}
	return resp.Items, nil
}

// GotoDefinition finds the definition of a symbol at a position.
func (c *LSPClient) GotoDefinition(ctx context.Context, uri string, line, char int) ([]Location, error) {
	result, err := c.call(ctx, "textDocument/definition", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": line, "character": char},
	})
	if err != nil {
		return nil, err
	}
	// Result can be a single Location or array
	var locs []Location
	if err := json.Unmarshal(result, &locs); err != nil {
		var loc Location
		if err2 := json.Unmarshal(result, &loc); err2 == nil {
			return []Location{loc}, nil
		}
	}
	return locs, nil
}

// FindReferences finds all references to a symbol at a position.
func (c *LSPClient) FindReferences(ctx context.Context, uri string, line, char int) ([]Location, error) {
	result, err := c.call(ctx, "textDocument/references", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": line, "character": char},
		"context":      map[string]interface{}{"includeDeclaration": true},
	})
	if err != nil {
		return nil, err
	}
	var locs []Location
	if err := json.Unmarshal(result, &locs); err != nil {
		return nil, nil
	}
	return locs, nil
}

// DocumentSymbol returns all symbols in a document.
func (c *LSPClient) DocumentSymbol(ctx context.Context, uri string) ([]SymbolInformation, error) {
	result, err := c.call(ctx, "textDocument/documentSymbol", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	})
	if err != nil {
		return nil, err
	}
	var symbols []SymbolInformation
	if err := json.Unmarshal(result, &symbols); err != nil {
		return nil, nil
	}
	return symbols, nil
}

// PrepareRename checks if a symbol at a position can be renamed.
func (c *LSPClient) PrepareRename(ctx context.Context, uri string, line, char int) (*Range, error) {
	result, err := c.call(ctx, "textDocument/prepareRename", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": line, "character": char},
	})
	if err != nil {
		return nil, err
	}
	var r Range
	if err := json.Unmarshal(result, &r); err != nil {
		return nil, nil // not renamable
	}
	return &r, nil
}

// Rename renames a symbol at a position across the workspace.
func (c *LSPClient) Rename(ctx context.Context, uri string, line, char int, newName string) (*WorkspaceEdit, error) {
	result, err := c.call(ctx, "textDocument/rename", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": line, "character": char},
		"newName":      newName,
	})
	if err != nil {
		return nil, err
	}
	var edit WorkspaceEdit
	if err := json.Unmarshal(result, &edit); err != nil {
		return nil, nil
	}
	return &edit, nil
}
