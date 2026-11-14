package main

import "github.com/charmbracelet/lipgloss"

var (
	colorPrimary   = lipgloss.Color("42")
	colorSecondary = lipgloss.Color("39")
	colorDim       = lipgloss.Color("245")
	colorBorder    = lipgloss.Color("240")

	styleAppTitle    = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	styleAppSubtitle = lipgloss.NewStyle().Foreground(colorDim)

	stylePane = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1)

	stylePaneActive = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(0, 1)

	styleStatusBar = lipgloss.NewStyle().Foreground(colorDim)
	styleStatusKey = lipgloss.NewStyle().Bold(true).Foreground(colorSecondary)
	styleSelected  = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	styleHint      = lipgloss.NewStyle().Foreground(colorDim)

	styleLabelActive   = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	styleLabelInactive = lipgloss.NewStyle().Foreground(colorDim)
)