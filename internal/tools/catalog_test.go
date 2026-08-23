package tools

import "testing"

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
		{"text", "Text", 1},
		{"image", "Image", 1},
		{"audio", "Audio", 5},
	}
	for i, w := range want {
		if cats[i].ID != w.id || cats[i].Name != w.name {
			t.Errorf("category %d: got %s/%s, want %s/%s", i, cats[i].ID, cats[i].Name, w.id, w.name)
		}
		if len(cats[i].Binaries) != w.count {
			t.Errorf("%s: got %d binaries, want %d", w.id, len(cats[i].Binaries), w.count)
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
	if len(piper.ExtraFiles) != 1 || len(piper.ExtraDirs) != 1 {
		t.Fatalf("piper sidecars: files=%d dirs=%d", len(piper.ExtraFiles), len(piper.ExtraDirs))
	}
	if piper.Size() <= int64(len(PiperBinary)) {
		t.Fatalf("piper size %d should include sidecars", piper.Size())
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{13_300_000, "12.7 MB"},
	}
	for _, tt := range tests {
		if got := FormatSize(tt.n); got != tt.want {
			t.Errorf("FormatSize(%d)=%q, want %q", tt.n, got, tt.want)
		}
	}
}
