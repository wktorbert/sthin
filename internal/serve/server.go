// Package serve is the Surface an out-of-process client such as the Desktop
// uses: a long-lived JSON-RPC 2.0 session over stdio with live notifications
// for stage progress, log lines and device list changes. Like mcpserver it is
// a thin adapter: every method goes through device.Provider, device.Controller
// or the rules in internal/ops, never to simctl or adb directly.
package serve

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/wktorbert/lean-sim/internal/device"
	"github.com/wktorbert/lean-sim/internal/doctor"
	"github.com/wktorbert/lean-sim/internal/lease"
	"github.com/wktorbert/lean-sim/internal/state"
)

// SchemaVersion is bumped when a method's params or result change shape.
const SchemaVersion = 1

// Deps is what the session needs from the host process.
type Deps struct {
	Reg     *device.Registry
	Pool    lease.Pool
	Prefs   state.Store
	Version string
	// Doctor runs the host checks; nil disables the doctor method.
	Doctor func(context.Context) []doctor.Check
	// PollInterval is how often a devices subscription re-lists (default 2 s).
	PollInterval time.Duration
	// LogFlush and LogBatch bound how log lines are batched into notifications
	// (default 50 ms and 2000 lines).
	LogFlush time.Duration
	LogBatch int
}

// Server is one session.
type Server struct {
	Deps
	w        *writer
	mu       sync.Mutex
	inflight map[string]context.CancelFunc // request id (raw JSON) → cancel
	kept     map[string]bool               // ids of subscriptions that outlive their handler
	methods  map[string]handler
	wg       sync.WaitGroup
}

// request is what a handler receives: the decoded envelope plus the ways to
// talk back before the final result.
type request struct {
	s      *Server
	id     json.RawMessage
	params json.RawMessage
	ctx    context.Context
}

type handler func(*request) (any, error)

// New builds a session with every method registered.
func New(d Deps) *Server {
	if d.PollInterval <= 0 {
		d.PollInterval = 2 * time.Second
	}
	if d.LogFlush <= 0 {
		d.LogFlush = 50 * time.Millisecond
	}
	if d.LogBatch <= 0 {
		d.LogBatch = 2000
	}
	s := &Server{Deps: d, inflight: map[string]context.CancelFunc{}, kept: map[string]bool{}}
	s.methods = map[string]handler{
		"initialize":        s.initialize,
		"devices_list":      s.devicesList,
		"devices_subscribe": s.devicesSubscribe,
		"boot":              s.boot,
		"restore":           s.restore,
		"shutdown":          s.shutdown,
		"rename":            s.rename,
		"measure":           s.measure,
		"doctor":            s.doctor,
		"profile":           s.profile,
		"prefs_get":         s.prefsGet,
		"leases":            s.leases,
		"adb_pair":          s.adbPair,
		"adb_connect":       s.adbConnect,
		"adb_disconnect":    s.adbDisconnect,
		"logs":              s.logs,
		"logs_follow":       s.logsFollow,
		"cancel":            s.cancel,
	}
	return s
}

// Run serves until in closes or ctx ends. Every request runs concurrently so a
// list stays responsive while a boot is in flight.
func (s *Server) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	s.w = newWriter(out)
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	err := readLines(ctx, in, func(line []byte) { s.dispatch(ctx, line) })
	stop()
	s.wg.Wait()
	return err
}

func (s *Server) dispatch(ctx context.Context, line []byte) {
	var m incoming
	if err := json.Unmarshal(line, &m); err != nil {
		s.w.fail(nil, &Error{Code: CodeParse, Message: "parse error: " + err.Error()})
		return
	}
	if m.Method == "" {
		s.w.fail(m.ID, &Error{Code: CodeInvalidReq, Message: "missing method"})
		return
	}
	h, ok := s.methods[m.Method]
	if !ok {
		if m.hasID() {
			s.w.fail(m.ID, &Error{Code: CodeMethodMissing, Message: "unknown method " + m.Method})
		}
		return
	}
	rctx, cancel := context.WithCancel(ctx)
	key := string(m.ID)
	if m.hasID() {
		s.mu.Lock()
		s.inflight[key] = cancel
		s.mu.Unlock()
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			if m.hasID() {
				s.finish(key)
			} else {
				cancel() // a notification has no id to cancel it by
			}
		}()
		res, err := h(&request{s: s, id: m.ID, params: m.Params, ctx: rctx})
		if !m.hasID() {
			return
		}
		if err != nil {
			s.w.fail(m.ID, toError(err))
			return
		}
		s.w.result(m.ID, res)
	}()
}

// finish forgets a request once its handler returned, unless the handler
// registered a subscription that outlives it (see request.keep).
func (s *Server) finish(key string) {
	s.mu.Lock()
	kept := s.kept[key]
	s.mu.Unlock()
	if !kept {
		s.forget(key)
	}
}

func (s *Server) forget(key string) {
	s.mu.Lock()
	cancel := s.inflight[key]
	delete(s.inflight, key)
	delete(s.kept, key)
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// decode unmarshals params into v; absent or null params decode as empty.
func (r *request) decode(v any) error {
	if len(r.params) == 0 || string(r.params) == "null" {
		return nil
	}
	if err := json.Unmarshal(r.params, v); err != nil {
		return invalidParams("invalid params: %v", err)
	}
	return nil
}

// progress sends a stage for this request while it is in flight.
func (r *request) progress(st device.Stage) {
	r.s.w.notify("progress", map[string]any{"id": r.id, "stage": st})
}

// notify sends a notification tagged with this request's id.
func (r *request) notify(method string, fields map[string]any) {
	fields["id"] = r.id
	r.s.w.notify(method, fields)
}

// keep marks this request as a subscription: it stays cancellable after the
// handler returned its initial result. The background goroutine must call
// r.end when it stops.
func (r *request) keep(run func(ctx context.Context)) {
	r.s.mu.Lock()
	r.s.kept[string(r.id)] = true
	r.s.mu.Unlock()
	ctx := r.ctx
	r.s.wg.Add(1)
	go func() {
		defer r.s.wg.Done()
		run(ctx)
	}()
}

// end reports why a subscription stopped and releases its id.
func (r *request) end(reason string) {
	r.notify("subscription_end", map[string]any{"reason": reason})
	r.s.forget(string(r.id))
}

// cancel stops an in-flight request or subscription by id.
func (s *Server) cancel(r *request) (any, error) {
	var in struct {
		ID json.RawMessage `json:"id"`
	}
	if err := r.decode(&in); err != nil {
		return nil, err
	}
	key := string(in.ID)
	s.mu.Lock()
	cancel, ok := s.inflight[key]
	s.mu.Unlock()
	if !ok || cancel == nil {
		return nil, invalidParams("no request in flight with id %s", key)
	}
	cancel()
	return map[string]any{"cancelled": true}, nil
}
