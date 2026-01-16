package gui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

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
