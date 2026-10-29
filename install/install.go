// Package install compiles inference tools from upstream source into ~/.llm/tools.
package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"llm/internal/compile"
	"llm/internal/scripts"
)

// ToolsDir returns the install directory, $HOME/.llm/tools.
// The directory is not created.
func ToolsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".llm", "tools"), nil
}

// IsInstalled reports whether name is already present in ToolsDir.
// The directory is not created. A tool is installed when its executable
// exists there. piper also needs an espeak-ng-data directory and a
// libonnxruntime dylib beside that executable.
// It returns an error when name is unknown or the install directory cannot be resolved.
func IsInstalled(name string) (bool, error) {
	if !scripts.Has(name) {
		return false, fmt.Errorf("unknown tool %q", name)
	}
	dir, err := ToolsDir()
	if err != nil {
		return false, err
	}
	ok, err := executableInstalled(filepath.Join(dir, name))
	if err != nil || !ok {
		return false, err
	}
	if name != "piper" {
		return true, nil
	}
	return piperExtrasInstalled(dir)
}

// Install compiles name from the latest upstream source into ToolsDir.
// It creates ToolsDir when that directory is missing.
// stdout and stderr receive the compiler log. A nil writer discards that stream.
// Install compiles even when IsInstalled already reports true.
func Install(ctx context.Context, name string, stdout, stderr io.Writer) error {
	if ctx == nil {
		return fmt.Errorf("nil context")
	}
	if !scripts.Has(name) {
		return fmt.Errorf("unknown tool %q", name)
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return fmt.Errorf("compiling %s requires darwin/arm64", name)
	}
	dest, err := ensureToolsDir()
	if err != nil {
		return err
	}
	return compile.Run(ctx, name, dest, stdout, stderr)
}

func executableInstalled(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat installed tool: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	return info.Mode()&0o111 != 0, nil
}

func piperExtrasInstalled(dir string) (bool, error) {
	info, err := os.Stat(filepath.Join(dir, "espeak-ng-data"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat espeak-ng-data: %w", err)
	}
	if !info.IsDir() {
		return false, nil
	}

	matches, err := filepath.Glob(filepath.Join(dir, "libonnxruntime*.dylib"))
	if err != nil {
		return false, fmt.Errorf("find onnx runtime: %w", err)
	}
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return false, fmt.Errorf("stat onnx runtime: %w", err)
		}
		if info.Mode().IsRegular() {
			return true, nil
		}
	}
	return false, nil
}

func ensureToolsDir() (string, error) {
	dir, err := ToolsDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create tools directory: %w", err)
	}
	return dir, nil
}
