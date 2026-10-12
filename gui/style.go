package gui

import (
	"image/color"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// themed returns a colour from the active app theme.
func themed(n fyne.ThemeColorName) color.Color {
	s := fyne.CurrentApp().Settings()
	return s.Theme().Color(n, s.ThemeVariant())
}

// Canvas primitives (rectangles, texts) do not follow theme changes on their
// own; they register a repaint callback that runs whenever the theme changes.
var (
	repaintMu sync.Mutex
	repaints  []func()
)

func onTheme(f func()) {
	repaintMu.Lock()
	repaints = append(repaints, f)
	repaintMu.Unlock()
	f()
}

func repaintAll() {
	repaintMu.Lock()
	fs := append([]func(){}, repaints...)
	repaintMu.Unlock()
	for _, f := range fs {
		f()
	}
}

func surface(radius float32) *canvas.Rectangle {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = radius
	bg.StrokeWidth = 1
	onTheme(func() {
		bg.FillColor = themed(colorCard)
		bg.StrokeColor = themed(colorCardStroke)
		bg.Refresh()
	})
	return bg
}

// card wraps content in a soft rounded surface with an optional title and
// one-line description.
func card(title, subtitle string, content ...fyne.CanvasObject) fyne.CanvasObject {
	var objs []fyne.CanvasObject
	if title != "" {
		objs = append(objs, sectionTitle(title))
	}
	if subtitle != "" {
		objs = append(objs, muted(subtitle))
	}
	objs = append(objs, content...)
	inner := container.New(layout.NewCustomPaddedLayout(12, 12, 14, 14), container.NewVBox(objs...))
	return container.NewStack(surface(16), inner)
}

// cardFill is like card but lets the content stretch (tables, lists, logs).
func cardFill(top fyne.CanvasObject, bottom fyne.CanvasObject, content fyne.CanvasObject) fyne.CanvasObject {
	inner := container.New(layout.NewCustomPaddedLayout(12, 12, 14, 14), container.NewBorder(top, bottom, nil, nil, content))
	return container.NewStack(surface(16), inner)
}

func themedText(text string, size float32, bold bool, c fyne.ThemeColorName) *canvas.Text {
	t := canvas.NewText(text, color.Transparent)
	t.TextSize = size
	t.TextStyle = fyne.TextStyle{Bold: bold}
	onTheme(func() { t.Color = themed(c); t.Refresh() })
	return t
}

func sectionTitle(text string) fyne.CanvasObject {
	return themedText(text, 16, true, theme.ColorNameForeground)
}

// muted is a wrapped secondary-coloured description line.
func muted(text string) fyne.CanvasObject {
	l := widget.NewRichText(&widget.TextSegment{Text: text, Style: widget.RichTextStyle{ColorName: colorMuted, Inline: true, SizeName: theme.SizeNameText}})
	l.Wrapping = fyne.TextWrapWord
	return l
}

// fieldLabel is a small bold caption above a control.
func fieldLabel(text string) fyne.CanvasObject {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	l.Wrapping = fyne.TextWrapWord
	return l
}

// labeled stacks a caption and its control.
func labeled(label string, control fyne.CanvasObject) fyne.CanvasObject {
	return container.NewVBox(fieldLabel(label), touchControl(control))
}

// pill renders a small rounded badge.
func pill(text string) fyne.CanvasObject {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = 10
	onTheme(func() { bg.FillColor = themed(colorPill); bg.Refresh() })
	t := themedText(text, 12, true, colorMuted)
	return container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(3, 3, 10, 10), t))
}

// header is the minimal top bar: icon, name, version and the theme toggle.
func header(version string, mobile bool, toggle fyne.CanvasObject) fyne.CanvasObject {
	title := themedText("Scaner", 20, true, theme.ColorNameForeground)
	icon := canvas.NewImageFromResource(Icon)
	icon.SetMinSize(fyne.NewSize(30, 30))
	icon.FillMode = canvas.ImageFillContain
	left := container.NewHBox(icon, container.NewCenter(title), container.NewCenter(pill("v"+version)))
	return container.New(layout.NewCustomPaddedLayout(8, 2, 16, 12), container.NewBorder(nil, nil, left, toggle))
}

// minWidth forces a minimum width on o (used for the desktop side column).
func minWidth(w float32, o fyne.CanvasObject) fyne.CanvasObject {
	floor := canvas.NewRectangle(color.Transparent)
	floor.SetMinSize(fyne.NewSize(w, 0))
	return container.NewStack(floor, o)
}

// gap is vertical breathing room between cards.
func gap(h float32) fyne.CanvasObject {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(0, h))
	return r
}

// page lays cards out in a single smooth vertical scroll with even margins.
func page(objs ...fyne.CanvasObject) fyne.CanvasObject {
	var spaced []fyne.CanvasObject
	for i, o := range objs {
		if i > 0 {
			spaced = append(spaced, gap(6))
		}
		spaced = append(spaced, o)
	}
	return container.NewVScroll(container.New(layout.NewCustomPaddedLayout(8, 16, 14, 14), container.NewVBox(spaced...)))
}
