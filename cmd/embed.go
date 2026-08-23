package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func copyEmbeddedBinary(name string, data []byte, version string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}
	binaryPath := filepath.Join(cwd, name)

	if _, err := os.Stat(binaryPath); err == nil {
		fmt.Printf("%s already exists at %s\n", name, binaryPath)
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to check existing %s binary: %w", name, err)
	}

	if len(data) == 0 {
		return fmt.Errorf("embedded %s binary is empty", name)
	}

	if err := writeFileAtomically(binaryPath, data, 0o755); err != nil {
		return fmt.Errorf("failed to write %s binary: %w", name, err)
	}

	fmt.Printf("%s %s copied to %s\n", name, version, binaryPath)
	return nil
}

func writeFileAtomically(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
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
