package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"piyazche/applog"
	"piyazche/scanner"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/table"
)

// --- Log tab -------------------------------------------------------------

func (m *model) refreshLog() {
	lines := applog.Snapshot()
	m.logView.SetContent(strings.Join(lines, "\n"))
	if m.logFollow {
		m.logView.GotoBottom()
	}
}

func (m model) viewLog() string {
	follow := errStyle.Render("paused")
	if m.logFollow {
		follow = okStyle.Render("following")
	}
	head := titleStyle.Render("Live log") + "  " + follow +
		hintStyle.Render("  ·  "+applog.Path()+"  ·  scroll with ↑↓ / wheel, f to toggle follow")
	return head + "\n" + m.logView.View()
}

// --- History tab ---------------------------------------------------------

type histEntry struct {
	path string
	mod  time.Time
	name string
	rows int
}

func (m *model) loadHistory() {
	if m.histLoaded {
		return
	}
	const dir = "results"
	entries, err := os.ReadDir(dir)
	if err != nil {
		m.histStatus = "No results/ directory yet — run a scan first."
		m.histLoaded = true
		m.histTable.SetRows(nil)
		m.history = nil
		return
	}

	var hs []histEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".csv") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		p := filepath.Join(dir, e.Name())
		rows := 0
		if rs, err := scanner.LoadResultsCSV(p); err == nil {
			rows = len(rs)
		}
		hs = append(hs, histEntry{path: p, mod: info.ModTime(), name: e.Name(), rows: rows})
	}
	sort.Slice(hs, func(i, j int) bool { return hs[i].mod.After(hs[j].mod) })

	m.history = hs
	tableRows := make([]table.Row, 0, len(hs))
	for _, h := range hs {
		tableRows = append(tableRows, table.Row{
			h.mod.Format("2006-01-02 15:04"),
			h.name,
			fmt.Sprintf("%d", h.rows),
		})
	}
	m.histTable.SetRows(tableRows)
	m.histLoaded = true
	if len(hs) == 0 {
		m.histStatus = "No saved scans in results/ yet."
	} else {
		m.histStatus = fmt.Sprintf("%d saved scans — newest first", len(hs))
	}
}

func (m *model) openHistory() {
	if m.scanning {
		m.histStatus = "Stop the running scan before opening a saved one."
		return
	}
	cur := m.histTable.Cursor()
	if cur < 0 || cur >= len(m.history) {
		return
	}
	h := m.history[cur]
	results, err := scanner.LoadResultsCSV(h.path)
	if err != nil {
		m.histStatus = "Open failed: " + err.Error()
		return
	}
	m.collector = scanner.NewResultCollectorFrom(results)
	m.rebuildTable()
	m.total, m.done, m.successCnt = len(results), len(results), len(results)
	m.status = "Loaded " + h.name + " — browse with ↑↓, c/C to copy"
	m.active = tabRun
	m.menuFocus = false
	m.syncFocus()
	applog.Logf("history: opened %s (%d IPs)", h.name, len(results))
}

func (m *model) copyHistoryIPs() {
	cur := m.histTable.Cursor()
	if cur < 0 || cur >= len(m.history) {
		return
	}
	results, err := scanner.LoadResultsCSV(m.history[cur].path)
	if err != nil {
		m.histStatus = "Copy failed: " + err.Error()
		return
	}
	if err := clipboard.WriteAll(joinIPs(results)); err != nil {
		m.histStatus = "Clipboard unavailable: " + err.Error()
		return
	}
	m.histStatus = fmt.Sprintf("Copied %d IPs from %s", len(results), m.history[cur].name)
}

func (m model) viewHistory() string {
	var b strings.Builder
	b.WriteString(hintStyle.Render("Past scans saved in results/.  ↑↓ pick · Enter open in Run · c copy IPs · r refresh") + "\n")
	if m.histStatus != "" {
		b.WriteString(hintStyle.Render(m.histStatus) + "\n")
	}
	b.WriteString("\n" + m.histTable.View())
	return b.String()
}

// --- copy helpers --------------------------------------------------------

func (m *model) copySelectedIP() {
	row := m.results.SelectedRow()
	if len(row) < 2 || row[1] == "" {
		m.status = "No IP selected."
		return
	}
	ip := row[1]
	if err := clipboard.WriteAll(ip); err != nil {
		m.status = "Clipboard unavailable: " + err.Error()
		return
	}
	m.status = "Copied " + ip
}

func (m *model) copyAllIPs() {
	if m.collector == nil {
		return
	}
	sorted := m.collector.GetSortedByLatency()
	if len(sorted) == 0 {
		m.status = "No results to copy."
		return
	}
	if err := clipboard.WriteAll(joinIPs(sorted)); err != nil {
		m.status = "Clipboard unavailable: " + err.Error()
		return
	}
	m.status = fmt.Sprintf("Copied %d IPs to clipboard", len(sorted))
}

func joinIPs(results []scanner.Result) string {
	var sb strings.Builder
	for _, r := range results {
		sb.WriteString(r.IP)
		sb.WriteByte('\n')
	}
	return sb.String()
}
