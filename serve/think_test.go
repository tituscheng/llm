package serve

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplyThink(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		reasoning string
		wantText  string
		wantTrace string
	}{
		{name: "plain", content: "hello", wantText: "hello"},
		{
			name:     "empty leading trace",
			content:  "<think>\n\n</think>\n\n How can",
			wantText: " How can",
		},
		{
			name:      "trace then answer",
			content:   "<think>\nsecret\n</think>\n\nanswer",
			wantText:  "answer",
			wantTrace: "secret",
		},
		{
			name:      "answer then trace",
			content:   "answer <think>secret</think>",
			wantText:  "answer ",
			wantTrace: "secret",
		},
		{
			name:      "unclosed",
			content:   "answer<think>secret",
			wantText:  "answer",
			wantTrace: "secret",
		},
		{name: "less than", content: "price < 5", wantText: "price < 5"},
		{
			name:      "two traces",
			content:   "<think>a</think>\n<think>b</think>\nok",
			wantText:  "ok",
			wantTrace: "a\nb",
		},
		{
			name:      "server trace wins",
			content:   "<think>secret</think>Yes",
			reasoning: "already",
			wantText:  "Yes",
			wantTrace: "already",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotText, gotTrace := applyThink(test.content, test.reasoning)
			if gotText != test.wantText || gotTrace != test.wantTrace {
				t.Fatalf("content %q trace %q", gotText, gotTrace)
			}
		})
	}
}

func TestThinkScanChunks(t *testing.T) {
	var scan thinkScan
	var content, reasoning string
	for _, chunk := range []string{"<thi", "nk>\nsecret</thi", "nk>Yes", " price < 5"} {
		c, r := scan.push(chunk)
		content += c
		reasoning += r
	}
	c, r := scan.flush()
	content += c
	reasoning += r
	if content != "Yes price < 5" || strings.TrimSpace(reasoning) != "secret" {
		t.Fatalf("content %q trace %q", content, reasoning)
	}

	scan = thinkScan{}
	content, reasoning = scan.push("<think>hello</")
	c, r = scan.flush()
	if content+c != "" || reasoning+r != "hello</" {
		t.Fatalf("partial close content %q trace %q", content+c, reasoning+r)
	}

	scan = thinkScan{}
	content, _ = scan.push("yes<")
	c, _ = scan.flush()
	if content+c != "yes<" {
		t.Fatalf("partial open %q", content+c)
	}
}

func TestChatStripsThink(t *testing.T) {
	var format string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/chat/completions":
			body, _ := io.ReadAll(r.Body)
			format = string(body)
			if strings.Contains(format, `"stream":true`) {
				_, _ = io.WriteString(w, ""+
					"data: {\"choices\":[{\"delta\":{\"content\":\"<thi\"}}]}\n\n"+
					"data: {\"choices\":[{\"delta\":{\"content\":\"nk>\\nsecret</think>\\nHi\"}}]}\n\n"+
					"data: [DONE]\n\n")
				return
			}
			_, _ = w.Write([]byte(`{"id":"chatcmpl-7","choices":[{"message":{"content":"<think>\nsecret\n</think>\n\nHi","reasoning_content":""}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	server := &Server{baseURL: srv.URL, client: srv.Client(), done: make(chan struct{})}
	messages := []Message{{Role: "user", Content: "hi"}}

	got, err := server.Chat(context.Background(), messages)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hi" {
		t.Fatalf("chat %q", got)
	}

	reply, err := server.ChatWith(context.Background(), messages, Reasoning{})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Content != "Hi" || reply.Reasoning != "secret" || reply.ID != "chatcmpl-7" {
		t.Fatalf("reply %+v", reply)
	}

	reply, err = server.ChatWith(context.Background(), messages, Reasoning{Format: FormatNone})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Content != "<think>\nsecret\n</think>\n\nHi" || reply.Reasoning != "" {
		t.Fatalf("kept reply %+v", reply)
	}

	off := false
	reply, err = server.ChatWith(context.Background(), messages, Reasoning{Strip: &off})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reply.Content, "<think>") {
		t.Fatalf("strip off content %q", reply.Content)
	}

	var tokens []string
	for token, err := range server.ChatStream(context.Background(), messages) {
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
	}
	if strings.Join(tokens, "") != "Hi" {
		t.Fatalf("stream %q", tokens)
	}

	stream, err := server.Stream(context.Background(), messages, Reasoning{})
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
	if content != "Hi" || strings.TrimSpace(reasoning) != "secret" {
		t.Fatalf("deltas content %q trace %q", content, reasoning)
	}
}
