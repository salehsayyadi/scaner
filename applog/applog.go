// Package applog is a tiny always-on file logger used to diagnose TUI/scan bugs.
// It writes timestamped lines to a log file and is a no-op until Init is called.
package applog

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	file    *os.File
	logPath string

	bufMu sync.Mutex
	buf   []string // in-memory ring buffer for the TUI Log tab
)

const maxBuf = 5000

// Init opens (or creates+truncates) the log file. Safe to call once at startup.
func Init(path string) {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return
	}
	file = f
	logPath = path
	fmt.Fprintf(file, "%s  === piyazche log started ===\n", stamp())
	file.Sync()
}

// Path returns the active log path (empty if not initialized).
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return logPath
}

// Logf appends a timestamped line. Synced immediately so a crash still leaves a
// complete trace.
func Logf(format string, args ...interface{}) {
	line := fmt.Sprintf("%s  %s", stamp(), fmt.Sprintf(format, args...))

	bufMu.Lock()
	buf = append(buf, line)
	if len(buf) > maxBuf {
		buf = buf[len(buf)-maxBuf:]
	}
	bufMu.Unlock()

	mu.Lock()
	defer mu.Unlock()
	if file == nil {
		return
	}
	fmt.Fprintln(file, line)
	file.Sync()
}

// Snapshot returns a copy of the in-memory log buffer (oldest first). Used by
// the TUI Log tab.
func Snapshot() []string {
	bufMu.Lock()
	defer bufMu.Unlock()
	out := make([]string, len(buf))
	copy(out, buf)
	return out
}

// Close flushes and closes the log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		fmt.Fprintf(file, "%s  === piyazche log closed ===\n", stamp())
		file.Close()
		file = nil
	}
}

func stamp() string {
	return time.Now().Format("2006-01-02 15:04:05.000")
}
