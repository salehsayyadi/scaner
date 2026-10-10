package gui

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"piyazche/appcore"
	"piyazche/config"
	"piyazche/scanner"
	"piyazche/utils"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

type mobileDevice struct{ fyne.Device }

func (mobileDevice) IsMobile() bool { return true }

type mobileDriver struct{ fyne.Driver }

func (d mobileDriver) Device() fyne.Device { return mobileDevice{d.Driver.Device()} }

type mobileApp struct{ fyne.App }

func (a mobileApp) Driver() fyne.Driver { return mobileDriver{a.App.Driver()} }

func TestEveryConfigOptionHasAControl(t *testing.T) {
	test.NewTempApp(t)
	e := NewEditor(config.DefaultConfig(), nil)
	var leaves []string
	var visit func(reflect.Value, string)
	visit = func(v reflect.Value, path string) {
		if v.Kind() == reflect.Pointer {
			v = v.Elem()
		}
		for i := 0; i < v.NumField(); i++ {
			key := strings.Split(v.Type().Field(i).Tag.Get("json"), ",")[0]
			p := key
			if path != "" {
				p = path + "." + key
			}
			value := v.Field(i)
			if value.Kind() == reflect.Pointer || value.Kind() == reflect.Struct {
				visit(value, p)
			} else {
				leaves = append(leaves, p)
			}
		}
	}
	visit(reflect.ValueOf(e.cfg).Elem(), "")
	for _, p := range leaves {
		if e.controls[p] == nil {
			t.Errorf("option omitted: %s", p)
		}
	}
	if len(e.controls) != len(leaves) {
		t.Fatalf("controls=%d fields=%d", len(e.controls), len(leaves))
	}
	t.Logf("All %d configuration fields have native controls.", len(leaves))
}

func TestMouseKeyboardImportAndPersistence(t *testing.T) {
	a := test.NewTempApp(t)
	w := a.NewWindow("test")
	dir := t.TempDir()
	u := New(a, w, dir, "2.0.3", false)
	defer u.Close()
	threads := u.Editor.controls["scan.threads"].(*widget.Entry)
	threads.TypedShortcut(&fyne.ShortcutSelectAll{})
	test.Type(threads, "8")
	if u.Session.Config.Scan.Threads != 8 {
		t.Fatalf("keyboard edit failed: %s", threads.Text)
	}
	download := u.Editor.controls["scan.downloadTest"].(*widget.Check)
	u.Tabs.SelectIndex(2)
	test.TapAt(download, fyne.NewPos(download.MinSize().Width/2, download.Size().Height/2))
	if !u.Session.Config.Scan.DownloadTest {
		t.Fatal("mouse toggle failed")
	}
	link := "vless://00000000-0000-0000-0000-000000000001@example.invalid:443?security=tls&type=ws&sni=example.invalid&path=%2Fws"
	test.Type(u.ImportText, link)
	test.Tap(u.Apply)
	if u.Session.Config.Proxy.Address != "example.invalid" || u.Session.Config.Scan.Threads != 8 || !u.Session.Config.Scan.DownloadTest {
		t.Fatal("import discarded tuning or failed")
	}
	u.Source.SetText("192.0.2.1")
	s, err := appcore.LoadSession(dir)
	if err != nil || s.Ranges != "192.0.2.1" || s.Config.Scan.Threads != 8 {
		t.Fatal("persistence failed", err)
	}
	u.selected = "192.0.2.1"
	u.copySelected()
	if a.Clipboard().Content() != "192.0.2.1" {
		t.Fatal("clipboard action failed")
	}
	threads = u.Editor.controls["scan.threads"].(*widget.Entry)
	threads.SetText("invalid")
	if _, err := u.Editor.Snapshot(); err == nil {
		t.Fatal("invalid numeric input accepted")
	}
	threads.SetText("4")
	if _, err := u.Editor.Snapshot(); err != nil {
		t.Fatal(err)
	}
}

func TestPresetMergingAndEmbeddedAssets(t *testing.T) {
	if len(Icon.Content()) < 500 {
		t.Fatal("application icon missing")
	}
	for _, name := range []string{"Cloudflare", "CF London", "Fastly"} {
		if Presets[name] == "" {
			t.Fatal("preset missing", name)
		}
	}
	s := mergePreset("104.16.0.0/13", "104.16.0.0/13\n192.0.2.0/24")
	if strings.Count(s, "104.16.0.0/13") != 1 || !strings.Contains(s, "192.0.2.0/24") {
		t.Fatal(s)
	}
}

// Set SCANER_QA_DIR to render the actual native widgets with Fyne's software
// painter. Screenshots are not HTML approximations of the application.
func TestRenderNativeScreens(t *testing.T) {
	out := os.Getenv("SCANER_QA_DIR")
	if out == "" {
		t.Skip("visual captures requested separately")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	for _, mobile := range []bool{false, true} {
		a := test.NewTempApp(t)
		if mobile {
			a = mobileApp{a}
			fyne.SetCurrentApp(a)
		}
		w := a.NewWindow("preview")
		u := New(a, w, t.TempDir(), "2.0.3", mobile)
		prefix := "desktop"
		size := fyne.NewSize(1100, 820)
		if mobile {
			prefix = "android"
			size = fyne.NewSize(390, 844)
		}
		u.ImportText.SetText("vless://00000000-0000-0000-0000-000000000001@example.invalid:443?type=ws&security=tls&sni=example.invalid&path=%2Fws")
		test.Tap(u.Apply)
		u.Source.SetText("192.0.2.0/24\n198.51.100.0/24")
		u.PerRange.SetText("10")
		u.results.Add(scanner.Result{IP: "192.0.2.10", Success: true, Latency: 48 * time.Millisecond, TCPOK: true, TCPMs: 12, HandshakeMs: 67, JitterMs: 3, LossPercent: 0, UploadTested: true, UploadMbps: 8.24, DownloadTested: true, DownloadMbps: 23.50})
		u.results.Add(scanner.Result{IP: "198.51.100.22", Success: true, Latency: 71 * time.Millisecond, TCPOK: true, TCPMs: 15, HandshakeMs: 91, JitterMs: 6, LossPercent: 0, UploadTested: true, UploadMbps: 5.13, DownloadTested: true, DownloadMbps: 18.10})
		u.results.Add(scanner.Result{IP: "192.0.2.99", Success: false, Error: "Connection timed out"})
		u.refreshResults()
		u.Log.SetText("13:48:01 · Scan started · 2 IPs · 1 worker\n13:48:02 · 192.0.2.10 · good · ping 48 ms · loss 0%\n13:48:03 · 198.51.100.22 · good · ping 71 ms · loss 0%")
		capture := func(name string) {
			w.Resize(size)
			img := w.Canvas().Capture()
			if img.Bounds().Dx() != int(size.Width) || img.Bounds().Dy() != int(size.Height) {
				t.Fatalf("%s: canvas expanded to %v", name, img.Bounds())
			}
			f, err := os.Create(filepath.Join(out, prefix+"-"+name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, img)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
		for i, name := range []string{"scan", "connection", "settings", "results", "history"} {
			u.Tabs.SelectIndex(i)
			capture(name)
		}
		u.Tabs.SelectIndex(4)
		u.Tabs.Items[4].Content.(*container.AppTabs).SelectIndex(1)
		capture("log")
		u.Tabs.SelectIndex(2)
		u.settingsFields.Objects[0].(*widget.Accordion).Open(1)
		capture("fragment")
		u.Tabs.SelectIndex(0)
		u.Progress.SetValue(.5)
		u.Status.SetText("Scanning… · 1 / 2 IPs tested")
		u.Start.Disable()
		u.Pause.Enable()
		u.Stop.Enable()
		u.Current.SetText("Now scanning 192.0.2.41 +7 more · range 192.0.2.0/24")
		u.Breakdown.SetText(formatRanges([]appcore.RangeStat{
			{RangeInfo: utils.RangeInfo{Entered: "192.0.2.0/24", Network: "192.0.2.0/24", Hosts: 254, Queued: 10}, Tested: 6, Good: 2},
			{RangeInfo: utils.RangeInfo{Entered: "198.51.100.0/24", Network: "198.51.100.0/24", Hosts: 254, Queued: 10}, Tested: 4, Good: 1},
		}, true, 15))
		capture("running")
		u.Close()
		w.Close()
		t.Log(fmt.Sprintf("%s native visual captures saved.", prefix))
	}
}
