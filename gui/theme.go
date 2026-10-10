package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type appTheme struct{}

func (appTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	// Headless Fyne tests use a non-preference variant; render our real dark
	// palette there rather than silently falling back to the stock theme.
	if v != theme.VariantLight && v != theme.VariantDark {
		v = theme.VariantDark
	}
	if v == theme.VariantLight {
		switch n {
		case theme.ColorNameBackground:
			return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		case theme.ColorNameForeground:
			return color.NRGBA{R: 44, G: 44, B: 43, A: 255}
		case theme.ColorNamePrimary, theme.ColorNameFocus:
			return color.NRGBA{R: 31, G: 110, B: 194, A: 255}
		case theme.ColorNameForegroundOnPrimary:
			return color.White
		case theme.ColorNameButton:
			return color.NRGBA{R: 240, G: 239, B: 237, A: 255}
		case theme.ColorNameInputBackground:
			return color.NRGBA{R: 249, G: 248, B: 247, A: 255}
		}
	}
	if v == theme.VariantDark {
		switch n {
		case theme.ColorNamePrimary, theme.ColorNameFocus:
			return color.NRGBA{R: 94, G: 159, B: 232, A: 255}
		case theme.ColorNameForegroundOnPrimary:
			return color.NRGBA{R: 25, G: 25, B: 25, A: 255}
		}
	}
	return theme.DefaultTheme().Color(n, v)
}
func (appTheme) Font(s fyne.TextStyle) fyne.Resource     { return theme.DefaultTheme().Font(s) }
func (appTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }
func (appTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNameText:
		return 16
	case theme.SizeNamePadding:
		return 8
	case theme.SizeNameInnerPadding:
		return 8
	case theme.SizeNameInlineIcon:
		return 28
	}
	return theme.DefaultTheme().Size(n)
}
