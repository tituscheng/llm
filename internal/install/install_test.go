package install

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"llm/internal/tools"
)

func TestToDirCopiesBinary(t *testing.T) {
	dir := t.TempDir()
	bin := tools.Binary{
		Name:    "demo",
		Version: "v1",
		Data:    []byte("payload"),
	}

	res, err := ToDir(dir, bin)
	if err != nil {
		t.Fatalf("ToDir: %v", err)
	}
	if len(res.Files) != 1 || !res.Files[0].Copied {
		t.Fatalf("unexpected result: %+v", res.Files)
	}

	got, err := os.ReadFile(filepath.Join(dir, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "payload" {
		t.Fatalf("got %q, want payload", got)
	}

	info, err := os.Stat(filepath.Join(dir, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("binary is not executable: %v", info.Mode())
	}
}

func TestToDirSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := ToDir(dir, tools.Binary{Name: "demo", Data: []byte("new")})
	if err != nil {
		t.Fatalf("ToDir: %v", err)
	}
	if res.Files[0].Copied {
		t.Fatal("expected existing file to be skipped")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("got %q, want old", got)
	}
}

func TestToDirCopiesSidecars(t *testing.T) {
	dir := t.TempDir()
	fsys := fstest.MapFS{
		"data/a.txt": {Data: []byte("a")},
		"data/b.txt": {Data: []byte("b")},
	}
	bin := tools.Binary{
		Name: "piper",
		Data: []byte("bin"),
		ExtraFiles: []tools.ExtraFile{
			{Name: "libonnxruntime.1.22.0.dylib", Data: []byte("onnx")},
		},
		ExtraDirs: []tools.ExtraDir{
			{Name: "espeak-ng-data", FS: fsys, Root: "data"},
		},
	}

	res, err := ToDir(dir, bin)
	if err != nil {
		t.Fatalf("ToDir: %v", err)
	}
	if len(res.Files) != 3 {
		t.Fatalf("got %d files, want 3", len(res.Files))
	}

	if _, err := os.Stat(filepath.Join(dir, "libonnxruntime.1.22.0.dylib")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "espeak-ng-data", "a.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestToDirRejectsEmptyBinary(t *testing.T) {
	if _, err := ToDir(t.TempDir(), tools.Binary{Name: "x"}); err == nil {
		t.Fatal("expected error")
	}
}
