package gui

import (
	"llm/prompt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	"github.com/atotto/clipboard"
)

// RustTab creates and returns the Rust tab content.
func MentorTab(window fyne.Window) *container.TabItem {
	var (
		resultBind = binding.NewString()
	)

	result := widget.NewMultiLineEntry()
	result.Bind(resultBind)

	socraticPromptingButton := widget.NewButton("Socratic Prompting Button", func() {
		clipboard.WriteAll(prompt.PromptMentorTemplate)
		resultBind.Set(prompt.PromptMentorTemplate)
		showBriefAlert(window, "Copied to Clipboard!")
	})

	optionsContainer := container.NewPadded(container.NewVBox(socraticPromptingButton))

	contentContainer := container.NewBorder(optionsContainer, nil, nil, nil, result)

	return container.NewTabItem("Mentor", contentContainer)
}
