package gui

import (
	"strings"
	"testing"

	"piyazche/appcore"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestScanTargetsControls(t *testing.T) {
	a := test.NewTempApp(t)
	w := a.NewWindow("targets")
	u := New(a, w, t.TempDir(), "test", false)
	defer u.Close()

	u.Source.SetText("185.133.35.0/16\n208.86.168.0/16\n212.183.88.0/16")
	if !strings.Contains(u.Breakdown.Text, "185.133.35.0/16 (= 185.133.0.0/16)") || !strings.Contains(u.Breakdown.Text, "65,536 IPs in range · 100 queued") {
		t.Fatalf("breakdown=%q", u.Breakdown.Text)
	}
	if !strings.Contains(u.Preview.Text, "300 IPs will be scanned from 3 range(s)") {
		t.Fatalf("preview=%q", u.Preview.Text)
	}

	// The Scan tab field and the Settings field edit the same value.
	u.PerRange.SetText("0")
	if u.Session.Config.Scan.SampleSize != 0 {
		t.Fatalf("sampleSize=%d", u.Session.Config.Scan.SampleSize)
	}
	if c := u.Editor.controls["scan.sampleSize"].(*widget.Entry); c.Text != "0" {
		t.Fatalf("settings field=%q", c.Text)
	}
	if !strings.Contains(u.Preview.Text, "196,608 IPs will be scanned") {
		t.Fatalf("preview=%q", u.Preview.Text)
	}
	u.Editor.controls["scan.sampleSize"].(*widget.Entry).SetText("25")
	if u.PerRange.Text != "25" || !strings.Contains(u.Breakdown.Text, "25 queued") {
		t.Fatalf("perRange=%q breakdown=%q", u.PerRange.Text, u.Breakdown.Text)
	}

	u.SourceKind.SetSelected(appcore.SourceIPList)
	if !u.PerRange.Disabled() || u.Session.Source != appcore.SourceIPList {
		t.Fatal("IP list must ignore IPs per range")
	}
	u.Source.SetText("1.1.1.1\n8.8.8.8")
	if !strings.Contains(u.Preview.Text, "2 IPs will be scanned") || !strings.Contains(u.Breakdown.Text, "Individual IPs") {
		t.Fatalf("preview=%q breakdown=%q", u.Preview.Text, u.Breakdown.Text)
	}

	// The misleading "Check connection" mode is gone; only two clear modes remain.
	if len(u.Mode.Options) != 2 || u.mode() != appcore.ModeXray {
		t.Fatalf("modes=%v", u.Mode.Options)
	}
	u.Mode.SetSelected(modeLabel(appcore.ModeConnectivity))
	if u.Session.Mode != appcore.ModeConnectivity {
		t.Fatalf("mode=%q", u.Session.Mode)
	}
	u.Mode.SetSelected(modeLabel(appcore.ModeXray))

	s, err := appcore.LoadSession(u.DataDir)
	if err != nil || s.Source != appcore.SourceIPList || s.Config.Scan.SampleSize != 25 {
		t.Fatalf("session=%+v err=%v", s, err)
	}
}

func TestFormatRangesLive(t *testing.T) {
	text := formatRanges([]appcore.RangeStat{{Tested: 3, Good: 1}}, true, 0)
	if !strings.Contains(text, "3 tested · 1 good") {
		t.Fatal(text)
	}
}
