// Package applog is a tiny always-on file logger used to diagnose TUI/scan bugs.
// It writes timestamped lines to a log file and is a no-op until Init is called.
//
// Performance: the original implementation called fsync after every line while
// holding a global lock. With dozens of workers logging several lines per IP
// this serialized the whole scan on disk flushes and froze the UI. Lines are now
// buffered and flushed to the OS at most every 500 ms (and on Close).
package applog

import (
	"bufio"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var (
	mu      sync.Mutex
	file    *os.File
	writer  *bufio.Writer
	logPath string
	stopCh  chan struct{}
	doneCh  chan struct{}

	bufMu sync.Mutex
	buf   []string // in-memory buffer for the Log views (last maxBuf lines)
	gen   atomic.Uint64
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
	writer = bufio.NewWriterSize(f, 256<<10)
	logPath = path
	fmt.Fprintf(writer, "%s  === piyazche log started ===\n", stamp())
	writer.Flush()
	stopCh, doneCh = make(chan struct{}), make(chan struct{})
	go flusher(stopCh, doneCh)
}

func flusher(stop, done chan struct{}) {
	defer close(done)
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			mu.Lock()
			if writer != nil {
				writer.Flush()
			}
			mu.Unlock()
		}
	}
}

// Path returns the active log path (empty if not initialized).
func Path() string {
	mu.Lock()
	defer mu.Unlock()
	return logPath
}

// Logf appends a timestamped line.
func Logf(format string, args ...interface{}) {
	line := stamp() + "  " + fmt.Sprintf(format, args...)

	bufMu.Lock()
	buf = append(buf, line)
	if len(buf) > 2*maxBuf {
		buf = append(make([]string, 0, 2*maxBuf+1), buf[len(buf)-maxBuf:]...)
	}
	bufMu.Unlock()
	gen.Add(1)

	mu.Lock()
	if writer != nil {
		writer.WriteString(line)
		writer.WriteByte('\n')
	}
	mu.Unlock()
}

// Generation changes whenever a line is logged, so views can skip redraws.
func Generation() uint64 { return gen.Load() }

// Snapshot returns a copy of the last lines (oldest first).
func Snapshot() []string { return Tail(maxBuf) }

// Tail returns a copy of the last n lines (oldest first).
func Tail(n int) []string {
	bufMu.Lock()
	defer bufMu.Unlock()
	if n <= 0 || n > len(buf) {
		n = len(buf)
	}
	if n > maxBuf {
		n = maxBuf
	}
	out := make([]string, n)
	copy(out, buf[len(buf)-n:])
	return out
}

// Close flushes and closes the log file.
func Close() {
	mu.Lock()
	stop, done := stopCh, doneCh
	stopCh, doneCh = nil, nil
	mu.Unlock()
	if stop != nil {
		close(stop)
		<-done
	}
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		fmt.Fprintf(writer, "%s  === piyazche log closed ===\n", stamp())
		writer.Flush()
		file.Sync()
		file.Close()
		file, writer = nil, nil
	}
}

func stamp() string {
	return time.Now().Format("2006-01-02 15:04:05.000")
}

// Clear erases on-disk and in-memory events without unlinking an open log file.
// Call only after scanner/optimizer tasks have finished.
func Clear() error {
	mu.Lock()
	if writer != nil {
		if err := writer.Flush(); err != nil {
			mu.Unlock()
			return err
		}
		if err := file.Truncate(0); err != nil {
			mu.Unlock()
			return err
		}
		if _, err := file.Seek(0, 0); err != nil {
			mu.Unlock()
			return err
		}
		writer.Reset(file)
	}
	mu.Unlock()
	bufMu.Lock()
	buf = nil
	bufMu.Unlock()
	gen.Add(1)
	return nil
}
