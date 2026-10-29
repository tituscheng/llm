package serve

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
	"regexp"
	"strings"
	"sync/atomic"
)

const maxSSELine = 1 << 20

// Effort values are the reasoning_effort levels llama-server documents.
// EffortNone disables thinking. A template may accept other levels, and those
// can be set with any single token on Reasoning.Effort.
const (
	EffortNone    = "none"
	EffortDefault = "default"
	EffortMinimal = "minimal"
	EffortLow     = "low"
	EffortMedium  = "medium"
	EffortHigh    = "high"
	EffortXHigh   = "xhigh"
	EffortMax     = "max"
)

// Format values are the reasoning_format modes llama-server accepts.
// An empty Reasoning.Format leaves the server default.
const (
	FormatNone           = "none"
	FormatAuto           = "auto"
	FormatDeepseek       = "deepseek"
	FormatDeepseekLegacy = "deepseek-legacy"
)

// Reasoning controls thinking on one chat request.
// The zero value leaves llama-server's defaults and omits the extra JSON fields.
type Reasoning struct {
	// Effort is sent as reasoning_effort. EffortNone disables thinking.
	Effort string
	// Format is sent as reasoning_format.
	// Use FormatNone, FormatAuto, FormatDeepseek, or FormatDeepseekLegacy.
	Format string
	// EnableThinking sets chat_template_kwargs.enable_thinking.
	// A nil pointer leaves the template default. A non-nil pointer is sent, including false.
	EnableThinking *bool
	// Control arms realtime reasoning control for this completion.
	// Stream.EndReasoning can then end the thinking block early.
	Control bool
	// Strip removes <think> traces from assistant content.
	// A nil pointer strips them, except when Format is none or deepseek-legacy.
	// A non-nil pointer forces stripping on or off.
	Strip *bool
}

// Sampling sets the token limits on one chat request.
// Pass one value to Chat, ChatWith, ChatStream, Stream, or Keep.
// A nil field is omitted, and llama-server keeps its default for that limit.
// A non-nil field is sent, including zero.
type Sampling struct {
	// Temperature is sent as temperature.
	// Zero is sent as zero. Greedy sampling is a TopK of 1.
	Temperature *float64
	// TopK limits the next token to the K most probable tokens.
	// Zero disables the limit. One keeps only the most probable token.
	TopK *int
	// TopP limits the next token to a cumulative probability of P.
	// 1 disables the limit.
	TopP *float64
	// MinP is the minimum probability relative to the most likely token.
	// Zero disables the limit.
	MinP *float64
	// MaxTokens is the maximum number of tokens to generate, sent as max_tokens.
	// -1 means no limit. Zero generates no tokens.
	MaxTokens *int
}

// Reply is one completed chat response.
type Reply struct {
	// ID is the completion id returned by llama-server.
	ID string
	// Content is the assistant text.
	Content string
	// Reasoning is the thinking trace. It is message.reasoning_content when the
	// server parsed thoughts, or the text removed from Content when it did not.
	Reasoning string
}

// Delta is one streamed piece of a chat completion.
// Content and Reasoning are empty when that piece did not include them.
type Delta struct {
	Content   string
	Reasoning string
}

// Message is one turn in a chat prompt.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatTemplateKwargs struct {
	EnableThinking *bool `json:"enable_thinking,omitempty"`
}

type chatRequest struct {
	Messages           []Message           `json:"messages"`
	Stream             bool                `json:"stream,omitempty"`
	ReasoningEffort    string              `json:"reasoning_effort,omitempty"`
	ReasoningFormat    string              `json:"reasoning_format,omitempty"`
	ReasoningControl   bool                `json:"reasoning_control,omitempty"`
	ChatTemplateKwargs *chatTemplateKwargs `json:"chat_template_kwargs,omitempty"`
	Temperature        *float64            `json:"temperature,omitempty"`
	TopK               *int                `json:"top_k,omitempty"`
	TopP               *float64            `json:"top_p,omitempty"`
	MinP               *float64            `json:"min_p,omitempty"`
	MaxTokens          *int                `json:"max_tokens,omitempty"`
	Model              string              `json:"model,omitempty"`
}

type sseEvent struct {
	id        string
	content   string
	reasoning string
	done      bool
}

var effortToken = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Chat sends messages to POST /v1/chat/completions and returns the assistant text.
// It returns an error when the server is not alive.
// Pass one Sampling value to set token limits. Leave it out to keep llama-server's defaults.
func (s *Server) Chat(ctx context.Context, messages []Message, sampling ...Sampling) (string, error) {
	reply, err := s.ChatWith(ctx, messages, Reasoning{}, sampling...)
	if err != nil {
		return "", err
	}
	return reply.Content, nil
}

// ChatWith sends messages and applies reasoning options.
// Pass one Sampling value to set token limits. Leave it out to keep llama-server's defaults.
// Reply.Content is the assistant text. Reply.Reasoning is the parsed reasoning text.
func (s *Server) ChatWith(ctx context.Context, messages []Message, reasoning Reasoning, sampling ...Sampling) (Reply, error) {
	if ctx == nil {
		return Reply{}, fmt.Errorf("nil context")
	}
	sample, err := oneSampling(sampling)
	if err != nil {
		return Reply{}, err
	}
	resp, err := s.openChat(ctx, messages, false, reasoning, "", sample)
	if err != nil {
		return Reply{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Reply{}, fmt.Errorf("read chat response: %w", err)
	}
	var decoded struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Reply{}, fmt.Errorf("decode chat response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return Reply{}, fmt.Errorf("chat response has no choices")
	}
	message := decoded.Choices[0].Message
	content, reasoningText := message.Content, message.ReasoningContent
	if reasoning.stripThink() {
		content, reasoningText = applyThink(content, reasoningText)
	}
	return Reply{
		ID:        decoded.ID,
		Content:   content,
		Reasoning: reasoningText,
	}, nil
}

// ChatStream sends messages with stream enabled and yields each assistant token.
// <think> traces are removed from those tokens. Use Stream to receive the trace
// and to end thinking early.
// Pass one Sampling value to set token limits. Leave it out to keep llama-server's defaults.
// The request stops when the loop ends, the caller breaks, or ctx is cancelled.
// An error from the server is yielded once, and then the iterator stops.
func (s *Server) ChatStream(ctx context.Context, messages []Message, sampling ...Sampling) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		if ctx == nil {
			yield("", fmt.Errorf("nil context"))
			return
		}
		sample, err := oneSampling(sampling)
		if err != nil {
			yield("", err)
			return
		}
		streamCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		resp, err := s.openChat(streamCtx, messages, true, Reasoning{}, "", sample)
		if err != nil {
			yield("", err)
			return
		}
		defer resp.Body.Close()

		var scan thinkScan
		err = readChatStream(resp.Body, nil, func(event sseEvent) bool {
			content, _ := scan.push(event.content)
			if content == "" {
				return true
			}
			return yield(content, nil)
		})
		if err == nil {
			if content, _ := scan.flush(); content != "" && !yield(content, nil) {
				err = errStreamStop
			}
		}
		if err == nil || errors.Is(err, errStreamStop) {
			return
		}
		if ctx.Err() != nil {
			yield("", ctx.Err())
			return
		}
		yield("", err)
	}
}

func (s *Server) openChat(
	ctx context.Context,
	messages []Message,
	stream bool,
	reasoning Reasoning,
	conv string,
	sampling Sampling,
) (*http.Response, error) {
	if err := validateMessages(messages); err != nil {
		return nil, err
	}
	if err := validateReasoning(reasoning); err != nil {
		return nil, err
	}
	if err := validateSampling(sampling); err != nil {
		return nil, err
	}
	if err := validateChatModel(s.model); err != nil {
		return nil, err
	}
	alive, err := s.IsAlive(ctx)
	if err != nil {
		return nil, err
	}
	if !alive {
		return nil, fmt.Errorf("llama-server is not running")
	}

	payload, err := json.Marshal(newChatRequest(messages, stream, reasoning, sampling, s.model))
	if err != nil {
		return nil, fmt.Errorf("encode chat: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	if conv != "" {
		req.Header.Set("X-Conversation-Id", conv)
	}

	resp, err := s.httpClient().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("chat: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("chat status %d", resp.StatusCode)
	}
	return resp, nil
}

func newChatRequest(messages []Message, stream bool, reasoning Reasoning, sampling Sampling, model string) chatRequest {
	req := chatRequest{
		Messages:         messages,
		Stream:           stream,
		ReasoningEffort:  reasoning.Effort,
		ReasoningFormat:  reasoning.Format,
		ReasoningControl: reasoning.Control,
		Temperature:      sampling.Temperature,
		TopK:             sampling.TopK,
		TopP:             sampling.TopP,
		MinP:             sampling.MinP,
		MaxTokens:        sampling.MaxTokens,
		Model:            model,
	}
	if reasoning.EnableThinking != nil {
		req.ChatTemplateKwargs = &chatTemplateKwargs{EnableThinking: reasoning.EnableThinking}
	}
	return req
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

func validateChatModel(name string) error {
	if strings.ContainsAny(name, "\r\n") {
		return fmt.Errorf("invalid model name")
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
	return finiteFloat("min_p", sampling.MinP)
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

func (r Reasoning) stripThink() bool {
	if r.Strip != nil {
		return *r.Strip
	}
	switch r.Format {
	case FormatNone, FormatDeepseekLegacy:
		return false
	default:
		return true
	}
}

func validateReasoning(reasoning Reasoning) error {
	if reasoning.Effort != "" && !effortToken.MatchString(reasoning.Effort) {
		return fmt.Errorf("invalid reasoning effort %q", reasoning.Effort)
	}
	switch reasoning.Format {
	case "", FormatNone, FormatAuto, FormatDeepseek, FormatDeepseekLegacy:
	default:
		return fmt.Errorf("invalid reasoning format %q", reasoning.Format)
	}
	return nil
}

var errStreamStop = errors.New("chat stream stopped")

// readChatStream parses SSE events. When offset is non-nil, it adds the raw
// size of each fully read line, including the trailing newline. The count
// happens before the line is parsed, so a caller that stops inside yield has
// already counted that line. A nil offset leaves the count unchanged.
func readChatStream(body io.Reader, offset *atomic.Int64, yield func(sseEvent) bool) error {
	reader := bufio.NewReader(body)
	for {
		line, n, err := readSSELine(reader)
		if offset != nil && n > 0 {
			offset.Add(int64(n))
		}
		if line != "" {
			event, ok, parseErr := parseSSELine(line)
			if parseErr != nil {
				return parseErr
			}
			if ok && event.done {
				return nil
			}
			if ok && !yield(event) {
				return errStreamStop
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("read chat stream: %w", err)
		}
	}
}

func readSSELine(reader *bufio.Reader) (string, int, error) {
	var buf bytes.Buffer
	for {
		frag, err := reader.ReadSlice('\n')
		if _, writeErr := buf.Write(frag); writeErr != nil {
			return "", buf.Len(), writeErr
		}
		n := buf.Len()
		if n > maxSSELine {
			return "", n, fmt.Errorf("chat stream line is too long")
		}
		if err == nil || errors.Is(err, io.EOF) {
			return strings.TrimRight(buf.String(), "\r\n"), n, err
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return "", n, err
		}
	}
}

func parseSSELine(line string) (sseEvent, bool, error) {
	if line == "" || strings.HasPrefix(line, ":") {
		return sseEvent{}, false, nil
	}
	payload, ok := strings.CutPrefix(line, "data:")
	if !ok {
		return sseEvent{}, false, nil
	}
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return sseEvent{}, false, nil
	}
	if payload == "[DONE]" {
		return sseEvent{done: true}, true, nil
	}

	var chunk struct {
		ID    string `json:"id"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Choices []struct {
			Delta struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return sseEvent{}, false, fmt.Errorf("decode chat stream: %w", err)
	}
	if chunk.Error != nil {
		if chunk.Error.Message == "" {
			return sseEvent{}, false, fmt.Errorf("chat stream failed")
		}
		return sseEvent{}, false, fmt.Errorf("chat stream: %s", chunk.Error.Message)
	}
	event := sseEvent{id: chunk.ID}
	if len(chunk.Choices) > 0 {
		event.content = chunk.Choices[0].Delta.Content
		event.reasoning = chunk.Choices[0].Delta.ReasoningContent
	}
	return event, true, nil
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
