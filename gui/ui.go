package gui

import (
	"llm/prompt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/atotto/clipboard"
)

// AboutTab creates and returns the About tab content
func UITab(window fyne.Window) *container.TabItem {
	promptEntry := widget.NewMultiLineEntry()

	copyButton := widget.NewButton("Debug", func() {
		// Copy to clipboard
		clipboard.WriteAll(prompt.UIDebugTemplate)
		promptEntry.SetText(prompt.UIDebugTemplate)

		// Show toast notification that auto-dismisses
		showBriefAlert(window, "Copied to clipboard!")
	})

	updateButton := widget.NewButton("Copy", func() {
		clipboard.WriteAll(promptEntry.Text)

		// Show toast notification that auto-dismisses
		showBriefAlert(window, "Copied to clipboard!")
	})

	content := container.NewBorder(
		container.NewPadded(copyButton),
		updateButton, nil, nil, promptEntry,
	)

	return container.NewTabItem("UI", content)
}
