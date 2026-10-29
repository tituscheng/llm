package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsInstalled(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		setup   func(t *testing.T, dir string)
		want    bool
		wantErr bool
	}{
		{
			name:    "unknown tool",
			tool:    "missing-tool",
			wantErr: true,
		},
		{
			name: "missing tools directory",
			tool: "llama-server",
		},
		{
			name: "missing executable",
			tool: "llama-server",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "directory in place of executable",
			tool: "llama-server",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(dir, "llama-server"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "non-executable file",
			tool: "sd-cli",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, filepath.Join(dir, "sd-cli"), 0o644)
			},
		},
		{
			name: "executable",
			tool: "llama-server",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, filepath.Join(dir, "llama-server"), 0o755)
			},
			want: true,
		},
		{
			name: "piper executable only",
			tool: "piper",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, filepath.Join(dir, "piper"), 0o755)
			},
		},
		{
			name: "piper without onnx runtime",
			tool: "piper",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, filepath.Join(dir, "piper"), 0o755)
				if err := os.MkdirAll(filepath.Join(dir, "espeak-ng-data"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "piper espeak data is a file",
			tool: "piper",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, filepath.Join(dir, "piper"), 0o755)
				writeFile(t, filepath.Join(dir, "espeak-ng-data"), 0o644)
				writeFile(t, filepath.Join(dir, "libonnxruntime.1.22.0.dylib"), 0o755)
			},
		},
		{
			name: "piper installed",
			tool: "piper",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeFile(t, filepath.Join(dir, "piper"), 0o755)
				if err := os.MkdirAll(filepath.Join(dir, "espeak-ng-data"), 0o755); err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(dir, "libonnxruntime.1.22.0.dylib"), 0o755)
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			if tt.setup != nil {
				tt.setup(t, filepath.Join(home, ".llm", "tools"))
			}

			got, err := IsInstalled(tt.tool)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("IsInstalled: %v", err)
			}
			if got != tt.want {
				t.Fatalf("IsInstalled(%q) = %v, want %v", tt.tool, got, tt.want)
			}
		})
	}
}

func writeFile(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("bin"), mode); err != nil {
		t.Fatal(err)
	}
}

func TestToolsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir, err := ToolsDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".llm", "tools")
	if dir != want {
		t.Fatalf("ToolsDir()=%q, want %q", dir, want)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("ToolsDir created the directory")
	}

	got, err := ensureToolsDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("ensureToolsDir()=%q, want %q", got, want)
	}
	info, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", got)
	}
}
