package cmd

import "github.com/spf13/cobra"

var imageCmd = &cobra.Command{
	Use:   "image",
	Short: "Manage stable-diffusion.cpp image inference",
	Long:  "Utility command for managing stable-diffusion.cpp image inference. Use --help to see available subcommands.",
}

func init() {
	rootCmd.AddCommand(imageCmd)
}
