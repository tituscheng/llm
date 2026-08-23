package cmd

import "github.com/spf13/cobra"

var textCmd = &cobra.Command{
	Use:   "text",
	Short: "Manage llama.cpp text inference",
	Long:  "Utility command for managing llama.cpp text inference. Use --help to see available subcommands.",
}

func init() {
	rootCmd.AddCommand(textCmd)
}
