package install

import (
	"os"
	"path/filepath"
	"testing"
)

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
