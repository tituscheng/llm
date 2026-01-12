package cmd

import (
	"os"
	"text/tabwriter"

	"llm/internal/input"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var countCmd = &cobra.Command{
	Use:   "count",
	Short: "Count tokens in text",
	Long: `Count the number of tokens in the given text using tiktoken encoding.

Supports multiple input sources:
  - File paths (existing files)
  - Directory paths (reads all files)
  - "@git" for modified/untracked files
  - "@md" for markdown files in current directory
  - "clipboard:", ":paste" for clipboard
  - Plain text`,
	Run: func(cmd *cobra.Command, args []string) {
		sources, err := input.ReadAuto(args)
		if err != nil {
			color.Red("Error: %v\n", err)
			os.Exit(1)
		}

		if len(sources) == 0 {
			color.Yellow("No sources found.\n")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
		cyan := color.New(color.FgCyan)
		green := color.New(color.FgGreen)

		for _, s := range sources {
			cyan.Fprintf(w, "%s:\t%d tokens\n", s.Name, s.TokenCount)
		}
		green.Fprintln(w, "──────────────────────")
		green.Fprintf(w, "Total:\t%d tokens\n", input.TotalTokens(sources))
		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(countCmd)
}
