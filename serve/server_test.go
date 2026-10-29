package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
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
		want string
	}{
		{name: "nil context", opts: Options{Model: model}, ctx: nil},
		{name: "empty model", opts: Options{}, ctx: context.Background()},
		{name: "missing model", opts: Options{Model: filepath.Join(t.TempDir(), "missing.gguf")}, ctx: context.Background()},
		{name: "model is directory", opts: Options{Model: filepath.Join(dir, "not-a-file")}, ctx: context.Background()},
		{name: "llama-server is not installed", opts: Options{Model: model}, ctx: context.Background()},
		{name: "bin is not executable", opts: Options{Model: model, Bin: bin}, ctx: context.Background()},
		{name: "invalid port", opts: Options{Model: model, Bin: bin, Port: -1}, ctx: context.Background()},
		{name: "invalid parallel", opts: Options{Model: model, Parallel: -1}, ctx: context.Background(), want: "invalid parallel -1"},
		{name: "invalid reasoning budget", opts: Options{Model: model, ReasoningBudget: -1}, ctx: context.Background(), want: "invalid reasoning budget -1"},
		{name: "invalid alias", opts: Options{Model: model, Alias: "bad\nname"}, ctx: context.Background(), want: "invalid alias"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Start(tt.ctx, tt.opts)
			if err == nil {
				t.Fatal("expected an error")
			}
			if tt.want != "" && err.Error() != tt.want {
				t.Fatalf("error %v", err)
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
	for _, flag := range []string{"--parallel", "--reasoning-budget", "--no-mmproj", "--alias"} {
		if slices.Contains(args, flag) {
			t.Fatalf("args include %s: %v", flag, args)
		}
	}
}

func TestStartLaunchFlags(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	bin := writeFile(t, filepath.Join(t.TempDir(), "llama-server"), helperScript(python), 0o755)
	model := writeFile(t, filepath.Join(t.TempDir(), "model.gguf"), "gguf", 0o644)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := Start(ctx, Options{
		Model:           model,
		Bin:             bin,
		Parallel:        1,
		ReasoningBudget: 1024,
		NoMMProj:        true,
		Alias:           "cover",
	})
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
	if !hasArgPair(args, "--parallel", "1") || !hasArgPair(args, "--reasoning-budget", "1024") || !hasArgPair(args, "-m", model) || !hasArgPair(args, "--alias", "cover") {
		t.Fatalf("args %v", args)
	}
	if !slices.Contains(args, "--no-mmproj") {
		t.Fatalf("args %v", args)
	}
}

func TestStartReusesListeningServer(t *testing.T) {
	model := writeFile(t, filepath.Join(t.TempDir(), "model.gguf"), "gguf", 0o644)
	tests := []struct {
		name  string
		id    string
		alias string
	}{
		{name: "file name", id: filepath.Base(model)},
		{name: "path", id: model},
		{name: "alias", id: "cover", alias: "cover"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var unloaded string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/health":
					_, _ = w.Write([]byte(`{"status":"ok"}`))
				case "/v1/models":
					_, _ = fmt.Fprintf(w, `{"data":[{"id":%q}]}`, tt.id)
				case "/v1/chat/completions":
					_, _ = w.Write([]byte(`{"id":"c","choices":[{"message":{"content":"ok"}}]}`))
				case "/models/unload":
					var body struct {
						Model string `json:"model"`
					}
					_ = json.NewDecoder(r.Body).Decode(&body)
					mu.Lock()
					unloaded = body.Model
					mu.Unlock()
					_, _ = w.Write([]byte(`{"success":true}`))
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(srv.Close)
			port := urlPort(t, srv.URL)
			got, err := Start(context.Background(), Options{
				Model: model,
				Port:  port,
				Alias: tt.alias,
				Bin:   filepath.Join(t.TempDir(), "missing-llama-server"),
			})
			if err != nil {
				t.Fatal(err)
			}
			reply, err := got.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
			if err != nil {
				t.Fatal(err)
			}
			if reply != "ok" {
				t.Fatalf("reply %q", reply)
			}
			other, err := got.WithModel("ggml-org/gemma-4-12B-it-GGUF:Q4_K_M")
			if err != nil {
				t.Fatal(err)
			}
			if err := other.Stop(); err != nil {
				t.Fatal(err)
			}
			if err := got.Stop(); err != nil {
				t.Fatal(err)
			}
			alive, err := got.IsAlive(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if alive {
				t.Fatal("reused server is alive after Stop")
			}
			mu.Lock()
			gotUnload := unloaded
			mu.Unlock()
			if gotUnload != tt.id {
				t.Fatalf("unload %q", gotUnload)
			}
			resp, err := http.Get(srv.URL + "/health")
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("listener status %d", resp.StatusCode)
			}
		})
	}
}

func TestStartRejectsADifferentListener(t *testing.T) {
	model := writeFile(t, filepath.Join(t.TempDir(), "model.gguf"), "gguf", 0o644)
	tests := []struct {
		name string
		h    http.HandlerFunc
	}{
		{name: "other model", h: func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/health":
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			case "/v1/models":
				_, _ = w.Write([]byte(`{"data":[{"id":"other"}]}`))
			default:
				http.NotFound(w, r)
			}
		}},
		{name: "not ready", h: func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "loading", http.StatusServiceUnavailable)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.h)
			t.Cleanup(srv.Close)
			port := urlPort(t, srv.URL)
			_, err := Start(context.Background(), Options{
				Model: model,
				Port:  port,
				Bin:   filepath.Join(t.TempDir(), "missing-llama-server"),
			})
			if err == nil || err.Error() != fmt.Sprintf("port %d is already in use", port) {
				t.Fatalf("error %v", err)
			}
		})
	}
}

func TestStartRouterReusesAnIdleServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	port := urlPort(t, srv.URL)
	got, err := StartRouter(context.Background(), Options{
		Port: port,
		Bin:  filepath.Join(t.TempDir(), "missing-llama-server"),
	})
	if err != nil {
		t.Fatal(err)
	}
	alive, err := got.IsAlive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !alive {
		t.Fatal("idle server is not alive")
	}
	if err := got.Stop(); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("listener status %d", resp.StatusCode)
	}
}

func TestStartRouterRejectsALoadedServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"model.gguf"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	port := urlPort(t, srv.URL)
	_, err := StartRouter(context.Background(), Options{
		Port: port,
		Bin:  filepath.Join(t.TempDir(), "missing-llama-server"),
	})
	if err == nil || err.Error() != fmt.Sprintf("port %d is already in use", port) {
		t.Fatalf("error %v", err)
	}
}

func TestStartSpawnsWhenPortIsFree(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	model := writeFile(t, filepath.Join(t.TempDir(), "model.gguf"), "gguf", 0o644)
	_, err = Start(context.Background(), Options{
		Model: model,
		Port:  port,
		Bin:   filepath.Join(t.TempDir(), "missing-llama-server"),
	})
	if err == nil || !strings.Contains(err.Error(), "llama-server") {
		t.Fatalf("error %v", err)
	}
}

func urlPort(t *testing.T, raw string) int {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func hasArgPair(args []string, name, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name && args[i+1] == value {
			return true
		}
	}
	return false
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
