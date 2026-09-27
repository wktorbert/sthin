package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/wktorbert/lean-sim/internal/device"
	"github.com/wktorbert/lean-sim/internal/lease"
)

// Wire format: JSON-RPC 2.0, one message per line, requests on stdin, responses
// and notifications on stdout. This file is the framing and the error model;
// the methods live in methods_*.go.

// JSON-RPC error codes. Protocol errors use the standard numbers; domain errors
// use Sthin's stable exit codes so the Desktop and the CLI agree on meaning.
const (
	CodeParse         = -32700
	CodeInvalidReq    = -32600
	CodeMethodMissing = -32601
	CodeInvalidParams = -32602
	CodeCancelled     = -32800 // the request was cancelled by the client
	CodeFailure       = 1      // sthin exit 1
	CodeUsage         = 2      // sthin exit 2: unknown device, bad argument, physical refused
	CodeNoDevice      = 3      // sthin exit 3: no device to lease
)

// Error is an error with a JSON-RPC code and optional structured data.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *Error) Error() string { return e.Message }

func invalidParams(format string, a ...any) *Error {
	return &Error{Code: CodeInvalidParams, Message: fmt.Sprintf(format, a...)}
}

// toError maps any error to the wire error the client sees.
func toError(err error) *Error {
	var e *Error
	switch {
	case errors.As(err, &e):
		return e
	case errors.Is(err, context.Canceled):
		return &Error{Code: CodeCancelled, Message: "request cancelled"}
	case errors.Is(err, device.ErrUsage):
		return &Error{Code: CodeUsage, Message: err.Error()}
	case errors.Is(err, lease.ErrNoDevice):
		return &Error{Code: CodeNoDevice, Message: err.Error()}
	default:
		return &Error{Code: CodeFailure, Message: err.Error()}
	}
}

// incoming is one request line as received.
type incoming struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// hasID reports whether the message is a request (needs a response) rather
// than a notification.
func (m incoming) hasID() bool { return len(m.ID) > 0 && string(m.ID) != "null" }

// writer serialises every outgoing message onto one stream.
type writer struct {
	mu  sync.Mutex
	out *bufio.Writer
}

func newWriter(w io.Writer) *writer { return &writer{out: bufio.NewWriter(w)} }

func (w *writer) send(msg any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.out.Write(append(b, '\n')); err != nil {
		return err
	}
	return w.out.Flush()
}

func (w *writer) result(id json.RawMessage, result any) error {
	return w.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (w *writer) fail(id json.RawMessage, e *Error) error {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return w.send(map[string]any{"jsonrpc": "2.0", "id": id, "error": e})
}

func (w *writer) notify(method string, params any) error {
	return w.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// readLines delivers each stdin line to fn until the reader ends or ctx is done.
// Lines are capped at 1 MiB, more than any request Sthin defines.
func readLines(ctx context.Context, r io.Reader, fn func([]byte)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := make([]byte, len(sc.Bytes()))
		copy(line, sc.Bytes())
		fn(line)
	}
	return sc.Err()
}
