package gui

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"piyazche/appcore"
	"piyazche/applog"
	"piyazche/scanner"

	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func (u *UI) connectionTestButton() *widget.Button {
	u.ConnectionTestStatus = wrapped("")
	u.TestConnection = widget.NewButtonWithIcon(T("Test connection"), theme.MediaPlayIcon(), func() {
		if u.run != nil {
			if u.run.Mode == appcore.ModeCheck {
				u.run.Stop()
				u.ConnectionTestStatus.SetText(T("Stopping…"))
			}
			return
		}
		// Test the applied/editor configuration, never silently replace manual edits
		// with stale text in the import box. Apply a newly pasted link first.
		u.startMode(appcore.ModeCheck)
	})
	return u.TestConnection
}

func (u *UI) resetAllowed() bool {
	if u.run != nil || u.optCancel != nil {
		u.showError(errors.New(T("Stop the scan or optimization and wait for it to finish before resetting.")))
		return false
	}
	return true
}

func (u *UI) chooseSettingsReset() {
	if !u.resetAllowed() {
		return
	}
	only := widget.NewButton(T("Settings only"), nil)
	all := widget.NewButton(T("All application data"), nil)
	all.Importance = widget.DangerImportance
	d := dialog.NewCustom(T("Reset"), T("Cancel"), widget.NewForm(widget.NewFormItem("", only), widget.NewFormItem("", all)), u.Window)
	only.OnTapped = func() { d.Hide(); u.confirmReset("settings") }
	all.OnTapped = func() { d.Hide(); u.confirmReset("all") }
	d.Show()
}

func (u *UI) confirmReset(scope string) {
	if !u.resetAllowed() {
		return
	}
	messages := map[string]string{
		"connection": "Reset address, UUID, flow and transport details? Settings, targets and saved results are kept.",
		"scan":       "Reset targets, scan mode, sampling and scan tuning? Connection, fragment settings and saved results are kept.",
		"settings":   "Restore measurement, fragment, output, Xray and appearance defaults? Connection, targets and saved results are kept.",
		"results":    "Clear displayed results and result filters? Saved files, connection and settings are kept.",
		"history":    "Permanently delete application-generated saved scans in the current output folder? Other files are kept.",
		"log":        "Permanently clear the event log and generated debug config? Connection and results are kept.",
		"all":        "Permanently reset connection, settings, targets, results, logs and preferences? Generated scans in the current/default output folders are deleted; manual exports and other output folders are not touched.",
	}
	dialog.ShowConfirm(T("Reset"), T(messages[scope]), func(ok bool) {
		if !ok || !u.resetAllowed() {
			return
		}
		if err := u.resetSection(scope); err != nil {
			u.showError(err)
		}
	}, u.Window)
}

func (u *UI) clearResults() {
	u.results = scanner.NewResultCollector()
	u.lastRun, u.selected = nil, ""
	u.sortCol, u.sortDesc = -1, false
	u.GoodOnly.SetChecked(true)
	u.Sort.SetSelected(T("Quality"))
	u.Top.SetText(strconv.Itoa(appcore.DefaultSession().Top))
	u.Progress.SetValue(0)
	u.Current.SetText("")
	u.refreshResults()
}

func (u *UI) clearLog() error {
	if err := applog.Clear(); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(u.DataDir, "first_config.jsonc")); err != nil && !os.IsNotExist(err) {
		return err
	}
	u.lastLog, u.lastLogGen = "", 0
	u.Log.SetText("")
	return nil
}

func (u *UI) resetSection(scope string) error {
	// Defensive guard also covers delayed confirmation dialogs.
	if u.run != nil || u.optCancel != nil {
		return errors.New(T("Wait for active tasks to finish."))
	}
	switch scope {
	case "connection":
		u.Session.ResetConnection()
		u.ImportText.SetText("")
		u.ConnectionTestStatus.SetText("")
	case "scan":
		u.Session.ResetScan()
		u.Source.SetText("")
		u.Mode.SetSelected(modeLabel(u.Session.Mode))
		u.SourceKind.SetSelected(u.Session.Source)
		u.Progress.SetValue(0)
		u.Current.SetText("")
		u.lastRun = nil
	case "settings":
		u.Session.ResetSettings()
		u.DebugCheck.SetChecked(false)
		u.setTheme(ThemeSystem)
	case "results":
		u.clearResults()
	case "history":
		u.saveMu.Lock()
		u.saveGeneration.Add(1)
		err := appcore.ClearSavedResults(appcore.OutputDirectory(u.DataDir, u.Session.Config))
		u.saveMu.Unlock()
		if err != nil {
			return err
		}
	case "log":
		if err := u.clearLog(); err != nil {
			return err
		}
	case "all":
		u.saveMu.Lock()
		u.saveGeneration.Add(1)
		err := appcore.ClearStoredSession(u.DataDir, u.Session)
		u.saveMu.Unlock()
		if err != nil {
			return err
		}
		if err := u.clearLog(); err != nil {
			return err
		}
		u.Session = appcore.DefaultSession()
		u.ImportText.SetText("")
		u.ConnectionTestStatus.SetText("")
		u.Source.SetText("")
		u.Mode.SetSelected(modeLabel(u.Session.Mode))
		u.SourceKind.SetSelected(u.Session.Source)
		u.DebugCheck.SetChecked(false)
		u.clearResults()
		u.App.Preferences().RemoveValue("lang")
		u.setTheme(ThemeSystem)
	default:
		return errors.New("unknown reset section")
	}
	if scope == "results" || scope == "history" || scope == "log" {
		u.refreshHistory()
		u.Status.SetText(T("Reset complete."))
		u.autoSave()
		return nil
	}
	u.rebuildEditor()
	u.PerRange.SetText(strconv.Itoa(u.Session.Config.Scan.SampleSize))
	u.syncSourceControls()
	u.updateConnectionInfo()
	u.updatePreview()
	if err := u.save(); err != nil {
		return err
	}
	u.refreshHistory()
	u.Status.SetText(T("Reset complete."))
	if scope == "all" || scope == "settings" {
		u.setLanguage(LangEnglish)
	}
	return nil
}
