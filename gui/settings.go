package gui

import (
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// SettingsTab creates and returns the Settings tab content.
func SettingsTab() *container.TabItem {
	content := container.NewVBox(
		widget.NewLabel("Settings"),
		widget.NewSeparator(),
		widget.NewLabel("Theme:"),
		container.NewHBox(
			widget.NewLabel("Light Mode"),
			widget.NewCheck("", func(b bool) {}),
		),
		widget.NewSeparator(),
		widget.NewLabel("Preferences:"),
		widget.NewCheck("Enable notifications", func(b bool) {}),
		widget.NewCheck("Auto-save", func(b bool) {}),
	)

	return container.NewTabItem("Settings", content)
}
