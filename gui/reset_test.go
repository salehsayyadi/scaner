package gui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"piyazche/appcore"
)

func TestNativeSectionResetsAndPersistence(t *testing.T) {
	a := test.NewTempApp(t)
	w := a.NewWindow("test")
	dir := t.TempDir()
	u := New(a, w, dir, "4.5.4", false)
	defer u.Close()
	u.ImportText.SetText("vless://11111111-1111-1111-1111-111111111111@origin.example:443?type=ws&security=tls&sni=origin.example&flow=xtls-rprx-vision")
	test.Tap(u.Apply)
	if u.TestConnection == nil || u.ConnectionTestStatus == nil {
		t.Fatal("connection test controls missing")
	}
	u.Editor.controls["scan.timeout"].(*widget.Entry).SetText("45")
	u.Source.SetText("192.0.2.4")
	if err := u.resetSection("connection"); err != nil {
		t.Fatal(err)
	}
	if u.Session.Config.Proxy.UUID != "" || u.Session.Config.Scan.Timeout != 45 || u.Source.Text != "192.0.2.4" {
		t.Fatal("connection reset modified scan data")
	}
	u.Session.Config.Proxy.UUID = "keep"
	u.Session.Config.Fragment.Manual.Length = "66-77"
	if err := u.resetSection("scan"); err != nil {
		t.Fatal(err)
	}
	if u.Source.Text != "" || u.Session.Config.Scan.Timeout != 10 || u.Session.Config.Proxy.UUID != "keep" || u.Session.Config.Fragment.Manual.Length != "66-77" {
		t.Fatal("scan reset modified unrelated data")
	}
	u.Source.SetText("192.0.2.8")
	u.DebugCheck.SetChecked(true)
	if err := u.resetSection("settings"); err != nil {
		t.Fatal(err)
	}
	if u.Session.Config.Proxy.UUID != "keep" || u.Source.Text != "192.0.2.8" || u.DebugCheck.Checked || u.Session.Config.Fragment.Enabled {
		t.Fatal("settings reset modified connection/targets")
	}
	stored, err := appcore.LoadSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Config.Proxy.UUID != "keep" || stored.Ranges != "192.0.2.8" {
		t.Fatal("reset not persisted")
	}
	// A reset must not race an active scan/optimizer, even if called directly.
	_, cancel := context.WithCancel(context.Background())
	u.optCancel = cancel
	if err := u.resetSection("all"); err == nil {
		t.Fatal("reset while optimizer active was allowed")
	}
	cancel()
	u.optCancel = nil
	if err := os.MkdirAll(filepath.Join(dir, "results"), 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "results", "2026-10-19_120000.123_results.json")
	if err := os.WriteFile(file, []byte("[]"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := u.resetSection("all"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("saved scan remains after full reset")
	}
	stored, err = appcore.LoadSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Config.Proxy.UUID != "" || stored.Ranges != "" || stored.Config.Scan.Timeout != 10 {
		t.Fatal("full reset did not persist defaults")
	}
}

func TestNewShareLinkDoesNotInheritConnectionModifiers(t *testing.T) {
	a := test.NewTempApp(t)
	w := a.NewWindow("test")
	u := New(a, w, t.TempDir(), "4.5.4", false)
	defer u.Close()
	u.Session.Config.Fragment.Enabled, u.Session.Config.Fragment.Mode = true, "manual"
	u.Session.Config.Xray.Mux.Enabled = true
	u.ImportText.SetText("vless://test@origin.example:443?type=ws&security=tls&sni=origin.example")
	test.Tap(u.Apply)
	if u.Session.Config.Fragment.Enabled || u.Session.Config.Xray.Mux.Enabled {
		t.Fatal("share link acquired saved fragment or mux")
	}
}
