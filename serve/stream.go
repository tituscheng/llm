package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"sync"
	"sync/atomic"
)

// Stream is an in-flight streaming chat completion.
// Call Close when the stream is no longer needed. Deltas reads tokens until
// the server finishes, the iterator stops, or the parent context is cancelled.
// EndReasoning is safe to call from another goroutine while ranging Deltas.
// Close releases this request. A stream started by Keep keeps running on the
// server until Shutdown.
type Stream struct {
	server  *Server
	parent  context.Context
	cancel  context.CancelFunc
	body    io.ReadCloser
	control bool
	strip   bool
	conv    string
	offset  atomic.Int64

	mu          sync.Mutex
	id          string
	readyClosed bool
	started     bool
	idReady     chan struct{}

	closeOnce sync.Once
	closeErr  error
}

// Stream starts a streaming completion with reasoning options.
// Set Reasoning.Control to arm EndReasoning for this completion.
// Pass one Sampling value to set token limits. Leave it out to keep llama-server's defaults.
// The server cancels the generation when this request ends.
func (s *Server) Stream(ctx context.Context, messages []Message, reasoning Reasoning, sampling ...Sampling) (*Stream, error) {
	return s.openStream(ctx, "", messages, reasoning, sampling...)
}

// Keep starts a streaming completion that llama-server retains after the
// client disconnects. id is sent as X-Conversation-Id. Close ends this
// request and leaves the generation running. Shutdown cancels it.
// Starting Keep again with the same id replaces the retained session.
// Pass one Sampling value to set token limits. Leave it out to keep llama-server's defaults.
func (s *Server) Keep(ctx context.Context, id string, messages []Message, reasoning Reasoning, sampling ...Sampling) (*Stream, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := validateConversation(id); err != nil {
		return nil, err
	}
	return s.openStream(ctx, id, messages, reasoning, sampling...)
}

func (s *Server) openStream(ctx context.Context, conv string, messages []Message, reasoning Reasoning, sampling ...Sampling) (*Stream, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	sample, err := oneSampling(sampling)
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	resp, err := s.openChat(streamCtx, messages, true, reasoning, conv, sample)
	if err != nil {
		cancel()
		return nil, err
	}
	return &Stream{
		server:  s,
		parent:  ctx,
		cancel:  cancel,
		body:    resp.Body,
		control: reasoning.Control,
		strip:   reasoning.stripThink(),
		conv:    conv,
		idReady: make(chan struct{}),
	}, nil
}

// ID returns the conversation id for a stream started by Keep or Reuse.
// A stream started by Stream returns an empty id.
func (st *Stream) ID() string {
	return st.conv
}

// Offset reports how many SSE bytes this stream has fully consumed.
// The count includes each complete line and its trailing newline.
// Bytes sitting in the read buffer, not yet returned as a line, are excluded,
// so the value is a line boundary. Pass it to Reuse to resume from there.
// A blank line after data: [DONE] can stay unread. Reading that line again
// yields no token.
func (st *Stream) Offset() int64 {
	return st.offset.Load()
}

// Deltas yields assistant text and reasoning text as they arrive.
// Role-only events are skipped. Breaking the loop stops the request.
// A second call returns an error.
func (st *Stream) Deltas() iter.Seq2[Delta, error] {
	return func(yield func(Delta, error) bool) {
		st.mu.Lock()
		if st.started {
			st.mu.Unlock()
			yield(Delta{}, fmt.Errorf("chat stream already started"))
			return
		}
		st.started = true
		st.mu.Unlock()

		defer func() {
			st.cancel()
			st.finish()
			_ = st.Close()
		}()

		var scan thinkScan
		err := readChatStream(st.body, &st.offset, func(event sseEvent) bool {
			st.noteID(event.id)
			content, reasoning := event.content, event.reasoning
			if st.strip {
				var trace string
				content, trace = scan.push(content)
				if reasoning == "" {
					reasoning = trace
				}
			}
			if content == "" && reasoning == "" {
				return true
			}
			return yield(Delta{Content: content, Reasoning: reasoning}, nil)
		})
		if err == nil && st.strip {
			content, reasoning := scan.flush()
			if (content != "" || reasoning != "") && !yield(Delta{Content: content, Reasoning: reasoning}, nil) {
				err = errStreamStop
			}
		}
		if errors.Is(err, errStreamStop) {
			return
		}
		if st.parent.Err() != nil {
			yield(Delta{}, st.parent.Err())
			return
		}
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}
		yield(Delta{}, err)
	}
}

// EndReasoning ends the thinking block of this completion and lets the model
// continue with the answer. It posts action reasoning_end to
// /v1/chat/completions/control. Reasoning.Control must be set on Stream.
// The call waits until Deltas has read the completion id.
func (st *Stream) EndReasoning(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("nil context")
	}
	if !st.control {
		return fmt.Errorf("reasoning control is not enabled")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-st.idReady:
	}
	st.mu.Lock()
	id := st.id
	st.mu.Unlock()
	if id == "" {
		return fmt.Errorf("chat completion id is missing")
	}
	if err := validateChatModel(st.server.model); err != nil {
		return err
	}
	return st.server.endReasoning(ctx, id)
}

// Close stops the streaming request and releases the response body.
// A second call does nothing.
func (st *Stream) Close() error {
	st.closeOnce.Do(func() {
		st.cancel()
		st.closeErr = st.body.Close()
		st.finish()
	})
	return st.closeErr
}

func (st *Stream) noteID(id string) {
	if id == "" {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.id != "" {
		return
	}
	st.id = id
	st.closeReady()
}

func (st *Stream) finish() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.closeReady()
}

// closeReady closes idReady once. The caller holds st.mu.
func (st *Stream) closeReady() {
	if st.readyClosed {
		return
	}
	st.readyClosed = true
	close(st.idReady)
}

func (s *Server) endReasoning(ctx context.Context, id string) error {
	alive, err := s.IsAlive(ctx)
	if err != nil {
		return err
	}
	if !alive {
		return fmt.Errorf("llama-server is not running")
	}
	payload, err := json.Marshal(struct {
		ID     string `json:"id"`
		Action string `json:"action"`
		Model  string `json:"model,omitempty"`
	}{ID: id, Action: "reasoning_end", Model: s.model})
	if err != nil {
		return fmt.Errorf("encode end reasoning: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/v1/chat/completions/control", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("end reasoning request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("end reasoning: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read end reasoning response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("end reasoning status %d", resp.StatusCode)
	}
	var decoded struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return fmt.Errorf("decode end reasoning response: %w", err)
	}
	if decoded.Success {
		return nil
	}
	if decoded.Message == "" {
		return fmt.Errorf("end reasoning failed")
	}
	return fmt.Errorf("end reasoning: %s", decoded.Message)
}
