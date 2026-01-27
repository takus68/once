package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

type colors struct {
	Primary    color.Color
	Secondary  color.Color
	Background color.Color
	Text       color.Color
	TextDark   color.Color
	Focused    color.Color
	Border     color.Color
}

var Colors = colors{
	Primary:    lipgloss.Color("#FF69B4"),
	Secondary:  lipgloss.Color("#9f3"),
	Background: lipgloss.Color("#000000"),
	Text:       lipgloss.Color("#FFFFFF"),
	TextDark:   lipgloss.Color("#000000"),
	Focused:    lipgloss.Color("#FFA500"),
	Border:     lipgloss.Color("#6272a4"),
}

type styles struct {
	Title         lipgloss.Style
	SubTitle      lipgloss.Style
	Label         lipgloss.Style
	Input         lipgloss.Style
	Button        lipgloss.Style
	ButtonPrimary lipgloss.Style
}

var Styles = styles{
	Title: lipgloss.NewStyle().
		Background(Colors.Primary).
		Foreground(Colors.TextDark).
		Bold(true),
	SubTitle: lipgloss.NewStyle().
		Foreground(Colors.Secondary).
		Underline(true),
	Label: lipgloss.NewStyle().
		Bold(true),
	Input: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Colors.Border).
		Padding(0, 1).
		MarginBottom(1),
	Button: lipgloss.NewStyle().
		Padding(0, 2).
		MarginRight(1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Colors.Border),
	ButtonPrimary: lipgloss.NewStyle().
		Padding(0, 2).
		MarginRight(1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Colors.Primary),
}

func (s styles) Focus(base lipgloss.Style, focused bool) lipgloss.Style {
	if focused {
		return base.BorderForeground(Colors.Focused)
	}
	return base
}
