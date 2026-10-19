package gui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestRender454ConnectionAndReset(t *testing.T) {
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
			w := a.NewWindow("Scaner 4.5.4")
			u := New(a, w, t.TempDir(), "4.5.4", mobile)
			prefix := "desktop-" + language
			size := fyne.NewSize(1100, 820)
			if mobile {
				prefix = "mobile-" + language
				size = fyne.NewSize(390, 844)
			}
			u.ImportText.SetText("vless://11111111-1111-1111-1111-111111111111@origin.example:443?type=ws&security=tls&sni=origin.example")
			test.Tap(u.Apply)
			for _, tab := range []int{1, 2} {
				u.Tabs.SelectIndex(tab)
				w.Resize(size)
				img := w.Canvas().Capture()
				if img.Bounds().Dx() != int(size.Width) || img.Bounds().Dy() != int(size.Height) {
					t.Fatalf("%s expanded: %v", prefix, img.Bounds())
				}
				name := map[int]string{1: "connection", 2: "settings"}[tab]
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
			u.Close()
			w.Close()
		}
	}
	lang = LangEnglish
}
