package tui

import "github.com/gdamore/tcell/v2"

// Theme defines the color theme for the application
type Theme struct {
	Primary      tcell.Color
	Secondary    tcell.Color
	Background   tcell.Color
	Surface      tcell.Color
	Text         tcell.Color
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
}

// DefaultTheme returns the default color theme
func DefaultTheme() *Theme {
	return &Theme{
		Primary:       tcell.ColorSteelBlue,
		Secondary:     tcell.ColorDarkSlateGray,
		Background:    tcell.ColorBlack,
		Surface:       tcell.Color236,   // dark gray
		Text:          tcell.ColorWhite,
		TextSecondary: tcell.ColorSilver,
		Accent:        tcell.ColorDodgerBlue,
		Error:         tcell.ColorRed,
		Success:       tcell.ColorGreen,
		Warning:       tcell.ColorOrange,
		Border:        tcell.ColorGray,
		BorderFocus:   tcell.ColorDodgerBlue,
		Highlight:     tcell.ColorBlue,
		Selection:     tcell.ColorDarkCyan,
		InputBg:       tcell.Color235,
	}
}

// Styles holds all the shared styles for the application
var Styles = DefaultTheme()
