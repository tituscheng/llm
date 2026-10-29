package serve

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"iter"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	got, err := server.Chat(context.Background(), []Message{
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "hi"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("reply %q", got)
	}
}

func TestChatRejects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"choices":[]}`))
		}
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}

	if _, err := server.Chat(nil, []Message{{Role: "user", Content: "hi"}}); err == nil {
		t.Fatal("nil context returned nil error")
	}
	if _, err := server.Chat(context.Background(), nil); err == nil {
		t.Fatal("empty messages returned nil error")
	}
	if _, err := server.Chat(context.Background(), []Message{{Role: "tool", Content: "hi"}}); err == nil {
		t.Fatal("invalid role returned nil error")
	}
	if _, err := server.Chat(context.Background(), []Message{{Role: "user", Content: ""}}); err == nil {
		t.Fatal("empty content returned nil error")
	}

	dead := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	close(dead.done)
	if _, err := dead.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}); err == nil {
		t.Fatal("stopped server returned nil error")
	}

	if _, err := server.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}); err == nil {
		t.Fatal("bad chat status returned nil error")
	}
}

func TestChatStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		var req struct {
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if !req.Stream {
			t.Errorf("stream flag is false")
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("accept %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		body := "" +
			": keep-alive\n\n" +
			"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"think\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n" +
			"data: [DONE]\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"ignored\"}}]}\n\n"
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	var got []string
	for token, err := range server.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, token)
	}
	if strings.Join(got, "") != "Hello" {
		t.Fatalf("tokens %q", got)
	}
}

func TestChatStreamStopCancels(t *testing.T) {
	sawCancel := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n")
		flusher.Flush()
		<-r.Context().Done()
		close(sawCancel)
	}))
	t.Cleanup(srv.Close)

	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	for token, err := range server.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}) {
		if err != nil {
			t.Fatal(err)
		}
		if token != "Hi" {
			t.Fatalf("token %q", token)
		}
		break
	}
	select {
	case <-sawCancel:
	case <-time.After(2 * time.Second):
		t.Fatal("breaking the stream did not cancel the request")
	}
}

func TestChatStreamErrors(t *testing.T) {
	var mode string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		switch mode {
		case "status":
			w.WriteHeader(http.StatusBadGateway)
		case "event":
			_, _ = io.WriteString(w, "data: {\"error\":{\"message\":\"overloaded\"}}\n\n")
		default:
			_, _ = io.WriteString(w, "data: not-json\n\n")
		}
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}

	for _, name := range []string{"status", "event", "json"} {
		t.Run(name, func(t *testing.T) {
			mode = name
			var got error
			for _, err := range server.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}) {
				got = err
			}
			if got == nil {
				t.Fatal("expected an error")
			}
		})
	}

	if _, err := rangeZero(server.ChatStream(nil, []Message{{Role: "user", Content: "hi"}})); err == nil {
		t.Fatal("nil context returned nil error")
	}
	dead := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	close(dead.done)
	if _, err := rangeZero(dead.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}})); err == nil {
		t.Fatal("stopped server returned nil error")
	}
}

func TestChatReasoning(t *testing.T) {
	var mu sync.Mutex
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/chat/completions":
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
			}
			mu.Lock()
			body = raw
			mu.Unlock()
			_, _ = w.Write([]byte(`{"id":"chatcmpl-9","choices":[{"message":{"content":"answer","reasoning_content":"thought"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	messages := []Message{{Role: "user", Content: "hi"}}

	reply, err := server.ChatWith(context.Background(), messages, Reasoning{})
	if err != nil {
		t.Fatal(err)
	}
	if reply.ID != "chatcmpl-9" || reply.Content != "answer" || reply.Reasoning != "thought" {
		t.Fatalf("reply %+v", reply)
	}
	mu.Lock()
	zeroBody := append([]byte(nil), body...)
	mu.Unlock()
	var zero map[string]json.RawMessage
	if err := json.Unmarshal(zeroBody, &zero); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"reasoning_effort", "reasoning_format", "reasoning_control", "chat_template_kwargs", "stream", "temperature", "top_k", "top_p", "min_p", "max_tokens", "model"} {
		if _, ok := zero[key]; ok {
			t.Fatalf("zero reasoning sent %s: %s", key, zeroBody)
		}
	}

	off := false
	reply, err = server.ChatWith(context.Background(), messages, Reasoning{
		Effort:         EffortHigh,
		Format:         FormatDeepseek,
		EnableThinking: &off,
		Control:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Content != "answer" || reply.Reasoning != "thought" {
		t.Fatalf("reply %+v", reply)
	}
	mu.Lock()
	sentBody := append([]byte(nil), body...)
	mu.Unlock()
	var sent struct {
		ReasoningEffort    string `json:"reasoning_effort"`
		ReasoningFormat    string `json:"reasoning_format"`
		ReasoningControl   bool   `json:"reasoning_control"`
		ChatTemplateKwargs struct {
			EnableThinking *bool `json:"enable_thinking"`
		} `json:"chat_template_kwargs"`
	}
	if err := json.Unmarshal(sentBody, &sent); err != nil {
		t.Fatal(err)
	}
	if sent.ReasoningEffort != EffortHigh || sent.ReasoningFormat != FormatDeepseek || !sent.ReasoningControl {
		t.Fatalf("request %s", sentBody)
	}
	if sent.ChatTemplateKwargs.EnableThinking == nil || *sent.ChatTemplateKwargs.EnableThinking {
		t.Fatalf("enable_thinking %s", sentBody)
	}
}

func TestReasoningValidation(t *testing.T) {
	server := &Server{done: make(chan struct{})}
	close(server.done)
	messages := []Message{{Role: "user", Content: "hi"}}
	if _, err := server.ChatWith(context.Background(), messages, Reasoning{Format: "xml"}); err == nil || !strings.Contains(err.Error(), "invalid reasoning format") {
		t.Fatalf("format: %v", err)
	}
	if _, err := server.ChatWith(context.Background(), messages, Reasoning{Effort: "very high"}); err == nil || !strings.Contains(err.Error(), "invalid reasoning effort") {
		t.Fatalf("effort: %v", err)
	}
	if _, err := server.ChatWith(nil, messages, Reasoning{}); err == nil {
		t.Fatal("nil context returned nil error")
	}
	if _, err := server.Stream(nil, messages, Reasoning{}); err == nil {
		t.Fatal("nil stream context returned nil error")
	}
	if err := (&Stream{}).EndReasoning(nil); err == nil {
		t.Fatal("nil context returned nil error")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := (&Stream{idReady: make(chan struct{})}).EndReasoning(ctx)
	if err == nil || !strings.Contains(err.Error(), "reasoning control") {
		t.Fatalf("end reasoning: %v", err)
	}
}

func TestEndReasoning(t *testing.T) {
	var mu sync.Mutex
	var chatBody []byte
	var controlBody []byte
	started := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/chat/completions/control":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read control: %v", err)
			}
			mu.Lock()
			controlBody = body
			mu.Unlock()
			_, _ = w.Write([]byte(`{"success":true}`))
			close(release)
		case "/v1/chat/completions":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read chat: %v", err)
			}
			mu.Lock()
			chatBody = body
			mu.Unlock()
			if r.Header.Get("Accept") != "text/event-stream" {
				t.Errorf("accept %q", r.Header.Get("Accept"))
			}
			w.Header().Set("Content-Type", "text/event-stream")
			flusher := w.(http.Flusher)
			_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"delta\":{\"reasoning_content\":\"think\"}}]}\n\n")
			flusher.Flush()
			close(started)
			<-release
			_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"delta\":{\"content\":\"ans\",\"reasoning_content\":\"\"}}]}\n\n")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}

	stream, err := server.Stream(context.Background(), []Message{{Role: "user", Content: "hi"}}, Reasoning{
		Control: true,
		Effort:  EffortHigh,
		Format:  FormatDeepseek,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	errc := make(chan error, 1)
	go func() {
		<-started
		errc <- stream.EndReasoning(context.Background())
	}()

	var content, reasoning string
	for delta, err := range stream.Deltas() {
		if err != nil {
			t.Fatal(err)
		}
		content += delta.Content
		reasoning += delta.Reasoning
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	if reasoning != "think" || content != "ans" {
		t.Fatalf("reasoning %q content %q", reasoning, content)
	}
	mu.Lock()
	defer mu.Unlock()
	var sent struct {
		Stream           bool   `json:"stream"`
		ReasoningControl bool   `json:"reasoning_control"`
		ReasoningEffort  string `json:"reasoning_effort"`
		ReasoningFormat  string `json:"reasoning_format"`
	}
	if err := json.Unmarshal(chatBody, &sent); err != nil {
		t.Fatal(err)
	}
	if !sent.Stream || !sent.ReasoningControl || sent.ReasoningEffort != EffortHigh || sent.ReasoningFormat != FormatDeepseek {
		t.Fatalf("chat request %s", chatBody)
	}
	var control struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	}
	if err := json.Unmarshal(controlBody, &control); err != nil {
		t.Fatal(err)
	}
	if control.ID != "chatcmpl-1" || control.Action != "reasoning_end" {
		t.Fatalf("control %s", controlBody)
	}

	var again error
	for _, err := range stream.Deltas() {
		again = err
	}
	if again == nil {
		t.Fatal("second deltas returned nil error")
	}
}

func TestEndReasoningFailure(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/chat/completions/control":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"success":false,"message":"busy"}`))
		default:
			w.Header().Set("Content-Type", "text/event-stream")
			flusher := w.(http.Flusher)
			_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-2\",\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
			flusher.Flush()
			<-release
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		}
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	stream, err := server.Stream(context.Background(), []Message{{Role: "user", Content: "hi"}}, Reasoning{Control: true})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	var endErr error
	for delta, err := range stream.Deltas() {
		if err != nil {
			t.Fatal(err)
		}
		if delta.Content != "" && endErr == nil {
			endErr = stream.EndReasoning(context.Background())
			close(release)
		}
	}
	if endErr == nil || !strings.Contains(endErr.Error(), "status 409") {
		t.Fatalf("end reasoning: %v", endErr)
	}
}

func TestEndReasoningRejected(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/chat/completions/control":
			_, _ = w.Write([]byte(`{"success":false,"message":"no active completion"}`))
		default:
			w.Header().Set("Content-Type", "text/event-stream")
			flusher := w.(http.Flusher)
			_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-4\",\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
			flusher.Flush()
			<-release
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		}
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	stream, err := server.Stream(context.Background(), []Message{{Role: "user", Content: "hi"}}, Reasoning{Control: true})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	var endErr error
	for _, err := range stream.Deltas() {
		if err != nil {
			t.Fatal(err)
		}
		if endErr == nil {
			endErr = stream.EndReasoning(context.Background())
			close(release)
		}
	}
	if endErr == nil || !strings.Contains(endErr.Error(), "no active completion") {
		t.Fatalf("end reasoning: %v", endErr)
	}
}

func TestEndReasoningMissingID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	stream, err := server.Stream(context.Background(), []Message{{Role: "user", Content: "hi"}}, Reasoning{Control: true})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	for _, err := range stream.Deltas() {
		if err != nil {
			t.Fatal(err)
		}
	}
	err = stream.EndReasoning(context.Background())
	if err == nil || !strings.Contains(err.Error(), "chat completion id is missing") {
		t.Fatalf("end reasoning: %v", err)
	}
}

func TestStreamStopCancels(t *testing.T) {
	sawCancel := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-3\",\"choices\":[{\"delta\":{\"content\":\"Hi\",\"reasoning_content\":\"why\"}}]}\n\n")
		flusher.Flush()
		<-r.Context().Done()
		close(sawCancel)
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	stream, err := server.Stream(context.Background(), []Message{{Role: "user", Content: "hi"}}, Reasoning{})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	for delta, err := range stream.Deltas() {
		if err != nil {
			t.Fatal(err)
		}
		if delta.Content != "Hi" || delta.Reasoning != "why" {
			t.Fatalf("delta %+v", delta)
		}
		break
	}
	select {
	case <-sawCancel:
	case <-time.After(2 * time.Second):
		t.Fatal("breaking the stream did not cancel the request")
	}
}

func TestStreamParentCancel(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = io.WriteString(w, ": wait\n\n")
		flusher.Flush()
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := server.Stream(ctx, []Message{{Role: "user", Content: "hi"}}, Reasoning{})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	go func() {
		<-started
		cancel()
	}()
	var got error
	for _, err := range stream.Deltas() {
		got = err
	}
	if !errors.Is(got, context.Canceled) {
		t.Fatalf("error %v", got)
	}
}

func rangeZero(seq iter.Seq2[string, error]) (string, error) {
	for token, err := range seq {
		return token, err
	}
	return "", nil
}

func TestSamplingLimits(t *testing.T) {
	var mu sync.Mutex
	var body []byte
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/chat/completions":
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
				return
			}
			mu.Lock()
			body = append([]byte(nil), raw...)
			hits++
			mu.Unlock()
			var req struct {
				Stream bool `json:"stream"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				t.Errorf("decode body: %v", err)
				return
			}
			if req.Stream {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
				return
			}
			_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"content":"ok"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	messages := []Message{{Role: "user", Content: "hi"}}
	ctx := context.Background()

	if _, err := server.ChatWith(ctx, messages, Reasoning{}); err != nil {
		t.Fatal(err)
	}
	assertSamplingAbsent(t, bodyCopy(&mu, &body))

	temperature := 0.0
	topK := 0
	topP := 0.0
	minP := 0.0
	maxTokens := -1
	limits := Sampling{
		Temperature: &temperature,
		TopK:        &topK,
		TopP:        &topP,
		MinP:        &minP,
		MaxTokens:   &maxTokens,
	}
	reply, err := server.ChatWith(ctx, messages, Reasoning{Effort: EffortHigh}, limits)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Content != "ok" {
		t.Fatalf("reply %q", reply.Content)
	}
	sent := bodyCopy(&mu, &body)
	assertSampling(t, sent, limits)
	var withEffort struct {
		Effort string `json:"reasoning_effort"`
	}
	if err := json.Unmarshal(sent, &withEffort); err != nil {
		t.Fatal(err)
	}
	if withEffort.Effort != EffortHigh {
		t.Fatalf("request %s", sent)
	}

	if _, err := server.Chat(ctx, messages, limits); err != nil {
		t.Fatal(err)
	}
	assertSampling(t, bodyCopy(&mu, &body), limits)

	var streamed string
	for token, err := range server.ChatStream(ctx, messages, limits) {
		if err != nil {
			t.Fatal(err)
		}
		streamed += token
	}
	if streamed != "ok" {
		t.Fatalf("tokens %q", streamed)
	}
	assertSampling(t, bodyCopy(&mu, &body), limits)

	stream, err := server.Stream(ctx, messages, Reasoning{}, limits)
	if err != nil {
		t.Fatal(err)
	}
	for delta, err := range stream.Deltas() {
		if err != nil {
			t.Fatal(err)
		}
		if delta.Content != "ok" {
			t.Fatalf("delta %q", delta.Content)
		}
	}
	assertSampling(t, bodyCopy(&mu, &body), limits)

	kept, err := server.Keep(ctx, "chat-1", messages, Reasoning{}, limits)
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range kept.Deltas() {
		if err != nil {
			t.Fatal(err)
		}
	}
	assertSampling(t, bodyCopy(&mu, &body), limits)

	mu.Lock()
	before := hits
	mu.Unlock()
	nan := math.NaN()
	if _, err := server.ChatWith(ctx, messages, Reasoning{}, Sampling{Temperature: &nan}); err == nil || !strings.Contains(err.Error(), "invalid temperature") {
		t.Fatalf("temperature: %v", err)
	}
	inf := math.Inf(1)
	if _, err := server.Stream(ctx, messages, Reasoning{}, Sampling{TopP: &inf}); err == nil || !strings.Contains(err.Error(), "invalid top_p") {
		t.Fatalf("top_p: %v", err)
	}
	neg := math.Inf(-1)
	if _, err := server.Chat(ctx, messages, Sampling{MinP: &neg}); err == nil || !strings.Contains(err.Error(), "invalid min_p") {
		t.Fatalf("min_p: %v", err)
	}
	if _, err := server.ChatWith(ctx, messages, Reasoning{}, limits, limits); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("repeat: %v", err)
	}
	if _, err := server.Chat(nil, messages, limits, limits); err == nil || !strings.Contains(err.Error(), "nil context") {
		t.Fatalf("nil context: %v", err)
	}
	if _, err := server.Keep(ctx, "", messages, Reasoning{}, limits, limits); err == nil || !strings.Contains(err.Error(), "conversation id is empty") {
		t.Fatalf("empty id: %v", err)
	}
	if _, err := rangeZero(server.ChatStream(ctx, messages, limits, limits)); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("stream repeat: %v", err)
	}
	mu.Lock()
	after := hits
	mu.Unlock()
	if before != after {
		t.Fatalf("invalid sampling reached the server: %d to %d", before, after)
	}
}

func bodyCopy(mu *sync.Mutex, body *[]byte) []byte {
	mu.Lock()
	defer mu.Unlock()
	return append([]byte(nil), (*body)...)
}

func assertSamplingAbsent(t *testing.T, raw []byte) {
	t.Helper()
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"temperature", "top_k", "top_p", "min_p", "max_tokens", "model"} {
		if _, ok := body[key]; ok {
			t.Fatalf("unset sampling sent %s: %s", key, raw)
		}
	}
}

func assertSampling(t *testing.T, raw []byte, limits Sampling) {
	t.Helper()
	var body struct {
		Temperature *float64 `json:"temperature"`
		TopK        *int     `json:"top_k"`
		TopP        *float64 `json:"top_p"`
		MinP        *float64 `json:"min_p"`
		MaxTokens   *int     `json:"max_tokens"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body.Temperature == nil || *body.Temperature != *limits.Temperature ||
		body.TopK == nil || *body.TopK != *limits.TopK ||
		body.TopP == nil || *body.TopP != *limits.TopP ||
		body.MinP == nil || *body.MinP != *limits.MinP ||
		body.MaxTokens == nil || *body.MaxTokens != *limits.MaxTokens {
		t.Fatalf("request %s", raw)
	}
}

func TestChatModel(t *testing.T) {
	const (
		gemma = "ggml-org/gemma-4-12B-it-GGUF:Q4_K_M"
		qwen  = "unsloth/Qwen3.5-4B-GGUF:Q4_K_M"
	)
	var mu sync.Mutex
	var body []byte
	var control []byte
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/chat/completions":
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
				return
			}
			mu.Lock()
			body = append([]byte(nil), raw...)
			hits++
			mu.Unlock()
			var req struct {
				Stream bool `json:"stream"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				t.Errorf("decode body: %v", err)
				return
			}
			if req.Stream {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
				return
			}
			_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"content":"ok"}}]}`))
		case "/v1/chat/completions/control":
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read control: %v", err)
				return
			}
			mu.Lock()
			control = append([]byte(nil), raw...)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	messages := []Message{{Role: "user", Content: "hi"}}
	ctx := context.Background()

	if _, err := server.Chat(ctx, messages); err != nil {
		t.Fatal(err)
	}
	assertModel(t, bodyCopy(&mu, &body), "")

	cover, err := server.WithModel(gemma)
	if err != nil {
		t.Fatal(err)
	}
	application, err := server.WithModel(qwen)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cover.Chat(ctx, messages); err != nil {
		t.Fatal(err)
	}
	assertModel(t, bodyCopy(&mu, &body), gemma)

	var streamed string
	for token, err := range application.ChatStream(ctx, messages) {
		if err != nil {
			t.Fatal(err)
		}
		streamed += token
	}
	if streamed != "ok" {
		t.Fatalf("tokens %q", streamed)
	}
	assertModel(t, bodyCopy(&mu, &body), qwen)

	stream, err := cover.Stream(ctx, messages, Reasoning{Control: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range stream.Deltas() {
		if err != nil {
			t.Fatal(err)
		}
	}
	assertModel(t, bodyCopy(&mu, &body), gemma)
	if err := stream.EndReasoning(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	gotControl := append([]byte(nil), control...)
	mu.Unlock()
	var decoded struct {
		ID     string `json:"id"`
		Action string `json:"action"`
		Model  string `json:"model"`
	}
	if err := json.Unmarshal(gotControl, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ID != "chatcmpl-1" || decoded.Action != "reasoning_end" || decoded.Model != gemma {
		t.Fatalf("control %s", gotControl)
	}

	plain, err := server.WithModel("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.Chat(ctx, messages); err != nil {
		t.Fatal(err)
	}
	assertModel(t, bodyCopy(&mu, &body), "")

	if _, err := server.WithModel("bad\nname"); err == nil || !strings.Contains(err.Error(), "invalid model name") {
		t.Fatalf("newline: %v", err)
	}
	if _, err := server.WithModel("bad\rname"); err == nil || !strings.Contains(err.Error(), "invalid model name") {
		t.Fatalf("carriage return: %v", err)
	}
	if _, err := (*Server)(nil).WithModel(gemma); err == nil || !strings.Contains(err.Error(), "nil server") {
		t.Fatalf("nil server: %v", err)
	}
	mu.Lock()
	before := hits
	mu.Unlock()
	cover.model = "bad\nname"
	if _, err := cover.Chat(ctx, messages); err == nil || !strings.Contains(err.Error(), "invalid model name") {
		t.Fatalf("chat model: %v", err)
	}
	mu.Lock()
	after := hits
	mu.Unlock()
	if before != after {
		t.Fatalf("invalid model reached the server: %d to %d", before, after)
	}
}

func assertModel(t *testing.T, raw []byte, want string) {
	t.Helper()
	var body struct {
		Model *string `json:"model"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if want == "" {
		if body.Model != nil {
			t.Fatalf("model %s", raw)
		}
		return
	}
	if body.Model == nil || *body.Model != want {
		t.Fatalf("model %s", raw)
	}
}

func TestChatEmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	if _, err := server.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}); err == nil {
		t.Fatal("empty choices returned nil error")
	}
}
