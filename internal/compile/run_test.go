package compile

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunUnknown(t *testing.T) {
	err := Run(context.Background(), "nope", t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunRequiresDirectory(t *testing.T) {
	err := Run(context.Background(), "llama-server", "", nil, nil)
	if err == nil {
		t.Fatal("expected empty dest error")
	}

	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = Run(context.Background(), "llama-server", file, nil, nil)
	if err == nil {
		t.Fatal("expected not-a-directory error")
	}
}

func TestRunNilContext(t *testing.T) {
	err := runScript(nil, "demo", []byte("echo hi\n"), t.TempDir(), nil, nil)
	if err == nil {
		t.Fatal("expected nil context error")
	}
}

func TestRunScriptRemovesWorkTree(t *testing.T) {
	dest := t.TempDir()
	script := []byte(`#!/bin/bash
set -euo pipefail
echo leftover > "${WORK}/src.txt"
printf '%s\n' "${WORK}" > "${DEST}/work.path"
echo built > "${DEST}/out.bin"
chmod 0755 "${DEST}/out.bin"
`)
	if err := runScript(context.Background(), "demo", script, dest, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "out.bin")); err != nil {
		t.Fatalf("binary missing: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "work.path"))
	if err != nil {
		t.Fatal(err)
	}
	work := strings.TrimSpace(string(data))
	if _, err := os.Stat(work); !os.IsNotExist(err) {
		t.Fatalf("work dir %s still present, err=%v", work, err)
	}
}

func TestRunScriptCancelKillsChild(t *testing.T) {
	dest := t.TempDir()
	script := []byte(`#!/bin/bash
set -euo pipefail
sleep 60 &
echo $! > "${DEST}/child.pid"
wait
`)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runScript(ctx, "demo", script, dest, io.Discard, io.Discard)
	}()

	pidPath := filepath.Join(dest, "child.pid")
	deadline := time.Now().Add(5 * time.Second)
	var pid int
	for {
		data, err := os.ReadFile(pidPath)
		if err == nil {
			pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil && pid > 0 {
				break
			}
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("child pid was not written")
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runScript did not return after cancel")
	}

	deadline = time.Now().Add(2 * time.Second)
	for {
		err := syscall.Kill(pid, 0)
		if err != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("child %d still running", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSafeName(t *testing.T) {
	if got := safeName("bark.cpp"); got != "bark-cpp" {
		t.Fatalf("got %q", got)
	}
	if got := safeName(""); got != "bin" {
		t.Fatalf("got %q", got)
	}
}
