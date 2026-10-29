package grok

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const completed = `{"id":"chat_1","choices":[{"message":{"role":"assistant","content":"Hello"}}]}`

func TestChat(t *testing.T) {
	var body []byte
	var header http.Header
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		header = r.Header.Clone()
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		_, _ = w.Write([]byte(completed))
	})

	reply, err := client.Chat(context.Background(), Turn{
		Messages: []Message{
			{Role: "system", Content: "be brief"},
			{Role: "user", Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Content != "Hello" || reply.ID != "chat_1" {
		t.Fatalf("reply %+v", reply)
	}
	if header.Get("Authorization") != "Bearer test-key" {
		t.Fatalf("authorization %q", header.Get("Authorization"))
	}
	if header.Get("Accept") != "application/json" {
		t.Fatalf("accept %q", header.Get("Accept"))
	}
	if header.Get("User-Agent") != "llm" {
		t.Fatalf("user agent %q", header.Get("User-Agent"))
	}
	req := decodeRequest(t, body)
	if string(req["model"]) != `"grok-4.7"` {
		t.Fatalf("model %s", req["model"])
	}
	if _, ok := req["stream"]; ok {
		t.Fatal("stream was sent")
	}
	for _, key := range []string{"reasoning", "reasoning_effort", "max_tokens", "max_output_tokens", "input"} {
		if _, ok := req[key]; ok {
			t.Fatalf("%s was sent: %s", key, body)
		}
	}
	var messages []map[string]string
	if err := json.Unmarshal(req["messages"], &messages); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0]["role"] != "system" || messages[1]["content"] != "Hi" {
		t.Fatalf("messages %s", req["messages"])
	}
}

func TestChatOptions(t *testing.T) {
	var body []byte
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		_, _ = w.Write([]byte(completed))
	})
	client, err := client.WithModel("grok-4")
	if err != nil {
		t.Fatal(err)
	}
	temp := 0.0
	topP := 0.95
	maxTokens := 0
	reply, err := client.Chat(context.Background(), Turn{
		Messages: []Message{{Role: "user", Content: "Next"}},
		Effort:   EffortHigh,
	}, Sampling{Temperature: &temp, TopP: &topP, MaxTokens: &maxTokens})
	if err != nil {
		t.Fatal(err)
	}
	if reply.ID != "chat_1" {
		t.Fatalf("id %s", reply.ID)
	}
	req := decodeRequest(t, body)
	if string(req["model"]) != `"grok-4"` {
		t.Fatalf("model %s", req["model"])
	}
	if string(req["temperature"]) != "0" || string(req["top_p"]) != "0.95" || string(req["max_completion_tokens"]) != "0" {
		t.Fatalf("sampling %s", body)
	}
	if string(req["reasoning_effort"]) != `"high"` {
		t.Fatalf("effort %s", req["reasoning_effort"])
	}
	for _, key := range []string{"reasoning", "max_tokens", "max_output_tokens"} {
		if _, ok := req[key]; ok {
			t.Fatalf("%s was sent: %s", key, body)
		}
	}
}

func TestChatEffort(t *testing.T) {
	var effort string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Effort string `json:"reasoning_effort"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		effort = body.Effort
		_, _ = w.Write([]byte(completed))
	})
	_, err := client.Chat(context.Background(), Turn{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Effort:   EffortXHigh,
	})
	if err != nil {
		t.Fatal(err)
	}
	if effort != EffortXHigh {
		t.Fatalf("effort %q", effort)
	}
	_, err = client.Chat(context.Background(), Turn{
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Effort:   EffortLow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if effort != EffortLow {
		t.Fatalf("effort %q", effort)
	}
}

func TestChatRejectsBeforeRequest(t *testing.T) {
	var hits atomic.Int32
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(completed))
	})
	ctx := context.Background()
	ok := Turn{Messages: []Message{{Role: "user", Content: "Hi"}}}
	temp := math.NaN()
	inf := math.Inf(1)
	negative := -1
	limits := Sampling{Temperature: &temp}

	cases := []struct {
		name string
		ctx  context.Context
		call func() error
	}{
		{"nil client", ctx, func() error {
			var client *Client
			_, err := client.Chat(ctx, ok)
			return err
		}},
		{"nil context", nil, func() error {
			_, err := client.Chat(nil, ok)
			return err
		}},
		{"empty messages", ctx, func() error {
			_, err := client.Chat(ctx, Turn{})
			return err
		}},
		{"invalid role", ctx, func() error {
			_, err := client.Chat(ctx, Turn{Messages: []Message{{Role: "tool", Content: "Hi"}}})
			return err
		}},
		{"empty content", ctx, func() error {
			_, err := client.Chat(ctx, Turn{Messages: []Message{{Role: "user"}}})
			return err
		}},
		{"bad effort", ctx, func() error {
			_, err := client.Chat(ctx, Turn{Effort: "medium", Messages: ok.Messages})
			return err
		}},
		{"two sampling values", ctx, func() error {
			_, err := client.Chat(ctx, ok, Sampling{}, Sampling{})
			return err
		}},
		{"temperature", ctx, func() error {
			_, err := client.Chat(ctx, ok, limits)
			return err
		}},
		{"top_p", ctx, func() error {
			_, err := client.Chat(ctx, ok, Sampling{TopP: &inf})
			return err
		}},
		{"max tokens", ctx, func() error {
			_, err := client.Chat(ctx, ok, Sampling{MaxTokens: &negative})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	if hits.Load() != 0 {
		t.Fatalf("handler hits %d", hits.Load())
	}
}

func TestChatHTTPErrors(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key\nline"}}`))
	})
	_, err := client.Chat(context.Background(), Turn{Messages: []Message{{Role: "user", Content: "Hi"}}})
	if err == nil || !strings.Contains(err.Error(), "grok status 401: bad key line") {
		t.Fatalf("error %v", err)
	}
	var status *StatusError
	if !errors.As(err, &status) || status.Status != http.StatusUnauthorized {
		t.Fatalf("status %#v", status)
	}
	if strings.Contains(err.Error(), "test-key") {
		t.Fatalf("error includes the api key: %v", err)
	}

	plain := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("nope"))
	})
	_, err = plain.Chat(context.Background(), Turn{Messages: []Message{{Role: "user", Content: "Hi"}}})
	if err == nil || err.Error() != "grok status 502" {
		t.Fatalf("error %v", err)
	}
	status = nil
	if !errors.As(err, &status) || status.Status != http.StatusBadGateway {
		t.Fatalf("status %#v", status)
	}

	for _, code := range []int{http.StatusForbidden, http.StatusPaymentRequired, http.StatusTooManyRequests} {
		coded := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(code)
		})
		_, err = coded.Chat(context.Background(), Turn{Messages: []Message{{Role: "user", Content: "Hi"}}})
		status = nil
		if !errors.As(err, &status) || status.Status != code {
			t.Fatalf("code %d status %#v err %v", code, status, err)
		}
	}

	empty := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"chat_1","choices":[{"message":{"role":"assistant","content":""}}]}`))
	})
	_, err = empty.Chat(context.Background(), Turn{Messages: []Message{{Role: "user", Content: "Hi"}}})
	if err == nil || err.Error() != "grok response has no text" {
		t.Fatalf("error %v", err)
	}
}

func TestChatStripsThink(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"id":"chat_1","choices":[{"message":{"role":"assistant","content":"<think>secret</think>Hello"}}]}`))
	})
	reply, err := client.Chat(context.Background(), Turn{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Content != "Hello" || reply.ID != "chat_1" {
		t.Fatalf("reply %+v", reply)
	}

	only := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"id":"chat_1","choices":[{"message":{"content":"<think>secret</think>"}}]}`))
	})
	_, err = only.Chat(context.Background(), Turn{Messages: []Message{{Role: "user", Content: "Hi"}}})
	if err == nil || err.Error() != "grok response has no text" {
		t.Fatalf("error %v", err)
	}

	streamed := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, ""+
			"data: {\"id\":\"chat_1\",\"choices\":[{\"delta\":{\"content\":\"<thi\"}}]}\n\n"+
			"data: {\"id\":\"chat_1\",\"choices\":[{\"delta\":{\"content\":\"nk>secret</think>Hel\"}}]}\n\n"+
			"data: {\"id\":\"chat_1\",\"choices\":[{\"delta\":{\"content\":\"lo\",\"reasoning_content\":\"hidden\"}}]}\n\n"+
			"data: [DONE]\n\n")
	})
	var texts []string
	var replyContent string
	for event, err := range streamed.ChatStream(context.Background(), Turn{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	}) {
		if err != nil {
			t.Fatal(err)
		}
		if event.Text != "" {
			texts = append(texts, event.Text)
		}
		if event.Reply != nil {
			replyContent = event.Reply.Content
		}
	}
	if strings.Join(texts, "") != "Hello" || replyContent != "Hello" {
		t.Fatalf("deltas %q reply %q", texts, replyContent)
	}
	if strings.Contains(strings.Join(texts, ""), "secret") || strings.Contains(replyContent, "hidden") {
		t.Fatalf("trace leaked %q", texts)
	}
}

func TestChatStream(t *testing.T) {
	var stream bool
	var accept string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		var req struct {
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		stream = req.Stream
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, ""+
			": ping\n"+
			"\n"+
			"data: {\"id\":\"chat_1\",\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n"+
			"\n"+
			"data: {\"id\":\"chat_1\",\"choices\":[{\"delta\":{\"reasoning_content\":\"secret\"}}]}\n"+
			"\n"+
			"data: {\"id\":\"chat_1\",\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n"+
			"\n"+
			"data: [DONE]\n"+
			"\n")
	})

	var texts []string
	var reply *Reply
	for event, err := range client.ChatStream(context.Background(), Turn{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	}) {
		if err != nil {
			t.Fatal(err)
		}
		if event.Text != "" {
			texts = append(texts, event.Text)
		}
		if event.Reply != nil {
			reply = event.Reply
		}
	}
	if !stream {
		t.Fatal("stream was not set")
	}
	if accept != "text/event-stream" {
		t.Fatalf("accept %q", accept)
	}
	if strings.Join(texts, "") != "Hello" {
		t.Fatalf("deltas %q", texts)
	}
	if reply == nil || reply.ID != "chat_1" || reply.Content != "Hello" {
		t.Fatalf("reply %+v", reply)
	}
	if strings.Contains(strings.Join(texts, ""), "secret") {
		t.Fatalf("reasoning leaked %q", texts)
	}
}

func TestChatStreamErrors(t *testing.T) {
	failed := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "data: {\"type\":\"error\",\"error\":{\"message\":\"overloaded\"}}\n\n")
	})
	var saw error
	for _, err := range failed.ChatStream(context.Background(), Turn{Messages: []Message{{Role: "user", Content: "Hi"}}}) {
		saw = err
	}
	if saw == nil || saw.Error() != "grok response failed: overloaded" {
		t.Fatalf("error %v", saw)
	}

	cut := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, ": ping\n\n")
	})
	saw = nil
	for _, err := range cut.ChatStream(context.Background(), Turn{Messages: []Message{{Role: "user", Content: "Hi"}}}) {
		if err != nil {
			saw = err
		}
	}
	if saw == nil || saw.Error() != "grok response has no text" {
		t.Fatalf("error %v", saw)
	}

	var client *Client
	saw = nil
	for _, err := range client.ChatStream(nil, Turn{}) {
		saw = err
	}
	if saw == nil || !strings.Contains(saw.Error(), "nil client") {
		t.Fatalf("error %v", saw)
	}
}

func TestChatStreamStop(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"id\":\"chat_1\",\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n")
		flusher.Flush()
		<-r.Context().Done()
	})
	for event, err := range client.ChatStream(context.Background(), Turn{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	}) {
		if err != nil {
			t.Fatal(err)
		}
		if event.Text != "Hi" {
			t.Fatalf("delta %q", event.Text)
		}
		break
	}
}

func TestChatConcurrent(t *testing.T) {
	var mu sync.Mutex
	var auths []string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auths = append(auths, r.Header.Get("Authorization"))
		mu.Unlock()
		_, _ = w.Write([]byte(completed))
	})
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reply, err := client.Chat(context.Background(), Turn{
				Messages: []Message{{Role: "user", Content: "Hi"}},
			})
			if err != nil {
				errCh <- err
				return
			}
			if reply.Content != "Hello" {
				errCh <- errString("reply " + reply.Content)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(auths) != 2 || auths[0] != "Bearer test-key" || auths[1] != "Bearer test-key" {
		t.Fatalf("auths %q", auths)
	}
}

func TestNewRejectsKey(t *testing.T) {
	if _, err := New(""); err == nil || err.Error() != "api key is empty" {
		t.Fatalf("empty %v", err)
	}
	if _, err := New("bad key"); err == nil || err.Error() != "invalid api key" {
		t.Fatalf("space %v", err)
	}
	if _, err := New("bad\nkey"); err == nil || err.Error() != "invalid api key" {
		t.Fatalf("newline %v", err)
	}
}

func TestWithModel(t *testing.T) {
	client, err := New("test-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.WithModel(""); err == nil || err.Error() != "model name is empty" {
		t.Fatalf("empty %v", err)
	}
	if _, err := client.WithModel("grok\n4"); err == nil || err.Error() != "invalid model name" {
		t.Fatalf("newline %v", err)
	}
	next, err := client.WithModel("grok-4")
	if err != nil {
		t.Fatal(err)
	}
	if next.model != "grok-4" || client.model != Model {
		t.Fatalf("models %s %s", client.model, next.model)
	}
	var nilClient *Client
	if _, err := nilClient.WithModel("grok-4"); err == nil {
		t.Fatal("nil client returned nil error")
	}
}

func TestWithBaseURL(t *testing.T) {
	client, err := New("test-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"", "file:///tmp", "https://api.x.ai/v1?q=1", "https://user:pass@api.x.ai/v1", "https://api.x.ai/v1#x"} {
		if _, err := client.WithBaseURL(raw); err == nil || err.Error() != "invalid base url" {
			t.Fatalf("%q %v", raw, err)
		}
	}
	next, err := client.WithBaseURL("https://api.x.ai/v1/")
	if err != nil {
		t.Fatal(err)
	}
	if next.baseURL != "https://api.x.ai/v1" {
		t.Fatalf("base %s", next.baseURL)
	}
	if _, err := client.WithHTTPClient(nil); err == nil || err.Error() != "nil http client" {
		t.Fatalf("nil http %v", err)
	}
}

func TestRedirectDropsAuthorization(t *testing.T) {
	var otherAuth string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(completed))
	}))
	t.Cleanup(other.Close)

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("source authorization %q", r.Header.Get("Authorization"))
		}
		http.Redirect(w, r, other.URL+"/chat/completions", http.StatusFound)
	})
	if _, err := client.Chat(context.Background(), Turn{Messages: []Message{{Role: "user", Content: "Hi"}}}); err != nil {
		t.Fatal(err)
	}
	if otherAuth != "" {
		t.Fatalf("forwarded authorization %q", otherAuth)
	}
}

func TestSameHostRedirectKeepsAuthorization(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			http.Redirect(w, r, "/done", http.StatusFound)
			return
		}
		auth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(completed))
	}))
	t.Cleanup(srv.Close)
	client, err := New("test-key")
	if err != nil {
		t.Fatal(err)
	}
	client, err = client.WithBaseURL(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Chat(context.Background(), Turn{Messages: []Message{{Role: "user", Content: "Hi"}}}); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer test-key" {
		t.Fatalf("authorization %q", auth)
	}
}

func TestChatCancel(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		<-r.Context().Done()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := client.Chat(ctx, Turn{Messages: []Message{{Role: "user", Content: "Hi"}}})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error %v", err)
	}
}

func newTestClient(t *testing.T, h func(http.ResponseWriter, *http.Request)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(h))
	client, err := New("test-key")
	if err != nil {
		t.Fatal(err)
	}
	client, err = client.WithBaseURL(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		client.httpClient.CloseIdleConnections()
		srv.CloseClientConnections()
		srv.Close()
	})
	return client
}

func decodeRequest(t *testing.T, body []byte) map[string]json.RawMessage {
	t.Helper()
	var req map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	return req
}

type errString string

func (e errString) Error() string { return string(e) }
