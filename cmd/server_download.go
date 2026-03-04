package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"llm/internal/tools"
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download llama-server binary",
	Long:  "Copies the embedded llama-server binary into the current directory.",
	RunE:  runDownload,
}

func runDownload(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}
	binaryPath := filepath.Join(cwd, "llama-server")

	if _, err := os.Stat(binaryPath); err == nil {
		fmt.Printf("llama-server already exists at %s\n", binaryPath)
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to check existing llama-server binary: %w", err)
	}

	if len(tools.LlamaServerBinary) == 0 {
		return fmt.Errorf("embedded llama-server binary is empty")
	}

	if err := writeFileAtomically(binaryPath, tools.LlamaServerBinary, 0o755); err != nil {
		return fmt.Errorf("failed to write llama-server binary: %w", err)
	}

	fmt.Printf("llama-server %s copied to %s\n", tools.LlamaServerVersion, binaryPath)
	return nil
}

func init() {
	serverCmd.AddCommand(downloadCmd)
}

func writeFileAtomically(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".llama-server-*")
	if err != nil {
		return err
	}

	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}

	if err = os.Rename(tmpPath, path); err != nil {
		return err
	}
	return nil
}
