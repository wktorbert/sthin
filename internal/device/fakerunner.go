package device

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// FakeResponse is one scripted result for an argv.
type FakeResponse struct {
	Stdout string
	Stderr string
	Err    error
}

// FakeRunner is a scripted Runner keyed by the space-joined argv. Each argv
// holds a queue of responses; the last one repeats once the queue drains.
// Every call is recorded. Unscripted calls fail.
type FakeRunner struct {
	mu      sync.Mutex
	script  map[string][]FakeResponse
	Calls   []string
	Started []string
	Passed  []string // argv handed to Passthrough, prefixed by the working directory
	streams map[string][]string
}

// NewFakeRunner returns an empty FakeRunner.
func NewFakeRunner() *FakeRunner {
	return &FakeRunner{script: map[string][]FakeResponse{}}
}

// On appends responses for argv (e.g. "xcrun simctl list -j runtimes").
func (f *FakeRunner) On(argv string, rs ...FakeResponse) *FakeRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.script[argv] = append(f.script[argv], rs...)
	return f
}

// OK scripts a successful call printing stdout.
func (f *FakeRunner) OK(argv, stdout string) *FakeRunner {
	return f.On(argv, FakeResponse{Stdout: stdout})
}

// Fail scripts a failing call.
func (f *FakeRunner) Fail(argv, stderr string) *FakeRunner {
	return f.On(argv, FakeResponse{Stderr: stderr, Err: errors.New("exit status 1")})
}

// Run implements Runner.
func (f *FakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	argv := strings.Join(append([]string{name}, args...), " ")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, argv)
	q, ok := f.script[argv]
	if !ok || len(q) == 0 {
		return nil, nil, fmt.Errorf("fake runner: unscripted command %q", argv)
	}
	r := q[0]
	if len(q) > 1 {
		f.script[argv] = q[1:]
	}
	return []byte(r.Stdout), []byte(r.Stderr), r.Err
}

// Start implements Runner; it records the argv and never spawns anything.
func (f *FakeRunner) Start(_ context.Context, logPath, name string, args ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Started = append(f.Started, strings.Join(append([]string{name}, args...), " "))
	return nil
}

// Passthrough implements Runner; it records the argv (prefixed by dir) and never spawns anything.
func (f *FakeRunner) Passthrough(_ context.Context, dir, name string, args ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Passed = append(f.Passed, dir+": "+strings.Join(append([]string{name}, args...), " "))
	return nil
}

// Streams scripts the lines Stream delivers for argv.
func (f *FakeRunner) Streams(argv string, lines ...string) *FakeRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.streams == nil {
		f.streams = map[string][]string{}
	}
	f.streams[argv] = lines
	return f
}

// Stream implements Runner: it records the argv and replays the scripted
// lines, then closes the channel (or stops early when ctx is cancelled).
func (f *FakeRunner) Stream(ctx context.Context, name string, args ...string) (<-chan string, error) {
	argv := strings.Join(append([]string{name}, args...), " ")
	f.mu.Lock()
	f.Calls = append(f.Calls, argv)
	lines, ok := f.streams[argv]
	f.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("fake runner: unscripted stream %q", argv)
	}
	ch := make(chan string, len(lines))
	go func() {
		defer close(ch)
		for _, l := range lines {
			select {
			case ch <- l:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// Called reports whether argv was run at least once.
func (f *FakeRunner) Called(argv string) bool {
	return f.Count(argv) > 0
}

// Count reports how many times argv was run.
func (f *FakeRunner) Count(argv string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.Calls {
		if c == argv {
			n++
		}
	}
	return n
}
