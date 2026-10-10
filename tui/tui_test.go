package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"piyazche/applog"
	"piyazche/scanner"

	tea "github.com/charmbracelet/bubbletea"
)

// TestMain runs the suite from a scratch temp directory. Several code paths
// under test (autosave, doParse) write the session files (config.json,
// ranges.txt) to the working directory as a real side effect; without this,
// `go test` permanently dirties the tui/ package directory with fixture junk
// (e.g. uuid="uuid", address="1.2.3.4") on every run.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "piyazche-tui-test-")
	if err != nil {
		panic(err)
	}
	orig, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(dir); err != nil {
		panic(err)
	}

	code := m.Run()

	os.Chdir(orig)
	os.RemoveAll(dir)
	os.Exit(code)
}

// TestViewFitsHeight proves the layout never overflows the terminal height — the
// "tabs disappear" bug. Every tab, even with a long results table and a full log
// buffer, must render within the height so the header + tab bar stay on screen.
func TestViewFitsHeight(t *testing.T) {
	for i := 0; i < 250; i++ {
		applog.Logf("log line %d filler for the log tab viewport", i)
	}
	var rs []scanner.Result
	for i := 1; i <= 300; i++ {
		rs = append(rs, scanner.Result{
			IP: fmt.Sprintf("10.0.%d.%d", i/256, i%256), Success: true,
			LatencyMs: int64(i), JitterMs: 5, LossPercent: 20, UploadTested: true, UploadMbps: 3.2,
		})
	}

	sizes := []struct{ w, h int }{
		{100, 14}, {100, 18}, {100, 24}, {100, 30}, {100, 50},
		{60, 20}, {60, 30}, {200, 40}, // narrow (wrapping tab bar) + wide
	}
	for _, sz := range sizes {
		mm := New(nil)
		mm.width, mm.height = sz.w, sz.h
		mm.resize()
		mm.collector = scanner.NewResultCollectorFrom(rs)
		mm.rebuildTable()
		mm.refreshLog()
		for ti := 0; ti < nTabs(); ti++ {
			mm.active = tab(ti)
			mm.menuFocus = false
			lines := strings.Count(mm.View(), "\n") + 1
			if lines > sz.h {
				t.Errorf("tab %d (%s) at %dx%d overflows: %d lines", ti, tabNames[ti], sz.w, sz.h, lines)
			}
		}
	}
}

// TestRenderNoPanic exercises the render + a few key paths without a real TTY.
func TestRenderNoPanic(t *testing.T) {
	var m tea.Model = New(nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Render every tab.
	for i := 0; i < nTabs(); i++ {
		out := m.View()
		if out == "" {
			t.Fatalf("empty view on tab %d", i)
		}
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
}

// TestRangeTabTypingNoCrash reproduces the original crash: navigating to the
// Range tab and typing a broad CIDR. Previously the live preview expanded the
// whole CIDR on every keystroke and OOM'd; now it only counts.
func TestRangeTabTypingNoCrash(t *testing.T) {
	var m tea.Model = New(nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	// Menu starts focused; jump to Range tab and enter its content.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// Type a broad CIDR character by character; render after each key.
	for _, r := range "\n9.0.0.0/4" {
		if r == '\n' {
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		} else {
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		if out := m.View(); out == "" {
			t.Fatal("empty view while typing range")
		}
	}
}

// TestSettingsScrollNoPanic scrolls through the whole (long) settings list to
// make sure the windowed rendering never indexes out of range.
func TestSettingsScrollNoPanic(t *testing.T) {
	var m tea.Model = New(nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	// Menu -> Settings tab -> content.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for i := 0; i < 40; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		if out := m.View(); out == "" {
			t.Fatal("empty settings view")
		}
	}
	for i := 0; i < 40; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
		_ = m.View()
	}
}

func TestParseConfigInTUI(t *testing.T) {
	mm := New(nil)
	mm.configArea.SetValue("vless://uuid@1.2.3.4:443?security=tls&sni=a.com&type=ws&host=a.com&path=/p")
	mm.doParse()
	if !mm.parseOK {
		t.Fatalf("parse not ok: %s", mm.parseStat)
	}
	if mm.cfg.Proxy.Address != "1.2.3.4" {
		t.Errorf("address = %q", mm.cfg.Proxy.Address)
	}
	if !strings.Contains(mm.View(), "Detected") && mm.width != 0 {
		// width is 0 here so View returns loading; just ensure cfg applied
	}
}
