package cmd

import (
	"github.com/spf13/cobra"

	"llm/internal/tools"
)

var imageDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download sd-cli binary",
	Long:  "Copies the embedded sd-cli binary into the current directory.",
	Args:  cobra.NoArgs,
	RunE:  runImageDownload,
}

func runImageDownload(cmd *cobra.Command, args []string) error {
	return copyEmbeddedBinary("sd-cli", tools.SDCLIBinary, tools.SDCLIVersion)
}

func init() {
	imageCmd.AddCommand(imageDownloadCmd)
}
