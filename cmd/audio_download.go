package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	bins := tools.AudioBinaries()
	names := make([]string, 0, len(bins))
	for _, bin := range bins {
		names = append(names, bin.Name)
	}
	return names
}

func runAudioDownload(cmd *cobra.Command, args []string) error {
	bins := tools.AudioBinaries()
	if len(bins) == 0 {
		return fmt.Errorf("no audio binaries are embedded")
	}

	var selected tools.Binary
	if len(args) == 1 {
		bin, ok := findAudioBinary(bins, args[0])
		if !ok {
			return fmt.Errorf("unknown audio binary %q", args[0])
		}
		selected = bin
	} else {
		bin, err := promptAudioBinary(cmd, bins)
		if err != nil {
			return err
		}
		selected = bin
	}

	if err := copyEmbeddedBinary(selected.Name, selected.Data, selected.Version); err != nil {
		return err
	}

	if selected.Name == "piper" {
		if err := copyPiperRuntime(); err != nil {
			return err
		}
	}

	return nil
}

func findAudioBinary(bins []tools.Binary, name string) (tools.Binary, bool) {
	for _, bin := range bins {
		if bin.Name == name {
			return bin, true
		}
	}
	return tools.Binary{}, false
}

func promptAudioBinary(cmd *cobra.Command, bins []tools.Binary) (tools.Binary, error) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Select an audio binary to copy into the current directory:")
	fmt.Fprintln(out)
	for i, bin := range bins {
		fmt.Fprintf(out, "  %d) %-10s  %s\n", i+1, bin.Name, bin.Version)
	}
	fmt.Fprintln(out)
	fmt.Fprint(out, "Enter number or name: ")

	scanner := bufio.NewScanner(cmd.InOrStdin())
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return tools.Binary{}, fmt.Errorf("read selection: %w", err)
		}
		if !isInteractive(cmd) {
			return tools.Binary{}, fmt.Errorf("select a binary: llm audio download <name>")
		}
		return tools.Binary{}, fmt.Errorf("no binary selected")
	}

	choice := strings.TrimSpace(scanner.Text())
	if choice == "" {
		return tools.Binary{}, fmt.Errorf("no binary selected")
	}

	if n, err := strconv.Atoi(choice); err == nil {
		if n < 1 || n > len(bins) {
			return tools.Binary{}, fmt.Errorf("invalid selection %d", n)
		}
		return bins[n-1], nil
	}

	bin, ok := findAudioBinary(bins, choice)
	if !ok {
		return tools.Binary{}, fmt.Errorf("unknown audio binary %q", choice)
	}
	return bin, nil
}

func isInteractive(cmd *cobra.Command) bool {
	f, ok := cmd.InOrStdin().(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func copyPiperRuntime() error {
	if err := copyEmbeddedBinary(
		"libonnxruntime.1.22.0.dylib",
		tools.PiperONNXRuntime,
		tools.PiperVersion,
	); err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}

	dest := filepath.Join(cwd, "espeak-ng-data")
	if err := copyEmbeddedTree(tools.PiperEspeakData, "piper-v1.7.0/espeak-ng-data", dest); err != nil {
		return fmt.Errorf("copy espeak-ng-data: %w", err)
	}
	fmt.Printf("espeak-ng-data copied to %s\n", dest)
	return nil
}

func init() {
	audioCmd.AddCommand(audioDownloadCmd)
}
