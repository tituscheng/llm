package serve

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStartValidation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	model := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(model, []byte("gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "not-a-file"), 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "llama-server")
	if err := os.WriteFile(bin, []byte("#!/bin/bash\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		opts Options
		ctx  context.Context
	}{
		{name: "nil context", opts: Options{Model: model}, ctx: nil},
		{name: "empty model", opts: Options{}, ctx: context.Background()},
		{name: "missing model", opts: Options{Model: filepath.Join(t.TempDir(), "missing.gguf")}, ctx: context.Background()},
		{name: "model is directory", opts: Options{Model: filepath.Join(dir, "not-a-file")}, ctx: context.Background()},
		{name: "llama-server is not installed", opts: Options{Model: model}, ctx: context.Background()},
		{name: "bin is not executable", opts: Options{Model: model, Bin: bin}, ctx: context.Background()},
		{name: "invalid port", opts: Options{Model: model, Bin: bin, Port: -1}, ctx: context.Background()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Start(tt.ctx, tt.opts)
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestStartStopAndAlive(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	bin := writeFile(t, filepath.Join(t.TempDir(), "llama-server"), helperScript(python), 0o755)
	model := writeFile(t, filepath.Join(t.TempDir(), "model.gguf"), "gguf", 0o644)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := Start(ctx, Options{Model: model, Bin: bin, Context: 2048})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	if srv.URL() == "" {
		t.Fatal("empty server url")
	}

	alive, err := srv.IsAlive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !alive {
		t.Fatal("server is not alive")
	}
	reply, err := srv.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if reply != "echo hello" {
		t.Fatalf("reply %q", reply)
	}
	var streamed string
	for token, err := range srv.ChatStream(context.Background(), []Message{{Role: "user", Content: "hello"}}) {
		if err != nil {
			t.Fatal(err)
		}
		streamed += token
	}
	if streamed != "hello" {
		t.Fatalf("stream %q", streamed)
	}

	if err := srv.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := srv.Stop(); err != nil {
		t.Fatal("second Stop")
	}
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	alive, err = srv.IsAlive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if alive {
		t.Fatal("server is alive after Stop")
	}
	if _, err := srv.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}}); err == nil {
		t.Fatal("chat after Stop returned nil error")
	}
}

func TestStartRouterOmitsModel(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	bin := writeFile(t, filepath.Join(t.TempDir(), "llama-server"), helperScript(python), 0o755)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := StartRouter(ctx, Options{Bin: bin, Context: 2048})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	resp, err := srv.httpClient().Get(srv.URL() + "/argv")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	if err := json.Unmarshal(raw, &args); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(args, "-m") {
		t.Fatalf("args include a model: %v", args)
	}
	if !slices.Contains(args, "--ctx-size") || !slices.Contains(args, "2048") {
		t.Fatalf("args %v", args)
	}
}

func TestStartCancelKillsChild(t *testing.T) {
	binPath := filepath.Join(t.TempDir(), "llama-server")
	script := "#!/bin/bash\nsleep 60 &\necho $! > " + shellQuote(binPath+".child") + "\nwait\n"
	writeFile(t, binPath, script, 0o755)
	model := writeFile(t, filepath.Join(t.TempDir(), "model.gguf"), "gguf", 0o644)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := Start(ctx, Options{Model: model, Bin: binPath})
		done <- err
	}()

	pidPath := binPath + ".child"
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
		t.Fatal("Start did not return after cancel")
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

func TestStartProcessExits(t *testing.T) {
	bin := writeFile(t, filepath.Join(t.TempDir(), "llama-server"), "#!/bin/bash\necho boom >&2\nexit 1\n", 0o755)
	model := writeFile(t, filepath.Join(t.TempDir(), "model.gguf"), "gguf", 0o644)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := Start(ctx, Options{Model: model, Bin: bin})
	if err == nil {
		t.Fatal("expected exit error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error %q does not include stderr", err)
	}
}

func TestIsAliveHealth(t *testing.T) {
	var status int
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{name: "ready", status: http.StatusOK, body: `{"status":"ok"}`, want: true},
		{name: "loading", status: http.StatusServiceUnavailable, body: `{"status":"loading"}`, want: false},
		{name: "ok status field loading on 200", status: http.StatusOK, body: `{"status":"loading"}`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status = tt.status
			body = tt.body
			got, err := server.IsAlive(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("IsAlive() = %v, want %v", got, tt.want)
			}
		})
	}

	close(server.done)
	got, err := server.IsAlive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Fatal("stopped server is alive")
	}
	if _, err := server.IsAlive(nil); err == nil {
		t.Fatal("nil context returned nil error")
	}
}

func helperScript(python string) string {
	return "#!" + python + `
import json, sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

args = sys.argv[1:]
host = "127.0.0.1"
port = 0
i = 0
while i < len(args):
    if args[i] == "--host" and i + 1 < len(args):
        host = args[i + 1]
        i += 2
        continue
    if args[i] == "--port" and i + 1 < len(args):
        port = int(args[i + 1])
        i += 2
        continue
    i += 1

class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def do_GET(self):
        path = self.path.split("?", 1)[0]
        if path == "/argv":
            body = json.dumps(args).encode()
        elif path == "/health":
            body = b'{"status":"ok"}'
        else:
            self.send_error(404)
            return
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        path = self.path.split("?", 1)[0]
        length = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(length)
        if path != "/v1/chat/completions":
            self.send_error(404)
            return
        req = json.loads(raw)
        message = req["messages"][-1]["content"]
        if req.get("stream"):
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()
            for part in (message[:2], message[2:]):
                if not part:
                    continue
                chunk = json.dumps({"choices": [{"delta": {"content": part}}]})
                self.wfile.write(("data: " + chunk + "\n\n").encode())
                self.wfile.flush()
            self.wfile.write(b"data: [DONE]\n\n")
            self.wfile.flush()
            return
        body = json.dumps({"choices": [{"message": {"role": "assistant", "content": "echo " + message}}]}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, fmt, *args):
        return

ThreadingHTTPServer((host, port), Handler).serve_forever()
`
}

func writeFile(t *testing.T, path, data string, mode os.FileMode) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
