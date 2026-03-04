package cmd

import "github.com/spf13/cobra"

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Manage llama.cpp server",
	Long:  "Utility command for managing llama.cpp server instances. Use --help to see available subcommands.",
}

func init() {
	rootCmd.AddCommand(serverCmd)
}
