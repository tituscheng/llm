package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"llm/internal/tools"
)

var textDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download an embedded text inference binary",
	Long:  "Copies the embedded llama-server binary into the current directory.",
	Args:  cobra.NoArgs,
	RunE:  runTextDownload,
}

func runTextDownload(cmd *cobra.Command, args []string) error {
	cat, ok := tools.CategoryByID("text")
	if !ok {
		return fmt.Errorf("text category is not defined")
	}
	bin, err := selectBinary(cmd, cat, args)
	if err != nil {
		return err
	}
	return installToCWD(bin)
}

func init() {
	textCmd.AddCommand(textDownloadCmd)
}
