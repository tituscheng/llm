package install

import (
	"testing"

	"llm/internal/scripts"
)

func TestCategories(t *testing.T) {
	cats := Categories()
	if len(cats) != 3 {
		t.Fatalf("got %d categories, want 3", len(cats))
	}

	want := []struct {
		id    string
		name  string
		count int
	}{
		{id: "text", name: "Text", count: 1},
		{id: "image", name: "Image", count: 1},
		{id: "audio", name: "Audio", count: 5},
	}
	for i, w := range want {
		if cats[i].ID != w.id || cats[i].Name != w.name {
			t.Errorf("category %d: got %s/%s, want %s/%s", i, cats[i].ID, cats[i].Name, w.id, w.name)
		}
		if len(cats[i].Tools) != w.count {
			t.Errorf("%s: got %d tools, want %d", w.id, len(cats[i].Tools), w.count)
		}
	}

	audio, ok := CategoryByID("audio")
	if !ok {
		t.Fatal("audio category missing")
	}
	piper, ok := audio.Find("piper")
	if !ok {
		t.Fatal("piper missing")
	}
	if piper.Note == "" {
		t.Fatal("piper note is empty")
	}
}

func TestToolsMatchScripts(t *testing.T) {
	tools := Tools()
	if len(tools) == 0 {
		t.Fatal("no tools")
	}
	seen := map[string]bool{}
	for _, tool := range tools {
		if seen[tool.Name] {
			t.Errorf("duplicate tool %s", tool.Name)
		}
		seen[tool.Name] = true
		if !scripts.Has(tool.Name) {
			t.Errorf("no install script for %s", tool.Name)
		}
	}
	for _, name := range scripts.Names() {
		if !seen[name] {
			t.Errorf("install script %s is not in Tools()", name)
		}
	}
}
