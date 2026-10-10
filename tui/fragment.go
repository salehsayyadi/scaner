package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"piyazche/applog"
	"piyazche/config"
	"piyazche/optimizer"
	"piyazche/utils"

	tea "github.com/charmbracelet/bubbletea"
)

// updateFragment handles the Fragment tab: editing fragment fields and the
// "optimize" action (which is fully separate from the IP scan).
func (m model) updateFragment(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "o", "O":
		if !m.optimizing {
			return m.startOptimize()
		}
		return m, nil
	}
	idx, cmd, changed := m.editFieldList(m.fragFields, m.fragIdx, msg)
	m.fragIdx = idx
	if changed {
		m.autosave()
	}
	return m, cmd
}

func optListen(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

// firstFragmentTarget picks an IP/host to optimize against: the server address
// from the config, falling back to the first IP in the range list.
func (m model) firstFragmentTarget() string {
	if m.cfg.Proxy.Address != "" {
		return m.cfg.Proxy.Address
	}
	if ips, err := utils.ParseIPsFromText(m.rangeArea.Value(), 1, true); err == nil && len(ips) > 0 {
		return ips[0]
	}
	return ""
}

// startOptimize runs the fragment optimizer in the background, streaming
// progress lines and ending with optDoneMsg. It tests the three zones
// (tlshello, 1-3, 1-5) across the configured size/interval search ranges and
// applies the best result — without touching the main scan.
func (m model) startOptimize() (tea.Model, tea.Cmd) {
	if m.scanning {
		m.optStatus = "Stop the scan before optimizing."
		return m, nil
	}
	if err := m.cfg.Validate(); err != nil {
		m.optStatus = "Config invalid: " + err.Error()
		return m, nil
	}
	target := m.firstFragmentTarget()
	if target == "" {
		m.optStatus = "No target — set a config or a range first."
		return m, nil
	}

	// Deep-copy the config so concurrent UI edits can't race the optimizer.
	cfgCopy := m.cfg.Clone()

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan tea.Msg, 256)
	m.optCh = ch
	m.optCancel = cancel
	m.optimizing = true
	m.optLogs = nil
	m.optStatus = "Optimizing fragment against " + target + " …"
	applog.Logf("optimize started against %s", target)

	go func() {
		runOptimize(ctx, cfgCopy, target, ch)
	}()

	return m, optListen(ch)
}

func runOptimize(ctx context.Context, cfg *config.Config, target string, ch chan tea.Msg) {
	emit := func(s string) {
		select {
		case ch <- optLogMsg(s):
		default:
		}
	}
	// Final result: delivered if the UI is still listening, abandoned cleanly if
	// the app is quitting (ctx cancelled) — never blocks forever.
	done := func(msg optDoneMsg) {
		select {
		case ch <- msg:
		case <-ctx.Done():
		}
	}

	tester := optimizer.NewFragmentTester(cfg, target).WithContext(ctx)

	maxTests := cfg.Fragment.Auto.MaxTests
	if maxTests <= 0 || maxTests > 30 {
		maxTests = 12
	}
	fc := optimizer.FinderConfig{
		MaxTriesPerZone:   maxTests,
		SuccessThreshold:  0.5,
		MinRangeWidth:     5,
		EnableCorrelation: true,
	}

	sizeRange := optimizer.Range{Min: cfg.Fragment.Auto.LengthRange.Min, Max: cfg.Fragment.Auto.LengthRange.Max}
	if !sizeRange.IsValid() {
		sizeRange = optimizer.Range{Min: 10, Max: 80}
	}
	intervalRange := optimizer.Range{Min: cfg.Fragment.Auto.IntervalRange.Min, Max: cfg.Fragment.Auto.IntervalRange.Max}
	if !intervalRange.IsValid() {
		intervalRange = optimizer.Range{Min: 5, Max: 40}
	}

	finder := optimizer.NewFinder(fc, tester.CreateTesterFunc())
	finder.SetProgressCallback(func(zone string, attempt, total int, success bool, sizeR, intervalR optimizer.Range, latency time.Duration) {
		mark := "✗"
		extra := ""
		if success {
			mark = "✓"
			extra = fmt.Sprintf(" %dms", latency.Milliseconds())
		}
		emit(fmt.Sprintf("[%s] %s try %d/%d  size=%s interval=%s%s", zone, mark, attempt, total, sizeR, intervalR, extra))
	})

	zones := []optimizer.Zone{
		{Name: "tlshello", SizeRange: sizeRange, IntervalRange: intervalRange},
		{Name: "1-3", SizeRange: sizeRange, IntervalRange: intervalRange},
		{Name: "1-5", SizeRange: sizeRange, IntervalRange: intervalRange},
	}

	var best *optimizer.ZoneResult
	for _, z := range zones {
		if ctx.Err() != nil {
			emit("cancelled")
			done(optDoneMsg{ok: false})
			return
		}
		emit("── testing zone " + z.Name + " ──")
		res := finder.FindOne(z)
		if res.Success {
			emit(fmt.Sprintf("zone %s best: size=%s interval=%s %dms", z.Name, res.SizeRange, res.IntervalRange, res.Latency.Milliseconds()))
			r := res
			if best == nil || r.Latency < best.Latency {
				best = &r
			}
		} else {
			emit("zone " + z.Name + ": no working range")
		}
	}

	if best != nil {
		done(optDoneMsg{
			ok:       true,
			packets:  best.Zone,
			length:   best.SizeRange.String(),
			interval: best.IntervalRange.String(),
			latency:  best.Latency.Milliseconds(),
		})
	} else {
		done(optDoneMsg{ok: false})
	}
}

func (m model) viewFragment() string {
	var b strings.Builder
	b.WriteString(hintStyle.Render("↑/↓ select · ←/→ change · Enter edit · ") + keyStyle.Render("o") + hintStyle.Render(" optimize") + "\n\n")

	for i, f := range m.fragFields {
		val := f.display(m.cfg)
		if i == m.fragIdx {
			b.WriteString(keyStyle.Render("▸") + selectedStyle.Render(fmt.Sprintf(" %-18s  %s", f.label, val)))
			if f.help != "" {
				b.WriteString("  " + hintStyle.Render("— "+f.help))
			}
		} else {
			b.WriteString(" " + labelStyle.Render(fmt.Sprintf("%-18s", f.label)) + "  " + valueStyle.Render(val))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(titleStyle.Render("Auto-optimize") + hintStyle.Render(" (separate from the IP scan)") + "\n")
	if m.optimizing {
		b.WriteString(okStyle.Render("● running — testing fragment sizes…") + "\n")
	} else if m.optStatus != "" {
		st := okStyle
		if strings.HasPrefix(m.optStatus, "✗") || strings.Contains(m.optStatus, "invalid") || strings.Contains(m.optStatus, "No ") {
			st = errStyle
		}
		b.WriteString(st.Render(m.optStatus) + "\n")
	} else {
		b.WriteString(hintStyle.Render("Press o to test all fragment sizes against your server and pick the best.") + "\n")
	}

	if len(m.optLogs) > 0 {
		b.WriteString("\n" + hintStyle.Render(strings.Join(m.optLogs, "\n")))
	}
	return b.String()
}
