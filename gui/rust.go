package gui

import (
	"fmt"
	"llm/prompt"
	"strings"
	"time"

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
		tokenCount := countTokens(s)
		tokenCountBind.Set(fmt.Sprintf("Tokens: %d", tokenCount))
	}

	// Initialize token count display
	tokenCountBind.Set("Tokens: 0")

	return container.NewTabItem("Rust", content)
}

// countTokens estimates the number of tokens in the given text.
// Uses a simple approximation: splits by whitespace and common punctuation.
// Tokens are roughly 4 characters per token on average for English text.
func countTokens(text string) int {
	if text == "" {
		return 0
	}

	// Replace common punctuation with spaces to split properly
	replacer := strings.NewReplacer(
		",", " ",
		".", " ",
		"!", " ",
		"?", " ",
		";", " ",
		":", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
		"\n", " ",
		"\t", " ",
	)
	normalized := replacer.Replace(text)

	// Split by whitespace and filter empty strings
	words := strings.Fields(normalized)

	// Estimate tokens: most tokens are words, but also count based on character length
	// Rule of thumb: ~4 characters = 1 token
	tokenCount := len(words)
	totalChars := len(strings.Join(words, ""))
	charBasedTokens := totalChars / 4

	// Use the maximum of word count and character-based estimation
	if charBasedTokens > tokenCount {
		return charBasedTokens
	}
	return tokenCount
}

// showBriefAlert displays a temporary toast notification using PopUp that auto-dismisses.
// No user interaction required - it appears and disappears automatically.
func showBriefAlert(window fyne.Window, message string) {
	// Create notification label
	notifLabel := widget.NewLabel("✓ " + message)

	// Create a PopUp centered at the top of the window
	popup := widget.NewPopUp(notifLabel, window.Canvas())

	// Get window size to center the popup
	size := window.Canvas().Size()
	popup.Move(fyne.NewPos(
		(size.Width-200)/2,
		50,
	))
	popup.Resize(fyne.NewSize(200, 40))

	// Show the popup
	popup.Show()

	// Auto-dismiss after 2 seconds using fyne.Do() for thread-safe UI updates
	go func() {
		time.Sleep(2 * time.Second)
		fyne.Do(func() {
			popup.Hide()
		})
	}()
}
