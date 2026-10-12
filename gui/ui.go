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
	sortCol                                  int
	sortDesc                                 bool
	Top                                      *widget.Entry
	Log                                      *widget.Entry
	History                                  *widget.List
	ThemeButton, LangButton                  *widget.Button
	LangSelect                               *widget.Select
	version                                  string
	ThemeSelect                              *widget.Select
	themeMode                                string
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
	lang = a.Preferences().StringWithFallback("lang", LangEnglish)
	if lang != LangPersian {
		lang = LangEnglish
	}
	u.version = version
	u.LangButton = widget.NewButton(map[string]string{LangEnglish: "فارسی", LangPersian: "English"}[lang], func() {
		u.setLanguage(map[string]string{LangEnglish: LangPersian, LangPersian: LangEnglish}[lang])
	})
	u.LangButton.Importance = widget.LowImportance
	u.ThemeButton = widget.NewButtonWithIcon("", theme.ColorPaletteIcon(), func() {
		next := map[string]string{ThemeSystem: ThemeLight, ThemeLight: ThemeDark, ThemeDark: ThemeSystem}[u.themeMode]
		u.setTheme(next)
	})
	u.ThemeButton.Importance = widget.LowImportance
	u.setTheme(a.Preferences().StringWithFallback("theme", ThemeSystem))
	a.Settings().AddListener(func(fyne.Settings) { fyne.Do(repaintAll) })
	w.SetIcon(Icon)
	u.Status = widget.NewLabel(T("Ready · choose a mode and add your connection."))
	u.Status.Wrapping = fyne.TextWrapWord
	u.Summary = widget.NewLabel(T("0 tested · 0 good"))
	u.Preview = widget.NewLabel(T("Add IPs or CIDRs below."))
	u.Preview.Wrapping = fyne.TextWrapWord
	u.ConnectionInfo = widget.NewLabel(T(""))
	u.ConnectionInfo.Truncation = fyne.TextTruncateEllipsis
	u.Progress = widget.NewProgressBar()
	u.Source = widget.NewMultiLineEntry()
	u.Source.Wrapping = fyne.TextWrapWord
	u.Source.SetMinRowsVisible(6)
	if mobile {
		u.Source.SetMinRowsVisible(5)
	}
	u.Source.SetPlaceHolder(T("One IP or CIDR per line, e.g. 104.16.0.0/24"))
	u.Source.SetText(s.Ranges)
	u.Source.OnChanged = func(string) { u.updatePreview(); u.autoSave() }
	u.ImportText = widget.NewMultiLineEntry()
	u.ImportText.Wrapping = fyne.TextWrapWord
	u.ImportText.SetMinRowsVisible(4)
	u.ImportText.SetPlaceHolder(T("Paste a vless:// link, Xray JSON, app JSON or Base64 subscription"))
	u.Mode = widget.NewSelect([]string{modeLabel(appcore.ModeXray), modeLabel(appcore.ModeConnectivity)}, nil)
	if s.Mode == appcore.ModeCheck {
		s.Mode = appcore.ModeXray
	}
	u.Mode.SetSelected(modeLabel(s.Mode))
	u.Mode.OnChanged = func(v string) { u.Session.Mode = modeValue(v); u.updatePreview(); u.autoSave() }
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
	u.PerRange.SetPlaceHolder(T("0 = every IP in the range"))
	u.PerRange.SetText(strconv.Itoa(s.Config.Scan.SampleSize))
	u.PerRange.OnChanged = u.perRangeChanged
	u.Breakdown = wrapped("")
	// One line in the always-visible action bar; truncation keeps its height exact.
	u.Current = widget.NewLabel(T(""))
	u.Current.Truncation = fyne.TextTruncateEllipsis
	u.syncSourceControls()
	u.Start = widget.NewButtonWithIcon(T("Start"), theme.MediaPlayIcon(), u.start)
	u.Start.Importance = widget.HighImportance
	u.Pause = widget.NewButtonWithIcon(T("Pause"), theme.MediaPauseIcon(), func() {
		if u.run == nil {
			return
		}
		if u.run.TogglePause() {
			u.Pause.SetText(T("Resume"))
			u.Status.SetText(T("Paused · in-flight tests finish first."))
		} else {
			u.Pause.SetText(T("Pause"))
			u.Status.SetText(T("Scanning…"))
		}
		applog.Logf("pause=%v", u.run.Paused())
	})
	u.Stop = widget.NewButtonWithIcon(T("Stop"), theme.CancelIcon(), func() {
		if u.run != nil {
			u.run.Stop()
			u.Stop.Disable()
			u.Pause.Disable()
			u.Status.SetText(T("Stopping…"))
		}
	})
	u.Pause.Importance = widget.WarningImportance
	u.Stop.Importance = widget.DangerImportance
	u.Optimize = widget.NewButtonWithIcon(T("Optimize fragments"), theme.SettingsIcon(), u.optimize)
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
		container.NewTabItemWithIcon(T(names[0]), theme.SearchIcon(), scanTab),
		container.NewTabItemWithIcon(T(names[1]), theme.ComputerIcon(), connectionTab),
		container.NewTabItemWithIcon(T(names[2]), theme.SettingsIcon(), settingsTab),
		container.NewTabItemWithIcon(T(names[3]), theme.ListIcon(), resultsTab),
		container.NewTabItemWithIcon(T(names[4]), theme.HistoryIcon(), activityTab),
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
	top := header(version, mobile, container.NewHBox(u.LangButton, u.ThemeButton))
	footer := container.NewVBox(widget.NewSeparator(), container.New(layout.NewCustomPaddedLayout(0, 0, 10, 10), u.Status))
	w.SetContent(container.NewBorder(top, footer, nil, nil, u.Tabs))
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
	add := widget.NewButtonWithIcon(T("Add preset"), theme.ContentAddIcon(), func() {
		u.Source.SetText(mergePreset(u.Source.Text, Presets[preset.Selected]))
	})
	importRanges := widget.NewButtonWithIcon(T("Import"), theme.FolderOpenIcon(), func() {
		u.readFile(func(text string) { u.Source.SetText(text) }, ".txt", ".csv")
	})
	copyRanges := widget.NewButtonWithIcon(T("Copy"), theme.ContentCopyIcon(), func() { u.App.Clipboard().SetContent(u.Source.Text); u.Status.SetText(T("Ranges copied.")) })
	clearRanges := widget.NewButtonWithIcon(T("Clear"), theme.ContentClearIcon(), func() { u.Source.SetText(T("")) })
	settings := widget.NewButtonWithIcon(T("Scan settings"), theme.SettingsIcon(), func() { u.Tabs.SelectIndex(2) })

	modeCard := card("Scan mode", "Connection test puts each IP into your config and really connects.",
		touchControl(u.Mode), u.ConnectionInfo)
	targetCard := card("Targets", "",
		touchControl(u.SourceKind), u.SourceHelp,
		labeled("IPs per range · 0 = all", u.PerRange),
		settings)
	rangesCard := card("IP ranges", "",
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
	parse := widget.NewButtonWithIcon(T("Apply connection"), theme.ConfirmIcon(), u.importConnection)
	u.Apply = parse
	parse.Importance = widget.HighImportance
	open := widget.NewButtonWithIcon(T("Import file"), theme.FolderOpenIcon(), func() {
		u.readFile(func(text string) { u.ImportText.SetText(text); u.importConnection() }, ".json", ".txt", ".jsonc")
	})
	copyConfig := widget.NewButtonWithIcon(T("Copy full config"), theme.ContentCopyIcon(), func() {
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
		u.Status.SetText(T("Full configuration copied · it may contain private credentials."))
	})
	reset := widget.NewButtonWithIcon(T("Clear connection"), theme.DeleteIcon(), func() {
		dialog.ShowConfirm(T("Clear connection"), T("Remove the saved connection (address, UUID, SNI…)? Scan settings are kept."), func(ok bool) {
			if !ok {
				return
			}
			c := config.DefaultConfig()
			c.Scan, c.Output, c.Xray, c.Fragment = u.Session.Config.Scan, u.Session.Config.Output, u.Session.Config.Xray, u.Session.Config.Fragment
			u.Session.Config = c
			u.ImportText.SetText(T(""))
			u.rebuildEditor()
			u.updateConnectionInfo()
			u.autoSave()
			u.Status.SetText(T("Connection cleared."))
		}, u.Window)
	})
	reset.Importance = widget.DangerImportance
	connButtons := fyne.CanvasObject(container.NewGridWithColumns(3, open, copyConfig, reset))
	if u.Mobile {
		copyConfig.SetText(T("Copy config"))
		reset.SetText(T("Clear"))
		connButtons = container.NewVBox(container.NewGridWithColumns(2, open, copyConfig), reset)
	}
	importCard := card("Your connection", "vless:// link, Xray JSON, app JSON or Base64 subscription. Stays on this device.",
		u.ImportText, parse, connButtons)
	details := card("Connection details", "", u.connectionFields)
	return page(importCard, details)
}

func (u *UI) settingsTab() fyne.CanvasObject {
	debug := widget.NewCheck(T("Save Xray debug config"), nil)
	debug.SetChecked(u.Session.Debug)
	debug.OnChanged = func(v bool) { u.Session.Debug = v; u.autoSave() }
	save := widget.NewButtonWithIcon(T("Save settings"), theme.DocumentSaveIcon(), func() {
		if err := u.save(); err != nil {
			u.showError(err)
		} else {
			u.Status.SetText(T("Settings saved in your application data folder."))
		}
	})
	save.Importance = widget.HighImportance
	u.ThemeSelect = widget.NewSelect([]string{T(ThemeSystem), T(ThemeLight), T(ThemeDark)}, nil)
	u.ThemeSelect.SetSelected(T(u.themeMode))
	u.ThemeSelect.OnChanged = func(v string) {
		for _, m := range []string{ThemeSystem, ThemeLight, ThemeDark} {
			if T(m) == v && m != u.themeMode {
				u.setTheme(m)
			}
		}
	}
	u.LangSelect = widget.NewSelect([]string{"English", "فارسی"}, nil)
	u.LangSelect.SetSelected(map[string]string{LangEnglish: "English", LangPersian: "فارسی"}[lang])
	u.LangSelect.OnChanged = func(v string) {
		u.setLanguage(map[string]string{"English": LangEnglish, "فارسی": LangPersian}[v])
	}
	appearance := card("Appearance", "", container.NewGridWithColumns(2, labeled("Theme", u.ThemeSelect), labeled("Language", u.LangSelect)))
	optimize := card("Fragment optimizer", "Finds working TLS fragment values. Tap again to stop.", u.Optimize)
	options := card("All options", "", u.settingsFields)
	misc := card("Debug & saving", "Debug output can contain your connection credentials. Keep it private.", debug, save)
	return page(appearance, optimize, options, misc)
}

// setTheme switches between System / Light / Dark and remembers the choice.
func (u *UI) setTheme(mode string) {
	modes := map[string]string{ThemeSystem: "", ThemeLight: "light", ThemeDark: "dark"}
	m, ok := modes[mode]
	if !ok {
		mode, m = ThemeSystem, ""
	}
	u.themeMode = mode
	u.App.Preferences().SetString("theme", mode)
	u.App.Settings().SetTheme(appTheme{mode: m})
	if u.ThemeButton != nil {
		u.ThemeButton.SetText(T(mode))
	}
	if u.ThemeSelect != nil && u.ThemeSelect.Selected != T(mode) {
		u.ThemeSelect.SetSelected(T(mode))
	}
	repaintAll()
}

// setLanguage switches the UI language and rebuilds the window in place.
func (u *UI) setLanguage(l string) {
	if l == lang || (l != LangEnglish && l != LangPersian) {
		return
	}
	if u.run != nil {
		u.Status.SetText(T("Stop the scan before changing the language."))
		if u.LangSelect != nil {
			u.LangSelect.SetSelected(map[string]string{LangEnglish: "English", LangPersian: "فارسی"}[lang])
		}
		return
	}
	_ = u.save()
	u.App.Preferences().SetString("lang", l)
	u.Close()
	nu := New(u.App, u.Window, u.DataDir, u.version, u.Mobile)
	nu.StartRefresh()
	u.Window.SetOnClosed(nu.Close)
}

func (u *UI) rebuildEditor() {
	u.Editor = NewEditor(u.Session.Config, func() { u.updatePreview(); u.updateConnectionInfo(); u.autoSave() })
	u.connectionFields.Objects = []fyne.CanvasObject{u.Editor.Sections["proxy"]}
	u.connectionFields.Refresh()
	accordion := widget.NewAccordion(
		widget.NewAccordionItem(T("Scan & quality measurements"), u.Editor.Sections["scan"]),
		widget.NewAccordionItem(T("TLS fragmentation"), u.Editor.Sections["fragment"]),
		widget.NewAccordionItem(T("Output & automatic saving"), u.Editor.Sections["output"]),
		widget.NewAccordionItem(T("Xray & multiplexing"), u.Editor.Sections["xray"]),
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
	u.Status.SetText(T("Connection applied · settings saved."))
}

func (u *UI) updateConnectionInfo() {
	if u.ConnectionInfo == nil {
		return
	}
	c := u.Session.Config
	if c.Proxy.UUID == "" {
		u.ConnectionInfo.SetText(T("No proxy · import in Connection."))
		return
	}
	u.ConnectionInfo.SetText(fmt.Sprintf("%s / %s · %s:%d", strings.ToUpper(c.Proxy.Method), strings.ToUpper(c.Proxy.Type), c.Proxy.Address, c.Proxy.Port))
}

// perRangeChanged keeps the Scan tab's "IPs per range" and the Settings field
// (scan.sampleSize) in sync; both edit the same configuration value.
func (u *UI) perRangeChanged(text string) {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || n < 0 {
		u.PerRange.SetValidationError(errors.New(T("enter 0 (all) or a positive number")))
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
		u.SourceHelp.SetText(T("Scans the addresses exactly as entered, in order. Nothing is sampled or shuffled; a CIDR in the list is expanded to every address."))
	} else {
		u.PerRange.Enable()
		u.SourceHelp.SetText(T("Generates the IPs from each CIDR range: the chosen number of random IPs per range (0 = every IP in the range), then scans them."))
	}
}

// Scan modes are shown with clear, translated names; the session keeps the
// stable internal values.
func modeLabel(v string) string {
	switch v {
	case appcore.ModeConnectivity:
		return T("Reachability (no proxy)")
	default:
		return T("Connection test (your config)")
	}
}

func modeValue(label string) string {
	if label == modeLabel(appcore.ModeConnectivity) {
		return appcore.ModeConnectivity
	}
	return appcore.ModeXray
}

func (u *UI) mode() string {
	if u.Mode == nil {
		return appcore.ModeXray
	}
	return modeValue(u.Mode.Selected)
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
		mode = u.mode()
	}
	var s string
	switch {
	case strings.TrimSpace(u.Source.Text) == "":
		s = T("Add IPs / CIDRs, or choose a built-in preset.")
	default:
		s = Tf("%s IPs will be scanned from %d range(s) · %d worker(s)", commaU(n), len(plan), cfg.Scan.Threads)
		if capped {
			s += Tf(" · limited by Maximum IPs (%s)", comma(cfg.Scan.MaxIPs))
		}
		if total > utils.MaxGeneratedTargets {
			s += Tf(" · more than %s IPs: set IPs per range or narrow the ranges", comma(utils.MaxGeneratedTargets))
		}
		if invalid > 0 {
			s += Tf(" · %d invalid entr%s ignored", invalid, map[bool]string{true: "y", false: "ies"}[invalid == 1])
		}
		if !ipList && cfg.Scan.SampleSize > 0 {
			s += Tf(" · %d IP(s) per range", cfg.Scan.SampleSize)
		}
		if mode == appcore.ModeConnectivity {
			s += T(" · ping mode: ") + utils.PingModeString() + T(" (ICMP requires privileges).")
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
			u.Current.SetText(T(""))
		}
	}
}

// formatRanges renders the per-range breakdown. live adds tested/good counts.
func formatRanges(stats []appcore.RangeStat, live bool, limit int) string {
	if len(stats) == 0 {
		return T("No ranges yet.")
	}
	var b strings.Builder
	for i, r := range stats {
		if limit > 0 && i == limit {
			b.WriteString(Tf("+%d more range(s)", len(stats)-limit))
			break
		}
		if i > 0 {
			b.WriteString("\n")
		}
		hosts := "∞"
		if r.Hosts != ^uint64(0) {
			hosts = commaU(r.Hosts)
		}
		b.WriteString(Tf("%s\n    %s IPs in range · %s queued", r.Label(), hosts, comma(r.Queued)))
		if live {
			b.WriteString(Tf(" · %s tested · %s good", comma(r.Tested), comma(r.Good)))
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
		u.Current.SetText(T(""))
		return
	}
	var text string
	switch {
	case r.Paused():
		text = T("Paused")
	case len(snap.Active) == 0:
		text = T("Preparing the scanner…")
		done, total := r.Progress()
		if total > 0 && done >= total {
			text = T("Finishing…")
		}
	default:
		text = T("Now scanning ") + strings.Join(snap.Active, ", ")
		if more := snap.ActiveTotal - len(snap.Active); more > 0 {
			text += Tf(" +%d more", more)
		}
		if snap.CurrentRange != "" {
			text += T(" · range ") + snap.CurrentRange
		}
	}
	u.Current.SetText(text)
}

func (u *UI) start() {
	if u.run != nil {
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
		r, err := appcore.NewRunWithSource(u.ctx, cfg, u.Source.Text, u.mode(), u.Session.Source, u.Session.Debug)
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
		u.Pause.SetText(T("Pause"))
		u.Progress.SetValue(0)
		u.Status.SetText(T("Scanning… · you can edit settings for the next run."))
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
				message := Tf("Done · %d good / %d tested", r.Results.SuccessCount(), r.Results.Count())
				if errors.Is(err, context.Canceled) {
					message = T("Stopped · partial results saved.")
				} else if err != nil {
					message = T("Scan failed: ") + err.Error()
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
	if n > 250000 && u.mode() != appcore.ModeCheck {
		dialog.NewConfirm(T("Large scan"), Tf("This queues %s IPs and can use substantial memory and data. Continue, or reduce IPs per range or Maximum IPs?", comma(n)), func(ok bool) {
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
		u.Status.SetText(T("Stopping fragment optimization…"))
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
	u.Optimize.SetText(T("Stop optimization"))
	u.Status.SetText(T("Optimizing fragments… · see Activity for individual tests."))
	debug := u.Session.Debug
	go func() {
		best, err := appcore.Optimize(ctx, cfg, target, debug)
		cancel()
		if u.ctx.Err() != nil {
			return
		}
		fyne.Do(func() {
			u.optCancel = nil
			u.Optimize.SetText(T("Optimize fragments"))
			u.Optimize.Enable()
			if err != nil {
				u.Status.SetText(T("Optimization: ") + err.Error())
				return
			}
			appcore.ApplyOptimization(u.Session.Config, best)
			u.rebuildEditor()
			u.autoSave()
			u.Status.SetText(Tf("Optimized · %s · length %s · interval %s · %d ms", best.Zone, best.SizeRange, best.IntervalRange, best.Latency.Milliseconds()))
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
		u.Status.SetText(T("Settings not saved: ") + err.Error())
	} else if strings.HasPrefix(u.Status.Text, "Settings not saved:") {
		u.Status.SetText(T("Settings saved."))
	}
}
func (u *UI) showError(err error) {
	err = errors.New(T(err.Error()))
	u.Status.SetText(err.Error())
	dialog.ShowError(err, u.Window)
}

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
	u.Summary.SetText(Tf("%d tested · %d good", n, u.results.SuccessCount()))
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
			u.Status.SetText(T("Settings saved."))
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
	l := widget.NewLabel(T(text))
	l.Alignment = textAlign()
	l.Wrapping = fyne.TextWrapWord
	return l
}

// A 44px minimum hit area also applies to dropdowns and text entries, not just
// buttons. The transparent floor does not intercept the native control.
func touchControl(o fyne.CanvasObject) fyne.CanvasObject {
	floor := canvas.NewRectangle(color.Transparent)
	floor.SetMinSize(fyne.NewSize(0, 44))
	return container.NewStack(floor, withScrollRelay(o))
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
	u.GoodOnly = widget.NewCheck(T("Good only"), func(bool) { u.refreshResults() })
	u.GoodOnly.SetChecked(true)
	u.sortCol = -1
	u.Sort = widget.NewSelect(nil, nil)
	u.Sort.OnChanged = func(v string) {
		col := -1
		for i, h := range resultHeaders {
			if T(h) == v {
				col = i
			}
		}
		if col != u.sortCol {
			u.sortCol, u.sortDesc = col, false
		}
		u.refreshResults()
	}
	opts := []string{T("Quality")}
	for _, h := range resultHeaders {
		opts = append(opts, T(h))
	}
	u.Sort.Options = opts
	u.Sort.SetSelected(T("Quality"))
	u.Top = widget.NewEntry()
	u.Top.SetPlaceHolder(T("0 = all"))
	u.Top.SetText(strconv.Itoa(u.Session.Top))
	u.Top.OnChanged = func(s string) {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < 0 {
			u.Top.SetValidationError(errors.New(T("enter 0 or a positive integer")))
			return
		}
		u.Top.SetValidationError(nil)
		u.Session.Top = n
		u.refreshResults()
		u.autoSave()
	}
	u.Table = widget.NewTable(
		func() (int, int) { return len(u.rows) + 1, len(resultHeaders) },
		func() fyne.CanvasObject {
			return container.NewBorder(nil, nil, nil, widget.NewIcon(nil), widget.NewLabel("000.000.000.000"))
		},
		func(id widget.TableCellID, o fyne.CanvasObject) {
			c := o.(*fyne.Container)
			l := c.Objects[0].(*widget.Label)
			icon := c.Objects[1].(*widget.Icon)
			l.TextStyle = fyne.TextStyle{}
			l.Importance = widget.MediumImportance
			icon.Hide()
			if id.Row == 0 {
				l.TextStyle.Bold = true
				if id.Col == u.sortCol {
					l.Importance = widget.HighImportance
					if u.sortDesc {
						icon.SetResource(theme.MoveDownIcon())
					} else {
						icon.SetResource(theme.MoveUpIcon())
					}
					icon.Show()
				}
				l.SetText(T(resultHeaders[id.Col]))
				return
			}
			if id.Row-1 >= len(u.rows) {
				l.SetText("")
				return
			}
			r := u.rows[id.Row-1]
			values := []string{r.IP, T("Failed"), fmt.Sprint(r.LatencyMs), "-", fmt.Sprint(r.HandshakeMs), fmt.Sprint(r.JitterMs), fmt.Sprintf("%.1f", r.LossPercent), "-", "-"}
			if r.Success {
				values[1] = T("Good")
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
			if id.Col == 1 {
				l.TextStyle.Bold = true
				if r.Success {
					l.Importance = widget.SuccessImportance
				} else {
					l.Importance = widget.DangerImportance
				}
			}
			l.SetText(values[id.Col])
		},
	)
	widths := []float32{165, 95, 100, 95, 95, 110, 95, 115, 130}
	for i, w := range widths {
		u.Table.SetColumnWidth(i, w)
	}
	u.Table.OnSelected = func(id widget.TableCellID) {
		if id.Row == 0 {
			// Header tap: sort by this column; tapping again flips the direction.
			u.Table.Unselect(id)
			u.sortBy(id.Col)
			return
		}
		if id.Row > 0 && id.Row <= len(u.rows) {
			u.selected = u.rows[id.Row-1].IP
		}
	}
	copySelected := widget.NewButtonWithIcon(T("Copy IP"), theme.ContentCopyIcon(), u.copySelected)
	copyAll := widget.NewButtonWithIcon(T("Copy good IPs"), theme.ContentCopyIcon(), func() {
		var ips []string
		for _, r := range u.results.GetSortedByLatency() {
			ips = append(ips, r.IP)
		}
		u.App.Clipboard().SetContent(strings.Join(ips, "\n"))
		u.Status.SetText(Tf("%d good IPs copied.", len(ips)))
	})
	details := widget.NewButtonWithIcon(T("Details"), theme.InfoIcon(), func() {
		for _, r := range u.results.GetResults() {
			if r.IP != u.selected {
				continue
			}
			b, _ := json.MarshalIndent(r, "", "  ")
			e := widget.NewMultiLineEntry()
			e.SetText(string(b))
			e.SetMinRowsVisible(12)
			d := dialog.NewCustom(T("All result metrics"), T("Close"), e, u.Window)
			d.Resize(fyne.NewSize(650, 470))
			if u.Mobile {
				d.Resize(fyne.NewSize(350, 600))
			}
			d.Show()
			return
		}
		u.showError(errors.New(T("select a result row first")))
	})
	csv := widget.NewButtonWithIcon(T("Export CSV"), theme.DocumentSaveIcon(), func() { u.export("csv", false) })
	js := widget.NewButtonWithIcon(T("Export JSON"), theme.DocumentSaveIcon(), func() { u.export("json", false) })
	copyJSON := widget.NewButtonWithIcon(T("Copy JSON"), theme.ContentCopyIcon(), func() { u.export("json", true) })
	filters := container.NewGridWithColumns(3, labeled("Sort by", u.Sort), labeled("Top results · 0 = all", u.Top), container.NewVBox(fieldLabel("Show"), touchControl(u.GoodOnly)))
	if u.Mobile {
		filters = container.NewVBox(container.NewGridWithColumns(2, labeled("Sort by", u.Sort), labeled("Top · 0 = all", u.Top)), u.GoodOnly)
	}
	top := container.NewVBox(sectionTitle("Live results"), muted("Tap a column title to sort; tap it again to reverse."), filters, gap(2))
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
	if u.sortCol >= 0 {
		col, desc := u.sortCol, u.sortDesc
		sort.SliceStable(u.rows, func(i, j int) bool {
			a, b := u.rows[i], u.rows[j]
			if col != 1 && a.Success != b.Success {
				return a.Success // failed rows always stay below good ones
			}
			c := compareResults(a, b, col)
			if desc {
				return c > 0
			}
			return c < 0
		})
	} else if !u.GoodOnly.Checked {
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
	u.Summary.SetText(Tf("%d tested · %d good", u.lastCount, u.results.SuccessCount()))
}

var resultHeaders = []string{"IP", "Status", "Ping ms", "TCP ms", "TLS ms", "Jitter ms", "Loss %", "Up Mbps", "Down Mbps"}

// sortBy sorts the results by a column: first tap low → high, next tap high → low.
func (u *UI) sortBy(col int) {
	if col == u.sortCol {
		u.sortDesc = !u.sortDesc
	} else {
		u.sortCol, u.sortDesc = col, false
	}
	if u.Sort != nil && u.Sort.Selected != T(resultHeaders[col]) {
		cb := u.Sort.OnChanged
		u.Sort.OnChanged = nil
		u.Sort.SetSelected(T(resultHeaders[col]))
		u.Sort.OnChanged = cb
	}
	u.refreshResults()
}

func compareResults(a, b scanner.Result, col int) int {
	cmpF := func(x, y float64) int {
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
		return 0
	}
	tcp := func(r scanner.Result) float64 {
		if !r.TCPOK {
			return 1e12
		}
		return float64(r.TCPMs)
	}
	switch col {
	case 0:
		return compareIP(a.IP, b.IP)
	case 1:
		if a.Success == b.Success {
			return 0
		}
		if a.Success {
			return -1
		}
		return 1
	case 2:
		return cmpF(float64(a.LatencyMs), float64(b.LatencyMs))
	case 3:
		return cmpF(tcp(a), tcp(b))
	case 4:
		return cmpF(float64(a.HandshakeMs), float64(b.HandshakeMs))
	case 5:
		return cmpF(float64(a.JitterMs), float64(b.JitterMs))
	case 6:
		return cmpF(a.LossPercent, b.LossPercent)
	case 7:
		return cmpF(a.UploadMbps, b.UploadMbps)
	case 8:
		return cmpF(a.DownloadMbps, b.DownloadMbps)
	}
	return 0
}

// compareIP orders dotted IPv4 addresses numerically, falling back to text.
func compareIP(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	if len(pa) == 4 && len(pb) == 4 {
		for i := 0; i < 4; i++ {
			x, e1 := strconv.Atoi(pa[i])
			y, e2 := strconv.Atoi(pb[i])
			if e1 != nil || e2 != nil {
				break
			}
			if x != y {
				if x < y {
					return -1
				}
				return 1
			}
		}
		if a == b {
			return 0
		}
	}
	return strings.Compare(a, b)
}

func (u *UI) copySelected() {
	if u.selected == "" {
		u.Status.SetText(T("Select an IP row first."))
		return
	}
	u.App.Clipboard().SetContent(u.selected)
	u.Status.SetText(T("IP copied: ") + u.selected)
}

func (u *UI) export(format string, copyToClipboard bool) {
	rows := u.results.GetResults()
	if u.GoodOnly.Checked {
		rows = u.results.GetSortedByLatency()
	}
	if len(rows) == 0 {
		u.showError(errors.New(T("no results to export")))
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
		u.Status.SetText(T("Results copied as ") + strings.ToUpper(format) + ".")
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
		u.Status.SetText(T("Export saved."))
	}, u.Window)
	d.SetFileName(time.Now().Format("2006-01-02_150405") + "_results." + format)
	d.SetFilter(storage.NewExtensionFileFilter([]string{"." + format}))
	d.Show()
}

func (u *UI) activityTab() fyne.CanvasObject {
	u.History = widget.NewList(func() int { return len(u.historyPaths) }, func() fyne.CanvasObject { return widget.NewLabel(T("2026-01-01_120000_results.csv")) }, func(i int, o fyne.CanvasObject) { o.(*widget.Label).SetText(filepath.Base(u.historyPaths[i])) })
	u.History.OnSelected = func(i int) { u.historySelected = i }
	open := widget.NewButtonWithIcon(T("Open scan"), theme.FolderOpenIcon(), func() {
		if u.historySelected < 0 || u.historySelected >= len(u.historyPaths) {
			u.showError(errors.New(T("select a saved scan")))
			return
		}
		if u.run != nil {
			u.showError(errors.New(T("wait for or stop the active scan before opening history")))
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
		u.Status.SetText(T("Opened ") + filepath.Base(path))
	})
	refresh := widget.NewButtonWithIcon(T("Refresh"), theme.ViewRefreshIcon(), u.refreshHistory)
	open.Importance = widget.HighImportance
	history := cardFill(container.NewVBox(sectionTitle("Saved scans"), muted("Final and intermediate results stay in the application data folder."), gap(2)), container.NewVBox(gap(2), container.NewGridWithColumns(2, open, refresh)), u.History)
	u.Log = widget.NewMultiLineEntry()
	u.Log.Wrapping = fyne.TextWrapWord
	u.Log.SetPlaceHolder(T("Scan and optimizer events appear here."))
	// Log text remains selectable/copyable; edits never affect the underlying file.
	copyLog := widget.NewButtonWithIcon(T("Copy full log"), theme.ContentCopyIcon(), func() {
		u.App.Clipboard().SetContent(strings.Join(applog.Snapshot(), "\n"))
		u.Status.SetText(T("Log copied."))
	})
	copyDebug := widget.NewButtonWithIcon(T("Copy Xray debug JSON"), theme.ContentCopyIcon(), func() {
		b, err := os.ReadFile(filepath.Join(u.DataDir, "first_config.jsonc"))
		if err != nil {
			u.showError(errors.New(T("no generated debug config yet; enable Debug in Settings and run a proxy test")))
			return
		}
		u.App.Clipboard().SetContent(string(b))
		u.Status.SetText(T("Debug JSON copied · it can contain connection credentials."))
	})
	logActions := container.NewGridWithColumns(2, copyLog, copyDebug)
	if u.Mobile {
		logActions = container.NewVBox(copyLog, copyDebug)
	}
	log := cardFill(container.NewVBox(sectionTitle("Run log"), muted("Recent 300 events are shown; the full log is saved privately."), gap(2)), container.NewVBox(gap(2), logActions), u.Log)
	pad := func(o fyne.CanvasObject) fyne.CanvasObject {
		return container.New(layout.NewCustomPaddedLayout(8, 12, 14, 14), o)
	}
	tabs := container.NewAppTabs(container.NewTabItemWithIcon(T("History"), theme.HistoryIcon(), pad(history)), container.NewTabItemWithIcon(T("Log"), theme.DocumentIcon(), pad(log)))
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
