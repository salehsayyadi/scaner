package tui

import (
	"strings"
	"testing"

	"piyazche/utils"

	tea "github.com/charmbracelet/bubbletea"
)

// TestMergeRangePresetDedup ensures pressing the same preset key twice (or
// stacking overlapping presets) never duplicates lines already in the range box.
func TestMergeRangePresetDedup(t *testing.T) {
	merged, added := mergeRangePreset("", presetCFPublic)
	if added != strings.Count(presetCFPublic, "\n")+1 {
		t.Fatalf("first merge added %d, want all lines", added)
	}
	again, added2 := mergeRangePreset(merged, presetCFPublic)
	if added2 != 0 {
		t.Fatalf("re-applying the same preset added %d new lines, want 0", added2)
	}
	if again != merged {
		t.Fatalf("re-applying the same preset changed the text")
	}

	// Every line of every preset must be a valid CIDR/IP so the scanner accepts it.
	for name, preset := range map[string]string{
		"cf-public": presetCFPublic, "cf-london": presetCFLondon, "fastly": presetFastly,
	} {
		for _, line := range strings.Split(preset, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if !utils.IsValidCIDR(line) && !utils.IsValidIP(line) {
				t.Errorf("%s: invalid entry %q", name, line)
			}
		}
	}
}

// TestRangePresetKeysInsert exercises the actual F2/F3/F4 handling through the
// Bubble Tea update loop (not just the pure merge helper) on the Range tab.
func TestRangePresetKeysInsert(t *testing.T) {
	var m tea.Model = New(nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")}) // Range tab
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyF2})
	mm := m.(model)
	if !strings.Contains(mm.rangeArea.Value(), "173.245.48.0/20") {
		t.Fatalf("F2 did not insert Cloudflare public ranges: %q", mm.rangeArea.Value())
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyF4})
	mm = m.(model)
	if !strings.Contains(mm.rangeArea.Value(), "151.101.0.0/16") {
		t.Fatalf("F4 did not insert Fastly ranges")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyF3})
	mm = m.(model)
	if !strings.Contains(mm.rangeArea.Value(), "154.92.9.0/24") {
		t.Fatalf("F3 did not insert Cloudflare London ranges")
	}

	if out := m.View(); out == "" {
		t.Fatal("empty view after inserting presets")
	}
}
