package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"piyazche/applog"
	"piyazche/config"
	"piyazche/scanner"
	"piyazche/utils"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// tickMsg drives periodic refresh of the Log tab while a scan streams.
type tickMsg struct{}

func tick() tea.Cmd {
	return tea.Tick(700*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

// Auto-saved session files (next to the working directory).
const (
	sessionConfigPath = "config.json"
	sessionRangesPath = "ranges.txt"
)

// defaultRanges seeds the Range tab with common Cloudflare ranges so a scan can
// start immediately.
const defaultRanges = `162.159.192.0/24
162.159.193.0/24
162.159.195.0/24
188.114.96.0/24
188.114.97.0/24
188.114.98.0/24
188.114.99.0/24`

type tab int

const (
	tabConfig tab = iota
	tabSettings
	tabRange
	tabFragment
	tabRun
	tabLog
	tabHistory
)

var tabNames = []string{"Config", "Settings", "Range", "Fragment", "Run", "Log", "History"}

func nTabs() int { return len(tabNames) }

// scan event messages pushed onto the model's event channel.
type scanResultMsg scanner.Result
type scanProgressMsg struct{ done, total int }
type scanDoneMsg struct {
	err        error
	outputPath string
}

// fragment-optimizer event messages.
type optLogMsg string
type optDoneMsg struct {
	ok       bool
	packets  string
	length   string
	interval string
	latency  int64
}

type model struct {
	width, height int
	active        tab
	menuFocus     bool // true: top menu has focus (arrows switch tabs); false: tab content
	cfg           *config.Config

	// Config tab
	configArea textarea.Model
	parseStat  string
	parseOK    bool

	// Settings tab
	fields    []sfield
	fieldIdx  int
	editing   bool
	editInput textinput.Model
	editApply func(*config.Config, string) // commit target for the active inline edit

	// Range tab
	rangeArea   textarea.Model
	rangeStatus string

	// Fragment tab
	fragFields []sfield
	fragIdx    int
	optimizing bool
	optLogs    []string
	optStatus  string
	optCh      chan tea.Msg
	optCancel  context.CancelFunc

	// Run tab
	scanning   bool
	paused     bool
	stopping   bool
	done       int
	total      int
	successCnt int
	results    table.Model
	cols       []table.Column
	collector  *scanner.ResultCollector
	logs       []string
	status     string
	evCh       chan tea.Msg
	sc         *scanner.Scanner

	// Log tab
	logView   viewport.Model
	logFollow bool

	// History tab
	history    []histEntry
	histIdx    int
	histTable  table.Model
	histStatus string
	histLoaded bool

	// layout
	bodyH int // budgeted body content height (set in resize)
}

// New builds the TUI model. initial may be nil. When initial is nil it restores
// the auto-saved session (config.json + ranges.txt) if present.
func New(initial *config.Config) model {
	cfg := initial
	if cfg == nil {
		if loaded, err := config.LoadConfig(sessionConfigPath); err == nil {
			cfg = loaded
			applog.Logf("loaded session config from %s", sessionConfigPath)
		} else {
			cfg = config.DefaultConfig()
		}
	}

	ca := textarea.New()
	ca.Placeholder = "Paste a vless:// link, an xray JSON config, or this app's config.json here…"
	ca.ShowLineNumbers = false
	ca.CharLimit = 0

	ra := textarea.New()
	ra.Placeholder = "One CIDR / IP per line, e.g. 104.16.0.0/24"
	ra.ShowLineNumbers = false
	ra.CharLimit = 0
	if saved, err := os.ReadFile(sessionRangesPath); err == nil && len(saved) > 0 {
		ra.SetValue(string(saved))
		applog.Logf("loaded session ranges from %s", sessionRangesPath)
	} else {
		ra.SetValue(defaultRanges)
	}

	ti := textinput.New()
	ti.CharLimit = 256

	cols := []table.Column{
		{Title: "#", Width: 3},
		{Title: "IP", Width: 16},
		{Title: "Ping", Width: 8},
		{Title: "TCP", Width: 7},
		{Title: "Jit", Width: 6},
		{Title: "Loss", Width: 6},
		{Title: "Up", Width: 8},
		{Title: "Down", Width: 8},
	}
	tbl := table.New(table.WithColumns(cols), table.WithHeight(10), table.WithFocused(true))
	st := table.DefaultStyles()
	st.Header = st.Header.Bold(true).Foreground(colAccent)
	st.Selected = st.Selected.Foreground(lipgloss.Color("231")).Background(colAccent)
	tbl.SetStyles(st)

	hist := table.New(
		table.WithColumns([]table.Column{
			{Title: "When", Width: 20},
			{Title: "File", Width: 28},
			{Title: "IPs", Width: 6},
		}),
		table.WithHeight(10),
		table.WithFocused(true),
	)
	hist.SetStyles(st)

	lv := viewport.New(80, 20)

	m := model{
		active:     tabConfig,
		menuFocus:  true, // land on the top menu so navigation is obvious
		cfg:        cfg,
		configArea: ca,
		rangeArea:  ra,
		editInput:  ti,
		fields:     buildFields(),
		fragFields: buildFragmentFields(),
		results:    tbl,
		cols:       cols,
		histTable:  hist,
		logView:    lv,
		logFollow:  true,
	}
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, tick())
}

func (m *model) resize() {
	innerW := m.width - 8
	if innerW < 20 {
		innerW = 20
	}
	// Body content budget: total height minus header(1) + tab bar(2) + box
	// border/padding(4) + footer(2). The View also hard-clips to this so the
	// header/tabs can never be pushed off-screen.
	m.bodyH = m.height - 9
	if m.bodyH < 4 {
		m.bodyH = 4
	}
	innerH := m.bodyH

	// Cap the paste/range editors so the summary + hints below them stay visible.
	m.configArea.SetWidth(innerW)
	m.configArea.SetHeight(clamp(innerH-8, 3, 9))
	m.rangeArea.SetWidth(innerW)
	m.rangeArea.SetHeight(clamp(innerH-6, 3, 14))
	m.editInput.Width = innerW - 20

	// Results table: state/status line(1) + blank(1) + table header(1) + a few
	// log lines. Leave room for ~6 rolling log lines.
	tableH := innerH - 9
	m.results.SetHeight(clamp(tableH, 3, 9999))
	m.histTable.SetHeight(clamp(innerH-4, 3, 9999))

	m.logView.Width = innerW
	m.logView.Height = clamp(innerH-2, 3, 9999)

	// IP column gets the leftover width (8 fixed-ish columns).
	if len(m.cols) == 8 {
		fixed := 3 + 8 + 7 + 6 + 6 + 8 + 8 + 16 // non-IP cols + padding/borders
		ipw := innerW - fixed
		if ipw < 14 {
			ipw = 14
		}
		m.cols[1].Width = ipw
		m.results.SetColumns(m.cols)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil

	case scanResultMsg:
		// Result/progress sends are lossy under backpressure (see startScan), so
		// these messages are only used for the rolling log. The authoritative
		// counts and table come from the collector.
		m.pushLog(scanner.Result(msg))
		if m.scanning {
			return m, listen(m.evCh)
		}
		return m, nil

	case scanProgressMsg:
		m.done, m.total = msg.done, msg.total
		if m.collector != nil {
			m.successCnt = m.collector.SuccessCount()
		}
		m.rebuildTable()
		if m.scanning {
			return m, listen(m.evCh)
		}
		return m, nil

	case scanDoneMsg:
		m.scanning = false
		m.paused = false
		m.stopping = false
		m.sc = nil
		if m.collector != nil {
			m.successCnt = m.collector.SuccessCount()
		}
		m.rebuildTable()
		if msg.err != nil {
			m.status = "Scan stopped. " + fmt.Sprintf("%d good IPs so far.", m.successCnt)
		} else if msg.outputPath != "" {
			m.status = fmt.Sprintf("Done. %d good IPs. Saved to %s", m.successCnt, msg.outputPath)
		} else {
			m.status = fmt.Sprintf("Done. %d good IPs found.", m.successCnt)
		}
		applog.Logf("scanDoneMsg processed; scanning=false, good=%d", m.successCnt)
		return m, nil

	case optLogMsg:
		m.optLogs = append(m.optLogs, string(msg))
		if len(m.optLogs) > 14 {
			m.optLogs = m.optLogs[len(m.optLogs)-14:]
		}
		if m.optimizing {
			return m, optListen(m.optCh)
		}
		return m, nil

	case optDoneMsg:
		m.optimizing = false
		m.optCancel = nil
		if msg.ok {
			m.cfg.Fragment.Enabled = true
			m.cfg.Fragment.Mode = "manual"
			m.cfg.Fragment.Packets = msg.packets
			m.cfg.Fragment.Manual.Length = msg.length
			m.cfg.Fragment.Manual.Interval = msg.interval
			m.optStatus = fmt.Sprintf("✓ Best: packets=%s length=%s interval=%s (%dms) — applied & saved",
				msg.packets, msg.length, msg.interval, msg.latency)
			m.autosave()
			applog.Logf("optimize done: packets=%s length=%s interval=%s latency=%d", msg.packets, msg.length, msg.interval, msg.latency)
		} else {
			m.optStatus = "✗ No working fragment settings found"
			applog.Logf("optimize done: no working settings")
		}
		return m, nil

	case tickMsg:
		if m.active == tabLog {
			m.refreshLog()
		}
		return m, tick()

	case tea.MouseMsg:
		// Mouse wheel scrolls the focused scrollable widget.
		var cmd tea.Cmd
		switch m.active {
		case tabLog:
			m.logView, cmd = m.logView.Update(msg)
		case tabRun:
			m.results, cmd = m.results.Update(msg)
		case tabHistory:
			m.histTable, cmd = m.histTable.Update(msg)
		}
		return m, cmd

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward other messages to the focused widget of the active tab.
	return m.forward(msg)
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Universal quit.
	if key == "ctrl+c" {
		if m.scanning && m.sc != nil {
			m.sc.Stop()
		}
		if m.optimizing && m.optCancel != nil {
			m.optCancel()
		}
		applog.Logf("quit (ctrl+c)")
		return m, tea.Quit
	}

	// Inline setting editor captures keys until committed/cancelled.
	if m.editing {
		switch key {
		case "enter":
			m.commitEdit()
			m.autosave()
			return m, nil
		case "esc":
			m.editing = false
			m.editInput.Blur()
			return m, nil
		case "tab":
			m.commitEdit()
			m.autosave()
			m.active = nextTabIdx(m.active, +1)
			m.syncFocus()
			return m, nil
		case "shift+tab":
			m.commitEdit()
			m.autosave()
			m.active = nextTabIdx(m.active, -1)
			m.syncFocus()
			return m, nil
		}
		var cmd tea.Cmd
		m.editInput, cmd = m.editInput.Update(msg)
		return m, cmd
	}

	// Global actions (work in both menu and content focus).
	switch key {
	case "ctrl+o": // parse pasted config
		m.doParse()
		return m, nil
	case "ctrl+s": // manual save (auto-save is always on too)
		m.doSave()
		return m, nil
	case "ctrl+r": // start / stop scan
		return m.toggleScan()
	}

	// Top menu has focus: arrows/tab move between tabs; Enter/Down dive in.
	if m.menuFocus {
		switch key {
		case "left", "h", "shift+tab":
			m.active = nextTabIdx(m.active, -1)
			m.afterTabChange()
		case "right", "l", "tab":
			m.active = nextTabIdx(m.active, +1)
			m.afterTabChange()
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			if idx := int(key[0] - '1'); idx < nTabs() {
				m.active = tab(idx)
				m.afterTabChange()
			}
		case "down", "enter", " ":
			m.menuFocus = false
			m.syncFocus()
			m.afterTabChange()
			return m, m.focusCmd()
		case "q":
			applog.Logf("quit from menu")
			return m, tea.Quit
		}
		return m, nil
	}

	// Tab content has focus.
	switch key {
	case "esc": // back up to the menu
		m.menuFocus = true
		m.syncFocus()
		return m, nil
	case "tab":
		m.active = nextTabIdx(m.active, +1)
		m.syncFocus()
		m.afterTabChange()
		return m, m.focusCmd()
	case "shift+tab":
		m.active = nextTabIdx(m.active, -1)
		m.syncFocus()
		m.afterTabChange()
		return m, m.focusCmd()
	}

	return m.updateActiveTab(msg)
}

// afterTabChange runs per-tab setup when the active tab changes (refresh the log
// viewport, (re)load the scan history).
func (m *model) afterTabChange() {
	switch m.active {
	case tabLog:
		m.refreshLog()
	case tabHistory:
		m.loadHistory()
	}
}

// nextTabIdx returns the tab `dir` steps away from cur, wrapping around.
func nextTabIdx(cur tab, dir int) tab {
	n := nTabs()
	return tab((int(cur) + dir + n) % n)
}

// focusCmd returns the cursor-blink command for the focused text widget (so the
// cursor is visible after entering a tab's content).
func (m *model) focusCmd() tea.Cmd {
	if m.menuFocus {
		return nil
	}
	switch m.active {
	case tabConfig:
		return m.configArea.Focus()
	case tabRange:
		return m.rangeArea.Focus()
	}
	return nil
}

func (m model) updateActiveTab(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.active {
	case tabConfig:
		var cmd tea.Cmd
		m.configArea, cmd = m.configArea.Update(msg)
		return m, cmd

	case tabRange:
		switch msg.String() {
		case "f2":
			m.insertRangePreset(presetNameCFPublic, presetCFPublic)
			return m, nil
		case "f3":
			m.insertRangePreset(presetNameCFLondon, presetCFLondon)
			return m, nil
		case "f4":
			m.insertRangePreset(presetNameFastly, presetFastly)
			return m, nil
		}
		var cmd tea.Cmd
		m.rangeArea, cmd = m.rangeArea.Update(msg)
		m.autosave() // persist ranges as they are typed
		return m, cmd

	case tabSettings:
		return m.updateSettings(msg)

	case tabFragment:
		return m.updateFragment(msg)

	case tabRun:
		switch msg.String() {
		case "s":
			return m.toggleScan()
		case "p":
			if m.scanning && m.sc != nil {
				m.paused = m.sc.TogglePause()
				if m.paused {
					m.status = "Paused — press p to resume"
				} else {
					m.status = "Resumed"
				}
				applog.Logf("scan paused=%v", m.paused)
			}
			return m, nil
		case "c":
			m.copySelectedIP()
			return m, nil
		case "C", "y":
			m.copyAllIPs()
			return m, nil
		}
		var cmd tea.Cmd
		m.results, cmd = m.results.Update(msg)
		return m, cmd

	case tabLog:
		switch msg.String() {
		case "f":
			m.logFollow = !m.logFollow
			if m.logFollow {
				m.logView.GotoBottom()
			}
			return m, nil
		case "g", "home":
			m.logFollow = false
			m.logView.GotoTop()
			return m, nil
		case "G", "end":
			m.logFollow = true
			m.logView.GotoBottom()
			return m, nil
		}
		// Manual scroll disables follow so the user can read back.
		if k := msg.String(); k == "up" || k == "k" || k == "pgup" || k == "pgdown" || k == "down" || k == "j" {
			if k == "up" || k == "k" || k == "pgup" {
				m.logFollow = false
			}
		}
		var cmd tea.Cmd
		m.logView, cmd = m.logView.Update(msg)
		return m, cmd

	case tabHistory:
		switch msg.String() {
		case "r":
			m.histLoaded = false
			m.loadHistory()
			return m, nil
		case "enter":
			m.openHistory()
			return m, nil
		case "c":
			m.copyHistoryIPs()
			return m, nil
		}
		var cmd tea.Cmd
		m.histTable, cmd = m.histTable.Update(msg)
		return m, cmd
	}
	return m, nil
}

// editFieldList is the shared up/down/left/right/enter handling for a settings-
// style field list (used by both the Settings and Fragment tabs). It returns the
// (possibly) updated index, a focus cmd if an inline edit started, and whether a
// value changed (so the caller can auto-save).
func (m *model) editFieldList(fields []sfield, idx int, msg tea.KeyMsg) (int, tea.Cmd, bool) {
	if len(fields) == 0 {
		return idx, nil, false
	}
	f := fields[idx]
	changed := false
	switch msg.String() {
	case "up", "k":
		if idx > 0 {
			idx--
		}
	case "down", "j":
		if idx < len(fields)-1 {
			idx++
		}
	case "left", "h":
		if f.step != nil {
			f.step(m.cfg, -1)
			changed = true
		}
	case "right", "l":
		if f.step != nil {
			f.step(m.cfg, +1)
			changed = true
		}
	case " ":
		if f.kind == fBool || f.kind == fChoice {
			f.step(m.cfg, +1)
			changed = true
		}
	case "enter":
		if f.kind == fBool || f.kind == fChoice {
			if f.step != nil {
				f.step(m.cfg, +1)
				changed = true
			}
		} else if f.editable() {
			m.editing = true
			m.editApply = f.apply
			m.editInput.SetValue(f.display(m.cfg))
			m.editInput.CursorEnd()
			return idx, m.editInput.Focus(), false
		}
	}
	return idx, nil, changed
}

func (m model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	idx, cmd, changed := m.editFieldList(m.fields, m.fieldIdx, msg)
	m.fieldIdx = idx
	if changed {
		m.autosave()
	}
	return m, cmd
}

// forward delegates non-key messages (cursor blink, etc.) to focused widgets.
func (m model) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if m.editing {
		m.editInput, cmd = m.editInput.Update(msg)
		return m, cmd
	}
	switch m.active {
	case tabConfig:
		m.configArea, cmd = m.configArea.Update(msg)
	case tabRange:
		m.rangeArea, cmd = m.rangeArea.Update(msg)
	}
	return m, cmd
}

func (m *model) syncFocus() {
	m.configArea.Blur()
	m.rangeArea.Blur()
	if m.editing {
		m.editing = false
		m.editInput.Blur()
	}
	if m.menuFocus {
		return // menu owns focus; no widget is active
	}
	switch m.active {
	case tabConfig:
		m.configArea.Focus()
	case tabRange:
		m.rangeArea.Focus()
	}
}

// --- actions -------------------------------------------------------------

func (m *model) doParse() {
	text := m.configArea.Value()
	cfg, err := config.ParseConfigInput(text)
	if err != nil {
		m.parseOK = false
		m.parseStat = "✗ " + err.Error()
		applog.Logf("parse failed: %v", err)
		return
	}
	// Preserve the user's measurement/scan tuning, take connection details.
	cfg.Scan = m.cfg.Scan
	m.cfg = cfg
	m.parseOK = true
	m.parseStat = "✓ Parsed and applied. Check the Settings tab."
	applog.Logf("parsed config: addr=%s port=%d method=%s type=%s sni=%s",
		cfg.Proxy.Address, cfg.Proxy.Port, cfg.Proxy.Method, cfg.Proxy.Type, sniOf(cfg))
	m.autosave()
}

func sniOf(c *config.Config) string {
	if c.Proxy.TLS != nil {
		return c.Proxy.TLS.SNI
	}
	if c.Proxy.Reality != nil {
		return c.Proxy.Reality.ServerName
	}
	return ""
}

// insertRangePreset appends a named preset range block onto the Range tab
// textarea (deduping lines already present) and autosaves.
func (m *model) insertRangePreset(name, preset string) {
	merged, added := mergeRangePreset(m.rangeArea.Value(), preset)
	m.rangeArea.SetValue(merged)
	m.rangeArea.CursorEnd()
	if added > 0 {
		m.rangeStatus = fmt.Sprintf("+ %d ranges from %s", added, name)
	} else {
		m.rangeStatus = name + " already fully present"
	}
	applog.Logf("range preset applied: %s (+%d)", name, added)
	m.autosave()
}

// autosave persists the current config + ranges to disk on every edit. Errors
// are logged but never interrupt the UI.
func (m *model) autosave() {
	if err := config.SaveConfig(m.cfg, sessionConfigPath); err != nil {
		applog.Logf("autosave config failed: %v", err)
	}
	if err := os.WriteFile(sessionRangesPath, []byte(m.rangeArea.Value()), 0644); err != nil {
		applog.Logf("autosave ranges failed: %v", err)
	}
}

func (m *model) commitEdit() {
	if !m.editing {
		return
	}
	if m.editApply != nil {
		m.editApply(m.cfg, m.editInput.Value())
	}
	m.editApply = nil
	m.editing = false
	m.editInput.Blur()
}

func (m *model) doSave() {
	if err := config.SaveConfig(m.cfg, sessionConfigPath); err != nil {
		m.status = "Save failed: " + err.Error()
		applog.Logf("manual save config failed: %v", err)
		return
	}
	if err := os.WriteFile(sessionRangesPath, []byte(m.rangeArea.Value()), 0644); err != nil {
		m.status = "Saved config but ranges failed: " + err.Error()
		return
	}
	cwd, _ := os.Getwd()
	m.status = fmt.Sprintf("Saved %s + %s in %s", sessionConfigPath, sessionRangesPath, cwd)
	applog.Logf("manual save ok -> %s", cwd)
}

func (m model) toggleScan() (tea.Model, tea.Cmd) {
	if m.scanning {
		if m.sc != nil && !m.stopping {
			m.stopping = true
			m.status = "Stopping…"
			applog.Logf("scan stop requested")
			m.sc.Stop()
		}
		return m, nil
	}
	return m.startScan()
}

func (m model) startScan() (tea.Model, tea.Cmd) {
	// Never start a second scan over a live one — the previous channel/goroutine
	// must drain first (guaranteed once scanDoneMsg flips scanning to false).
	if m.scanning {
		return m, nil
	}
	if m.optimizing {
		m.active = tabFragment
		m.status = "Fragment optimization is running — wait for it to finish."
		return m, nil
	}

	m.active = tabRun
	m.syncFocus()

	if err := m.cfg.Validate(); err != nil {
		m.status = "Config invalid: " + err.Error() + " — paste a config in the Config tab."
		applog.Logf("scan start blocked: invalid config: %v", err)
		return m, nil
	}

	ips, err := utils.ParseIPsFromText(m.rangeArea.Value(), m.cfg.Scan.SampleSize, m.cfg.Scan.Shuffle)
	if err != nil {
		m.status = "Range error: " + err.Error()
		return m, nil
	}
	if m.cfg.Scan.MaxIPs > 0 && m.cfg.Scan.MaxIPs < len(ips) {
		ips = ips[:m.cfg.Scan.MaxIPs]
	}

	// The TUI scan never runs the auto-fragment optimizer (that's the Fragment
	// tab's job). Normalize the mode so "auto" can't disrupt the IP scan.
	if m.cfg.Fragment.Enabled {
		m.cfg.Fragment.Mode = "manual"
	} else {
		m.cfg.Fragment.Mode = "off"
	}

	sc := scanner.NewScannerWithDebug(m.cfg, false)
	sc.SetSilent(true)
	sc.SetIPs(ips)

	ch := make(chan tea.Msg, 4096)
	// Non-blocking sends: if the UI lags or stops reading (quit / second scan),
	// workers must never block on the channel. Dropped log/progress events are
	// harmless — the collector still has every result, and the table + counts
	// are rebuilt from it. The terminal scanDoneMsg below is sent blocking so it
	// is always delivered while the UI is still listening.
	sc.SetOnResult(func(r scanner.Result) {
		select {
		case ch <- scanResultMsg(r):
		default:
		}
	})
	sc.SetOnProgress(func(d, t int) {
		select {
		case ch <- scanProgressMsg{d, t}:
		default:
		}
	})

	m.sc = sc
	m.collector = sc.GetResults()
	m.evCh = ch
	m.scanning = true
	m.paused = false
	m.stopping = false
	m.done, m.total, m.successCnt = 0, len(ips), 0
	m.logs = nil
	m.status = fmt.Sprintf("Scanning %d IPs with %d threads…", len(ips), m.cfg.Scan.Threads)
	m.results.SetRows(nil)

	applog.Logf("scan started: %d IPs, %d threads, upload=%v download=%v handshake=%v",
		len(ips), m.cfg.Scan.Threads, m.cfg.Scan.UploadTest, m.cfg.Scan.DownloadTest, m.cfg.Scan.HandshakeTest)

	go func() {
		runErr := sc.Run()
		var outPath string
		if sc.GetResults().SuccessCount() > 0 {
			outPath = scanner.GenerateOutputPath("csv")
			_ = sc.SaveResults("csv", outPath)
		}
		applog.Logf("scan finished: good=%d err=%v out=%s", sc.GetResults().SuccessCount(), runErr, outPath)
		ch <- scanDoneMsg{err: runErr, outputPath: outPath}
	}()

	return m, listen(ch)
}

func listen(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func (m *model) pushLog(r scanner.Result) {
	var line string
	if r.Success {
		line = okStyle.Render("✓ ") + fmt.Sprintf("%-16s %4dms  loss %3.0f%%", r.IP, r.LatencyMs, r.LossPercent)
		if r.UploadMbps > 0 {
			line += fmt.Sprintf("  ↑%.1f", r.UploadMbps)
		}
	} else {
		msg := r.Error
		if len(msg) > 32 {
			msg = msg[:32]
		}
		line = errStyle.Render("✗ ") + fmt.Sprintf("%-16s %s", r.IP, hintStyle.Render(msg))
	}
	m.logs = append(m.logs, line)
	if len(m.logs) > 6 {
		m.logs = m.logs[len(m.logs)-6:]
	}
}

func (m *model) rebuildTable() {
	if m.collector == nil {
		return
	}
	sorted := m.collector.GetSortedByLatency() // all successful IPs, scrollable
	rows := make([]table.Row, 0, len(sorted))
	for i, r := range sorted {
		rows = append(rows, table.Row{
			fmt.Sprintf("%d", i+1),
			r.IP,
			fmt.Sprintf("%dms", r.LatencyMs),
			tcpCell(r.TCPMs, r.TCPOK),
			fmt.Sprintf("%dms", r.JitterMs),
			fmt.Sprintf("%.0f%%", r.LossPercent),
			mbpsCell(r.UploadMbps, r.UploadTested),
			mbpsCell(r.DownloadMbps, r.DownloadTested),
		})
	}
	m.results.SetRows(rows)
}

// tcpCell formats the raw TCP-connect cell: "fail" when the probe couldn't
// connect at all, otherwise the connect latency.
func tcpCell(ms int64, ok bool) string {
	if !ok {
		return "fail"
	}
	return fmt.Sprintf("%dms", ms)
}

// mbpsCell formats a throughput cell: "-" when not tested, "fail" when tested
// but zero (no throughput), otherwise the value.
func mbpsCell(mbps float64, tested bool) string {
	if !tested {
		return "-"
	}
	if mbps <= 0 {
		return "fail"
	}
	return fmt.Sprintf("%.1f", mbps)
}

// --- view ----------------------------------------------------------------

func (m model) View() string {
	if m.width == 0 {
		return "loading…"
	}

	// Top menu bar. When the menu has focus the active tab is shown with ‹ ›
	// markers so it's obvious arrows will move the selection. On narrow terminals
	// use short labels so the bar stays on one line.
	short := m.width < 90
	abbrev := []string{"Cfg", "Set", "Rng", "Frag", "Run", "Log", "Hist"}
	var tabs []string
	for i, name := range tabNames {
		label := fmt.Sprintf("%d %s", i+1, name)
		if short && i < len(abbrev) {
			label = fmt.Sprintf("%d%s", i+1, abbrev[i])
		}
		if tab(i) == m.active {
			if m.menuFocus {
				tabs = append(tabs, tabActive.Render("‹ "+label+" ›"))
			} else {
				tabs = append(tabs, tabActive.Render(label))
			}
		} else {
			tabs = append(tabs, tabInactive.Render(label))
		}
	}
	bar := tabBar.Render(lipgloss.JoinHorizontal(lipgloss.Top, tabs...))

	var body string
	switch m.active {
	case tabConfig:
		body = m.viewConfig()
	case tabSettings:
		body = m.viewSettings()
	case tabRange:
		body = m.viewRange()
	case tabFragment:
		body = m.viewFragment()
	case tabRun:
		body = m.viewRun()
	case tabLog:
		body = m.viewLog()
	case tabHistory:
		body = m.viewHistory()
	}

	hint := "←/→ pick tab · Enter to open"
	if !m.menuFocus {
		hint = "editing " + tabNames[m.active] + " · Esc for menu"
	}
	title := headerBar.Render("🧅 Piyazche Scanner v2")
	gap := m.width - lipgloss.Width(title) - lipgloss.Width(hint) - 2
	if gap < 1 {
		gap = 1
	}
	header := title + strings.Repeat(" ", gap) + hintStyle.Render(hint)
	footer := m.footer()

	// Fit the body into whatever height remains after the (possibly wrapped)
	// header, tab bar and footer. Each line is truncated to the width FIRST so
	// long lines don't wrap and inflate the height — line wrapping is what pushed
	// the tab bar off-screen before ("tabs disappear").
	budget := m.height - lipgloss.Height(header) - lipgloss.Height(bar) - lipgloss.Height(footer)
	if budget < 1 {
		budget = 1
	}
	body = fitBody(body, budget, m.width)

	return header + "\n" + bar + "\n" + body + "\n" + footer
}

// fitBody truncates each line of s to width (so nothing wraps) and pads/clips to
// exactly maxLines, so the surrounding chrome always stays on screen and the
// footer stays anchored at the bottom.
func fitBody(s string, maxLines, width int) string {
	if width < 1 {
		width = 1
	}
	lines := strings.Split(s, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "")
	}
	for len(lines) < maxLines {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m model) viewConfig() string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Paste your config (vless:// link or JSON) then press ") +
		keyStyle.Render("Ctrl+O") + labelStyle.Render(" to parse.\n\n"))
	b.WriteString(m.configArea.View())
	b.WriteString("\n\n")
	if m.parseStat != "" {
		if m.parseOK {
			b.WriteString(okStyle.Render(m.parseStat))
		} else {
			b.WriteString(errStyle.Render(m.parseStat))
		}
		b.WriteString("\n")
	}
	if m.cfg.Proxy.UUID != "" {
		b.WriteString("\n" + m.connSummary())
	}
	return b.String()
}

func (m model) connSummary() string {
	c := m.cfg
	uuid := c.Proxy.UUID
	if len(uuid) > 8 {
		uuid = uuid[:8] + "…"
	}
	det := []string{
		kv("Address", c.Proxy.Address),
		kv("Port", fmt.Sprintf("%d", c.Proxy.Port)),
		kv("Method", c.Proxy.Method),
		kv("Network", c.Proxy.Type),
		kv("UUID", uuid),
	}
	if c.Proxy.Method == "tls" && c.Proxy.TLS != nil {
		det = append(det, kv("SNI", c.Proxy.TLS.SNI))
	}
	if c.Proxy.Method == "reality" && c.Proxy.Reality != nil {
		det = append(det, kv("ServerName", c.Proxy.Reality.ServerName))
	}
	if c.Proxy.WS != nil {
		det = append(det, kv("Host", c.Proxy.WS.Host), kv("Path", c.Proxy.WS.Path))
	}
	if c.Proxy.Xhttp != nil {
		det = append(det, kv("Host", c.Proxy.Xhttp.Host), kv("Path", c.Proxy.Xhttp.Path))
	}
	if c.Proxy.Grpc != nil {
		det = append(det, kv("Service", c.Proxy.Grpc.ServiceName))
	}
	return titleStyle.Render("Detected config:") + "\n" + strings.Join(det, "\n")
}

func kv(k, v string) string {
	if v == "" {
		v = "(empty)"
	}
	return "  " + labelStyle.Render(fmt.Sprintf("%-12s", k)) + " " + valueStyle.Render(v)
}

func (m model) viewSettings() string {
	var b strings.Builder
	b.WriteString(hintStyle.Render("↑/↓ select · ←/→ adjust · Enter edit/toggle") + "\n\n")

	// Window the field list so it scrolls instead of overflowing the screen.
	visible := m.height - 16
	if visible < 4 {
		visible = 4
	}
	if visible > len(m.fields) {
		visible = len(m.fields)
	}
	start := m.fieldIdx - visible/2
	if start < 0 {
		start = 0
	}
	if start+visible > len(m.fields) {
		start = len(m.fields) - visible
	}
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > len(m.fields) {
		end = len(m.fields)
	}

	if start > 0 {
		b.WriteString(hintStyle.Render("        ▲ more above") + "\n")
	} else {
		b.WriteString("\n")
	}

	for i := start; i < end; i++ {
		f := m.fields[i]
		val := f.display(m.cfg)
		row := fmt.Sprintf(" %-20s  %s", f.label, val)
		if i == m.fieldIdx {
			b.WriteString(keyStyle.Render("▸") + selectedStyle.Render(row))
			if f.help != "" {
				b.WriteString("  " + hintStyle.Render("— "+f.help))
			}
		} else {
			b.WriteString(" " + labelStyle.Render(fmt.Sprintf("%-20s", f.label)) + "  " + valueStyle.Render(val))
		}
		b.WriteString("\n")
	}

	if end < len(m.fields) {
		b.WriteString(hintStyle.Render("        ▼ more below"))
	}

	if m.editing {
		b.WriteString("\n" + keyStyle.Render("edit ▸ ") + m.editInput.View() + hintStyle.Render("  (Enter=ok, Esc=cancel)"))
	}
	return b.String()
}

func (m model) viewRange() string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Enter the ranges to scan (CIDR or single IPs), one per line.\n\n"))
	b.WriteString(m.rangeArea.View())
	b.WriteString("\n\n")
	// Cheap math count only — never expand CIDRs in the render path.
	if n, ok := utils.CountRanges(m.rangeArea.Value(), m.cfg.Scan.SampleSize); ok {
		b.WriteString(okStyle.Render(fmt.Sprintf("≈ %d IPs queued", n)) +
			hintStyle.Render(fmt.Sprintf("  (sample/subnet = %d)", m.cfg.Scan.SampleSize)))
	} else {
		b.WriteString(hintStyle.Render("no valid ranges yet"))
	}
	b.WriteString("\n\n" + labelStyle.Render("Presets: ") +
		keyStyle.Render("F2") + " " + presetNameCFPublic + "  " +
		keyStyle.Render("F3") + " " + presetNameCFLondon + "  " +
		keyStyle.Render("F4") + " " + presetNameFastly)
	if m.rangeStatus != "" {
		b.WriteString("\n" + okStyle.Render(m.rangeStatus))
	}
	b.WriteString("\n\n" + labelStyle.Render("Press ") + keyStyle.Render("Ctrl+R") + labelStyle.Render(" to start the scan."))
	return b.String()
}

func (m model) viewRun() string {
	var b strings.Builder

	state := okStyle.Render("● scanning")
	if m.scanning && m.paused {
		state = lipgloss.NewStyle().Foreground(colYellow).Render("⏸ paused")
	} else if !m.scanning {
		state = hintStyle.Render("○ idle")
	}
	b.WriteString(state)
	if m.total > 0 {
		b.WriteString("   " + labelStyle.Render("done ") + statStyle.Render(fmt.Sprintf("%d/%d", m.done, m.total)))
		b.WriteString("   " + labelStyle.Render("good ") + okStyle.Render(fmt.Sprintf("%d", m.successCnt)))
		bad := m.done - m.successCnt
		if bad < 0 {
			bad = 0
		}
		b.WriteString("   " + labelStyle.Render("bad ") + errStyle.Render(fmt.Sprintf("%d", bad)))
		b.WriteString("   " + progressBar(m.done, m.total, 24))
	}
	b.WriteString("\n")
	if m.status != "" {
		b.WriteString(hintStyle.Render(m.status) + "\n")
	}
	b.WriteString("\n" + m.results.View() + "\n")

	if len(m.logs) > 0 {
		b.WriteString("\n")
		b.WriteString(strings.Join(m.logs, "\n"))
	}
	return b.String()
}

func (m model) footer() string {
	var parts []string
	if m.menuFocus {
		// Navigation mode.
		parts = []string{
			keyStyle.Render("←/→") + " tabs",
			keyStyle.Render("Enter") + " open",
			keyStyle.Render("Ctrl+R") + " scan",
			keyStyle.Render("q") + " quit",
		}
	} else {
		// Content mode — show tab-specific hints.
		switch m.active {
		case tabConfig:
			parts = append(parts, keyStyle.Render("Ctrl+O")+" parse")
		case tabSettings:
			parts = append(parts, keyStyle.Render("↑↓")+" pick", keyStyle.Render("←→")+" change", keyStyle.Render("Enter")+" edit")
		case tabRange:
			parts = append(parts, keyStyle.Render("type")+" ranges",
				keyStyle.Render("F2")+" CF public", keyStyle.Render("F3")+" CF London", keyStyle.Render("F4")+" Fastly")
		case tabFragment:
			parts = append(parts, keyStyle.Render("↑↓")+" pick", keyStyle.Render("←→")+" change", keyStyle.Render("o")+" optimize")
		case tabRun:
			parts = append(parts, keyStyle.Render("↑↓")+" rows", keyStyle.Render("s")+" start/stop", keyStyle.Render("p")+" pause", keyStyle.Render("c")+" copy IP", keyStyle.Render("C")+" copy all")
		case tabLog:
			parts = append(parts, keyStyle.Render("↑↓/wheel")+" scroll", keyStyle.Render("f")+" follow", keyStyle.Render("g/G")+" top/end")
		case tabHistory:
			parts = append(parts, keyStyle.Render("↑↓")+" pick", keyStyle.Render("Enter")+" open", keyStyle.Render("c")+" copy IPs", keyStyle.Render("r")+" refresh")
		}
		parts = append(parts,
			keyStyle.Render("Esc")+" menu",
			keyStyle.Render("Ctrl+R")+" scan",
			keyStyle.Render("Ctrl+C")+" quit",
		)
	}
	return footerStyle.Render(strings.Join(parts, hintStyle.Render("  ·  ")))
}

func progressBar(done, total, width int) string {
	if total <= 0 {
		return ""
	}
	filled := done * width / total
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	pct := done * 100 / total
	return lipgloss.NewStyle().Foreground(colGreen).Render(bar) + fmt.Sprintf(" %d%%", pct)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Run launches the TUI. initial may be nil.
func Run(initial *config.Config) error {
	applog.Init("piyazche.log")
	defer applog.Close()
	applog.Logf("TUI starting")
	p := tea.NewProgram(New(initial), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	if err != nil {
		applog.Logf("TUI exited with error: %v", err)
	}
	return err
}
