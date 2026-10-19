package gui

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"piyazche/netroute"
	"strings"
)

func (u *UI) networkSettings() fyne.CanvasObject {
	u.NetworkSelect = widget.NewSelect(nil, nil)
	u.NetworkInfo = wrapped("")
	u.NetworkSelect.OnChanged = func(label string) {
		name, ok := u.networkNames[label]
		if !ok {
			return
		}
		if u.run != nil || u.optCancel != nil {
			u.refreshNetworkChoices()
			u.Status.SetText(T("Stop active tests before changing the test network."))
			return
		}
		u.Session.Config.Network.Interface = name
		u.updateNetworkInfo()
		u.updateConnectionInfo()
		u.updatePreview()
		u.autoSave()
	}
	refresh := widget.NewButtonWithIcon(T("Refresh network adapters"), theme.ViewRefreshIcon(), u.refreshNetworkChoices)
	u.refreshNetworkChoices()
	return card("Test internet connection", "Choose the adapter used by test traffic, not the VPN route. Automatic follows system routing.", labeled("Network adapter", u.NetworkSelect), refresh, u.NetworkInfo, muted("A VPN kill switch or forced interception can still block this path. This does not disable the VPN or change system routes. DNS uses system DNS servers on the selected adapter; VPN-only DNS may fail."))
}
func (u *UI) refreshNetworkChoices() {
	if u.NetworkSelect == nil {
		return
	}
	automatic := T("Automatic (system route)")
	options := []string{automatic}
	u.networkNames = map[string]string{automatic: ""}
	selected := automatic
	adapters, err := netroute.Interfaces()
	for _, adapter := range adapters {
		name := []rune(adapter.Name)
		if len(name) > 26 {
			name = append(name[:25], '…')
		}
		label := fmt.Sprintf("%s · #%d", string(name), adapter.Index)
		options = append(options, label)
		u.networkNames[label] = adapter.Name
		if adapter.Name == u.Session.Config.Network.Interface {
			selected = label
		}
	}
	if name := u.Session.Config.Network.Interface; name != "" && selected == automatic {
		selected = T("Unavailable adapter") + " · " + name
		options = append(options, selected)
		u.networkNames[selected] = name
	}
	cb := u.NetworkSelect.OnChanged
	u.NetworkSelect.OnChanged = nil
	u.NetworkSelect.Options = options
	u.NetworkSelect.SetSelected(selected)
	u.NetworkSelect.Refresh()
	u.NetworkSelect.OnChanged = cb
	if !netroute.Supported() && u.Session.Config.Network.Interface == "" {
		u.NetworkSelect.Disable()
	} else {
		u.NetworkSelect.Enable()
	}
	u.updateNetworkInfo()
	if err != nil {
		u.NetworkInfo.SetText(T("Cannot list network adapters: ") + err.Error())
	}
}
func (u *UI) updateNetworkInfo() {
	if u.NetworkInfo == nil {
		return
	}
	name := u.Session.Config.Network.Interface
	if !netroute.Supported() {
		u.NetworkInfo.SetText(T("Manual adapter binding is available on Windows and desktop Linux. Android uses the system route."))
		return
	}
	if name == "" {
		u.NetworkInfo.SetText(T("Automatic: tests follow the system route, which may be a VPN. Explicit adapter mode never silently falls back to Automatic."))
		return
	}
	detail := name
	if adapters, err := netroute.Interfaces(); err == nil {
		for _, adapter := range adapters {
			if adapter.Name == name {
				detail = name + "\n" + strings.Join(adapter.Addresses, ", ")
				break
			}
		}
	}
	u.NetworkInfo.SetText(T("Selected test adapter: ") + detail + T(" · applies to the next scan, connection test and fragment optimization. Connectivity mode uses TCP on this adapter."))
}
