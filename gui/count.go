package gui

import (
	"fmt"
	"llm/internal/llm"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
)

// RustTab creates and returns the Rust tab content.
func CountTab(window fyne.Window) *container.TabItem {
	var (
		tokenCountBind = binding.NewString()
	)
	promptEntry := widget.NewMultiLineEntry()

	countLabel := widget.NewLabelWithData(tokenCountBind)

	content := container.NewBorder(
		nil,
		countLabel, nil, nil, promptEntry,
	)

	// Update token count in real-time whenever text changes
	promptEntry.OnChanged = func(s string) {
		tokenCount := llm.CountToken(s)
		tokenCountBind.Set(fmt.Sprintf("Tokens: %d", tokenCount))
	}

	// Initialize token count display
	tokenCountBind.Set("Tokens: 0")

	return container.NewTabItem("Token Count", content)
}
