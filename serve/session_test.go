package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const (
	sseLineA    = "data: {\"id\":\"c\",\"choices\":[{\"delta\":{\"content\":\"A\"}}]}\n"
	sseLineB    = "data: {\"id\":\"c\",\"choices\":[{\"delta\":{\"content\":\"B\"}}]}\n"
	sseLineDone = "data: [DONE]\n"
)

func TestKeepReuseOffset(t *testing.T) {
	body := sseLineA + "\n" + sseLineB + "\n" + sseLineDone + "\n"
	var mu sync.Mutex
	var header string
	var deletes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/chat/completions":
			if r.Method != http.MethodPost {
				t.Errorf("method %s", r.Method)
			}
			mu.Lock()
			header = r.Header.Get("X-Conversation-Id")
			mu.Unlock()
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read chat: %v", err)
			}
			var decoded struct {
				Stream bool `json:"stream"`
			}
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Errorf("decode chat: %v", err)
			}
			if !decoded.Stream {
				t.Error("stream is false")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, body)
		case "/v1/stream":
			if r.Method == http.MethodDelete {
				deletes.Add(1)
				if r.URL.Query().Get("conv_id") != "org/repo::model" {
					t.Errorf("delete conv_id %q", r.URL.Query().Get("conv_id"))
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if r.Method != http.MethodGet {
				t.Errorf("method %s", r.Method)
			}
			from := 0
			if raw := r.URL.Query().Get("from"); raw != "" {
				n, err := strconv.Atoi(raw)
				if err != nil {
					t.Errorf("from %q: %v", raw, err)
					return
				}
				from = n
			}
			if from < 0 || from > len(body) {
				http.Error(w, "bad from", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, body[from:])
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	messages := []Message{{Role: "user", Content: "hi"}}
	const id = "org/repo::model"

	plain, err := server.Stream(context.Background(), messages, Reasoning{})
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range plain.Deltas() {
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	plain.Close()
	mu.Lock()
	gotHeader := header
	mu.Unlock()
	if gotHeader != "" {
		t.Fatalf("stream header %q", gotHeader)
	}

	stream, err := server.Keep(context.Background(), id, messages, Reasoning{Control: true})
	if err != nil {
		t.Fatal(err)
	}
	if stream.ID() != id {
		t.Fatalf("id %q", stream.ID())
	}
	var content string
	stopWatch := make(chan struct{})
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(func() { close(stopWatch) }) }
	t.Cleanup(stop)
	go func() {
		for {
			select {
			case <-stopWatch:
				return
			default:
				_ = stream.Offset()
			}
		}
	}()
	for delta, err := range stream.Deltas() {
		if err != nil {
			t.Fatal(err)
		}
		content += delta.Content
		if content == "A" {
			break
		}
	}
	stop()
	if content != "A" {
		t.Fatalf("content %q", content)
	}
	if stream.Offset() != int64(len(sseLineA)) {
		t.Fatalf("partial offset %d", stream.Offset())
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if deletes.Load() != 0 {
		t.Fatalf("close issued %d shutdowns", deletes.Load())
	}
	mu.Lock()
	gotHeader = header
	mu.Unlock()
	if gotHeader != id {
		t.Fatalf("keep header %q", gotHeader)
	}

	again, err := server.Reuse(context.Background(), id, stream.Offset())
	if err != nil {
		t.Fatal(err)
	}
	if again.ID() != id {
		t.Fatalf("reused id %q", again.ID())
	}
	if err := again.EndReasoning(context.Background()); err == nil || !strings.Contains(err.Error(), "reasoning control") {
		t.Fatalf("end reasoning: %v", err)
	}
	content = ""
	for delta, err := range again.Deltas() {
		if err != nil {
			t.Fatal(err)
		}
		content += delta.Content
	}
	if content != "B" {
		t.Fatalf("resumed content %q", content)
	}
	if again.Offset() != int64(len(body)-1) {
		t.Fatalf("resumed offset %d want %d", again.Offset(), len(body)-1)
	}
	again.Close()

	if err := server.Shutdown(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if err := server.Shutdown(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if deletes.Load() != 2 {
		t.Fatalf("shutdowns %d", deletes.Load())
	}
}

func TestReuseQuery(t *testing.T) {
	var mu sync.Mutex
	type call struct {
		conv string
		from string
		has  bool
	}
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/stream":
			q := r.URL.Query()
			_, has := q["from"]
			mu.Lock()
			calls = append(calls, call{conv: q.Get("conv_id"), from: q.Get("from"), has: has})
			mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, sseLineDone+"\n")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}

	first, err := server.Reuse(context.Background(), "conv/1", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range first.Deltas() {
		if err != nil {
			t.Fatal(err)
		}
	}
	if first.Offset() != int64(len(sseLineDone)) {
		t.Fatalf("offset %d", first.Offset())
	}
	first.Close()

	second, err := server.Reuse(context.Background(), "conv/1", 12)
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range second.Deltas() {
		if err != nil {
			t.Fatal(err)
		}
	}
	if second.Offset() != 12+int64(len(sseLineDone)) {
		t.Fatalf("offset %d", second.Offset())
	}
	second.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("calls %d", len(calls))
	}
	if calls[0].conv != "conv/1" || calls[0].has {
		t.Fatalf("first query %+v", calls[0])
	}
	if calls[1].conv != "conv/1" || !calls[1].has || calls[1].from != "12" {
		t.Fatalf("second query %+v", calls[1])
	}
}

func TestReuseNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"Stream not found or expired"}}`))
		}
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	_, err := server.Reuse(context.Background(), "missing", 0)
	if err == nil || !strings.Contains(err.Error(), "Stream not found or expired") {
		t.Fatalf("reuse: %v", err)
	}
}

func TestReuseThinkReset(t *testing.T) {
	line := "data: {\"id\":\"c\",\"choices\":[{\"delta\":{\"content\":\"secret</think>Hi\"}}]}\n"
	body := line + "\n" + sseLineDone + "\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, body)
		}
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	stream, err := server.Reuse(context.Background(), "chat-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var content, reasoning string
	for delta, err := range stream.Deltas() {
		if err != nil {
			t.Fatal(err)
		}
		content += delta.Content
		reasoning += delta.Reasoning
	}
	if content != "secret</think>Hi" || reasoning != "" {
		t.Fatalf("content %q reasoning %q", content, reasoning)
	}
}

func TestSessionRejects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	messages := []Message{{Role: "user", Content: "hi"}}
	ctx := context.Background()

	if _, err := server.Keep(nil, "chat-1", messages, Reasoning{}); err == nil || !strings.Contains(err.Error(), "nil context") {
		t.Fatalf("keep nil: %v", err)
	}
	if _, err := server.Reuse(nil, "chat-1", 0); err == nil || !strings.Contains(err.Error(), "nil context") {
		t.Fatalf("reuse nil: %v", err)
	}
	if err := server.Shutdown(nil, "chat-1"); err == nil || !strings.Contains(err.Error(), "nil context") {
		t.Fatalf("shutdown nil: %v", err)
	}
	if _, err := server.Lookup(nil, nil); err == nil || !strings.Contains(err.Error(), "nil context") {
		t.Fatalf("lookup nil: %v", err)
	}
	if _, err := server.Keep(ctx, "", messages, Reasoning{}); err == nil || !strings.Contains(err.Error(), "conversation id is empty") {
		t.Fatalf("empty: %v", err)
	}
	if _, err := server.Reuse(ctx, "bad\nid", 0); err == nil || !strings.Contains(err.Error(), "invalid conversation id") {
		t.Fatalf("newline: %v", err)
	}
	if err := server.Shutdown(ctx, "bad\rid"); err == nil || !strings.Contains(err.Error(), "invalid conversation id") {
		t.Fatalf("cr: %v", err)
	}
	if _, err := server.Reuse(ctx, "chat-1", -1); err == nil || !strings.Contains(err.Error(), "invalid stream offset") {
		t.Fatalf("offset: %v", err)
	}
	if _, err := server.Lookup(ctx, []string{"bad\nid"}); err == nil || !strings.Contains(err.Error(), "invalid conversation id") {
		t.Fatalf("lookup id: %v", err)
	}

	done := make(chan struct{})
	close(done)
	dead := &Server{baseURL: srv.URL, client: srv.Client(), done: done}
	if _, err := dead.Keep(ctx, "chat-1", messages, Reasoning{}); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("keep dead: %v", err)
	}
	if _, err := dead.Reuse(ctx, "chat-1", 0); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("reuse dead: %v", err)
	}
	if err := dead.Shutdown(ctx, "chat-1"); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("shutdown dead: %v", err)
	}
	if _, err := dead.Lookup(ctx, []string{"chat-1"}); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("lookup dead: %v", err)
	}
}

func TestLookup(t *testing.T) {
	var mu sync.Mutex
	var gotBody []byte
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/streams/lookup":
			hits.Add(1)
			if r.Method != http.MethodPost {
				t.Errorf("method %s", r.Method)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read lookup: %v", err)
			}
			mu.Lock()
			gotBody = append([]byte(nil), body...)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"conversation_id":"conv/1::model","is_done":true,"total_bytes":40,"started_at":10,"completed_at":20}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}

	if sessions, err := server.Lookup(context.Background(), nil); err != nil || sessions != nil {
		t.Fatalf("nil ids: %v %v", sessions, err)
	}
	if sessions, err := server.Lookup(context.Background(), []string{}); err != nil || sessions != nil {
		t.Fatalf("empty ids: %v %v", sessions, err)
	}
	if hits.Load() != 0 {
		t.Fatalf("hits %d", hits.Load())
	}

	sessions, err := server.Lookup(context.Background(), []string{"conv/1", "other"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions %d", len(sessions))
	}
	got := sessions[0]
	if got.ID != "conv/1::model" || !got.Done || got.TotalBytes != 40 || got.StartedAt != 10 || got.CompletedAt != 20 {
		t.Fatalf("session %+v", got)
	}
	mu.Lock()
	defer mu.Unlock()
	var decoded struct {
		IDs []string `json:"conversation_ids"`
	}
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.IDs) != 2 || decoded.IDs[0] != "conv/1" || decoded.IDs[1] != "other" {
		t.Fatalf("ids %v", decoded.IDs)
	}
}

func TestReadSSELineCountsConsumedBytes(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("data: hi\r\n\n"))
	line, n, err := readSSELine(reader)
	if err != nil || line != "data: hi" || n != len("data: hi\r\n") {
		t.Fatalf("line %q n %d err %v", line, n, err)
	}
	line, n, err = readSSELine(reader)
	if err != nil || line != "" || n != 1 {
		t.Fatalf("blank %q n %d err %v", line, n, err)
	}

	over := strings.Repeat("x", maxSSELine+1)
	line, n, err = readSSELine(bufio.NewReader(strings.NewReader(over)))
	if err == nil || !strings.Contains(err.Error(), "too long") || n != maxSSELine+1 || line != "" {
		t.Fatalf("long line %q n %d err %v", line, n, err)
	}
}
