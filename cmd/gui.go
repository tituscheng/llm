package cmd

import (
	"llm/gui"

	"fyne.io/fyne/v2/app"
	"github.com/spf13/cobra"
)

var guiCmd = &cobra.Command{
	Use:   "gui",
	Short: "Write your description here",
	Long:  "Write your description here",
	Run: func(cmd *cobra.Command, args []string) {
		myApp := app.New()
		myWindow := gui.NewMainWindow(myApp)
		myWindow.ShowAndRun()
	},
}

func init() {
	rootCmd.AddCommand(guiCmd)
}
