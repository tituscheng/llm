package cmd

import "github.com/spf13/cobra"

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Write your description here",
	Long:  "Write your description here",
	Run: func(cmd *cobra.Command, args []string) {

	},
}

func init() {
	rootCmd.AddCommand(chatCmd)
}
