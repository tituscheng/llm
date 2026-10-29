package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"llm/install"
)

var imageDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Compile the latest image inference binary",
	Long:  "Compiles the latest sd-cli from upstream source into ~/.llm/tools.",
	Args:  cobra.NoArgs,
	RunE:  runImageDownload,
}

func runImageDownload(cmd *cobra.Command, args []string) error {
	cat, ok := install.CategoryByID("image")
	if !ok {
		return fmt.Errorf("image category is not defined")
	}
	tool, err := selectTool(cmd, cat, args)
	if err != nil {
		return err
	}
	return compileTool(cmd, tool)
}

func init() {
	imageCmd.AddCommand(imageDownloadCmd)
}
