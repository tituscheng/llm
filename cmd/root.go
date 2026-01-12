package cmd

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "llm",
	Short: "llm ",
	Long:  "llm",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		color.Red(err.Error())
	}
}
