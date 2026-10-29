// Package install compiles inference tools from upstream source into ~/.llm/tools.
package install

import (
	"context"
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

// Install compiles name from the latest upstream source into ToolsDir.
// It creates ToolsDir when that directory is missing.
// stdout and stderr receive the compiler log. A nil writer discards that stream.
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
