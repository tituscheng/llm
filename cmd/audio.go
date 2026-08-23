package cmd

import "github.com/spf13/cobra"

var audioCmd = &cobra.Command{
	Use:   "audio",
	Short: "Manage local audio inference binaries",
	Long:  "Utility command for managing local audio inference tools. Use --help to see available subcommands.",
}

func init() {
	rootCmd.AddCommand(audioCmd)
}
