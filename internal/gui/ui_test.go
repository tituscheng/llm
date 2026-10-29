package gui

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"llm/install"
)

func TestSyncBinarySelectsFirstInCategory(t *testing.T) {
	ui := &ui{cats: install.Categories()}
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

func TestStartInstallOpensModal(t *testing.T) {
	orig := runInstall
	done := make(chan struct{})
	runInstall = func(ctx context.Context, name string, stdout, stderr io.Writer) error {
		if _, err := io.WriteString(stdout, "cloning "+name+"\n"); err != nil {
			return err
		}
		close(done)
		return nil
	}
	t.Cleanup(func() { runInstall = orig })

	ui := newUI(nil, nil, t.TempDir())
	ui.category.Value = "text"
	ui.binary.Value = "llama-server"
	ui.startInstall()

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
	if title != "Install — llama-server" {
		t.Fatalf("title=%q", title)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("installer was not called")
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
	t.Fatal("install did not finish")
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
