package cmd

import (
	"github.com/spf13/cobra"

	"llm/internal/tools"
)

var textDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download llama-server binary",
	Long:  "Copies the embedded llama-server binary into the current directory.",
	Args:  cobra.NoArgs,
	RunE:  runTextDownload,
}

func runTextDownload(cmd *cobra.Command, args []string) error {
	return copyEmbeddedBinary("llama-server", tools.LlamaServerBinary, tools.LlamaServerVersion)
}

func init() {
	textCmd.AddCommand(textDownloadCmd)
}
