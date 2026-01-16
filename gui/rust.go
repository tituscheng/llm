package gui

import (
	"fmt"
	"llm/internal/llm"
	"llm/prompt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	"github.com/atotto/clipboard"
)

// RustTab creates and returns the Rust tab content.
func RustTab(window fyne.Window) *container.TabItem {
	var (
		referenceBind  = binding.NewString()
		tokenCountBind = binding.NewString()
	)
	promptEntry := widget.NewMultiLineEntry()

	rustTemplateButton := widget.NewButton("Rust template", func() {
		// Copy to clipboard
		clipboard.WriteAll(prompt.RustTemplate)
		promptEntry.SetText(prompt.RustTemplate)

		// Show toast notification that auto-dismisses
		showBriefAlert(window, "Copied to clipboard!")
	})

	referenceEntry := widget.NewEntryWithData(referenceBind)
	referenceButton := widget.NewButton("Reference", func() {
		// reference, err := referenceBind.Get()
		// if err != nil {
		// 	showBriefAlert(window, err.Error())
		// 	return
		// }
		// // updatedText := promptEntry.Text + fmt.Sprintf("\n%s", reference)
	})
	referenceContainer := container.NewBorder(nil, nil, nil, referenceButton, referenceEntry)

	top := container.NewVBox(rustTemplateButton, referenceContainer)

	copyButton := widget.NewButton("Copy", func() {
		clipboard.WriteAll(promptEntry.Text)
		showBriefAlert(window, "Updated prompt copied to clipboard!")
	})

	tokenBox := widget.NewLabelWithData(tokenCountBind)
	bottom := container.NewVBox(tokenBox, copyButton)

	content := container.NewBorder(
		container.NewPadded(top),
		bottom, nil, nil, promptEntry,
	)

	// Update token count in real-time whenever text changes
	promptEntry.OnChanged = func(s string) {
		tokenCount := llm.CountToken(s)
		tokenCountBind.Set(fmt.Sprintf("Tokens: %d", tokenCount))
	}

	// Initialize token count display
	tokenCountBind.Set("Tokens: 0")

	return container.NewTabItem("Rust", content)
}
