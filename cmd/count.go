package cmd

import (
	"fmt"
	"os"

	"llm/internal/input"
	"llm/internal/llm"

	"github.com/spf13/cobra"
)

var countCmd = &cobra.Command{
	Use:   "count",
	Short: "Count tokens in text",
	Long: `Count the number of tokens in the given text using tiktoken encoding.

Supports multiple input sources that are concatenated:
  - Existing files or directories
  - "clipboard:" or ":paste" to read from clipboard
  - Plain text (any string that doesn't match a file/directory)

Examples:
  llm count "Hello world"
  llm count ./file.txt
  llm count ./dir/
  llm count clipboard: ./file.txt "additional text"`,
	Run: func(cmd *cobra.Command, args []string) {
		text, err := input.ReadAuto(args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			os.Exit(1)
		}

		if text == "" {
			fmt.Println("0")
			return
		}

		count := llm.CountToken(text)
		fmt.Println(count)
	},
}

func init() {
	rootCmd.AddCommand(countCmd)
}
