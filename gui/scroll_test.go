package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// The mouse wheel over a single-line field must still scroll the page.
func TestWheelOverEntryScrollsPage(t *testing.T) {
	a := test.NewTempApp(t)
	w := a.NewWindow("scroll")
	var objs []fyne.CanvasObject
	for i := 0; i < 40; i++ {
		c := touchControl(widget.NewEntry()).(*fyne.Container)
		objs = append(objs, c)
	}
	s := page(objs...).(*container.Scroll)
	w.SetContent(s)
	w.Resize(fyne.NewSize(300, 300))
	// Dispatch like the real driver does: at a point on top of the first entry.
	test.Scroll(w.Canvas(), fyne.NewPos(100, 30), 0, -120)
	if s.Offset.Y <= 0 {
		t.Fatalf("page did not scroll: offset=%v", s.Offset)
	}
}
