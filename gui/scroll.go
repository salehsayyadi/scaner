package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// scrollRelay sits on top of a single-line entry. Fyne entries swallow mouse
// wheel events (they scroll their own text sideways), so with the pointer on
// a field the page would not scroll. The relay only implements Scrollable:
// taps, typing, hover and the text cursor still reach the entry, while the
// wheel is forwarded to the nearest vertical page scroll.
type scrollRelay struct {
	widget.BaseWidget
}

func newScrollRelay() *scrollRelay {
	r := &scrollRelay{}
	r.ExtendBaseWidget(r)
	return r
}

func (r *scrollRelay) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}

func (r *scrollRelay) MinSize() fyne.Size { return fyne.NewSize(0, 0) }

func (r *scrollRelay) Scrolled(ev *fyne.ScrollEvent) {
	c := fyne.CurrentApp().Driver().CanvasForObject(r)
	if c == nil {
		return
	}
	if s := enclosingScroll(c.Content(), r); s != nil {
		s.Scrolled(ev)
	}
}

// enclosingScroll returns the innermost vertical scroll container that holds target.
func enclosingScroll(root, target fyne.CanvasObject) *container.Scroll {
	var found *container.Scroll
	var walk func(o fyne.CanvasObject, nearest *container.Scroll) bool
	walk = func(o fyne.CanvasObject, nearest *container.Scroll) bool {
		if o == nil || !o.Visible() {
			return false
		}
		if o == target {
			found = nearest
			return true
		}
		switch v := o.(type) {
		case *container.Scroll:
			n := nearest
			if v.Direction != container.ScrollHorizontalOnly && v.Direction != container.ScrollNone {
				n = v
			}
			return walk(v.Content, n)
		case *fyne.Container:
			for _, c := range v.Objects {
				if walk(c, nearest) {
					return true
				}
			}
		case *container.AppTabs:
			if t := v.Selected(); t != nil {
				return walk(t.Content, nearest)
			}
		case *container.Split:
			return walk(v.Leading, nearest) || walk(v.Trailing, nearest)
		case *widget.Accordion:
			for _, it := range v.Items {
				if it.Open && walk(it.Detail, nearest) {
					return true
				}
			}
		}
		return false
	}
	walk(root, nil)
	return found
}

// withScrollRelay overlays a relay on single-line entries.
func withScrollRelay(o fyne.CanvasObject) fyne.CanvasObject {
	if e, ok := o.(*widget.Entry); ok && !e.MultiLine {
		return container.NewStack(o, newScrollRelay())
	}
	return o
}
