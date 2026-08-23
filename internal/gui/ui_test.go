package gui

import (
	"testing"

	"llm/internal/install"
	"llm/internal/tools"
)

func TestSyncBinarySelectsFirstInCategory(t *testing.T) {
	ui := &ui{cats: tools.Categories()}
	ui.category.Value = "audio"
	ui.binary.Value = "llama-server"
	ui.syncBinary()
	if ui.binary.Value != "piper" {
		t.Fatalf("got %q, want piper", ui.binary.Value)
	}

	ui.category.Value = "image"
	ui.syncBinary()
	if ui.binary.Value != "sd-cli" {
		t.Fatalf("got %q, want sd-cli", ui.binary.Value)
	}
}

func TestSummarize(t *testing.T) {
	tests := []struct {
		name string
		res  install.Result
		want string
	}{
		{
			name: "single copy",
			res: install.Result{Files: []install.File{
				{Name: "llama-server", Path: "/tmp/llama-server", Copied: true},
			}},
			want: "Installed /tmp/llama-server",
		},
		{
			name: "copy with sidecars",
			res: install.Result{Files: []install.File{
				{Name: "piper", Path: "/tmp/piper", Copied: true},
				{Name: "libonnxruntime.1.22.0.dylib", Path: "/tmp/libonnxruntime.1.22.0.dylib", Copied: true},
			}},
			want: "Installed /tmp/piper and 1 sidecar(s).",
		},
		{
			name: "already exists",
			res: install.Result{Files: []install.File{
				{Name: "sd-cli", Path: "/tmp/sd-cli"},
			}},
			want: "sd-cli already exists in the working directory.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summarize(tt.res); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
