// Package logs is the unified device log model: one line shape and one level
// scale for iOS (log stream, log show) and Android (logcat), plus the filter
// and the bounded buffer the viewer and the CLI share.
package logs

import (
	"regexp"
	"strings"
	"sync"
)

// Level is the normalised severity across platforms.
type Level int

const (
	Verbose Level = iota
	Debug
	Info
	Warn
	Error
	Fatal
)

// Levels in display order; String returns the short name used in filters.
var levelNames = []string{"verbose", "debug", "info", "warn", "error", "fatal"}

func (l Level) String() string {
	if int(l) < len(levelNames) {
		return levelNames[l]
	}
	return "info"
}

// ParseLevel accepts the long or single-letter names; ok is false when unknown.
func ParseLevel(s string) (Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "v", "verbose":
		return Verbose, true
	case "d", "debug":
		return Debug, true
	case "i", "info", "default":
		return Info, true
	case "w", "warn", "warning":
		return Warn, true
	case "e", "error":
		return Error, true
	case "f", "fatal", "fault":
		return Fatal, true
	}
	return Info, false
}

// Line is one normalised log line.
type Line struct {
	Time    string `json:"time"` // as printed by the platform
	Level   Level  `json:"level"`
	Process string `json:"process"` // iOS process name, Android tag
	Message string `json:"message"`
	Raw     string `json:"raw"`
}

// iOS `log stream --style compact` / `log show --style compact`:
//
//	2026-09-25 14:55:19.281 E  Spotlight[39903:5e2956] [com.apple.siri:Client] message
//
// type token: Df (default), I, Db, E, F.
var iosRe = regexp.MustCompile(`^(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d\.\d+)\s+(Df|Db|I|E|F)\s+([^\[\s]+)\[[^\]]*\]\s*(.*)$`)

// Android `logcat -v threadtime`:
//
//	09-25 14:55:19.281  1234  5678 I ActivityManager: message
var logcatRe = regexp.MustCompile(`^(\d\d-\d\d \d\d:\d\d:\d\d\.\d+)\s+\d+\s+\d+\s+([VDIWEF])\s+(.*?)\s*:\s?(.*)$`)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// Clean strips ANSI colour escapes and leading control characters that a
// pseudo-terminal adds to a captured line.
func Clean(s string) string {
	s = ansiRe.ReplaceAllString(s, "")
	return strings.TrimLeft(strings.TrimRight(s, "\r"), "\x00\x01\x02\x03\x04\x05\x06\x07\x08")
}

// ParseIOS parses one compact unified-log line; unrecognised lines become Info with the raw text.
func ParseIOS(s string) Line {
	s = Clean(s)
	m := iosRe.FindStringSubmatch(s)
	if m == nil {
		return Line{Level: Info, Message: s, Raw: s}
	}
	lvl := Info
	switch m[2] {
	case "Db":
		lvl = Debug
	case "E":
		lvl = Error
	case "F":
		lvl = Fatal
	}
	return Line{Time: m[1], Level: lvl, Process: m[3], Message: m[4], Raw: s}
}

// ParseLogcat parses one threadtime logcat line; unrecognised lines become Info with the raw text.
func ParseLogcat(s string) Line {
	m := logcatRe.FindStringSubmatch(s)
	if m == nil {
		return Line{Level: Info, Message: s, Raw: s}
	}
	lvl, _ := ParseLevel(m[2])
	return Line{Time: m[1], Level: lvl, Process: m[3], Message: m[4], Raw: s}
}

// Filter narrows lines by minimum level and a case-insensitive substring.
type Filter struct {
	MinLevel Level
	Text     string
}

// Match reports whether l passes the filter.
func (f Filter) Match(l Line) bool {
	if l.Level < f.MinLevel {
		return false
	}
	if f.Text == "" {
		return true
	}
	t := strings.ToLower(f.Text)
	return strings.Contains(strings.ToLower(l.Raw), t)
}

// Ring keeps the most recent Max lines; safe for one writer and readers.
type Ring struct {
	Max   int
	mu    sync.Mutex
	lines []Line
	total int
}

// Add appends lines, dropping the oldest beyond Max.
func (r *Ring) Add(ls ...Line) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, ls...)
	r.total += len(ls)
	if r.Max > 0 && len(r.lines) > r.Max {
		r.lines = append([]Line(nil), r.lines[len(r.lines)-r.Max:]...)
	}
}

// Snapshot returns the lines matching f, and how many lines were ever added.
func (r *Ring) Snapshot(f Filter) ([]Line, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Line, 0, len(r.lines))
	for _, l := range r.lines {
		if f.Match(l) {
			out = append(out, l)
		}
	}
	return out, r.total
}

// Clear drops every buffered line.
func (r *Ring) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines, r.total = nil, 0
}

// Format renders a line for the terminal and files: time, level tag, process, message.
func Format(l Line) string {
	if l.Time == "" {
		return l.Raw
	}
	tag := strings.ToUpper(l.Level.String()[:1])
	return l.Time + " " + tag + " " + l.Process + ": " + l.Message
}
