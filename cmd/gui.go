package cmd

import (
	"github.com/spf13/cobra"

	"llm/internal/gui"
)

var guiCmd = &cobra.Command{
	Use:   "gui",
	Short: "Install inference binaries with a graphical selector",
	Long:  "Opens a window to pick a category (text, image, or audio) and compile the selected tool from the latest upstream source into ~/.llm/tools.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return gui.Main()
	},
}

func init() {
	rootCmd.AddCommand(guiCmd)
}
