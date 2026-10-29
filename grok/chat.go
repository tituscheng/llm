package grok

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"net/http"
	"net/url"
	"strings"
)

const (
	maxBody    = 32 << 20
	maxSSELine = 32 << 20
	maxErrRune = 200
)

// Message is one input message.
// Role is system, user, or assistant. Content is the text.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Turn is one chat completion.
// Messages are required.
// Effort is sent as reasoning_effort. Empty leaves the API default.
// Use EffortLow, EffortHigh, or EffortXHigh.
type Turn struct {
	Messages []Message
	Effort   string
}

// Sampling sets the limits on one call.
// A nil field is omitted. A non-nil field is sent, including zero.
type Sampling struct {
	// Temperature is sent as temperature. Zero is sent as zero.
	Temperature *float64
	// TopP is sent as top_p.
	TopP *float64
	// MaxTokens is sent as max_completion_tokens.
	// It caps the visible answer. Zero is sent as zero. A negative value is rejected.
	MaxTokens *int
}

// Reply is one completed chat completion.
type Reply struct {
	ID      string
	Content string
}

// Event is one streamed piece.
// Text is an assistant-text delta. Reply is set once, when the completion
// finishes, and Text is empty on that event.
type Event struct {
	Text  string
	Reply *Reply
}

// StatusError is an HTTP response from the Grok API other than 200.
// Status is the status code. A caller uses it to tell a rejected key (401 or 403),
// a rate limit (429), and missing credits (402) apart.
type StatusError struct {
	Status  int
	Message string
}

func (e *StatusError) Error() string {
	if e == nil {
		return "grok status 0"
	}
	if e.Message == "" {
		return fmt.Sprintf("grok status %d", e.Status)
	}
	return fmt.Sprintf("grok status %d: %s", e.Status, e.Message)
}

type apiRequest struct {
	Model               string    `json:"model"`
	Messages            []Message `json:"messages"`
	Stream              bool      `json:"stream,omitempty"`
	Temperature         *float64  `json:"temperature,omitempty"`
	TopP                *float64  `json:"top_p,omitempty"`
	MaxCompletionTokens *int      `json:"max_completion_tokens,omitempty"`
	ReasoningEffort     string    `json:"reasoning_effort,omitempty"`
}

var (
	errStreamStop = errors.New("grok stream stopped")
	errStreamDone = errors.New("grok stream done")
)

// Chat posts turn to /v1/chat/completions and returns the assistant text.
// A <think> trace is removed from that text, including a trace split across the reply.
// Pass one Sampling value to set limits. Leave it out to keep the API defaults.
func (c *Client) Chat(ctx context.Context, turn Turn, sampling ...Sampling) (Reply, error) {
	if c == nil {
		return Reply{}, fmt.Errorf("nil client")
	}
	if ctx == nil {
		return Reply{}, fmt.Errorf("nil context")
	}
	sample, err := oneSampling(sampling)
	if err != nil {
		return Reply{}, err
	}
	resp, err := c.open(ctx, turn, sample, false)
	if err != nil {
		return Reply{}, err
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body)
	if err != nil {
		return Reply{}, err
	}
	return decodeReply(body)
}

// ChatStream posts turn with stream set and yields text deltas.
// A <think> trace is removed from those deltas. The last successful event carries
// the completed Reply and an empty Text.
// Pass one Sampling value to set limits. Leave it out to keep the API defaults.
// The request stops when the loop ends, the caller breaks, or ctx is cancelled.
// An error from the API is yielded once, and then the iterator stops.
func (c *Client) ChatStream(ctx context.Context, turn Turn, sampling ...Sampling) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		if c == nil {
			yield(Event{}, fmt.Errorf("nil client"))
			return
		}
		if ctx == nil {
			yield(Event{}, fmt.Errorf("nil context"))
			return
		}
		sample, err := oneSampling(sampling)
		if err != nil {
			yield(Event{}, err)
			return
		}
		streamCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		resp, err := c.open(streamCtx, turn, sample, true)
		if err != nil {
			yield(Event{}, err)
			return
		}
		defer resp.Body.Close()

		var text strings.Builder
		var id string
		var filter thinkFilter
		stopped := false
		emit := func(delta string) bool {
			if delta == "" {
				return true
			}
			text.WriteString(delta)
			if !yield(Event{Text: delta}, nil) {
				stopped = true
				return false
			}
			return true
		}
		err = readChatStream(resp.Body, func(delta, chunkID string) bool {
			if chunkID != "" {
				id = chunkID
			}
			return emit(filter.push(delta))
		})
		if stopped {
			return
		}
		if err != nil {
			if ctx.Err() != nil {
				yield(Event{}, ctx.Err())
				return
			}
			yield(Event{}, err)
			return
		}
		if !emit(filter.flush()) {
			return
		}
		if text.Len() == 0 {
			yield(Event{}, fmt.Errorf("grok response has no text"))
			return
		}
		yield(Event{Reply: &Reply{ID: id, Content: text.String()}}, nil)
	}
}

func (c *Client) open(ctx context.Context, turn Turn, sampling Sampling, stream bool) (*http.Response, error) {
	if err := validateKey(c.key); err != nil {
		return nil, err
	}
	if err := validateModel(c.model); err != nil {
		return nil, err
	}
	if err := validateTurn(turn); err != nil {
		return nil, err
	}
	if err := validateSampling(sampling); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(apiRequest{
		Model:               c.model,
		Messages:            turn.Messages,
		Stream:              stream,
		Temperature:         sampling.Temperature,
		TopP:                sampling.TopP,
		MaxCompletionTokens: sampling.MaxTokens,
		ReasoningEffort:     turn.Effort,
	})
	if err != nil {
		return nil, fmt.Errorf("encode grok request: %w", err)
	}
	endpoint, err := url.JoinPath(c.baseURL, "chat/completions")
	if err != nil {
		return nil, fmt.Errorf("grok url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("grok request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("User-Agent", userAgent)
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	resp, err := c.httpDo().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("grok request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, readErr := readLimited(resp.Body)
		if readErr != nil {
			return nil, readErr
		}
		return nil, statusError(resp.StatusCode, body)
	}
	return resp, nil
}

func decodeReply(body []byte) (Reply, error) {
	var decoded struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Reply{}, fmt.Errorf("decode grok response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return Reply{}, fmt.Errorf("grok response has no text")
	}
	content := stripThink(decoded.Choices[0].Message.Content)
	if content == "" {
		return Reply{}, fmt.Errorf("grok response has no text")
	}
	return Reply{ID: decoded.ID, Content: content}, nil
}

func oneSampling(sampling []Sampling) (Sampling, error) {
	switch len(sampling) {
	case 0:
		return Sampling{}, nil
	case 1:
		return sampling[0], nil
	default:
		return Sampling{}, fmt.Errorf("sampling limits provided more than once")
	}
}

func validateTurn(turn Turn) error {
	if err := validateMessages(turn.Messages); err != nil {
		return err
	}
	switch turn.Effort {
	case "", EffortLow, EffortHigh, EffortXHigh:
		return nil
	default:
		return fmt.Errorf("invalid reasoning effort %q", turn.Effort)
	}
}

func validateMessages(messages []Message) error {
	if len(messages) == 0 {
		return fmt.Errorf("chat messages are empty")
	}
	for _, message := range messages {
		switch message.Role {
		case "system", "user", "assistant":
		default:
			return fmt.Errorf("invalid chat role %q", message.Role)
		}
		if message.Content == "" {
			return fmt.Errorf("chat message content is empty")
		}
	}
	return nil
}

func validateSampling(sampling Sampling) error {
	if err := finiteFloat("temperature", sampling.Temperature); err != nil {
		return err
	}
	if err := finiteFloat("top_p", sampling.TopP); err != nil {
		return err
	}
	if sampling.MaxTokens != nil && *sampling.MaxTokens < 0 {
		return fmt.Errorf("invalid max tokens")
	}
	return nil
}

func finiteFloat(name string, value *float64) error {
	if value == nil {
		return nil
	}
	if math.IsNaN(*value) || math.IsInf(*value, 0) {
		return fmt.Errorf("invalid %s", name)
	}
	return nil
}

func readLimited(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("read grok response: %w", err)
	}
	if len(body) > maxBody {
		return nil, fmt.Errorf("grok response is too large")
	}
	return body, nil
}

func statusError(code int, body []byte) error {
	return &StatusError{Status: code, Message: errorMessage(body)}
}

func errorMessage(body []byte) string {
	var decoded struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(body, &decoded) != nil {
		return ""
	}
	if len(decoded.Error) > 0 && decoded.Error[0] == '"' {
		var text string
		if json.Unmarshal(decoded.Error, &text) == nil {
			return oneLine(text)
		}
	}
	var obj struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(decoded.Error, &obj) == nil && obj.Message != "" {
		return oneLine(obj.Message)
	}
	return oneLine(decoded.Message)
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > maxErrRune {
		return string(r[:maxErrRune])
	}
	return s
}

func readChatStream(r io.Reader, yield func(delta, id string) bool) error {
	reader := bufio.NewReader(r)
	var data string
	flush := func() error {
		if data == "" {
			return nil
		}
		payload := data
		data = ""
		if payload == "[DONE]" {
			return errStreamDone
		}
		delta, id, err := parseChunk(payload)
		if err != nil {
			return err
		}
		if !yield(delta, id) {
			return errStreamStop
		}
		return nil
	}
	for {
		line, err := readLine(reader)
		if line != "" {
			name, value, ok := sseField(line)
			if ok && name == "data" {
				if data == "" {
					data = value
				} else {
					data += "\n" + value
				}
			}
		} else if flushErr := flush(); flushErr != nil {
			if errors.Is(flushErr, errStreamStop) || errors.Is(flushErr, errStreamDone) {
				return nil
			}
			return flushErr
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				flushErr := flush()
				if flushErr != nil && !errors.Is(flushErr, errStreamStop) && !errors.Is(flushErr, errStreamDone) {
					return flushErr
				}
				return nil
			}
			return fmt.Errorf("read grok stream: %w", err)
		}
	}
}

func parseChunk(payload string) (string, string, error) {
	var decoded struct {
		ID      string          `json:"id"`
		Error   json.RawMessage `json:"error"`
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		return "", "", fmt.Errorf("decode grok stream: %w", err)
	}
	if msg := streamError(decoded.Error); msg != "" {
		return "", "", fmt.Errorf("grok response failed: %s", msg)
	}
	if len(decoded.Choices) == 0 {
		return "", decoded.ID, nil
	}
	text := decoded.Choices[0].Delta.Content
	if text == "" {
		text = decoded.Choices[0].Message.Content
	}
	return text, decoded.ID, nil
}

func streamError(raw json.RawMessage) string {
	if len(raw) == 0 || !json.Valid(raw) || string(bytes.TrimSpace(raw)) == "null" {
		return ""
	}
	wrapped := append([]byte(`{"error":`), raw...)
	wrapped = append(wrapped, '}')
	return errorMessage(wrapped)
}

func readLine(r *bufio.Reader) (string, error) {
	var buf bytes.Buffer
	for {
		chunk, err := r.ReadSlice('\n')
		if len(chunk) > 0 {
			if buf.Len()+len(chunk) > maxSSELine {
				return "", fmt.Errorf("grok stream line is too long")
			}
			buf.Write(chunk)
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		line := strings.TrimRight(buf.String(), "\r\n")
		if err != nil {
			return line, err
		}
		return line, nil
	}
}

func sseField(line string) (string, string, bool) {
	if line == "" || strings.HasPrefix(line, ":") {
		return "", "", false
	}
	name, value, ok := strings.Cut(line, ":")
	if !ok {
		return name, "", true
	}
	return name, strings.TrimPrefix(value, " "), true
}
