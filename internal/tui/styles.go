package tui

import "github.com/gdamore/tcell/v2"

type Theme struct {
	Primary      tcell.Color
	Secondary    tcell.Color
	Background   tcell.Color
	Surface      tcell.Color
	Text         tcell.Color
	TextDim      tcell.Color
	TextSecondary tcell.Color
	Accent       tcell.Color
	Error        tcell.Color
	Success      tcell.Color
	Warning      tcell.Color
	Border       tcell.Color
	BorderFocus  tcell.Color
	Highlight    tcell.Color
	Selection    tcell.Color
	InputBg      tcell.Color
	SurfaceAlt   tcell.Color
}

func DefaultTheme() *Theme {
	return &Theme{
		Primary:      tcell.ColorCornflowerBlue,
		Secondary:    tcell.ColorDeepSkyBlue,
		Background:   tcell.Color232,
		Surface:      tcell.Color236,
		SurfaceAlt:   tcell.Color237,
		Text:         tcell.Color255,
		TextDim:      tcell.Color246,
		TextSecondary: tcell.Color250,
		Accent:       tcell.Color117,
		Error:        tcell.Color203,
		Success:      tcell.Color150,
		Warning:      tcell.Color215,
		Border:       tcell.Color239,
		BorderFocus:  tcell.Color117,
		Highlight:    tcell.Color24,
		Selection:    tcell.Color23,
		InputBg:      tcell.Color234,
	}
}

var Styles = DefaultTheme()
