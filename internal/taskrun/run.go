// Package taskrun executes embedded compile Taskfiles with go-task.
package taskrun

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/go-task/task/v3"
	"github.com/go-task/task/v3/taskfile/ast"

	"llm/internal/scripts"
)

// Run compiles the named binary from latest upstream source into dest.
// stdout and stderr receive Task and compiler output.
func Run(ctx context.Context, name, dest string, stdout, stderr io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
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

	yaml, err := scripts.File(name)
	if err != nil {
		return err
	}

	work, err := os.MkdirTemp("", "llm-"+safeName(name)+"-*")
	if err != nil {
		return fmt.Errorf("create work directory: %w", err)
	}
	defer os.RemoveAll(work)

	taskfilePath := filepath.Join(work, "Taskfile.yaml")
	if err := os.WriteFile(taskfilePath, yaml, 0o644); err != nil {
		return fmt.Errorf("write taskfile: %w", err)
	}

	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	vars := ast.NewVars()
	vars.Set("DEST", ast.Var{Value: dest})
	vars.Set("WORK", ast.Var{Value: work})

	ex := task.NewExecutor(
		task.WithDir(work),
		task.WithEntrypoint(taskfilePath),
		task.WithStdout(stdout),
		task.WithStderr(stderr),
		task.WithForce(true),
		task.WithSilent(false),
		task.WithColor(false),
		task.WithAssumeYes(true),
		task.WithInteractive(false),
		task.WithVersionCheck(true),
	)
	if err := ex.Setup(); err != nil {
		return fmt.Errorf("task setup: %w", err)
	}
	if err := ex.Run(ctx, &task.Call{Task: "default", Vars: vars}); err != nil {
		return fmt.Errorf("compile %s: %w", name, err)
	}
	return nil
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
