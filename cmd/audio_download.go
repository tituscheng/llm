package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"llm/internal/tools"
)

var audioDownloadCmd = &cobra.Command{
	Use:       "download [binary]",
	Short:     "Download an embedded audio inference binary",
	Long:      "Lists the embedded Apple Silicon audio binaries and copies the selected one into the current directory.",
	Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
	ValidArgs: audioBinaryNames(),
	RunE:      runAudioDownload,
}

func audioBinaryNames() []string {
	cat, ok := tools.CategoryByID("audio")
	if !ok {
		return []string{}
	}
	return binaryNames(cat.Binaries)
}

func runAudioDownload(cmd *cobra.Command, args []string) error {
	cat, ok := tools.CategoryByID("audio")
	if !ok {
		return fmt.Errorf("audio category is not defined")
	}
	bin, err := selectBinary(cmd, cat, args)
	if err != nil {
		return err
	}
	return installToCWD(bin)
}

func init() {
	audioCmd.AddCommand(audioDownloadCmd)
}
