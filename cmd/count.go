package cmd

import (
	"bufio"
	"fmt"

	"os"
	"strings"

	"llm/internal/llm"

	"github.com/atotto/clipboard"
	"github.com/spf13/cobra"
)

var (
	textFlag      string
	clipboardFlag bool
	fileFlag      string
)

var countCmd = &cobra.Command{
	Use:   "count",
	Short: "Count tokens in text",
	Long:  "Count the number of tokens in the given text using tiktoken encoding. Supports text, clipboard, file input.",
	Run: func(cmd *cobra.Command, args []string) {
		var text string

		switch {
		case textFlag != "":
			text = textFlag
		case clipboardFlag:
			text, _ = clipboard.ReadAll()
		case fileFlag != "":
			data, err := os.ReadFile(fileFlag)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
				os.Exit(1)
			}
			text = string(data)
		case len(args) > 0:
			text = strings.Join(args, " ")
		default:
			scanner := bufio.NewScanner(os.Stdin)
			var lines []string
			for scanner.Scan() {
				lines = append(lines, scanner.Text())
			}
			text = strings.Join(lines, "\n")
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
	countCmd.Flags().StringVarP(&textFlag, "text", "t", "", "Text to count tokens for")
	countCmd.Flags().BoolVarP(&clipboardFlag, "clipboard", "c", false, "Read text from clipboard")
	countCmd.Flags().StringVarP(&fileFlag, "file", "f", "", "Read text from file")
	rootCmd.AddCommand(countCmd)
}
