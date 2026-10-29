// Package compile runs an embedded install.sh with bash and deletes the work tree.
package compile

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"llm/internal/scripts"
)

// Run compiles the named tool into dest.
// stdout and stderr receive the script output. A nil writer discards that stream.
func Run(ctx context.Context, name, dest string, stdout, stderr io.Writer) error {
	script, err := scripts.File(name)
	if err != nil {
		return err
	}
	return runScript(ctx, name, script, dest, stdout, stderr)
}

func runScript(ctx context.Context, name string, script []byte, dest string, stdout, stderr io.Writer) error {
	if ctx == nil {
		return fmt.Errorf("nil context")
	}
	if dest == "" {
		return fmt.Errorf("destination is empty")
	}
	info, err := os.Stat(dest)
	if err != nil {
		return fmt.Errorf("destination: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("destination %q is not a directory", dest)
	}
	if len(script) == 0 {
		return fmt.Errorf("install script %q is empty", name)
	}

	work, err := os.MkdirTemp("", "llm-"+safeName(name)+"-*")
	if err != nil {
		return fmt.Errorf("create work directory: %w", err)
	}
	defer os.RemoveAll(work)

	scriptPath := filepath.Join(work, "install.sh")
	if err := os.WriteFile(scriptPath, script, 0o644); err != nil {
		return fmt.Errorf("write install script: %w", err)
	}

	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	cmd := exec.CommandContext(ctx, "bash", scriptPath)
	cmd.Dir = work
	cmd.Env = scriptEnv(dest, work)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	configureCommand(cmd)

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("compile %s: %w", name, ctx.Err())
		}
		return fmt.Errorf("compile %s: %w", name, err)
	}
	if ctx.Err() != nil {
		return fmt.Errorf("compile %s: %w", name, ctx.Err())
	}
	return nil
}

func scriptEnv(dest, work string) []string {
	base := os.Environ()
	env := make([]string, 0, len(base)+2)
	for _, entry := range base {
		if strings.HasPrefix(entry, "DEST=") || strings.HasPrefix(entry, "WORK=") {
			continue
		}
		env = append(env, entry)
	}
	env = append(env, "DEST="+dest, "WORK="+work)
	return env
}

func safeName(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out = append(out, r)
		default:
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "bin"
	}
	return string(out)
}
