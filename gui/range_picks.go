package gui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"piyazche/appcore"
	"piyazche/scanner"
	"piyazche/utils"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const rangePicksTabIndex = 5

type pickListRow struct {
	header bool
	group  int
	result scanner.Result
}
type rangePicksView struct {
	Count          *widget.Entry
	Rank           *widget.Select
	Choose         *widget.Button
	Summary, Empty *widget.Label
	List           *widget.List
	Specs          []appcore.RangeSpec
	Groups         []appcore.RangeGroup
	Rows           []pickListRow
	lastCount      int
	lastCollector  *scanner.ResultCollector
	lastRefresh    time.Time
}

func (u *UI) rangePicksTab() fyne.CanvasObject {
	v := &rangePicksView{lastCount: -1}
	u.RangePicks = v
	v.Count = widget.NewEntry()
	v.Count.Validator = func(text string) error {
		n, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil || n < 1 || n > 1000 {
			return errors.New(T("Enter a number from 1 to 1000."))
		}
		return nil
	}
	v.Count.SetText(strconv.Itoa(u.Session.Picks.PerRange))
	v.Count.OnChanged = func(text string) {
		n, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil || n < 1 || n > 1000 {
			v.Count.SetValidationError(errors.New(T("Enter a number from 1 to 1000.")))
			return
		}
		v.Count.SetValidationError(nil)
		u.Session.Picks.PerRange = n
		u.refreshRangePicks(true)
		u.autoSave()
	}
	labels := []string{T(appcore.RankQuality), T(appcore.RankPing), T(appcore.RankDownload), T(appcore.RankUpload)}
	v.Rank = widget.NewSelect(labels, nil)
	v.Rank.SetSelected(T(u.Session.Picks.Rank))
	v.Rank.OnChanged = func(label string) {
		for _, key := range []string{appcore.RankQuality, appcore.RankPing, appcore.RankDownload, appcore.RankUpload} {
			if T(key) == label {
				u.Session.Picks.Rank = key
				break
			}
		}
		u.refreshRangePicks(true)
		u.autoSave()
	}
	v.Choose = widget.NewButtonWithIcon(T("Select ranges"), theme.ListIcon(), u.choosePickRanges)
	v.Summary = wrapped("")
	v.Empty = wrapped("")
	v.List = widget.NewList(func() int { return len(v.Rows) }, func() fyne.CanvasObject { return newPickRowWidget(u.Mobile) }, func(i widget.ListItemID, o fyne.CanvasObject) {
		if i < 0 || i >= len(v.Rows) {
			return
		}
		item := o.(*pickRowWidget)
		row := v.Rows[i]
		group := v.Groups[row.group]
		if row.header {
			item.body.Hide()
			item.header.Show()
			item.groupTitle.SetText(group.Label)
			item.groupCount.SetText(Tf("%d shown / %d eligible", len(group.Rows), group.Eligible))
			item.copyGroup.OnTapped = func() { u.copyPicks([]appcore.RangeGroup{group}, false) }
			if len(group.Rows) == 0 {
				item.copyGroup.Disable()
			} else {
				item.copyGroup.Enable()
			}
		} else {
			item.header.Hide()
			item.body.Show()
			r := row.result
			item.ip.SetText(r.IP)
			up, down := "—", "—"
			if r.UploadTested {
				up = fmt.Sprintf("%.2f Mbps", r.UploadMbps)
			}
			if r.DownloadTested {
				down = fmt.Sprintf("%.2f Mbps", r.DownloadMbps)
			}
			values := []string{fmt.Sprintf("%d ms", r.LatencyMs), down, up, fmt.Sprintf("%.1f%%", r.LossPercent)}
			for j, value := range values {
				item.metrics[j].SetText(value)
			}
			item.copyIP.OnTapped = func() { u.App.Clipboard().SetContent(r.IP); u.Status.SetText(T("IP copied: ") + r.IP) }
			item.details.OnTapped = func() {
				b, _ := json.MarshalIndent(r, "", "  ")
				e := widget.NewMultiLineEntry()
				e.SetText(string(b))
				e.SetMinRowsVisible(10)
				d := dialog.NewCustom(T("All result metrics"), T("Close"), e, u.Window)
				size := fyne.NewSize(650, 470)
				if u.Mobile {
					size = fyne.NewSize(340, 530)
				}
				d.Resize(size)
				d.Show()
			}
		}
	})
	v.List.HideSeparators = true
	choose := labeled("Ranges to display", v.Choose)
	tuning := container.NewGridWithColumns(2, labeled("IPs from each range", v.Count), labeled("Rank within each range", v.Rank))
	controls := fyne.CanvasObject(container.NewGridWithColumns(2, choose, tuning))
	if u.Mobile {
		controls = container.NewVBox(choose, tuning)
	}
	top := card("Range picks", "Select ranges; each group gets its own best IPs.", controls, muted("Only successful results are eligible. Speed ranking requires a measured speed; untested is not zero."), v.Empty)
	copyIPs := widget.NewButtonWithIcon(T("Copy shown IPs"), theme.ContentCopyIcon(), func() { u.copyPicks(v.Groups, false) })
	copyGroups := widget.NewButtonWithIcon(T("Copy grouped"), theme.ContentCopyIcon(), func() { u.copyPicks(v.Groups, true) })
	csv := widget.NewButtonWithIcon(T("Export CSV"), theme.DocumentSaveIcon(), func() { u.exportPicks("csv") })
	js := widget.NewButtonWithIcon(T("Export JSON"), theme.DocumentSaveIcon(), func() { u.exportPicks("json") })
	reset := widget.NewButtonWithIcon(T("Reset range picks"), theme.ViewRefreshIcon(), func() { u.confirmReset("range-picks") })
	buttons := container.NewGridWithColumns(4, copyIPs, copyGroups, csv, js)
	if u.Mobile {
		buttons = container.NewGridWithColumns(2, copyIPs, copyGroups, csv, js)
	}
	bottom := card("", "", container.NewBorder(nil, nil, nil, reset, v.Summary), buttons)
	if u.Mobile {
		filters := widget.NewButtonWithIcon(T("Range picks"), theme.SettingsIcon(), func() {
			d := dialog.NewCustom(T("Range picks"), T("Close"), top, u.Window)
			d.Resize(fyne.NewSize(350, 570))
			d.Show()
		})
		more := widget.NewButtonWithIcon(T("More actions"), theme.MoreHorizontalIcon(), func() {
			d := dialog.NewCustom(T("More actions"), T("Close"), container.NewVBox(copyGroups, csv, js, reset), u.Window)
			d.Resize(fyne.NewSize(340, 350))
			d.Show()
		})
		topMobile := container.NewVBox(filters, v.Summary)
		bottomMobile := container.NewGridWithColumns(2, copyIPs, more)
		return container.NewPadded(container.NewBorder(topMobile, bottomMobile, nil, nil, v.List))
	}
	return container.NewPadded(container.NewBorder(top, bottom, nil, nil, v.List))
}

type pickRowWidget struct {
	widget.BaseWidget
	header, body               *fyne.Container
	groupTitle, groupCount, ip *widget.Label
	metrics                    []*widget.Label
	copyGroup, copyIP, details *widget.Button
}

func newPickRowWidget(mobile bool) *pickRowWidget {
	p := &pickRowWidget{}
	p.groupTitle = wrapped("")
	p.groupTitle.TextStyle.Bold = true
	p.groupCount = wrapped("")
	p.copyGroup = widget.NewButtonWithIcon(T("Copy"), theme.ContentCopyIcon(), nil)
	p.header = container.NewVBox(widget.NewSeparator(), container.NewBorder(nil, nil, nil, p.copyGroup, container.NewVBox(p.groupTitle, p.groupCount)))
	p.ip = wrapped("")
	p.ip.TextStyle.Bold = true
	p.copyIP = widget.NewButtonWithIcon(T("Copy IP"), theme.ContentCopyIcon(), nil)
	p.details = widget.NewButtonWithIcon("", theme.InfoIcon(), nil)
	var fields []fyne.CanvasObject
	for _, label := range []string{"Ping ms", "Down Mbps", "Up Mbps", "Loss %"} {
		value := wrapped("")
		p.metrics = append(p.metrics, value)
		fields = append(fields, container.NewVBox(fieldLabel(label), value))
	}
	columns := 4
	if mobile {
		columns = 2
	}
	p.body = container.NewPadded(container.NewVBox(container.NewBorder(nil, nil, nil, container.NewHBox(p.copyIP, p.details), p.ip), container.NewGridWithColumns(columns, fields...)))
	p.ExtendBaseWidget(p)
	return p
}
func (p *pickRowWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewStack(p.header, p.body))
}

func (u *UI) pickSpecs(rows []scanner.Result) []appcore.RangeSpec {
	var infos []utils.RangeInfo
	if u.lastRun != nil {
		for _, stat := range u.lastRun.Snapshot(0).Ranges {
			infos = append(infos, stat.RangeInfo)
		}
	} else {
		stored := false
		for _, row := range rows {
			if row.Range != "" {
				stored = true
				break
			}
		}
		if !stored {
			infos, _, _ = utils.PlanRanges(u.Source.Text, u.Session.Config.Scan.SampleSize, u.Session.Source == appcore.SourceIPList)
		}
	}
	return appcore.PickRangeSpecs(rows, infos)
}
func (u *UI) refreshRangePicks(force bool) {
	v := u.RangePicks
	if v == nil || u.results == nil {
		return
	}
	n := u.results.Count()
	if !force && v.lastCollector == u.results && (v.lastCount == n || time.Since(v.lastRefresh) < 600*time.Millisecond) {
		return
	}
	rows := u.results.GetResults()
	v.Specs = u.pickSpecs(rows)
	if !u.Session.Picks.Initialized && len(v.Specs) > 0 {
		u.Session.Picks.Selected = nil
		for _, spec := range v.Specs {
			u.Session.Picks.Selected = append(u.Session.Picks.Selected, spec.Key)
		}
		u.Session.Picks.Initialized = true
	}
	prefs := u.Session.Picks
	prefs.Normalize()
	// Synchronize controls after a full reset without firing filter callbacks.
	if v.Count.Text != strconv.Itoa(prefs.PerRange) {
		cb := v.Count.OnChanged
		v.Count.OnChanged = nil
		v.Count.SetText(strconv.Itoa(prefs.PerRange))
		v.Count.OnChanged = cb
	}
	if v.Rank.Selected != T(prefs.Rank) {
		cb := v.Rank.OnChanged
		v.Rank.OnChanged = nil
		v.Rank.SetSelected(T(prefs.Rank))
		v.Rank.OnChanged = cb
	}
	v.Groups = appcore.BuildRangePicks(rows, v.Specs, prefs)
	v.Rows = nil
	total := 0
	for i, group := range v.Groups {
		v.Rows = append(v.Rows, pickListRow{header: true, group: i})
		for _, row := range group.Rows {
			v.Rows = append(v.Rows, pickListRow{group: i, result: row})
			total++
		}
	}
	v.Choose.SetText(Tf("Select ranges · %d / %d", len(v.Groups), len(v.Specs)))
	v.Summary.SetText(Tf("%d ranges · %d shown IPs", len(v.Groups), total))
	message := ""
	switch {
	case len(rows) == 0:
		message = T("Run a scan or open saved results to populate this view.")
	case len(v.Groups) == 0:
		message = T("No ranges selected. Choose ranges to display.")
	case total == 0:
		message = T("No eligible results for these filters. Speed sorting needs upload/download tests.")
	}
	v.Empty.SetText(message)
	if message == "" {
		v.Empty.Hide()
	} else {
		v.Empty.Show()
	}
	v.List.Refresh()
	for i, row := range v.Rows {
		height := float32(148)
		if u.Mobile {
			height = 220
		}
		if row.header {
			height = 84
			if u.Mobile {
				height = 100
			}
		}
		v.List.SetItemHeight(i, height)
	}
	v.lastCount, v.lastCollector, v.lastRefresh = n, u.results, time.Now()
}
func (u *UI) choosePickRanges() {
	u.refreshRangePicks(true)
	v := u.RangePicks
	if len(v.Specs) == 0 {
		u.showError(errors.New(T("Add ranges or load results first.")))
		return
	}
	selected := map[string]bool{}
	for _, key := range u.Session.Picks.Selected {
		selected[key] = true
	}
	search := widget.NewEntry()
	search.SetPlaceHolder(T("Search ranges"))
	checks := widget.NewCheckGroup(nil, nil)
	checks.Horizontal = false
	var visible []string
	refresh := func(text string) {
		visible = nil
		for _, spec := range v.Specs {
			if strings.Contains(strings.ToLower(spec.Key+" "+spec.Label), strings.ToLower(strings.TrimSpace(text))) {
				visible = append(visible, spec.Key)
			}
		}
		checks.OnChanged = nil
		checks.Options = visible
		checks.Refresh()
		var values []string
		for _, key := range visible {
			if selected[key] {
				values = append(values, key)
			}
		}
		checks.SetSelected(values)
		checks.OnChanged = func(values []string) {
			for _, key := range visible {
				delete(selected, key)
			}
			for _, key := range values {
				selected[key] = true
			}
		}
	}
	search.OnChanged = refresh
	refresh("")
	all := widget.NewButton(T("Select all ranges"), func() {
		for _, spec := range v.Specs {
			selected[spec.Key] = true
		}
		refresh(search.Text)
	})
	none := widget.NewButton(T("Select none"), func() { selected = map[string]bool{}; refresh(search.Text) })
	scroll := container.NewVScroll(checks)
	scroll.SetMinSize(fyne.NewSize(300, 320))
	content := container.NewBorder(container.NewVBox(search, container.NewGridWithColumns(2, all, none)), nil, nil, nil, scroll)
	d := dialog.NewCustomConfirm(T("Select ranges"), T("Apply"), T("Cancel"), content, func(ok bool) {
		if !ok {
			return
		}
		u.Session.Picks.Selected = nil
		for _, spec := range v.Specs {
			if selected[spec.Key] {
				u.Session.Picks.Selected = append(u.Session.Picks.Selected, spec.Key)
			}
		}
		u.Session.Picks.Initialized = true
		u.refreshRangePicks(true)
		u.autoSave()
	}, u.Window)
	size := fyne.NewSize(590, 540)
	if u.Mobile {
		size = fyne.NewSize(345, 510)
	}
	d.Resize(size)
	d.Show()
}
func (u *UI) copyPicks(groups []appcore.RangeGroup, grouped bool) {
	text := appcore.RangePicksText(groups, grouped)
	if text == "" {
		u.showError(errors.New(T("No shown IPs to copy or export.")))
		return
	}
	u.App.Clipboard().SetContent(text)
	u.Status.SetText(T("Shown range picks copied."))
}
func (u *UI) exportPicks(format string) {
	rows := appcore.FlattenRangePicks(u.RangePicks.Groups)
	if len(rows) == 0 {
		u.showError(errors.New(T("No shown IPs to copy or export.")))
		return
	}
	// Freeze the exact displayed selection before the asynchronous save dialog.
	write := func(w io.Writer) error { return appcore.EncodeResults(w, rows, format) }
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
	d.SetFileName(time.Now().Format("2006-01-02_150405") + "_range-picks." + format)
	d.SetFilter(storage.NewExtensionFileFilter([]string{"." + format}))
	d.Show()
}
