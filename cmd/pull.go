package cmd

import "github.com/spf13/cobra"

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Write your description here",
	Long:  "Write your description here",
	Run: func(cmd *cobra.Command, args []string) {

	},
}

func init() {
	rootCmd.AddCommand(pullCmd)
}
