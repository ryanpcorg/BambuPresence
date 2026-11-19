package ui

import (
	"image/color"

	"gioui.org/font/gofont"
	"gioui.org/text"
	"gioui.org/widget/material"
)

var (
	colorBg      = color.NRGBA{R: 0x16, G: 0x18, B: 0x1C, A: 0xFF}
	colorSurface = color.NRGBA{R: 0x1F, G: 0x22, B: 0x28, A: 0xFF}
	colorOverlay = color.NRGBA{R: 0x26, G: 0x2A, B: 0x32, A: 0xFF}
	colorBorder  = color.NRGBA{R: 0x30, G: 0x35, B: 0x40, A: 0xFF}
	colorPrimary = color.NRGBA{R: 0x00, G: 0xAE, B: 0x42, A: 0xFF}
	colorDanger  = color.NRGBA{R: 0xE5, G: 0x48, B: 0x4D, A: 0xFF}
	colorWarning = color.NRGBA{R: 0xF5, G: 0xA6, B: 0x23, A: 0xFF}
	colorFg      = color.NRGBA{R: 0xE8, G: 0xEC, B: 0xF1, A: 0xFF}
	colorMuted   = color.NRGBA{R: 0x8C, G: 0x96, B: 0xA5, A: 0xFF}
)

func newTheme() *material.Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	th.Palette = material.Palette{
		Bg:         colorBg,
		Fg:         colorFg,
		ContrastBg: colorPrimary,
		ContrastFg: color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
	}
	return th
}
