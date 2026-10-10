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
	Mode                                     *widget.Select
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
		}
	}
	title := canvas.NewText("Scaner", theme.ForegroundColor())
	title.TextSize = 27
	title.TextStyle = fyne.TextStyle{Bold: true}
	icon := canvas.NewImageFromResource(Icon)
	icon.SetMinSize(fyne.NewSize(40, 40))
	icon.FillMode = canvas.ImageFillContain
	brand := container.NewHBox(icon, container.NewVBox(title, widget.NewLabel("Piyazche engine")))
	header := container.NewBorder(nil, nil, brand, widget.NewLabel("v"+version))
	footer := container.NewVBox(widget.NewSeparator(), u.Status)
	w.SetContent(container.NewBorder(container.NewPadded(header), container.NewPadded(footer), nil, nil, u.Tabs))
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
	importRanges := widget.NewButtonWithIcon("Import IP file", theme.FolderOpenIcon(), func() {
		u.readFile(func(text string) { u.Source.SetText(text) }, ".txt", ".csv")
	})
	copyRanges := widget.NewButtonWithIcon("Copy ranges", theme.ContentCopyIcon(), func() { u.App.Clipboard().SetContent(u.Source.Text); u.Status.SetText("Ranges copied.") })
	ranges := container.NewVBox(
		heading("IP ranges"),
		u.Source, u.Preview,
		container.NewGridWithColumns(2, touchControl(preset), add),
		container.NewGridWithColumns(2, importRanges, copyRanges),
	)
	setup := container.NewVBox(heading("Find the best route"),
		wrapped("Test your proxy, one connection, or reachability without a proxy."),
		widget.NewLabel("Scan mode"), touchControl(u.Mode), u.ConnectionInfo,
		widget.NewButtonWithIcon("Scan settings", theme.SettingsIcon(), func() { u.Tabs.SelectIndex(2) }),
		wrapped("No scan starts automatically. Only test targets you are allowed to probe."),
	)
	var body fyne.CanvasObject
	if u.Mobile {
		body = container.NewVScroll(container.NewPadded(container.NewVBox(
			widget.NewLabel("Scan mode"), touchControl(u.Mode), u.ConnectionInfo, ranges,
		)))
	} else {
		split := container.NewHSplit(container.NewVScroll(container.NewPadded(setup)), container.NewVScroll(container.NewPadded(ranges)))
		split.Offset = .32
		body = split
	}
	// Start/pause/stop and progress never disappear below a scrollable form.
	actions := container.NewPadded(container.NewVBox(widget.NewSeparator(), u.Summary, u.Progress,
		container.NewGridWithColumns(3, u.Start, u.Pause, u.Stop)))
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
	body := container.NewVBox(heading("Your connection"), wrapped("Paste once. Connection details and every advanced field remain editable below."), u.ImportText, parse, container.NewGridWithColumns(2, open, copyConfig), widget.NewSeparator(), u.connectionFields)
	return container.NewVScroll(container.NewPadded(body))
}

func (u *UI) settingsTab() fyne.CanvasObject {
	debug := widget.NewCheck("Debug: save the generated Xray configuration", nil)
	debug.SetChecked(u.Session.Debug)
	debug.OnChanged = func(v bool) { u.Session.Debug = v; u.autoSave() }
	body := container.NewVBox(heading("Full control"), wrapped("Every option from the original configuration is available. Numeric units are shown in the field labels."), u.Optimize, wrapped("Optimization runs independently of an active scan. Stop it with the same button."), u.settingsFields, widget.NewSeparator(), debug, wrapped("Debug output can contain your connection credentials. Keep it private."), widget.NewButtonWithIcon("Save settings", theme.DocumentSaveIcon(), func() {
		if err := u.save(); err != nil {
			u.showError(err)
		} else {
			u.Status.SetText("Settings saved in your application data folder.")
		}
	}))
	return container.NewVScroll(container.NewPadded(body))
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

func (u *UI) updatePreview() {
	if u.Source == nil || u.Preview == nil {
		return
	}
	n, valid := utils.CountRanges(u.Source.Text, u.Session.Config.Scan.SampleSize)
	if max := u.Session.Config.Scan.MaxIPs; max > 0 && n > max {
		n = max
	}
	s := fmt.Sprintf("%s queued IPs · %d worker(s)", comma(n), u.Session.Config.Scan.Threads)
	if strings.TrimSpace(u.Source.Text) == "" {
		s = "Add IPs / CIDRs, or choose a built-in preset."
	} else if !valid {
		s += " · invalid lines will be ignored."
	}
	if u.Mode != nil && u.Mode.Selected == appcore.ModeConnectivity {
		s += " · ping mode: " + utils.PingModeString() + " (ICMP requires privileges)."
	}
	u.Preview.SetText(s)
}

func (u *UI) start() {
	if u.run != nil {
		return
	}
	cfg, err := u.Editor.Snapshot()
	if err != nil {
		u.showError(err)
		return
	}
	start := func() {
		r, err := appcore.NewRun(u.ctx, cfg, u.Source.Text, u.Mode.Selected, u.Session.Debug)
		if err != nil {
			u.showError(err)
			return
		}
		u.run = r
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
	n, _ := utils.CountRanges(u.Source.Text, cfg.Scan.SampleSize)
	if cfg.Scan.MaxIPs > 0 && n > cfg.Scan.MaxIPs {
		n = cfg.Scan.MaxIPs
	}
	if n > 250000 && u.Mode.Selected != appcore.ModeCheck {
		dialog.NewConfirm("Large scan", fmt.Sprintf("This queues %s IPs and can use substantial memory and data. Continue, or reduce IPs per subnet / Maximum IPs in Settings?", comma(n)), func(ok bool) {
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
						u.refreshResults()
						if r.Config.Output.SaveIntermediateResults && time.Since(u.lastSave) > 5*time.Second {
							u.lastSave = time.Now()
							stamp := u.runStamp
							go func() {
								u.saveMu.Lock()
								defer u.saveMu.Unlock()
								if _, err := appcore.SaveResults(u.DataDir, r.Config, r.Results, stamp); err != nil {
									applog.Logf("intermediate save failed: %v", err)
								}
							}()
						}
					}
					lines := applog.Snapshot()
					if len(lines) > 300 {
						lines = lines[len(lines)-300:]
					}
					text := strings.Join(lines, "\n")
					if text != u.lastLog {
						u.lastLog = text
						u.Log.SetText(text)
					}
				})
			}
		}
	}()
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
	top := container.NewVBox(heading("Live results"), wrapped("Quality sorts by packet-loss tier, then latency. Scroll the table sideways to see every metric."),
		container.NewGridWithColumns(2, container.NewVBox(widget.NewLabel("Sort by"), touchControl(u.Sort)), container.NewVBox(widget.NewLabel("Top results · 0 = all"), touchControl(u.Top))), u.GoodOnly)
	bottom := container.NewVBox(container.NewGridWithColumns(3, copySelected, copyAll, details), container.NewGridWithColumns(3, csv, js, copyJSON))
	if u.Mobile {
		bottom = container.NewVBox(container.NewGridWithColumns(2, copySelected, copyAll), details, container.NewGridWithColumns(2, csv, js), copyJSON)
	}
	return container.NewPadded(container.NewBorder(top, bottom, nil, nil, u.Table))
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
	u.Summary.SetText(fmt.Sprintf("%d tested · %d good", u.results.Count(), u.results.SuccessCount()))
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
	history := container.NewBorder(container.NewVBox(heading("Saved scans"), wrapped("Final and intermediate results stay in the application data folder.")), container.NewGridWithColumns(2, open, refresh), nil, nil, u.History)
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
	log := container.NewBorder(container.NewVBox(heading("Run log"), wrapped("Recent 300 events are shown; the full log is saved privately.")), logActions, nil, nil, u.Log)
	tabs := container.NewAppTabs(container.NewTabItem("History", container.NewPadded(history)), container.NewTabItem("Log", container.NewPadded(log)))
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
