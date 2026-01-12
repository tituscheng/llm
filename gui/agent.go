package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/atotto/clipboard"
)

// RustTab creates and returns the Rust tab content.
func AgentTab(window fyne.Window) *container.TabItem {
	haikuAgentButton := widget.NewButton("Haiku", func() {
		clipboard.WriteAll("Write a prompt fine tuned to Claude Haiku 4.5 for agentic coding.")
		showBriefAlert(window, "Copied prompt to clipboard")
	})

	mainContainer := container.NewBorder(haikuAgentButton, nil, nil, nil, nil)
	return container.NewTabItem("Agent", mainContainer)
}
