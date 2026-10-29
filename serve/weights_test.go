package serve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestDownloadWeights(t *testing.T) {
	var mu sync.Mutex
	var body []byte
	var unload []byte
	var deleted string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/models":
			switch r.Method {
			case http.MethodPost:
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read download: %v", err)
				}
				mu.Lock()
				body = append([]byte(nil), raw...)
				mu.Unlock()
				_, _ = w.Write([]byte(`{"success":true}`))
			case http.MethodDelete:
				mu.Lock()
				deleted = r.URL.Query().Get("model")
				mu.Unlock()
				_, _ = w.Write([]byte(`{"success":true}`))
			default:
				http.NotFound(w, r)
			}
		case "/models/unload":
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read unload: %v", err)
			}
			mu.Lock()
			unload = append([]byte(nil), raw...)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	const model = "ggml-org/gemma-3-4b-it-GGUF:Q4_K_M"

	if err := server.Download(context.Background(), model, nil); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got := append([]byte(nil), body...)
	mu.Unlock()
	var decoded map[string]any
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["model"] != model {
		t.Fatalf("model %#v", decoded["model"])
	}
	if _, ok := decoded["sidecars"]; ok {
		t.Fatalf("sidecars present: %s", got)
	}

	if err := server.Download(context.Background(), model, []string{SidecarMMProj, SidecarMTP}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got = append([]byte(nil), body...)
	mu.Unlock()
	var withSidecars struct {
		Sidecars []string `json:"sidecars"`
	}
	if err := json.Unmarshal(got, &withSidecars); err != nil {
		t.Fatal(err)
	}
	if len(withSidecars.Sidecars) != 2 || withSidecars.Sidecars[0] != SidecarMMProj || withSidecars.Sidecars[1] != SidecarMTP {
		t.Fatalf("sidecars %v", withSidecars.Sidecars)
	}

	if err := server.PauseDownload(context.Background(), model); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got = append([]byte(nil), unload...)
	mu.Unlock()
	if !strings.Contains(string(got), model) {
		t.Fatalf("unload %s", got)
	}

	if err := server.RemoveDownload(context.Background(), model); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if deleted != model {
		t.Fatalf("deleted %q", deleted)
	}
	mu.Unlock()
}

func TestDownloadEvents(t *testing.T) {
	const body = "" +
		": ping\n\n" +
		"data: {\"model\":\"org/repo:Q4_K_M\",\"event\":\"download_progress\",\"data\":{\"https://b\":{\"done\":2,\"total\":4},\"https://a\":{\"done\":1,\"total\":3}}}\n\n" +
		"data: {\"model\":\"org/repo:Q4_K_M\",\"event\":\"model_status\",\"data\":{\"status\":\"downloading\",\"progress\":{\"https://a\":{\"done\":3,\"total\":3}}}}\n\n" +
		"data: {\"model\":\"org/repo:Q4_K_M\",\"event\":\"download_failed\",\"data\":{\"error\":{\"message\":\"model validation failed, unable to download\"}}}\n\n" +
		"data: {\"model\":\"org/repo:Q4_K_M\",\"event\":\"download_finished\"}\n\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/models/sse":
			if r.Header.Get("Accept") != "text/event-stream" {
				t.Errorf("accept %q", r.Header.Get("Accept"))
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}

	var events []DownloadEvent
	for event, err := range server.DownloadEvents(context.Background()) {
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	if len(events) != 4 {
		t.Fatalf("events %d", len(events))
	}
	if events[0].Event != "download_progress" || len(events[0].Weights) != 2 {
		t.Fatalf("progress %+v", events[0])
	}
	if events[0].Weights[0].URL != "https://a" || events[0].Weights[0].Done != 1 || events[0].Weights[0].Total != 3 {
		t.Fatalf("first weight %+v", events[0].Weights[0])
	}
	if events[0].Weights[1].URL != "https://b" || events[0].Weights[1].Done != 2 || events[0].Weights[1].Total != 4 {
		t.Fatalf("second weight %+v", events[0].Weights[1])
	}
	if events[1].Status != "downloading" || len(events[1].Weights) != 1 || events[1].Weights[0].Done != 3 {
		t.Fatalf("status %+v", events[1])
	}
	if events[2].Event != "download_failed" || events[2].Message != "model validation failed, unable to download" {
		t.Fatalf("failed %+v", events[2])
	}
	if events[3].Event != "download_finished" || events[3].Model != "org/repo:Q4_K_M" {
		t.Fatalf("finished %+v", events[3])
	}
}

func TestModelsListsDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"org/repo:Q4_K_M","path":"/cache/model.gguf","sidecars":["mmproj"],"incomplete_sidecars":["mtp"],"status":{"value":"downloading","progress":{"https://b":{"done":2,"total":8},"https://a":{"done":1,"total":4}}}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	models, err := server.Models(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("models %d", len(models))
	}
	got := models[0]
	if got.ID != "org/repo:Q4_K_M" || got.Path != "/cache/model.gguf" || got.Status != "downloading" {
		t.Fatalf("model %+v", got)
	}
	if len(got.Sidecars) != 1 || got.Sidecars[0] != "mmproj" || len(got.IncompleteSidecars) != 1 || got.IncompleteSidecars[0] != "mtp" {
		t.Fatalf("sidecars %+v", got)
	}
	if len(got.Weights) != 2 || got.Weights[0].URL != "https://a" || got.Weights[0].Done != 1 || got.Weights[1].Total != 8 {
		t.Fatalf("weights %+v", got.Weights)
	}
}

func TestDownloadRejects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"model validation failed, unable to download"}}`))
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	ctx := context.Background()
	const model = "org/repo:Q4_K_M"

	if err := server.Download(nil, model, nil); err == nil || !strings.Contains(err.Error(), "nil context") {
		t.Fatalf("nil: %v", err)
	}
	if err := server.Download(ctx, "", nil); err == nil || !strings.Contains(err.Error(), "model name is empty") {
		t.Fatalf("empty: %v", err)
	}
	if err := server.Download(ctx, "bad\nname", nil); err == nil || !strings.Contains(err.Error(), "invalid model name") {
		t.Fatalf("newline: %v", err)
	}
	if err := server.PauseDownload(ctx, "bad\rname"); err == nil || !strings.Contains(err.Error(), "invalid model name") {
		t.Fatalf("cr: %v", err)
	}
	if err := server.Download(ctx, model, []string{"lora"}); err == nil || !strings.Contains(err.Error(), "invalid sidecar") {
		t.Fatalf("sidecar: %v", err)
	}
	if err := server.Download(ctx, model, nil); err == nil || !strings.Contains(err.Error(), "model validation failed, unable to download") {
		t.Fatalf("status: %v", err)
	}

	done := make(chan struct{})
	close(done)
	dead := &Server{baseURL: srv.URL, client: srv.Client(), done: done}
	if err := dead.Download(ctx, model, nil); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("dead: %v", err)
	}
	if _, err := dead.Models(ctx); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("models: %v", err)
	}
	var saw bool
	for _, err := range dead.DownloadEvents(ctx) {
		saw = true
		if err == nil || !strings.Contains(err.Error(), "not running") {
			t.Fatalf("events: %v", err)
		}
	}
	if !saw {
		t.Fatal("events yielded nothing")
	}
}

func TestStartRouter(t *testing.T) {
	if _, err := StartRouter(context.Background(), Options{Model: "model.gguf"}); err == nil || !strings.Contains(err.Error(), "does not load a model") {
		t.Fatalf("model: %v", err)
	}
	if _, err := StartRouter(nil, Options{}); err == nil || !strings.Contains(err.Error(), "nil context") {
		t.Fatalf("nil: %v", err)
	}
}
