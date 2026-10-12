package gui

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"piyazche/appcore"
	"piyazche/applog"
	"piyazche/config"
	"piyazche/scanner"
	"piyazche/utils"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type UI struct {
	App                                      fyne.App
	Window                                   fyne.Window
	DataDir                                  string
	Mobile                                   bool
	Session                                  *appcore.Session
	Editor                                   *Editor
	Tabs                                     *container.AppTabs
	Source, ImportText                       *widget.Entry
	Mode, SourceKind                         *widget.Select
	PerRange                                 *widget.Entry
	SourceHelp, Breakdown, Current           *widget.Label
	Start, Pause, Stop, Optimize, Apply      *widget.Button
	Status, Summary, Preview, ConnectionInfo *widget.Label
	Progress                                 *widget.ProgressBar
	Table                                    *widget.Table
	GoodOnly                                 *widget.Check
	Sort                                     *widget.Select
	Top                                      *widget.Entry
	Log                                      *widget.Entry
	History                                  *widget.List
	connectionFields, settingsFields         *fyne.Container
	run                                      *appcore.Run
	lastRun                                  *appcore.Run
	results                                  *scanner.ResultCollector
	rows                                     []scanner.Result
	selected                                 string
	historyPaths                             []string
	historySelected                          int
	ctx                                      context.Context
	cancel                                   context.CancelFunc
	optCancel                                context.CancelFunc
	runStamp                                 string
	lastSave                                 time.Time
	saveMu                                   sync.Mutex
	loading                                  bool
	lastLog                                  string
	lastLogGen                               uint64
	lastTable                                time.Time
	lastCount                                int
}

func New(a fyne.App, w fyne.Window, dataDir, version string, mobile bool) *UI {
	ctx, cancel := context.WithCancel(context.Background())
	s, loadErr := appcore.LoadSession(dataDir)
	u := &UI{App: a, Window: w, DataDir: dataDir, Mobile: mobile, Session: s, ctx: ctx, cancel: cancel, historySelected: -1, loading: true, results: scanner.NewResultCollector()}
	a.Settings().SetTheme(appTheme{})
	w.SetIcon(Icon)
	u.Status = widget.NewLabel("Ready · choose a mode and add your connection.")
	u.Status.Wrapping = fyne.TextWrapWord
	u.Summary = widget.NewLabel("0 tested · 0 good")
	u.Preview = widget.NewLabel("Add IPs or CIDRs below.")
	u.Preview.Wrapping = fyne.TextWrapWord
	u.ConnectionInfo = widget.NewLabel("")
	u.ConnectionInfo.Truncation = fyne.TextTruncateEllipsis
	u.Progress = widget.NewProgressBar()
	u.Source = widget.NewMultiLineEntry()
	u.Source.Wrapping = fyne.TextWrapWord
	u.Source.SetMinRowsVisible(6)
	if mobile {
		u.Source.SetMinRowsVisible(5)
	}
	u.Source.SetPlaceHolder("One IP or CIDR per line, e.g. 104.16.0.0/24")
	u.Source.SetText(s.Ranges)
	u.Source.OnChanged = func(string) { u.updatePreview(); u.autoSave() }
	u.ImportText = widget.NewMultiLineEntry()
	u.ImportText.Wrapping = fyne.TextWrapWord
	u.ImportText.SetMinRowsVisible(4)
	u.ImportText.SetPlaceHolder("Paste a vless:// link, Xray JSON, app JSON or Base64 subscription")
	u.Mode = widget.NewSelect([]string{appcore.ModeXray, appcore.ModeConnectivity, appcore.ModeCheck}, nil)
	u.Mode.SetSelected(s.Mode)
	u.Mode.OnChanged = func(v string) { u.Session.Mode = v; u.updatePreview(); u.autoSave() }
	u.SourceKind = widget.NewSelect([]string{appcore.SourceRanges, appcore.SourceIPList}, nil)
	u.SourceKind.SetSelected(s.Source)
	u.SourceKind.OnChanged = func(v string) {
		u.Session.Source = v
		u.syncSourceControls()
		u.updatePreview()
		u.autoSave()
	}
	u.SourceHelp = wrapped("")
	u.PerRange = widget.NewEntry()
	u.PerRange.SetPlaceHolder("0 = every IP in the range")
	u.PerRange.SetText(strconv.Itoa(s.Config.Scan.SampleSize))
	u.PerRange.OnChanged = u.perRangeChanged
	u.Breakdown = wrapped("")
	// One line in the always-visible action bar; truncation keeps its height exact.
	u.Current = widget.NewLabel("")
	u.Current.Truncation = fyne.TextTruncateEllipsis
	u.syncSourceControls()
	u.Start = widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), u.start)
	u.Start.Importance = widget.HighImportance
	u.Pause = widget.NewButtonWithIcon("Pause", theme.MediaPauseIcon(), func() {
		if u.run == nil {
			return
		}
		if u.run.TogglePause() {
			u.Pause.SetText("Resume")
			u.Status.SetText("Paused · in-flight tests finish first.")
		} else {
			u.Pause.SetText("Pause")
			u.Status.SetText("Scanning…")
		}
		applog.Logf("pause=%v", u.run.Paused())
	})
	u.Stop = widget.NewButtonWithIcon("Stop", theme.CancelIcon(), func() {
		if u.run != nil {
			u.run.Stop()
			u.Stop.Disable()
			u.Pause.Disable()
			u.Status.SetText("Stopping…")
		}
	})
	u.Pause.Importance = widget.WarningImportance
	u.Stop.Importance = widget.DangerImportance
	u.Optimize = widget.NewButtonWithIcon("Optimize fragments", theme.SettingsIcon(), u.optimize)
	u.Pause.Disable()
	u.Stop.Disable()
	u.connectionFields = container.NewVBox()
	u.settingsFields = container.NewVBox()
	u.rebuildEditor()
	scanTab := u.scanTab()
	connectionTab := u.connectionTab()
	settingsTab := u.settingsTab()
	resultsTab := u.resultsTab()
	activityTab := u.activityTab()
	names := []string{"Scan", "Connection", "Settings", "Results", "Activity"}
	if mobile {
		names = []string{"Scan", "Link", "Setup", "IPs", "Logs"}
	}
	u.Tabs = container.NewAppTabs(
		container.NewTabItemWithIcon(names[0], theme.SearchIcon(), scanTab),
		container.NewTabItemWithIcon(names[1], theme.ComputerIcon(), connectionTab),
		container.NewTabItemWithIcon(names[2], theme.SettingsIcon(), settingsTab),
		container.NewTabItemWithIcon(names[3], theme.ListIcon(), resultsTab),
		container.NewTabItemWithIcon(names[4], theme.HistoryIcon(), activityTab),
	)
	if mobile {
		u.Tabs.SetTabLocation(container.TabLocationBottom)
	}
	u.Tabs.OnSelected = func(t *container.TabItem) {
		if t == u.Tabs.Items[4] {
			u.refreshHistory()
			u.refreshLog(true)
		}
		if t == u.Tabs.Items[3] {
			u.refreshResults()
		}
	}
	header := container.New(layout.NewCustomPaddedLayout(10, 4, 14, 14), banner(version, mobile))
	statusBg := canvas.NewRectangle(themed(colorCard))
	statusBg.StrokeColor = themed(colorCardStroke)
	statusBg.StrokeWidth = 1
	footer := container.NewStack(statusBg, container.New(layout.NewCustomPaddedLayout(2, 2, 12, 12), u.Status))
	w.SetContent(container.NewBorder(header, footer, nil, nil, u.Tabs))
	w.Resize(fyne.NewSize(1100, 820))
	if mobile {
		w.Resize(fyne.NewSize(390, 844))
	}
	u.shortcuts()
	u.loading = false
	u.updatePreview()
	u.updateConnectionInfo()
	if loadErr != nil {
		u.Status.SetText(loadErr.Error() + " · defaults loaded; the damaged file is not overwritten until you edit.")
	}
	u.refreshHistory()
	return u
}

func (u *UI) scanTab() fyne.CanvasObject {
	preset := widget.NewSelect([]string{"Cloudflare", "CF London", "Fastly"}, nil)
	preset.SetSelected("Cloudflare")
	add := widget.NewButtonWithIcon("Add preset", theme.ContentAddIcon(), func() {
		u.Source.SetText(mergePreset(u.Source.Text, Presets[preset.Selected]))
	})
	importRanges := widget.NewButtonWithIcon("Import", theme.FolderOpenIcon(), func() {
		u.readFile(func(text string) { u.Source.SetText(text) }, ".txt", ".csv")
	})
	copyRanges := widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() { u.App.Clipboard().SetContent(u.Source.Text); u.Status.SetText("Ranges copied.") })
	clearRanges := widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), func() { u.Source.SetText("") })
	settings := widget.NewButtonWithIcon("Scan settings", theme.SettingsIcon(), func() { u.Tabs.SelectIndex(2) })

	modeCard := card("Scan mode", "Proxy scan puts every IP into your config and really connects through it.",
		touchControl(u.Mode), u.ConnectionInfo)
	targetCard := card("Targets", "",
		touchControl(u.SourceKind), u.SourceHelp,
		labeled("IPs per range · 0 = all", u.PerRange),
		settings)
	rangesCard := card("IP ranges", "One IP or CIDR per line. Comments after # are ignored.",
		u.Source,
		container.NewBorder(nil, nil, nil, add, touchControl(preset)),
		container.NewGridWithColumns(3, importRanges, copyRanges, clearRanges))
	planCard := card("Scan plan", "", u.Preview,
		fieldLabel("Per range · IPs in range · queued · tested · good"),
		u.Breakdown)
	note := muted("No scan starts automatically. Only test targets you are allowed to probe.")

	var body fyne.CanvasObject
	if u.Mobile {
		body = page(modeCard, targetCard, rangesCard, planCard, note)
	} else {
		// One scroll for the whole page: the mouse wheel always moves the page,
		// never a nested pane, and both columns stay aligned.
		left := minWidth(330, container.NewVBox(modeCard, gap(4), targetCard, gap(4), note))
		right := container.NewVBox(rangesCard, gap(4), planCard)
		body = page(container.NewBorder(nil, nil, left, nil, container.New(layout.NewCustomPaddedLayout(0, 0, 8, 0), right)))
	}
	// Start/pause/stop and progress never disappear below a scrollable form.
	buttons := container.NewGridWithColumns(3, u.Start, u.Pause, u.Stop)
	var bar fyne.CanvasObject
	if u.Mobile {
		bar = card("", "", container.NewBorder(nil, nil, u.Summary, nil, u.Current), u.Progress, buttons)
	} else {
		info := container.NewBorder(nil, nil, u.Summary, nil, u.Current)
		bar = card("", "", info, container.NewBorder(nil, nil, nil, minWidth(460, buttons), container.NewPadded(u.Progress)))
	}
	actions := container.New(layout.NewCustomPaddedLayout(2, 6, 14, 14), bar)
	return container.NewBorder(nil, actions, nil, nil, body)
}

func (u *UI) connectionTab() fyne.CanvasObject {
	parse := widget.NewButtonWithIcon("Apply connection", theme.ConfirmIcon(), u.importConnection)
	u.Apply = parse
	parse.Importance = widget.HighImportance
	open := widget.NewButtonWithIcon("Import file", theme.FolderOpenIcon(), func() {
		u.readFile(func(text string) { u.ImportText.SetText(text); u.importConnection() }, ".json", ".txt", ".jsonc")
	})
	copyConfig := widget.NewButtonWithIcon("Copy full config", theme.ContentCopyIcon(), func() {
		c, err := u.Editor.Snapshot()
		if err != nil {
			u.showError(err)
			return
		}
		b, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			u.showError(err)
			return
		}
		u.App.Clipboard().SetContent(string(b))
		u.Status.SetText("Full configuration copied · it may contain private credentials.")
	})
	reset := widget.NewButtonWithIcon("Clear connection", theme.DeleteIcon(), func() {
		dialog.ShowConfirm("Clear connection", "Remove the saved connection (address, UUID, SNI…)? Scan settings are kept.", func(ok bool) {
			if !ok {
				return
			}
			c := config.DefaultConfig()
			c.Scan, c.Output, c.Xray, c.Fragment = u.Session.Config.Scan, u.Session.Config.Output, u.Session.Config.Xray, u.Session.Config.Fragment
			u.Session.Config = c
			u.ImportText.SetText("")
			u.rebuildEditor()
			u.updateConnectionInfo()
			u.autoSave()
			u.Status.SetText("Connection cleared.")
		}, u.Window)
	})
	reset.Importance = widget.DangerImportance
	connButtons := fyne.CanvasObject(container.NewGridWithColumns(3, open, copyConfig, reset))
	if u.Mobile {
		copyConfig.SetText("Copy config")
		reset.SetText("Clear")
		connButtons = container.NewVBox(container.NewGridWithColumns(2, open, copyConfig), reset)
	}
	importCard := card("Your connection", "Paste a vless:// link, an Xray JSON, this app's JSON or a Base64 subscription. Everything stays on this device.",
		u.ImportText, parse, connButtons)
	details := card("Connection details", "Every field stays editable.", u.connectionFields)
	return page(importCard, details)
}

func (u *UI) settingsTab() fyne.CanvasObject {
	debug := widget.NewCheck("Save Xray debug config", nil)
	debug.SetChecked(u.Session.Debug)
	debug.OnChanged = func(v bool) { u.Session.Debug = v; u.autoSave() }
	save := widget.NewButtonWithIcon("Save settings", theme.DocumentSaveIcon(), func() {
		if err := u.save(); err != nil {
			u.showError(err)
		} else {
			u.Status.SetText("Settings saved in your application data folder.")
		}
	})
	save.Importance = widget.HighImportance
	optimize := card("Fragment optimizer", "Finds working TLS fragment values. Runs independently of a scan; tap again to stop.", u.Optimize)
	options := card("All options", "Numeric units are shown in the field names.", u.settingsFields)
	misc := card("Debug & saving", "Debug output can contain your connection credentials. Keep it private.", debug, save)
	return page(optimize, options, misc)
}

func (u *UI) rebuildEditor() {
	u.Editor = NewEditor(u.Session.Config, func() { u.updatePreview(); u.updateConnectionInfo(); u.autoSave() })
	u.connectionFields.Objects = []fyne.CanvasObject{u.Editor.Sections["proxy"]}
	u.connectionFields.Refresh()
	accordion := widget.NewAccordion(
		widget.NewAccordionItem("Scan & quality measurements", u.Editor.Sections["scan"]),
		widget.NewAccordionItem("TLS fragmentation", u.Editor.Sections["fragment"]),
		widget.NewAccordionItem("Output & automatic saving", u.Editor.Sections["output"]),
		widget.NewAccordionItem("Xray & multiplexing", u.Editor.Sections["xray"]),
	)
	accordion.MultiOpen = true
	accordion.Open(0)
	u.settingsFields.Objects = []fyne.CanvasObject{accordion}
	u.settingsFields.Refresh()
}

func (u *UI) importConnection() {
	c, err := config.ParseConfigInput(u.ImportText.Text)
	if err != nil {
		u.showError(err)
		return
	}
	// Share links describe a connection, not the user's measurement tuning.
	// Full app JSON, on the other hand, is restored in its entirety.
	raw := strings.TrimSpace(u.ImportText.Text)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, e := enc.DecodeString(raw); e == nil {
			raw = strings.TrimSpace(string(b))
			break
		}
	}
	if strings.HasPrefix(strings.ToLower(raw), "vless://") {
		c.Scan = u.Session.Config.Scan
		c.Output = u.Session.Config.Output
		c.Xray = u.Session.Config.Xray
		c.Fragment = u.Session.Config.Fragment
	}
	u.Session.Config = c
	u.rebuildEditor()
	u.updateConnectionInfo()
	u.autoSave()
	u.Status.SetText("Connection applied · settings saved.")
}

func (u *UI) updateConnectionInfo() {
	if u.ConnectionInfo == nil {
		return
	}
	c := u.Session.Config
	if c.Proxy.UUID == "" {
		u.ConnectionInfo.SetText("No proxy · import in Connection.")
		return
	}
	u.ConnectionInfo.SetText(fmt.Sprintf("%s / %s · %s:%d", strings.ToUpper(c.Proxy.Method), strings.ToUpper(c.Proxy.Type), c.Proxy.Address, c.Proxy.Port))
}

// perRangeChanged keeps the Scan tab's "IPs per range" and the Settings field
// (scan.sampleSize) in sync; both edit the same configuration value.
func (u *UI) perRangeChanged(text string) {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || n < 0 {
		u.PerRange.SetValidationError(errors.New("enter 0 (all) or a positive number"))
		return
	}
	u.PerRange.SetValidationError(nil)
	if u.Session.Config.Scan.SampleSize == n {
		return
	}
	u.Session.Config.Scan.SampleSize = n
	if u.Editor != nil {
		if c, ok := u.Editor.controls["scan.sampleSize"].(*widget.Entry); ok && c.Text != strconv.Itoa(n) {
			c.SetText(strconv.Itoa(n))
		}
	}
	u.updatePreview()
	u.autoSave()
}

func (u *UI) syncSourceControls() {
	if u.SourceKind == nil || u.PerRange == nil || u.SourceHelp == nil {
		return
	}
	if u.SourceKind.Selected == appcore.SourceIPList {
		u.PerRange.Disable()
		u.SourceHelp.SetText("Scans the addresses exactly as entered, in order. Nothing is sampled or shuffled; a CIDR in the list is expanded to every address.")
	} else {
		u.PerRange.Enable()
		u.SourceHelp.SetText("Generates the IPs from each CIDR range: the chosen number of random IPs per range (0 = every IP in the range), then scans them.")
	}
}

func (u *UI) updatePreview() {
	if u.Source == nil || u.Preview == nil {
		return
	}
	cfg := u.Session.Config
	if u.PerRange != nil && u.PerRange.Text != strconv.Itoa(cfg.Scan.SampleSize) {
		if n, err := strconv.Atoi(strings.TrimSpace(u.PerRange.Text)); err != nil || n != cfg.Scan.SampleSize {
			u.PerRange.SetText(strconv.Itoa(cfg.Scan.SampleSize))
		}
	}
	ipList := u.Session.Source == appcore.SourceIPList
	plan, total, invalid := utils.PlanRanges(u.Source.Text, cfg.Scan.SampleSize, ipList)
	n := total
	capped := false
	if max := cfg.Scan.MaxIPs; max > 0 && n > uint64(max) {
		n, capped = uint64(max), true
	}
	mode := ""
	if u.Mode != nil {
		mode = u.Mode.Selected
	}
	var s string
	switch {
	case mode == appcore.ModeCheck:
		addr := cfg.Proxy.Address
		if addr == "" {
			addr = "(not set)"
		}
		s = "Check connection tests only the server address " + addr + " from the Connection tab. The IPs and ranges below are not scanned; choose Proxy scan to test them."
	case strings.TrimSpace(u.Source.Text) == "":
		s = "Add IPs / CIDRs, or choose a built-in preset."
	default:
		s = fmt.Sprintf("%s IPs will be scanned from %d range(s) · %d worker(s)", commaU(n), len(plan), cfg.Scan.Threads)
		if capped {
			s += fmt.Sprintf(" · limited by Maximum IPs (%s)", comma(cfg.Scan.MaxIPs))
		}
		if total > utils.MaxGeneratedTargets {
			s += fmt.Sprintf(" · more than %s IPs: set IPs per range or narrow the ranges", comma(utils.MaxGeneratedTargets))
		}
		if invalid > 0 {
			s += fmt.Sprintf(" · %d invalid entr%s ignored", invalid, map[bool]string{true: "y", false: "ies"}[invalid == 1])
		}
		if !ipList && cfg.Scan.SampleSize > 0 {
			s += fmt.Sprintf(" · %d IP(s) per range", cfg.Scan.SampleSize)
		}
		if mode == appcore.ModeConnectivity {
			s += " · ping mode: " + utils.PingModeString() + " (ICMP requires privileges)."
		}
	}
	u.Preview.SetText(s)
	if u.Breakdown != nil && u.run == nil {
		u.lastRun = nil
		stats := make([]appcore.RangeStat, len(plan))
		for i, r := range plan {
			stats[i] = appcore.RangeStat{RangeInfo: r}
		}
		u.Breakdown.SetText(formatRanges(stats, false, 15))
		if mode != appcore.ModeCheck {
			u.Current.SetText("")
		}
	}
}

// formatRanges renders the per-range breakdown. live adds tested/good counts.
func formatRanges(stats []appcore.RangeStat, live bool, limit int) string {
	if len(stats) == 0 {
		return "No ranges yet."
	}
	var b strings.Builder
	for i, r := range stats {
		if limit > 0 && i == limit {
			fmt.Fprintf(&b, "+%d more range(s)", len(stats)-limit)
			break
		}
		if i > 0 {
			b.WriteString("\n")
		}
		hosts := "∞"
		if r.Hosts != ^uint64(0) {
			hosts = commaU(r.Hosts)
		}
		fmt.Fprintf(&b, "%s\n    %s IPs in range · %s queued", r.Label(), hosts, comma(r.Queued))
		if live {
			fmt.Fprintf(&b, " · %s tested · %s good", comma(r.Tested), comma(r.Good))
		}
	}
	return b.String()
}

// refreshLive shows which IPs are being tested now and the per-range progress.
func (u *UI) refreshLive(r *appcore.Run, running bool) {
	if r == nil || u.Breakdown == nil {
		return
	}
	maxActive := 3
	if u.Mobile {
		maxActive = 1
	}
	snap := r.Snapshot(maxActive)
	u.Breakdown.SetText(formatRanges(snap.Ranges, true, 15))
	if !running {
		u.Current.SetText("")
		return
	}
	var text string
	switch {
	case r.Paused():
		text = "Paused"
	case len(snap.Active) == 0:
		text = "Preparing the scanner…"
		done, total := r.Progress()
		if total > 0 && done >= total {
			text = "Finishing…"
		}
	default:
		text = "Now scanning " + strings.Join(snap.Active, ", ")
		if more := snap.ActiveTotal - len(snap.Active); more > 0 {
			text += fmt.Sprintf(" +%d more", more)
		}
		if snap.CurrentRange != "" {
			text += " · range " + snap.CurrentRange
		}
	}
	u.Current.SetText(text)
}

func (u *UI) start() {
	if u.run != nil {
		return
	}
	if u.Mode.Selected == appcore.ModeCheck && strings.TrimSpace(u.Source.Text) != "" {
		addr := u.Session.Config.Proxy.Address
		if a := strings.ToUpper(strings.TrimSpace(addr)); a == "" || a == "IP.IP.IP.IP" || a == "IP" {
			// A placeholder server address can only be meant as "scan my IPs".
			u.Mode.SetSelected(appcore.ModeXray)
			u.startNow()
			return
		}
		msg := wrapped("Check connection tests only your server address (" + addr + ") from the Connection tab. It does not scan the IPs or ranges you entered.\n\nTo test those IPs through your proxy, use Proxy scan.")
		dialog.NewCustomConfirm("Check connection", "Scan my IPs (Proxy scan)", "Check server only", msg, func(scanRanges bool) {
			if scanRanges {
				u.Mode.SetSelected(appcore.ModeXray)
				u.startNow()
				return
			}
			u.startNow()
		}, u.Window).Show()
		return
	}
	u.startNow()
}

func (u *UI) startNow() {
	if u.run != nil {
		return
	}
	cfg, err := u.Editor.Snapshot()
	if err != nil {
		u.showError(err)
		return
	}
	start := func() {
		r, err := appcore.NewRunWithSource(u.ctx, cfg, u.Source.Text, u.Mode.Selected, u.Session.Source, u.Session.Debug)
		if err != nil {
			u.showError(err)
			return
		}
		u.run = r
		u.lastRun = r
		u.results = r.Results
		u.selected = ""
		u.runStamp = time.Now().Format("2006-01-02_150405.000")
		u.lastSave = time.Now()
		stamp := u.runStamp
		u.Start.Disable()
		u.Pause.Enable()
		if r.Mode == appcore.ModeCheck {
			u.Pause.Disable()
		}
		u.Stop.Enable()
		u.Pause.SetText("Pause")
		u.Progress.SetValue(0)
		u.Status.SetText("Scanning… · you can edit settings for the next run.")
		u.refreshResults()
		u.refreshLive(r, true)
		u.autoSave()
		go func() {
			err := r.Execute()
			u.saveMu.Lock()
			path, saveErr := appcore.SaveResults(u.DataDir, r.Config, r.Results, stamp)
			u.saveMu.Unlock()
			if u.ctx.Err() != nil {
				return
			}
			fyne.Do(func() {
				if u.run != r {
					return
				}
				u.run = nil
				u.Start.Enable()
				u.Pause.Disable()
				u.Stop.Disable()
				done, total := r.Progress()
				if total > 0 {
					u.Progress.SetValue(float64(done) / float64(total))
				}
				u.refreshResults()
				u.refreshLive(r, false)
				message := fmt.Sprintf("Done · %d good / %d tested", r.Results.SuccessCount(), r.Results.Count())
				if errors.Is(err, context.Canceled) {
					message = "Stopped · partial results saved."
				} else if err != nil {
					message = "Scan failed: " + err.Error()
				}
				if saveErr != nil {
					message += " · saving failed: " + saveErr.Error()
				} else {
					message += " · " + filepath.Base(path)
				}
				u.Status.SetText(message)
				applog.Logf("%s", message)
				u.refreshHistory()
			})
		}()
	}
	_, planned, _ := utils.PlanRanges(u.Source.Text, cfg.Scan.SampleSize, u.Session.Source == appcore.SourceIPList)
	if planned > utils.MaxGeneratedTargets {
		planned = utils.MaxGeneratedTargets
	}
	n := int(planned)
	if cfg.Scan.MaxIPs > 0 && n > cfg.Scan.MaxIPs {
		n = cfg.Scan.MaxIPs
	}
	if n > 250000 && u.Mode.Selected != appcore.ModeCheck {
		dialog.NewConfirm("Large scan", fmt.Sprintf("This queues %s IPs and can use substantial memory and data. Continue, or reduce IPs per range or Maximum IPs?", comma(n)), func(ok bool) {
			if ok {
				start()
			}
		}, u.Window).Show()
		return
	}
	start()
}

func (u *UI) optimize() {
	if u.optCancel != nil {
		u.optCancel()
		u.Optimize.Disable()
		u.Status.SetText("Stopping fragment optimization…")
		return
	}
	cfg, err := u.Editor.Snapshot()
	if err != nil {
		u.showError(err)
		return
	}
	target := cfg.Fragment.Auto.TestIP
	if cfg.Proxy.Method == "reality" || target == "" {
		target = cfg.Proxy.Address
	}
	if target == "" {
		ips, e := utils.ParseIPsFromText(u.Source.Text, 1, true)
		if e == nil && len(ips) > 0 {
			target = ips[0]
		}
	}
	if err := cfg.Validate(); err != nil {
		u.showError(err)
		return
	}
	ctx, cancel := context.WithCancel(u.ctx)
	u.optCancel = cancel
	u.Optimize.SetText("Stop optimization")
	u.Status.SetText("Optimizing fragments… · see Activity for individual tests.")
	debug := u.Session.Debug
	go func() {
		best, err := appcore.Optimize(ctx, cfg, target, debug)
		cancel()
		if u.ctx.Err() != nil {
			return
		}
		fyne.Do(func() {
			u.optCancel = nil
			u.Optimize.SetText("Optimize fragments")
			u.Optimize.Enable()
			if err != nil {
				u.Status.SetText("Optimization: " + err.Error())
				return
			}
			appcore.ApplyOptimization(u.Session.Config, best)
			u.rebuildEditor()
			u.autoSave()
			u.Status.SetText(fmt.Sprintf("Optimized · %s · length %s · interval %s · %d ms", best.Zone, best.SizeRange, best.IntervalRange, best.Latency.Milliseconds()))
		})
	}()
}

func (u *UI) save() error {
	c, err := u.Editor.Snapshot()
	if err != nil {
		return err
	}
	s := *u.Session
	s.Config = c
	s.Ranges = u.Source.Text
	return appcore.SaveSession(u.DataDir, &s)
}
func (u *UI) autoSave() {
	if u.loading || u.Editor == nil || u.Source == nil {
		return
	}
	if err := u.save(); err != nil {
		u.Status.SetText("Settings not saved: " + err.Error())
	} else if strings.HasPrefix(u.Status.Text, "Settings not saved:") {
		u.Status.SetText("Settings saved.")
	}
}
func (u *UI) showError(err error) { u.Status.SetText(err.Error()); dialog.ShowError(err, u.Window) }

// StartRefresh uses Fyne's main-thread queue for all UI mutations. Closing the
// window cancels both tasks and this loop rather than leaving background workers.
func (u *UI) StartRefresh() {
	go func() {
		t := time.NewTicker(300 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-u.ctx.Done():
				return
			case <-t.C:
				fyne.Do(func() {
					if u.ctx.Err() != nil {
						return
					}
					if r := u.run; r != nil {
						done, total := r.Progress()
						if total > 0 {
							u.Progress.SetValue(float64(done) / float64(total))
						}
						u.refreshResultsThrottled()
						u.refreshLive(r, true)
						// Rewriting every result file costs more as results grow;
						// space intermediate saves out accordingly (5–60 s).
						every := 5*time.Second + time.Duration(r.Results.Count()/2000)*time.Second
						if every > time.Minute {
							every = time.Minute
						}
						if r.Config.Output.SaveIntermediateResults && time.Since(u.lastSave) > every {
							u.lastSave = time.Now()
							stamp := u.runStamp
							go func() {
								if !u.saveMu.TryLock() {
									return // previous save still running
								}
								defer u.saveMu.Unlock()
								if _, err := appcore.SaveResults(u.DataDir, r.Config, r.Results, stamp); err != nil {
									applog.Logf("intermediate save failed: %v", err)
								}
							}()
						}
					}
					u.refreshLog(false)
				})
			}
		}
	}()
}

// refreshResultsThrottled keeps the UI thread free during big scans: the
// result table is only re-sorted when it is visible and results changed, at
// most every 0.4 s (1.5 s above 5,000 results). The summary is always cheap.
func (u *UI) refreshResultsThrottled() {
	if u.results == nil {
		return
	}
	n := u.results.Count()
	visible := u.Tabs != nil && u.Tabs.SelectedIndex() == 3
	interval := 400 * time.Millisecond
	if n > 5000 {
		interval = 1500 * time.Millisecond
	}
	if visible && n != u.lastCount && time.Since(u.lastTable) >= interval {
		u.refreshResults()
		return
	}
	u.Summary.SetText(fmt.Sprintf("%d tested · %d good", n, u.results.SuccessCount()))
}

// refreshLog only rebuilds the log view when it is visible and new lines exist.
func (u *UI) refreshLog(force bool) {
	if u.Log == nil {
		return
	}
	if !force && (u.Tabs == nil || u.Tabs.SelectedIndex() != 4) {
		return
	}
	g := applog.Generation()
	if !force && g == u.lastLogGen {
		return
	}
	u.lastLogGen = g
	text := strings.Join(applog.Tail(300), "\n")
	if text != u.lastLog {
		u.lastLog = text
		u.Log.SetText(text)
	}
}

func (u *UI) Close() {
	u.cancel()
	if u.run != nil {
		u.run.Stop()
	}
	if u.optCancel != nil {
		u.optCancel()
	}
}

func (u *UI) readFile(apply func(string), exts ...string) {
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil {
			u.showError(err)
			return
		}
		if r == nil {
			return
		}
		defer r.Close()
		b, err := io.ReadAll(r)
		if err != nil {
			u.showError(err)
			return
		}
		apply(string(b))
	}, u.Window)
	d.SetFilter(storage.NewExtensionFileFilter(exts))
	d.Show()
}

func (u *UI) shortcuts() {
	c := u.Window.Canvas()
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyC, Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift}, func(fyne.Shortcut) { u.copySelected() })
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierControl}, func(fyne.Shortcut) {
		if err := u.save(); err != nil {
			u.showError(err)
		} else {
			u.Status.SetText("Settings saved.")
		}
	})
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyO, Modifier: fyne.KeyModifierControl}, func(fyne.Shortcut) { u.Tabs.SelectIndex(1); u.importConnection() })
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: fyne.KeyModifierControl}, func(fyne.Shortcut) {
		if u.run != nil {
			u.run.Stop()
		} else {
			u.start()
		}
	})
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyP, Modifier: fyne.KeyModifierControl}, func(fyne.Shortcut) {
		if u.run != nil && u.run.Mode != appcore.ModeCheck {
			u.Pause.OnTapped()
		}
	})
}

func heading(text string) fyne.CanvasObject {
	t := canvas.NewText(text, theme.ForegroundColor())
	t.TextSize = 23
	t.TextStyle = fyne.TextStyle{Bold: true}
	return t
}
func wrapped(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return l
}

// A 44px minimum hit area also applies to dropdowns and text entries, not just
// buttons. The transparent floor does not intercept the native control.
func touchControl(o fyne.CanvasObject) fyne.CanvasObject {
	floor := canvas.NewRectangle(color.Transparent)
	floor.SetMinSize(fyne.NewSize(0, 44))
	return container.NewStack(floor, o)
}
func commaU(n uint64) string {
	if n > uint64(^uint(0)>>1) {
		return strconv.FormatUint(n, 10)
	}
	return comma(int(n))
}

func comma(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func mergePreset(current, preset string) string {
	seen := map[string]bool{}
	for _, line := range strings.FieldsFunc(current, func(r rune) bool { return r == '\n' || r == ',' || r == ' ' || r == '\t' || r == '\r' }) {
		seen[line] = true
	}
	var add []string
	for _, line := range strings.Split(preset, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !seen[line] {
			add = append(add, line)
			seen[line] = true
		}
	}
	if len(add) == 0 {
		return current
	}
	if strings.TrimSpace(current) == "" {
		return strings.Join(add, "\n")
	}
	return strings.TrimRight(current, "\r\n") + "\n" + strings.Join(add, "\n")
}

func (u *UI) resultsTab() fyne.CanvasObject {
	u.GoodOnly = widget.NewCheck("Good only", func(bool) { u.refreshResults() })
	u.GoodOnly.SetChecked(true)
	u.Sort = widget.NewSelect([]string{"Quality", "Ping", "Upload", "Download", "TCP"}, func(string) { u.refreshResults() })
	u.Sort.SetSelected("Quality")
	u.Top = widget.NewEntry()
	u.Top.SetPlaceHolder("0 = all")
	u.Top.SetText(strconv.Itoa(u.Session.Top))
	u.Top.OnChanged = func(s string) {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < 0 {
			u.Top.SetValidationError(fmt.Errorf("enter 0 or a positive integer"))
			return
		}
		u.Top.SetValidationError(nil)
		u.Session.Top = n
		u.refreshResults()
		u.autoSave()
	}
	headers := []string{"IP", "Status", "Ping ms", "TCP ms", "TLS ms", "Jitter ms", "Loss %", "Up Mbps", "Down Mbps"}
	u.Table = widget.NewTable(
		func() (int, int) { return len(u.rows) + 1, len(headers) },
		func() fyne.CanvasObject { return widget.NewLabel("000.000.000.000") },
		func(id widget.TableCellID, o fyne.CanvasObject) {
			l := o.(*widget.Label)
			l.TextStyle = fyne.TextStyle{}
			if id.Row == 0 {
				l.SetText(headers[id.Col])
				l.TextStyle.Bold = true
				l.Refresh()
				return
			}
			if id.Row-1 >= len(u.rows) {
				return
			}
			r := u.rows[id.Row-1]
			values := []string{r.IP, "Failed", fmt.Sprint(r.LatencyMs), "-", fmt.Sprint(r.HandshakeMs), fmt.Sprint(r.JitterMs), fmt.Sprintf("%.1f", r.LossPercent), "-", "-"}
			if r.Success {
				values[1] = "Good"
			}
			if r.TCPOK {
				values[3] = fmt.Sprint(r.TCPMs)
			}
			if r.UploadTested {
				values[7] = fmt.Sprintf("%.2f", r.UploadMbps)
			}
			if r.DownloadTested {
				values[8] = fmt.Sprintf("%.2f", r.DownloadMbps)
			}
			l.SetText(values[id.Col])
		},
	)
	widths := []float32{155, 80, 88, 85, 85, 95, 80, 100, 115}
	for i, w := range widths {
		u.Table.SetColumnWidth(i, w)
	}
	u.Table.OnSelected = func(id widget.TableCellID) {
		if id.Row > 0 && id.Row <= len(u.rows) {
			u.selected = u.rows[id.Row-1].IP
		}
	}
	copySelected := widget.NewButtonWithIcon("Copy IP", theme.ContentCopyIcon(), u.copySelected)
	copyAll := widget.NewButtonWithIcon("Copy good IPs", theme.ContentCopyIcon(), func() {
		var ips []string
		for _, r := range u.results.GetSortedByLatency() {
			ips = append(ips, r.IP)
		}
		u.App.Clipboard().SetContent(strings.Join(ips, "\n"))
		u.Status.SetText(fmt.Sprintf("%d good IPs copied.", len(ips)))
	})
	details := widget.NewButtonWithIcon("Details", theme.InfoIcon(), func() {
		for _, r := range u.results.GetResults() {
			if r.IP != u.selected {
				continue
			}
			b, _ := json.MarshalIndent(r, "", "  ")
			e := widget.NewMultiLineEntry()
			e.SetText(string(b))
			e.SetMinRowsVisible(12)
			d := dialog.NewCustom("All result metrics", "Close", e, u.Window)
			d.Resize(fyne.NewSize(650, 470))
			if u.Mobile {
				d.Resize(fyne.NewSize(350, 600))
			}
			d.Show()
			return
		}
		u.showError(fmt.Errorf("select a result row first"))
	})
	csv := widget.NewButtonWithIcon("Export CSV", theme.DocumentSaveIcon(), func() { u.export("csv", false) })
	js := widget.NewButtonWithIcon("Export JSON", theme.DocumentSaveIcon(), func() { u.export("json", false) })
	copyJSON := widget.NewButtonWithIcon("Copy JSON", theme.ContentCopyIcon(), func() { u.export("json", true) })
	filters := container.NewGridWithColumns(3, labeled("Sort by", u.Sort), labeled("Top results · 0 = all", u.Top), container.NewVBox(fieldLabel("Show"), touchControl(u.GoodOnly)))
	if u.Mobile {
		filters = container.NewVBox(container.NewGridWithColumns(2, labeled("Sort by", u.Sort), labeled("Top · 0 = all", u.Top)), u.GoodOnly)
	}
	top := container.NewVBox(sectionTitle("Live results"), muted("Quality sorts by packet-loss tier, then latency. Scroll the table sideways for every metric."), filters, gap(2))
	copyAll.Importance = widget.HighImportance
	bottom := container.NewVBox(gap(2), container.NewGridWithColumns(3, copySelected, copyAll, details), container.NewGridWithColumns(3, csv, js, copyJSON))
	if u.Mobile {
		bottom = container.NewVBox(gap(2), container.NewGridWithColumns(2, copySelected, copyAll), details, container.NewGridWithColumns(2, csv, js), copyJSON)
	}
	return container.New(layout.NewCustomPaddedLayout(10, 12, 14, 14), cardFill(top, bottom, u.Table))
}

func (u *UI) refreshResults() {
	if u.results == nil || u.Table == nil {
		return
	}
	if u.GoodOnly.Checked {
		u.rows = u.results.GetSortedByLatency()
	} else {
		u.rows = u.results.GetResults()
	}
	switch u.Sort.Selected {
	case "Ping":
		sort.SliceStable(u.rows, func(i, j int) bool { return u.rows[i].LatencyMs < u.rows[j].LatencyMs })
	case "Upload":
		sort.SliceStable(u.rows, func(i, j int) bool { return u.rows[i].UploadMbps > u.rows[j].UploadMbps })
	case "Download":
		sort.SliceStable(u.rows, func(i, j int) bool { return u.rows[i].DownloadMbps > u.rows[j].DownloadMbps })
	case "TCP":
		sort.SliceStable(u.rows, func(i, j int) bool { return u.rows[i].TCPMs < u.rows[j].TCPMs })
	default:
		if !u.GoodOnly.Checked {
			sort.SliceStable(u.rows, func(i, j int) bool {
				a, b := u.rows[i], u.rows[j]
				if a.Success != b.Success {
					return a.Success
				}
				if int(a.LossPercent)/10 != int(b.LossPercent)/10 {
					return a.LossPercent < b.LossPercent
				}
				return a.LatencyMs < b.LatencyMs
			})
		}
	}
	if n := u.Session.Top; n > 0 && n < len(u.rows) {
		u.rows = u.rows[:n]
	}
	if u.selected != "" {
		found := false
		for _, r := range u.rows {
			if r.IP == u.selected {
				found = true
			}
		}
		if !found {
			u.selected = ""
			u.Table.UnselectAll()
		}
	}
	u.Table.Refresh()
	u.lastTable = time.Now()
	u.lastCount = u.results.Count()
	u.Summary.SetText(fmt.Sprintf("%d tested · %d good", u.lastCount, u.results.SuccessCount()))
}

func (u *UI) copySelected() {
	if u.selected == "" {
		u.Status.SetText("Select an IP row first.")
		return
	}
	u.App.Clipboard().SetContent(u.selected)
	u.Status.SetText("IP copied: " + u.selected)
}

func (u *UI) export(format string, copyToClipboard bool) {
	rows := u.results.GetResults()
	if u.GoodOnly.Checked {
		rows = u.results.GetSortedByLatency()
	}
	if len(rows) == 0 {
		u.showError(fmt.Errorf("no results to export"))
		return
	}
	write := func(w io.Writer) error { return appcore.EncodeResults(w, rows, format) }
	if copyToClipboard {
		var b bytes.Buffer
		if err := write(&b); err != nil {
			u.showError(err)
			return
		}
		u.App.Clipboard().SetContent(b.String())
		u.Status.SetText("Results copied as " + strings.ToUpper(format) + ".")
		return
	}
	d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil {
			u.showError(err)
			return
		}
		if w == nil {
			return
		}
		err = write(w)
		if closeErr := w.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			u.showError(err)
			return
		}
		u.Status.SetText("Export saved.")
	}, u.Window)
	d.SetFileName(time.Now().Format("2006-01-02_150405") + "_results." + format)
	d.SetFilter(storage.NewExtensionFileFilter([]string{"." + format}))
	d.Show()
}

func (u *UI) activityTab() fyne.CanvasObject {
	u.History = widget.NewList(func() int { return len(u.historyPaths) }, func() fyne.CanvasObject { return widget.NewLabel("2026-01-01_120000_results.csv") }, func(i int, o fyne.CanvasObject) { o.(*widget.Label).SetText(filepath.Base(u.historyPaths[i])) })
	u.History.OnSelected = func(i int) { u.historySelected = i }
	open := widget.NewButtonWithIcon("Open scan", theme.FolderOpenIcon(), func() {
		if u.historySelected < 0 || u.historySelected >= len(u.historyPaths) {
			u.showError(fmt.Errorf("select a saved scan"))
			return
		}
		if u.run != nil {
			u.showError(fmt.Errorf("wait for or stop the active scan before opening history"))
			return
		}
		path := u.historyPaths[u.historySelected]
		var rows []scanner.Result
		var err error
		if strings.EqualFold(filepath.Ext(path), ".csv") {
			rows, err = scanner.LoadResultsCSV(path)
		} else {
			var b []byte
			b, err = os.ReadFile(path)
			if err == nil {
				err = json.Unmarshal(b, &rows)
			}
		}
		if err != nil {
			u.showError(err)
			return
		}
		u.results = scanner.NewResultCollectorFrom(rows)
		u.selected = ""
		u.refreshResults()
		u.Tabs.SelectIndex(3)
		u.Status.SetText("Opened " + filepath.Base(path))
	})
	refresh := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), u.refreshHistory)
	open.Importance = widget.HighImportance
	history := cardFill(container.NewVBox(sectionTitle("Saved scans"), muted("Final and intermediate results stay in the application data folder."), gap(2)), container.NewVBox(gap(2), container.NewGridWithColumns(2, open, refresh)), u.History)
	u.Log = widget.NewMultiLineEntry()
	u.Log.Wrapping = fyne.TextWrapWord
	u.Log.SetPlaceHolder("Scan and optimizer events appear here.")
	// Log text remains selectable/copyable; edits never affect the underlying file.
	copyLog := widget.NewButtonWithIcon("Copy full log", theme.ContentCopyIcon(), func() {
		u.App.Clipboard().SetContent(strings.Join(applog.Snapshot(), "\n"))
		u.Status.SetText("Log copied.")
	})
	copyDebug := widget.NewButtonWithIcon("Copy Xray debug JSON", theme.ContentCopyIcon(), func() {
		b, err := os.ReadFile(filepath.Join(u.DataDir, "first_config.jsonc"))
		if err != nil {
			u.showError(fmt.Errorf("no generated debug config yet; enable Debug in Settings and run a proxy test"))
			return
		}
		u.App.Clipboard().SetContent(string(b))
		u.Status.SetText("Debug JSON copied · it can contain connection credentials.")
	})
	logActions := container.NewGridWithColumns(2, copyLog, copyDebug)
	if u.Mobile {
		logActions = container.NewVBox(copyLog, copyDebug)
	}
	log := cardFill(container.NewVBox(sectionTitle("Run log"), muted("Recent 300 events are shown; the full log is saved privately."), gap(2)), container.NewVBox(gap(2), logActions), u.Log)
	pad := func(o fyne.CanvasObject) fyne.CanvasObject { return container.New(layout.NewCustomPaddedLayout(8, 12, 14, 14), o) }
	tabs := container.NewAppTabs(container.NewTabItemWithIcon("History", theme.HistoryIcon(), pad(history)), container.NewTabItemWithIcon("Log", theme.DocumentIcon(), pad(log)))
	return tabs
}

func (u *UI) refreshHistory() {
	if u.History == nil {
		return
	}
	dir := appcore.OutputDirectory(u.DataDir, u.Session.Config)
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		applog.Logf("history: %v", err)
	}
	u.historyPaths = nil
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !e.IsDir() && (ext == ".csv" || ext == ".json") {
			u.historyPaths = append(u.historyPaths, filepath.Join(dir, e.Name()))
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(u.historyPaths)))
	u.historySelected = -1
	u.History.UnselectAll()
	u.History.Refresh()
}
