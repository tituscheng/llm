package gui

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"llm/internal/install"
	"llm/internal/scripts"
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

func TestStartScriptOpensModal(t *testing.T) {
	orig := runScript
	done := make(chan struct{})
	runScript = func(ctx context.Context, name, dest string, stdout, stderr io.Writer) error {
		if _, err := io.WriteString(stdout, "cloning "+name+"\n"); err != nil {
			return err
		}
		close(done)
		return nil
	}
	t.Cleanup(func() { runScript = orig })

	if !scripts.Has("llama-server") {
		t.Fatal("llama-server script missing")
	}
	ui := newUI(nil, nil, t.TempDir())
	ui.category.Value = "text"
	ui.binary.Value = "llama-server"
	ui.startScript()

	ui.mu.Lock()
	shown := ui.showModal
	busy := ui.busy
	title := ui.modalTitle
	ui.mu.Unlock()
	if !shown {
		t.Fatal("modal not shown")
	}
	if !busy {
		t.Fatal("expected busy")
	}
	if title != "Install Script — llama-server" {
		t.Fatalf("title=%q", title)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("script runner was not called")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ui.mu.Lock()
		busy = ui.busy
		kind := ui.kind
		ui.mu.Unlock()
		if !busy && kind == statusOK {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("script did not finish")
}

func TestLineWriter(t *testing.T) {
	var got []string
	w := &lineWriter{append: func(s string) { got = append(got, s) }}
	if _, err := w.Write([]byte("hello\nwor")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("ld\rprogress\n")); err != nil {
		t.Fatal(err)
	}
	w.flush()
	want := []string{"hello", "world", "progress"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAppendCapped(t *testing.T) {
	lines := make([]string, maxLogLines)
	got := appendCapped(lines, "tail")
	if len(got) != maxLogLines {
		t.Fatalf("len=%d", len(got))
	}
	if got[len(got)-1] != "tail" {
		t.Fatalf("last=%q", got[len(got)-1])
	}
}
