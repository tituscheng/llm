package cmd

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "llm",
	Short: "LLM CLI tool",
	Long:  "A command-line interface for working with LLM models via llama.cpp.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		color.Red(err.Error())
	}
}
