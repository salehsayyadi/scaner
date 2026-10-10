package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

const userLink = "vless://932a23c7-2864-4874-a184-0aad82ee6708@89.116.180.87:8443?encryption=none&security=tls&sni=irmnets.com&fp=chrome&insecure=0&allowInsecure=0&type=ws&host=irmnets.com&path=%2Firmnets#x"

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// TestFullFlowAutosave drives the whole user journey in a temp working dir and
// asserts auto-save writes config.json + ranges.txt with the parsed config.
func TestFullFlowAutosave(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	mm := New(nil)
	mm.configArea.SetValue(userLink)
	var m tea.Model = mm
	m, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 40})

	// Parse (global Ctrl+O works from the menu too).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	mod := m.(model)
	if !mod.parseOK {
		t.Fatalf("parse not ok: %s", mod.parseStat)
	}
	if mod.cfg.Proxy.Port != 8443 || mod.cfg.Proxy.Address != "89.116.180.87" {
		t.Fatalf("config not applied: %+v", mod.cfg.Proxy)
	}

	// Parse triggers autosave -> config.json must exist with the right port.
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Fatalf("config.json not auto-saved: %v", err)
	}

	// Type a range in the Range tab; that must auto-save ranges.txt.
	m, _ = m.Update(key("3"))         // menu -> Range tab
	m, _ = m.Update(key("enter"))     // enter content
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	for _, r := range "89.116.180.0/24" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	data, err := os.ReadFile(filepath.Join(dir, "ranges.txt"))
	if err != nil {
		t.Fatalf("ranges.txt not auto-saved: %v", err)
	}
	if !strings.Contains(string(data), "89.116.180.0/24") {
		t.Errorf("ranges.txt missing the typed range: %q", string(data))
	}

	// Reload: a fresh model should restore the saved session.
	m2 := New(nil)
	if m2.cfg.Proxy.Port != 8443 {
		t.Errorf("session config not reloaded: port=%d", m2.cfg.Proxy.Port)
	}
	if !strings.Contains(m2.rangeArea.Value(), "89.116.180.0/24") {
		t.Errorf("session ranges not reloaded: %q", m2.rangeArea.Value())
	}
}

// TestAllTabsRender cycles through all five tabs (menu nav) and renders each.
func TestAllTabsRender(t *testing.T) {
	var m tea.Model = New(nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	for i := 0; i < nTabs(); i++ {
		mod := m.(model)
		if int(mod.active) != i {
			t.Fatalf("expected active tab %d, got %d", i, mod.active)
		}
		if out := m.View(); out == "" {
			t.Fatalf("empty view on tab %d", i)
		}
		// Dive in and back out, then advance.
		m, _ = m.Update(key("enter"))
		m, _ = m.Update(key("esc"))
		m, _ = m.Update(key("right"))
	}
}

// TestFragmentTabFields ensures the Fragment tab has its own fields and the
// optimize action is wired without panicking (it no-ops without a real server).
func TestFragmentTabRender(t *testing.T) {
	var m tea.Model = New(nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	m, _ = m.Update(key("4")) // Fragment tab
	m, _ = m.Update(key("enter"))
	mod := m.(model)
	if mod.active != tabFragment {
		t.Fatalf("not on fragment tab: %d", mod.active)
	}
	if len(mod.fragFields) == 0 {
		t.Fatal("no fragment fields")
	}
	// Toggle Enabled and edit nothing destructive; just render.
	m, _ = m.Update(key(" "))
	if out := m.View(); !strings.Contains(out, "optimize") && !strings.Contains(out, "Auto-optimize") {
		t.Errorf("fragment view missing optimize section")
	}
}

// TestScanStateReset verifies a completed/stopped scan resets cleanly so a new
// scan can start (the reported pause/stop -> can't restart bug).
func TestScanStateReset(t *testing.T) {
	mm := New(nil)
	mm.scanning = true
	mm.paused = true
	var m tea.Model = mm
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	// Simulate the scanner goroutine finishing.
	m, _ = m.Update(scanDoneMsg{})
	mod := m.(model)
	if mod.scanning || mod.paused || mod.sc != nil {
		t.Fatalf("state not reset after done: scanning=%v paused=%v sc=%v", mod.scanning, mod.paused, mod.sc != nil)
	}
}
