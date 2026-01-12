package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"llm/internal/input"

	"github.com/spf13/cobra"
)

var countCmd = &cobra.Command{
	Use:   "count",
	Short: "Count tokens in text",
	Long: `Count the number of tokens in the given text using tiktoken encoding.

Supports multiple input sources that are concatenated:
  - Existing files or directories
  - "@git" to read all modified and untracked files from git
  - "@md" to read all markdown files in the current directory
  - "clipboard:" or ":paste" to read from clipboard
  - Plain text (any string that doesn't match a file/directory)

Examples:
  llm count "Hello world"
  llm count ./file.txt
  llm count ./dir/
  llm count clipboard: ./file.txt "additional text"
  llm count @md`,
	Run: func(cmd *cobra.Command, args []string) {
		sources, err := input.ReadAuto(args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
			os.Exit(1)
		}

		// Print per-source breakdown
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
		for _, s := range sources {
			fmt.Fprintf(w, "%s:\t%d tokens\n", s.Name, s.TokenCount)
		}
		fmt.Fprintln(w, "──────────────────────")
		fmt.Fprintf(w, "Total:\t%d tokens\n", input.TotalTokens(sources))
		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(countCmd)
}
