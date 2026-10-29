package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"llm/install"
)

var audioDownloadCmd = &cobra.Command{
	Use:       "download [binary]",
	Short:     "Compile the latest audio inference binary",
	Long:      "Compiles a selected Apple Silicon audio tool from upstream source into ~/.llm/tools.",
	Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
	ValidArgs: audioToolNames(),
	RunE:      runAudioDownload,
}

func audioToolNames() []string {
	cat, ok := install.CategoryByID("audio")
	if !ok {
		return []string{}
	}
	return toolNames(cat.Tools)
}

func runAudioDownload(cmd *cobra.Command, args []string) error {
	cat, ok := install.CategoryByID("audio")
	if !ok {
		return fmt.Errorf("audio category is not defined")
	}
	tool, err := selectTool(cmd, cat, args)
	if err != nil {
		return err
	}
	return compileTool(cmd, tool)
}

func init() {
	audioCmd.AddCommand(audioDownloadCmd)
}
