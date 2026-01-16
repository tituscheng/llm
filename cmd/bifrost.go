package cmd

import (
	"fmt"
	"llm/internal/bifrost"

	"github.com/spf13/cobra"
)

var bifrostCmd = &cobra.Command{
	Use:   "bifrost",
	Short: "Write your description here",
	Long:  "Write your description here",
}

var bifrostConfigCmd = &cobra.Command{
	Use:   "bifrost",
	Short: "Bifrost config",
	Long:  "Bifrost config",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(bifrost.BifrostConfigJSONSTring)
	},
}

func init() {
	bifrostCmd.AddCommand(bifrostConfigCmd)
	rootCmd.AddCommand(bifrostCmd)
}
