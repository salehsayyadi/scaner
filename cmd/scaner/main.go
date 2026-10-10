package main

import (
	"os"
	"path/filepath"

	"piyazche/applog"
	"piyazche/gui"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

var Version = "2.0.3"

func main() {
	a := app.NewWithID("com.piyazche.scaner")
	w := a.NewWindow("Scaner " + Version)
	// App-private storage on Android, per-user app data on Windows. The program
	// directory needs no writable config, DLL, separate Xray binary, or IP file.
	dir := a.Storage().RootURI().Path()
	mobile := fyne.CurrentDevice().IsMobile()
	if !mobile {
		base, err := os.UserConfigDir()
		if err != nil {
			panic(err)
		}
		dir = filepath.Join(base, "Scaner")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		panic(err)
	}
	if err := os.Chdir(dir); err != nil {
		panic(err)
	}
	applog.Init(filepath.Join(dir, "piyazche.log"))
	defer applog.Close()
	u := gui.New(a, w, dir, Version, mobile)
	u.StartRefresh()
	w.SetOnClosed(u.Close)
	w.ShowAndRun()
}
