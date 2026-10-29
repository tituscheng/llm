package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"llm/install"
)

var textDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Compile the latest text inference binary",
	Long:  "Compiles the latest llama-server from upstream source into ~/.llm/tools.",
	Args:  cobra.NoArgs,
	RunE:  runTextDownload,
}

func runTextDownload(cmd *cobra.Command, args []string) error {
	cat, ok := install.CategoryByID("text")
	if !ok {
		return fmt.Errorf("text category is not defined")
	}
	tool, err := selectTool(cmd, cat, args)
	if err != nil {
		return err
	}
	return compileTool(cmd, tool)
}

func init() {
	textCmd.AddCommand(textDownloadCmd)
}
