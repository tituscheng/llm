package scripts

import "testing"

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
		if !Has(name) {
			t.Errorf("missing Taskfile for %s", name)
		}
		data, err := File(name)
		if err != nil {
			t.Errorf("File(%q): %v", name, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("File(%q) is empty", name)
		}
	}
	if _, err := File("missing"); err == nil {
		t.Fatal("File(missing) succeeded")
	}
	if _, err := File("../llama-server"); err == nil {
		t.Fatal("File with path succeeded")
	}
}
