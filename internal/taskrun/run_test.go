package taskrun

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-task/task/v3"
	"github.com/go-task/task/v3/taskfile/ast"

	"llm/internal/scripts"
)

func TestRunUnknown(t *testing.T) {
	err := Run(context.Background(), "nope", t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunRequiresDirectory(t *testing.T) {
	err := Run(context.Background(), "llama-server", "", nil, nil)
	if err == nil {
		t.Fatal("expected empty dest error")
	}

	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = Run(context.Background(), "llama-server", file, nil, nil)
	if err == nil {
		t.Fatal("expected not-a-directory error")
	}
}

func TestExecutorCleanupLeavesOnlyBinary(t *testing.T) {
	dest := t.TempDir()
	work := t.TempDir()
	taskfile := filepath.Join(work, "Taskfile.yaml")
	yaml := []byte(`version: "3"
tasks:
  default:
    cmds:
      - |
        set -euo pipefail
        trap 'rm -f "{{.WORK}}/src.txt"' EXIT
        echo leftover > "{{.WORK}}/src.txt"
        echo built > "{{.DEST}}/out.bin"
        chmod 0755 "{{.DEST}}/out.bin"
`)
	if err := os.WriteFile(taskfile, yaml, 0o644); err != nil {
		t.Fatal(err)
	}

	vars := ast.NewVars()
	vars.Set("DEST", ast.Var{Value: dest})
	vars.Set("WORK", ast.Var{Value: work})

	var out strings.Builder
	ex := task.NewExecutor(
		task.WithDir(work),
		task.WithEntrypoint(taskfile),
		task.WithStdout(&out),
		task.WithStderr(&out),
		task.WithForce(true),
		task.WithColor(false),
		task.WithAssumeYes(true),
		task.WithInteractive(false),
	)
	if err := ex.Setup(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := ex.Run(ctx, &task.Call{Task: "default", Vars: vars}); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}

	if _, err := os.Stat(filepath.Join(dest, "out.bin")); err != nil {
		t.Fatalf("binary missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "src.txt")); !os.IsNotExist(err) {
		t.Fatalf("cleanup left src.txt, err=%v", err)
	}
}

func TestEmbeddedTaskfilesSetup(t *testing.T) {
	for _, name := range []string{
		"llama-server", "sd-cli", "piper", "bark.cpp", "tts.cpp", "llama-tts", "audio.cpp",
	} {
		t.Run(name, func(t *testing.T) {
			work := t.TempDir()
			yaml, err := scripts.File(name)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(work, "Taskfile.yaml")
			if err := os.WriteFile(path, yaml, 0o644); err != nil {
				t.Fatal(err)
			}
			vars := ast.NewVars()
			vars.Set("DEST", ast.Var{Value: work})
			vars.Set("WORK", ast.Var{Value: work})
			ex := task.NewExecutor(
				task.WithDir(work),
				task.WithEntrypoint(path),
				task.WithStdout(io.Discard),
				task.WithStderr(io.Discard),
				task.WithColor(false),
				task.WithAssumeYes(true),
				task.WithInteractive(false),
			)
			if err := ex.Setup(); err != nil {
				t.Fatalf("setup: %v", err)
			}
			if _, err := ex.GetTask(&task.Call{Task: "default", Vars: vars}); err != nil {
				t.Fatalf("default task: %v", err)
			}
		})
	}
}

func TestSafeName(t *testing.T) {
	if got := safeName("bark.cpp"); got != "bark-cpp" {
		t.Fatalf("got %q", got)
	}
	if got := safeName(""); got != "bin" {
		t.Fatalf("got %q", got)
	}
}
