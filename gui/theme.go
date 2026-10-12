package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Soft "pastel" palette: lavender primary, mint / peach / rose accents on a
// creamy background (light) or a deep plum night sky (dark).
type appTheme struct {
	// mode is "light", "dark" or "" (follow the system setting).
	mode string
}

// Theme mode labels used by the header toggle and Settings.
const (
	ThemeSystem = "System"
	ThemeLight  = "Light"
	ThemeDark   = "Dark"
)

func (t appTheme) variant(v fyne.ThemeVariant) fyne.ThemeVariant {
	switch t.mode {
	case "light":
		return theme.VariantLight
	case "dark":
		return theme.VariantDark
	}
	return v
}

// Custom colour names used by the card and banner helpers.
const (
	colorCard       fyne.ThemeColorName = "scanerCard"
	colorCardStroke fyne.ThemeColorName = "scanerCardStroke"
	colorBannerA    fyne.ThemeColorName = "scanerBannerA"
	colorBannerB    fyne.ThemeColorName = "scanerBannerB"
	colorPill       fyne.ThemeColorName = "scanerPill"
	colorMuted      fyne.ThemeColorName = "scanerMuted"
)

func rgb(r, g, b uint8) color.NRGBA      { return color.NRGBA{R: r, G: g, B: b, A: 255} }
func rgba(r, g, b, a uint8) color.NRGBA { return color.NRGBA{R: r, G: g, B: b, A: a} }

var pastelLight = map[fyne.ThemeColorName]color.Color{
	theme.ColorNameBackground:          rgb(247, 245, 252),
	theme.ColorNameForeground:          rgb(58, 52, 84),
	theme.ColorNamePrimary:             rgb(141, 123, 240),
	theme.ColorNameFocus:               rgba(141, 123, 240, 150),
	theme.ColorNameForegroundOnPrimary: rgb(255, 255, 255),
	theme.ColorNameButton:              rgb(236, 231, 255),
	theme.ColorNameDisabledButton:      rgb(240, 237, 248),
	theme.ColorNameDisabled:            rgb(183, 176, 206),
	theme.ColorNameInputBackground:     rgb(255, 255, 255),
	theme.ColorNameInputBorder:         rgb(220, 212, 248),
	theme.ColorNameMenuBackground:      rgb(255, 255, 255),
	theme.ColorNameOverlayBackground:   rgb(255, 255, 255),
	theme.ColorNameHeaderBackground:    rgb(239, 234, 255),
	theme.ColorNameHover:               rgba(141, 123, 240, 28),
	theme.ColorNamePressed:             rgba(141, 123, 240, 60),
	theme.ColorNameSelection:           rgba(141, 123, 240, 70),
	theme.ColorNamePlaceHolder:         rgb(160, 152, 186),
	theme.ColorNameScrollBar:           rgba(141, 123, 240, 120),
	theme.ColorNameScrollBarBackground: rgba(141, 123, 240, 18),
	theme.ColorNameSeparator:           rgb(230, 224, 250),
	theme.ColorNameShadow:              rgba(110, 90, 200, 30),
	theme.ColorNameSuccess:             rgb(86, 196, 160),
	theme.ColorNameForegroundOnSuccess: rgb(255, 255, 255),
	theme.ColorNameWarning:             rgb(244, 172, 102),
	theme.ColorNameForegroundOnWarning: rgb(255, 255, 255),
	theme.ColorNameError:               rgb(236, 124, 146),
	theme.ColorNameForegroundOnError:   rgb(255, 255, 255),
	theme.ColorNameHyperlink:           rgb(120, 100, 230),
	colorCard:                          rgb(255, 255, 255),
	colorCardStroke:                    rgb(238, 234, 252),
	colorBannerA:                       rgb(255, 214, 231),
	colorBannerB:                       rgb(206, 222, 255),
	colorPill:                          rgba(255, 255, 255, 190),
	colorMuted:                         rgb(122, 114, 150),
}

var pastelDark = map[fyne.ThemeColorName]color.Color{
	theme.ColorNameBackground:          rgb(29, 26, 44),
	theme.ColorNameForeground:          rgb(236, 232, 255),
	theme.ColorNamePrimary:             rgb(184, 168, 255),
	theme.ColorNameFocus:               rgba(184, 168, 255, 150),
	theme.ColorNameForegroundOnPrimary: rgb(29, 26, 44),
	theme.ColorNameButton:              rgb(48, 43, 70),
	theme.ColorNameDisabledButton:      rgb(40, 36, 58),
	theme.ColorNameDisabled:            rgb(110, 103, 140),
	theme.ColorNameInputBackground:     rgb(38, 34, 56),
	theme.ColorNameInputBorder:         rgb(66, 59, 96),
	theme.ColorNameMenuBackground:      rgb(42, 38, 62),
	theme.ColorNameOverlayBackground:   rgb(42, 38, 62),
	theme.ColorNameHeaderBackground:    rgb(46, 41, 70),
	theme.ColorNameHover:               rgba(184, 168, 255, 30),
	theme.ColorNamePressed:             rgba(184, 168, 255, 60),
	theme.ColorNameSelection:           rgba(184, 168, 255, 80),
	theme.ColorNamePlaceHolder:         rgb(128, 120, 160),
	theme.ColorNameScrollBar:           rgba(184, 168, 255, 120),
	theme.ColorNameScrollBarBackground: rgba(184, 168, 255, 18),
	theme.ColorNameSeparator:           rgb(56, 50, 82),
	theme.ColorNameShadow:              rgba(0, 0, 0, 70),
	theme.ColorNameSuccess:             rgb(140, 226, 196),
	theme.ColorNameForegroundOnSuccess: rgb(29, 26, 44),
	theme.ColorNameWarning:             rgb(255, 205, 150),
	theme.ColorNameForegroundOnWarning: rgb(29, 26, 44),
	theme.ColorNameError:               rgb(255, 170, 188),
	theme.ColorNameForegroundOnError:   rgb(29, 26, 44),
	theme.ColorNameHyperlink:           rgb(200, 186, 255),
	colorCard:                          rgb(37, 33, 56),
	colorCardStroke:                    rgb(56, 50, 84),
	colorBannerA:                       rgb(92, 62, 110),
	colorBannerB:                       rgb(56, 70, 120),
	colorPill:                          rgba(255, 255, 255, 36),
	colorMuted:                         rgb(170, 162, 200),
}

func (t appTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	v = t.variant(v)
	palette := pastelLight
	if v == theme.VariantDark {
		palette = pastelDark
	}
	if c, ok := palette[n]; ok {
		return c
	}
	if v != theme.VariantDark {
		v = theme.VariantLight
	}
	return theme.DefaultTheme().Color(n, v)
}

func (appTheme) Font(s fyne.TextStyle) fyne.Resource     { return theme.DefaultTheme().Font(s) }
func (appTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }
func (appTheme) Size(n fyne.ThemeSizeName) float32 {
	switch n {
	case theme.SizeNameText:
		return 15
	case theme.SizeNameHeadingText:
		return 22
	case theme.SizeNameSubHeadingText:
		return 17
	case theme.SizeNameCaptionText:
		return 12
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInnerPadding:
		return 10
	case theme.SizeNameInlineIcon:
		return 22
	case theme.SizeNameInputRadius:
		return 12
	case theme.SizeNameSelectionRadius:
		return 10
	case theme.SizeNameInputBorder:
		return 1.5
	case theme.SizeNameScrollBar:
		return 10
	case theme.SizeNameScrollBarSmall:
		return 5
	case theme.SizeNameScrollBarRadius:
		return 5
	case theme.SizeNameSeparatorThickness:
		return 1
	}
	return theme.DefaultTheme().Size(n)
}
