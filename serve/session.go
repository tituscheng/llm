package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Session is one resumable generation retained by llama-server.
type Session struct {
	// ID is the conversation id. It can be the requested id or an id stored
	// with a "::model" suffix.
	ID string `json:"conversation_id"`
	// Done is true after the generation has finished.
	Done bool `json:"is_done"`
	// TotalBytes is the number of SSE bytes stored for this session.
	TotalBytes int64 `json:"total_bytes"`
	// StartedAt is the unix time, in seconds, when the session was created.
	StartedAt int64 `json:"started_at"`
	// CompletedAt is the unix time, in seconds, when the generation finished.
	// It is zero while the generation is still running.
	CompletedAt int64 `json:"completed_at"`
}

// Reuse resumes a kept stream by reading GET /v1/stream.
// from is a byte offset into the server's SSE buffer. Zero reads from the
// start and omits the from query. Stream.Offset is a line boundary from an
// earlier read. A negative from returns an error.
// The resumed stream strips <think> traces. Its scanner starts empty, so a
// think block that opened before from can appear in the content.
// EndReasoning is not armed. The original stream can still end thinking when
// its completion id has already been read.
func (s *Server) Reuse(ctx context.Context, id string, from int64) (*Stream, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := validateConversation(id); err != nil {
		return nil, err
	}
	if from < 0 {
		return nil, fmt.Errorf("invalid stream offset")
	}
	if err := s.requireAlive(ctx); err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("conv_id", id)
	if from > 0 {
		query.Set("from", strconv.FormatInt(from, 10))
	}
	streamCtx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, s.baseURL+"/v1/stream?"+query.Encode(), nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("reuse stream request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := s.httpClient().Do(req)
	if err != nil {
		cancel()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("reuse stream: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		cancel()
		if readErr != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, streamHTTPError("reuse stream", resp.StatusCode, body)
	}
	st := &Stream{
		server:  s,
		parent:  ctx,
		cancel:  cancel,
		body:    resp.Body,
		strip:   true,
		conv:    id,
		idReady: make(chan struct{}),
	}
	if from > 0 {
		st.offset.Store(from)
	}
	return st, nil
}

// Shutdown cancels the kept generation and drops its buffer.
// It sends DELETE /v1/stream. llama-server reports success when the id is
// already gone. Close on a Stream does not call Shutdown.
func (s *Server) Shutdown(ctx context.Context, id string) error {
	if ctx == nil {
		return fmt.Errorf("nil context")
	}
	if err := validateConversation(id); err != nil {
		return err
	}
	if err := s.requireAlive(ctx); err != nil {
		return err
	}
	query := url.Values{}
	query.Set("conv_id", id)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.baseURL+"/v1/stream?"+query.Encode(), nil)
	if err != nil {
		return fmt.Errorf("shutdown request: %w", err)
	}
	resp, err := s.httpClient().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("shutdown: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read shutdown response: %w", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		return streamHTTPError("shutdown", resp.StatusCode, body)
	}
	return nil
}

// Lookup returns sessions for the given conversation ids.
// It posts the ids to /v1/streams/lookup. An id also matches a session the
// server stored as "<id>::<model>". An empty or nil list returns nil and
// makes no request.
func (s *Server) Lookup(ctx context.Context, ids []string) ([]Session, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if len(ids) == 0 {
		return nil, nil
	}
	for _, id := range ids {
		if err := validateConversation(id); err != nil {
			return nil, err
		}
	}
	if err := s.requireAlive(ctx); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(struct {
		IDs []string `json:"conversation_ids"`
	}{IDs: ids})
	if err != nil {
		return nil, fmt.Errorf("encode lookup: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/v1/streams/lookup", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("lookup request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("lookup: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read lookup response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, streamHTTPError("lookup", resp.StatusCode, body)
	}
	var sessions []Session
	if err := json.Unmarshal(body, &sessions); err != nil {
		return nil, fmt.Errorf("decode lookup: %w", err)
	}
	return sessions, nil
}

func (s *Server) requireAlive(ctx context.Context) error {
	alive, err := s.IsAlive(ctx)
	if err != nil {
		return err
	}
	if !alive {
		return fmt.Errorf("llama-server is not running")
	}
	return nil
}

func validateConversation(id string) error {
	if id == "" {
		return fmt.Errorf("conversation id is empty")
	}
	if strings.ContainsAny(id, "\r\n") {
		return fmt.Errorf("invalid conversation id")
	}
	return nil
}

func streamHTTPError(action string, status int, body []byte) error {
	var decoded struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &decoded) == nil && decoded.Error.Message != "" {
		return fmt.Errorf("%s: %s", action, decoded.Error.Message)
	}
	return fmt.Errorf("%s status %d", action, status)
}
