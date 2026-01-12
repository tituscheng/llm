package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// NewMainWindow creates and returns the new Fyne application window with tabs.
func NewMainWindow(app fyne.App) fyne.Window {
	myWindow := app.NewWindow("LLM Helper")
	myWindow.Resize(fyne.NewSize(800, 800))

	// Create tabs
	tabs := container.NewAppTabs(
		RustTab(myWindow),
		UITab(myWindow),
		CountTab(myWindow),
		AgentTab(myWindow),
		SettingsTab(),
	)

	// Set window content
	myWindow.SetContent(tabs)

	return myWindow
}
