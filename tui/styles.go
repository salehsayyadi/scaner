package tui

import "github.com/charmbracelet/lipgloss"

var (
	colAccent  = lipgloss.Color("39") // cyan/blue
	colAccent2 = lipgloss.Color("63") // indigo
	colGreen   = lipgloss.Color("42")
	colYellow  = lipgloss.Color("220")
	colRed     = lipgloss.Color("203")
	colMagenta = lipgloss.Color("171")
	colBlue    = lipgloss.Color("75")
	colGray    = lipgloss.Color("245")
	colDim     = lipgloss.Color("240")
	colInk     = lipgloss.Color("231")

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colInk)

	headerBar = lipgloss.NewStyle().
			Bold(true).
			Foreground(colInk).
			Background(colAccent2).
			Padding(0, 1)

	tabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(colInk).
			Background(colAccent).
			Padding(0, 2)

	tabInactive = lipgloss.NewStyle().
			Foreground(colGray).
			Padding(0, 2)

	tabBar = lipgloss.NewStyle().Padding(0, 0, 1, 0)

	bodyStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colDim).
			Padding(1, 2)

	footerStyle = lipgloss.NewStyle().
			Foreground(colGray).
			Padding(1, 0, 0, 0)

	labelStyle    = lipgloss.NewStyle().Foreground(colGray)
	valueStyle    = lipgloss.NewStyle().Foreground(colInk)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(colInk).Background(colAccent2)
	okStyle       = lipgloss.NewStyle().Foreground(colGreen)
	errStyle      = lipgloss.NewStyle().Foreground(colRed)
	hintStyle     = lipgloss.NewStyle().Foreground(colDim)
	keyStyle      = lipgloss.NewStyle().Foreground(colYellow).Bold(true)
	statStyle     = lipgloss.NewStyle().Bold(true).Foreground(colInk)
)
