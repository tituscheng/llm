package huggingface

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/models" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.URL.Query().Get("search") != "LiquidAI/LFM3-1.7B" {
			t.Errorf("search %q", r.URL.Query().Get("search"))
		}
		if r.URL.Query().Get("limit") != "20" {
			t.Errorf("limit %q", r.URL.Query().Get("limit"))
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":"LiquidAI/LFM3-1.7B","downloads":15885,"likes":466,"pipeline_tag":"text-generation","library_name":"transformers","tags":["safetensors"],"private":false,"gated":false},
			{"id":"LiquidAI/LFM3-1.7B-GGUF","author":"LiquidAI","downloads":61499,"likes":124,"pipeline_tag":"text-generation","library_name":"gguf","tags":["gguf"],"gated":"auto"}
		]`))
	}))
	t.Cleanup(srv.Close)

	models, err := search(context.Background(), &hub{
		baseURL: srv.URL,
		http:    srv.Client(),
		token:   "secret",
	}, "  LiquidAI/LFM3-1.7B  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2", len(models))
	}
	got := models[0]
	if got.ID != "LiquidAI/LFM3-1.7B" || got.Author != "LiquidAI" {
		t.Fatalf("first model = %+v", got)
	}
	if got.Downloads != 15885 || got.Likes != 466 || got.PipelineTag != "text-generation" {
		t.Fatalf("stats = %+v", got)
	}
	if got.LibraryName != "transformers" || len(got.Tags) != 1 || got.Tags[0] != "safetensors" {
		t.Fatalf("library = %+v", got)
	}
	if got.Gated || got.Private {
		t.Fatalf("flags = %+v", got)
	}
	if models[1].Gated != true || models[1].Author != "LiquidAI" {
		t.Fatalf("second model = %+v", models[1])
	}
}

func TestSearchErrors(t *testing.T) {
	_, err := search(context.Background(), defaultHub, "  ")
	if err == nil {
		t.Fatal("empty query returned nil error")
	}
	_, err = search(nil, defaultHub, "lfm")
	if err == nil {
		t.Fatal("nil context returned nil error")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	_, err = search(context.Background(), &hub{baseURL: srv.URL, http: srv.Client()}, "lfm")
	if err == nil {
		t.Fatal("forbidden search returned nil error")
	}
}

func TestGatedFlag(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "false", in: `false`, want: false},
		{name: "true", in: `true`, want: true},
		{name: "null", in: `null`, want: false},
		{name: "auto", in: `"auto"`, want: true},
		{name: "manual", in: `"manual"`, want: true},
		{name: "string false", in: `"false"`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var model apiModel
			raw := []byte(`{"id":"LiquidAI/LFM3-1.7B","gated":` + tt.in + `}`)
			if err := json.Unmarshal(raw, &model); err != nil {
				t.Fatal(err)
			}
			if bool(model.Gated) != tt.want {
				t.Fatalf("gated = %v, want %v", bool(model.Gated), tt.want)
			}
		})
	}
}
