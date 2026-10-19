package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestNetworkSettingsSelectionAndReset(t *testing.T) {
	a := test.NewTempApp(t)
	w := a.NewWindow("network")
	u := New(a, w, t.TempDir(), "5.6.4", false)
	defer u.Close()
	if u.NetworkSelect == nil || u.NetworkInfo == nil {
		t.Fatal("network setting missing")
	}
	u.Session.Config.Network.Interface = "missing-adapter"
	u.refreshNetworkChoices()
	if u.networkNames[u.NetworkSelect.Selected] != "missing-adapter" {
		t.Fatal("unavailable selection silently changed")
	}
	u.NetworkSelect.SetSelected(T("Automatic (system route)"))
	if u.Session.Config.Network.Interface != "" {
		t.Fatal("automatic not applied")
	}
	u.Session.Config.Network.Interface = "missing-adapter"
	u.resetSection("settings")
	if u.Session.Config.Network.Interface != "" || u.NetworkSelect.Selected != T("Automatic (system route)") {
		t.Fatal("network reset not reflected")
	}
}
func TestRender564NetworkSettings(t *testing.T) {
	out := os.Getenv("SCANER_QA_DIR")
	if out == "" {
		t.Skip("optional visual QA")
	}
	os.MkdirAll(out, 0755)
	for _, mobile := range []bool{false, true} {
		a := test.NewTempApp(t)
		a.Preferences().SetString("lang", LangPersian)
		if mobile {
			a = mobileApp{a}
			fyne.SetCurrentApp(a)
		}
		w := a.NewWindow("5.6.4")
		u := New(a, w, t.TempDir(), "5.6.4", mobile)
		u.Tabs.SelectIndex(2)
		size, name := fyne.NewSize(1100, 900), "desktop"
		if mobile {
			size, name = fyne.NewSize(390, 844), "mobile"
		}
		w.Resize(size)
		img := w.Canvas().Capture()
		if img.Bounds().Dx() != int(size.Width) || img.Bounds().Dy() != int(size.Height) {
			t.Fatal("network settings overflow", img.Bounds())
		}
		f, e := os.Create(filepath.Join(out, name+"-settings.png"))
		if e != nil {
			t.Fatal(e)
		}
		png.Encode(f, img)
		f.Close()
		u.Close()
		w.Close()
	}
	lang = LangEnglish
}
