package gui

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"piyazche/appcore"
	"piyazche/scanner"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func seedRangePicks(u *UI) {
	for i := 0; i < 40; i++ {
		for n := 1; n <= 5; n++ {
			u.results.Add(scanner.Result{IP: fmt.Sprintf("198.18.%d.%d", i, n), Range: fmt.Sprintf("198.18.%d.0/24", i), Success: true, Latency: time.Duration(20+n*9) * time.Millisecond, UploadTested: true, UploadMbps: float64(5 + n), DownloadTested: true, DownloadMbps: float64(50 - n*3), LossPercent: float64(n)})
		}
	}
	u.Session.Picks = appcore.PickPreferences{Selected: []string{"198.18.0.0/24", "198.18.12.0/24", "198.18.39.0/24"}, PerRange: 2, Rank: appcore.RankPing, Initialized: true}
	u.refreshRangePicks(true)
}
func TestRangePicksControlsCopyAndReset(t *testing.T) {
	a := test.NewTempApp(t)
	w := a.NewWindow("test")
	u := New(a, w, t.TempDir(), "4.6.4", false)
	defer u.Close()
	seedRangePicks(u)
	if len(u.RangePicks.Specs) != 40 || len(u.RangePicks.Groups) != 3 || len(u.RangePicks.Rows) != 9 {
		t.Fatal("grouped view has wrong rows")
	}
	u.copyPicks(u.RangePicks.Groups, false)
	if len(strings.Split(a.Clipboard().Content(), "\n")) != 6 {
		t.Fatal("copy does not match shown six IPs")
	}
	u.RangePicks.Count.SetText("1")
	if len(appcore.FlattenRangePicks(u.RangePicks.Groups)) != 3 {
		t.Fatal("per-range limit ignored")
	}
	u.RangePicks.Rank.SetSelected(T(appcore.RankDownload))
	if !strings.HasSuffix(u.RangePicks.Groups[0].Rows[0].IP, ".1") {
		t.Fatal("download ranking not descending")
	}
	u.RangePicks.Count.SetText("0")
	if u.RangePicks.Count.Validate() == nil {
		t.Fatal("zero limit accepted")
	}
	u.RangePicks.Count.SetText("2")
	before := u.results.Count()
	if err := u.resetSection("range-picks"); err != nil {
		t.Fatal(err)
	}
	if u.results.Count() != before || u.Session.Picks.PerRange != 2 || u.Session.Picks.Rank != appcore.RankQuality {
		t.Fatal("reset deleted scan data or failed")
	}
}
func TestRender464RangePicks(t *testing.T) {
	out := os.Getenv("SCANER_QA_DIR")
	if out == "" {
		t.Skip("optional visual QA")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{LangEnglish, LangPersian} {
		for _, mobile := range []bool{false, true} {
			a := test.NewTempApp(t)
			a.Preferences().SetString("lang", language)
			if mobile {
				a = mobileApp{a}
				fyne.SetCurrentApp(a)
			}
			w := a.NewWindow("Scaner 4.6.4")
			u := New(a, w, t.TempDir(), "4.6.4", mobile)
			seedRangePicks(u)
			u.Tabs.SelectIndex(rangePicksTabIndex)
			size := fyne.NewSize(1100, 900)
			prefix := "desktop-" + language
			if mobile {
				size = fyne.NewSize(390, 844)
				prefix = "mobile-" + language
			}
			capture := func(name string) {
				w.Resize(size)
				img := w.Canvas().Capture()
				if img.Bounds().Dx() != int(size.Width) || img.Bounds().Dy() != int(size.Height) {
					t.Fatalf("%s overflow: %v", prefix, img.Bounds())
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
			capture("ranges")
			if language == LangPersian {
				u.choosePickRanges()
				capture("selector")
			}
			u.Close()
			w.Close()
		}
	}
	lang = LangEnglish
}
