package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFile(t *testing.T) {
	want := []string{
		"llama-server",
		"sd-cli",
		"piper",
		"bark.cpp",
		"tts.cpp",
		"llama-tts",
		"audio.cpp",
	}
	names := Names()
	if len(names) != len(want) {
		t.Fatalf("Names()=%v, want %d entries", names, len(want))
	}
	seen := map[string]bool{}
	for _, name := range names {
		seen[name] = true
	}
	for _, name := range want {
		if !seen[name] {
			t.Errorf("Names() missing %s", name)
		}
	}
	for _, name := range want {
		t.Run(name, func(t *testing.T) {
			if !Has(name) {
				t.Fatalf("missing install script for %s", name)
			}
			data, err := File(name)
			if err != nil {
				t.Fatalf("File(%q): %v", name, err)
			}
			if len(data) == 0 {
				t.Fatal("script is empty")
			}
			path := filepath.Join(t.TempDir(), "install.sh")
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command("bash", "-n", path).CombinedOutput()
			if err != nil {
				t.Fatalf("bash -n: %v\n%s", err, out)
			}
		})
	}
	if _, err := File("missing"); err == nil {
		t.Fatal("File(missing) succeeded")
	}
	if _, err := File("../llama-server"); err == nil {
		t.Fatal("File with path succeeded")
	}
}
