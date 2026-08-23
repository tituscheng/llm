package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"llm/internal/tools"
)

var imageDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download an embedded image inference binary",
	Long:  "Copies the embedded sd-cli binary into the current directory.",
	Args:  cobra.NoArgs,
	RunE:  runImageDownload,
}

func runImageDownload(cmd *cobra.Command, args []string) error {
	cat, ok := tools.CategoryByID("image")
	if !ok {
		return fmt.Errorf("image category is not defined")
	}
	bin, err := selectBinary(cmd, cat, args)
	if err != nil {
		return err
	}
	return installToCWD(bin)
}

func init() {
	imageCmd.AddCommand(imageDownloadCmd)
}
