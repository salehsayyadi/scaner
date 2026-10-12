package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// themed returns a colour from the active app theme.
func themed(n fyne.ThemeColorName) color.Color {
	return fyne.CurrentApp().Settings().Theme().Color(n, fyne.CurrentApp().Settings().ThemeVariant())
}

// card wraps content in a soft rounded "pastel" surface with an optional
// title and one-line description.
func card(title, subtitle string, content ...fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(themed(colorCard))
	bg.CornerRadius = 18
	bg.StrokeColor = themed(colorCardStroke)
	bg.StrokeWidth = 1
	var objs []fyne.CanvasObject
	if title != "" {
		objs = append(objs, sectionTitle(title))
	}
	if subtitle != "" {
		objs = append(objs, muted(subtitle))
	}
	objs = append(objs, content...)
	inner := container.New(layout.NewCustomPaddedLayout(14, 14, 16, 16), container.NewVBox(objs...))
	return container.NewStack(bg, inner)
}

// cardFill is like card but lets the content stretch (tables, lists, logs).
func cardFill(top fyne.CanvasObject, bottom fyne.CanvasObject, content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(themed(colorCard))
	bg.CornerRadius = 18
	bg.StrokeColor = themed(colorCardStroke)
	bg.StrokeWidth = 1
	inner := container.New(layout.NewCustomPaddedLayout(14, 14, 16, 16), container.NewBorder(top, bottom, nil, nil, content))
	return container.NewStack(bg, inner)
}

func sectionTitle(text string) fyne.CanvasObject {
	t := canvas.NewText(text, themed(theme.ColorNameForeground))
	t.TextSize = 18
	t.TextStyle = fyne.TextStyle{Bold: true}
	return t
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
	bg := canvas.NewRectangle(themed(colorPill))
	bg.CornerRadius = 12
	t := canvas.NewText(text, themed(theme.ColorNameForeground))
	t.TextSize = 13
	t.TextStyle = fyne.TextStyle{Bold: true}
	return container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(4, 4, 12, 12), t))
}

// banner is the soft gradient header with the app name, tagline and version.
func banner(version string, mobile bool) fyne.CanvasObject {
	grad := canvas.NewHorizontalGradient(themed(colorBannerA), themed(colorBannerB))
	title := canvas.NewText("Scaner", themed(theme.ColorNameForeground))
	title.TextSize = 26
	title.TextStyle = fyne.TextStyle{Bold: true}
	if mobile {
		title.TextSize = 22
	}
	tag := canvas.NewText("Find clean, fast IPs for your connection", themed(colorMuted))
	tag.TextSize = 13
	icon := canvas.NewImageFromResource(Icon)
	icon.SetMinSize(fyne.NewSize(44, 44))
	icon.FillMode = canvas.ImageFillContain
	text := container.NewVBox(title, tag)
	if mobile {
		icon.SetMinSize(fyne.NewSize(36, 36))
		text = container.NewVBox(title)
	}
	row := container.NewBorder(nil, nil, container.NewHBox(icon, text), container.NewCenter(pill("v"+version)))
	return container.NewStack(grad, container.New(layout.NewCustomPaddedLayout(12, 12, 18, 18), row))
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
			spaced = append(spaced, gap(4))
		}
		spaced = append(spaced, o)
	}
	return container.NewVScroll(container.New(layout.NewCustomPaddedLayout(10, 16, 14, 14), container.NewVBox(spaced...)))
}
