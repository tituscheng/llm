package cmd

import "github.com/spf13/cobra"

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the llama.cpp server",
	Long:  "Start the llama.cpp server for serving LLM models via API.",
	Run: func(cmd *cobra.Command, args []string) {

	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}
