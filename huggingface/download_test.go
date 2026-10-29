package huggingface

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownload(t *testing.T) {
	var hits int
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/api/models/LiquidAI/LFM3-1.7B/tree/main", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") == "" && r.URL.Query().Get("recursive") != "true" {
			t.Errorf("recursive %q", r.URL.Query().Get("recursive"))
		}
		if r.URL.Query().Get("cursor") == "" {
			w.Header().Set("Link", `<`+srv.URL+`/api/models/LiquidAI/LFM3-1.7B/tree/main?cursor=2>; rel="next"`)
			_, _ = w.Write([]byte(`[
				{"type":"directory","path":"assets","size":0},
				{"type":"file","path":"config.json","size":5}
			]`))
			return
		}
		_, _ = w.Write([]byte(`[
			{"type":"file","path":"assets/banner.txt","size":3,"lfs":{"size":3}},
			{"type":"file","path":"config.json","size":5}
		]`))
	})
	mux.HandleFunc("/LiquidAI/LFM3-1.7B/resolve/main/", func(w http.ResponseWriter, r *http.Request) {
		hits++
		switch strings.TrimPrefix(r.URL.Path, "/LiquidAI/LFM3-1.7B/resolve/main/") {
		case "config.json":
			_, _ = w.Write([]byte("hello"))
		case "assets/banner.txt":
			_, _ = w.Write([]byte("img"))
		default:
			http.NotFound(w, r)
		}
	})

	dest := t.TempDir()
	model := Model{ID: "LiquidAI/LFM3-1.7B", hub: &hub{baseURL: srv.URL, http: srv.Client()}}
	got, err := model.Download(context.Background(), dest)
	if err != nil {
		t.Fatal(err)
	}
	if got != dest {
		t.Fatalf("Download dest = %q, want %q", got, dest)
	}
	assertFile(t, filepath.Join(dest, "config.json"), "hello")
	assertFile(t, filepath.Join(dest, "assets", "banner.txt"), "img")
	if hits != 2 {
		t.Fatalf("download requests = %d, want 2", hits)
	}

	if _, err := model.Download(context.Background(), dest); err != nil {
		t.Fatal(err)
	}
	if hits != 2 {
		t.Fatalf("second download requests = %d, want 2", hits)
	}
}

func TestDownloadRejectsTraversal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"type":"file","path":"../outside.txt","size":1}]`))
	}))
	t.Cleanup(srv.Close)

	dest := t.TempDir()
	_, err := Model{
		ID:  "LiquidAI/LFM3-1.7B",
		hub: &hub{baseURL: srv.URL, http: srv.Client()},
	}.Download(context.Background(), dest)
	if err == nil {
		t.Fatal("path traversal returned nil error")
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(dest), "outside.txt")); !os.IsNotExist(statErr) {
		t.Fatal("wrote a file outside dest")
	}
}

func TestIsDownloaded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/resolve/") {
			t.Errorf("resolve request %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{"type":"file","path":"config.json","size":5},
			{"type":"file","path":"model.safetensors","size":4}
		]`))
	}))
	t.Cleanup(srv.Close)

	model := Model{ID: "LiquidAI/LFM3-1.7B", hub: &hub{baseURL: srv.URL, http: srv.Client()}}
	dest := t.TempDir()
	assertDownloaded(t, model, dest, false)

	writeFile(t, filepath.Join(dest, "config.json"), "hello")
	assertDownloaded(t, model, dest, false)

	writeFile(t, filepath.Join(dest, "model.safetensors"), "data")
	assertDownloaded(t, model, dest, true)

	writeFile(t, filepath.Join(dest, "model.safetensors"), "x")
	assertDownloaded(t, model, dest, false)
}

func TestIsDownloadedDefaultDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"type":"file","path":"config.json","size":2}]`))
	}))
	t.Cleanup(srv.Close)

	model := Model{ID: "LiquidAI/LFM3-1.7B", hub: &hub{baseURL: srv.URL, http: srv.Client()}}
	assertDownloaded(t, model, "", false)
	root := filepath.Join(home, ".llm", "models")
	if _, err := os.Stat(root); err == nil {
		t.Fatal("IsDownloaded created the models directory")
	}

	dir := filepath.Join(root, "LiquidAI", "LFM3-1.7B")
	writeFile(t, filepath.Join(dir, "config.json"), "ok")
	assertDownloaded(t, model, "", true)
}

func TestIsDownloadedRejects(t *testing.T) {
	model := Model{ID: "LiquidAI/LFM3-1.7B"}
	if _, err := model.IsDownloaded(nil, t.TempDir()); err == nil {
		t.Fatal("nil context returned nil error")
	}
	model.ID = "../secret"
	if _, err := model.IsDownloaded(context.Background(), t.TempDir()); err == nil {
		t.Fatal("invalid id returned nil error")
	}
	model.ID = "LiquidAI/LFM3-1.7B"
	model.Revision = "refs/pr/1"
	if _, err := model.IsDownloaded(context.Background(), t.TempDir()); err == nil {
		t.Fatal("invalid revision returned nil error")
	}
}

func TestDownloadDefaultDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models/LiquidAI/LFM3-1.7B/tree/main":
			_, _ = w.Write([]byte(`[{"type":"file","path":"config.json","size":2}]`))
		case "/LiquidAI/LFM3-1.7B/resolve/main/config.json":
			_, _ = w.Write([]byte("ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	dir, err := Model{
		ID:  "LiquidAI/LFM3-1.7B",
		hub: &hub{baseURL: srv.URL, http: srv.Client()},
	}.Download(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".llm", "models", "LiquidAI", "LFM3-1.7B")
	if dir != want {
		t.Fatalf("Download dir = %q, want %q", dir, want)
	}
	assertFile(t, filepath.Join(want, "config.json"), "ok")
}

func TestDownloadRejects(t *testing.T) {
	model := Model{ID: "LiquidAI/LFM3-1.7B"}
	if _, err := model.Download(nil, t.TempDir()); err == nil {
		t.Fatal("nil context returned nil error")
	}
	model.ID = "../secret"
	if _, err := model.Download(context.Background(), t.TempDir()); err == nil {
		t.Fatal("invalid id returned nil error")
	}
	model.ID = "LiquidAI/LFM3-1.7B"
	model.Revision = "refs/pr/1"
	if _, err := model.Download(context.Background(), t.TempDir()); err == nil {
		t.Fatal("invalid revision returned nil error")
	}
}

func TestRedirectDropsAuthorization(t *testing.T) {
	var otherAuth string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(other.Close)

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/file" {
			if r.Header.Get("Authorization") != "Bearer secret" {
				t.Errorf("same-host authorization %q", r.Header.Get("Authorization"))
			}
			_, _ = w.Write([]byte("ok"))
			return
		}
		http.Redirect(w, r, other.URL+"/file", http.StatusFound)
	}))
	t.Cleanup(origin.Close)

	h := &hub{baseURL: origin.URL, http: newHTTPClient(), token: "secret"}
	resp, err := h.get(context.Background(), origin.URL+"/start")
	if err != nil {
		t.Fatal(err)
	}
	drain(resp.Body)
	if otherAuth != "" {
		t.Fatalf("authorization forwarded: %q", otherAuth)
	}

	resp, err = h.get(context.Background(), origin.URL+"/file")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("same-host body %q", body)
	}
}

func TestModelsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := ModelsDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".llm", "models")
	if got != want {
		t.Fatalf("ModelsDir() = %q, want %q", got, want)
	}
	if _, err := os.Stat(got); err == nil {
		t.Fatal("ModelsDir created the directory")
	}
}

func assertDownloaded(t *testing.T, model Model, dest string, want bool) {
	t.Helper()
	got, err := model.IsDownloaded(context.Background(), dest)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("IsDownloaded() = %v, want %v", got, want)
	}
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
	}
}
