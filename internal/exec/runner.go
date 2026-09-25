// Package exec is the real Runner over os/exec.
package exec

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"time"
)

// DefaultTimeout bounds every command that does not carry its own deadline.
const DefaultTimeout = 60 * time.Second

// Runner runs host commands with a context timeout.
type Runner struct {
	Timeout time.Duration
}

// New returns a Runner with the default timeout.
func New() *Runner { return &Runner{Timeout: DefaultTimeout} }

// Run executes name with args, returning stdout and stderr.
func (r *Runner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	if _, ok := ctx.Deadline(); !ok && r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	cmd := osexec.CommandContext(ctx, name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("%s: timed out: %w", name, ctx.Err())
	}
	return out.Bytes(), errb.Bytes(), err
}

// Stream starts name and forwards stdout lines on the returned channel until
// the process exits or ctx is cancelled. Lines up to 1 MiB are accepted.
func (r *Runner) Stream(ctx context.Context, name string, args ...string) (<-chan string, error) {
	cmd := osexec.CommandContext(ctx, name, args...)
	cmd.Stderr = nil
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	ch := make(chan string, 1024)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			select {
			case ch <- sc.Text():
			case <-ctx.Done():
				_ = cmd.Wait()
				return
			}
		}
		_ = cmd.Wait()
	}()
	return ch, nil
}

// Passthrough runs name in dir attached to this process's terminal and waits.
func (r *Runner) Passthrough(ctx context.Context, dir, name string, args ...string) error {
	cmd := osexec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// Start launches name detached in its own session, appending output to logPath.
func (r *Runner) Start(_ context.Context, logPath, name string, args ...string) error {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	cmd := osexec.Command(name, args...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = f, f, nil
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
