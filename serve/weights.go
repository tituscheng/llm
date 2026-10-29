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
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// Sidecar kinds llama-server downloads with a model.
// An empty list leaves the server default, which also fetches mmproj.
const (
	SidecarMMProj = "mmproj"
	SidecarMTP    = "mtp"
	SidecarDFlash = "dflash"
	SidecarEagle3 = "eagle3"
	SidecarDSpark = "dspark"
)

// Weight is one file in a model download.
// Done and Total are byte counts. The web UI shows these for each weight.
type Weight struct {
	URL   string
	Done  int64
	Total int64
}

// DownloadEvent is one event from GET /models/sse.
// Weights is set when the event carries per-file download progress.
type DownloadEvent struct {
	Model   string
	Event   string
	Status  string
	Message string
	Weights []Weight
}

// CachedModel is one model returned by GET /models.
// Weights is set while its status is downloading.
type CachedModel struct {
	ID                 string
	Path               string
	Status             string
	Failed             bool
	ExitCode           int
	Weights            []Weight
	Sidecars           []string
	IncompleteSidecars []string
}

// Download asks llama-server to fetch a Hugging Face model tag.
// model looks like "ggml-org/gemma-3-4b-it-GGUF:Q4_K_M".
// A nil or empty sidecars list is omitted, so the server also fetches mmproj
// when the repository has one. The call returns when the server accepts the
// download. Follow the files with DownloadEvents or Models.
// The server must be running in router mode. StartRouter launches that mode.
func (s *Server) Download(ctx context.Context, model string, sidecars []string) error {
	if ctx == nil {
		return fmt.Errorf("nil context")
	}
	if err := validateModelName(model); err != nil {
		return err
	}
	if err := validateSidecars(sidecars); err != nil {
		return err
	}
	payload, err := json.Marshal(struct {
		Model    string   `json:"model"`
		Sidecars []string `json:"sidecars,omitempty"`
	}{Model: model, Sidecars: sidecars})
	if err != nil {
		return fmt.Errorf("encode download: %w", err)
	}
	return s.postModel(ctx, "/models", payload, "download")
}

// PauseDownload posts the model to /models/unload.
// The server keeps the partial weight files and lists the model as incomplete.
// Download the same tag again to continue.
func (s *Server) PauseDownload(ctx context.Context, model string) error {
	if ctx == nil {
		return fmt.Errorf("nil context")
	}
	if err := validateModelName(model); err != nil {
		return err
	}
	payload, err := json.Marshal(struct {
		Model string `json:"model"`
	}{Model: model})
	if err != nil {
		return fmt.Errorf("encode pause download: %w", err)
	}
	return s.postModel(ctx, "/models/unload", payload, "pause download")
}

// RemoveDownload deletes a cached model and the partial files of a download.
// It sends DELETE /models. The model name is the same tag passed to Download.
func (s *Server) RemoveDownload(ctx context.Context, model string) error {
	if ctx == nil {
		return fmt.Errorf("nil context")
	}
	if err := validateModelName(model); err != nil {
		return err
	}
	if err := s.requireAlive(ctx); err != nil {
		return err
	}
	query := url.Values{}
	query.Set("model", model)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.baseURL+"/models?"+query.Encode(), nil)
	if err != nil {
		return fmt.Errorf("remove download request: %w", err)
	}
	resp, err := s.httpClient().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("remove download: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read remove download response: %w", err)
	}
	return modelActionResult("remove download", resp.StatusCode, body)
}

// Models lists the models the server knows about, including a download in progress.
func (s *Server) Models(ctx context.Context) ([]CachedModel, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := s.requireAlive(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("models request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("models: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read models response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, streamHTTPError("models", resp.StatusCode, body)
	}
	var decoded struct {
		Data []struct {
			ID                 string   `json:"id"`
			Path               string   `json:"path"`
			Sidecars           []string `json:"sidecars"`
			IncompleteSidecars []string `json:"incomplete_sidecars"`
			Status             struct {
				Value    string                  `json:"value"`
				Failed   bool                    `json:"failed"`
				ExitCode int                     `json:"exit_code"`
				Progress map[string]fileProgress `json:"progress"`
			} `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}
	models := make([]CachedModel, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		models = append(models, CachedModel{
			ID:                 item.ID,
			Path:               item.Path,
			Status:             item.Status.Value,
			Failed:             item.Status.Failed,
			ExitCode:           item.Status.ExitCode,
			Weights:            weightsFrom(item.Status.Progress),
			Sidecars:           item.Sidecars,
			IncompleteSidecars: item.IncompleteSidecars,
		})
	}
	return models, nil
}

// DownloadEvents reads GET /models/sse until the caller stops or ctx is cancelled.
// A download_progress event carries one Weight per file. download_finished and
// download_failed report the end of that download. Breaking the loop stops the request.
func (s *Server) DownloadEvents(ctx context.Context) iter.Seq2[DownloadEvent, error] {
	return func(yield func(DownloadEvent, error) bool) {
		if ctx == nil {
			yield(DownloadEvent{}, fmt.Errorf("nil context"))
			return
		}
		if err := s.requireAlive(ctx); err != nil {
			yield(DownloadEvent{}, err)
			return
		}
		streamCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, s.baseURL+"/models/sse", nil)
		if err != nil {
			yield(DownloadEvent{}, fmt.Errorf("download events request: %w", err))
			return
		}
		req.Header.Set("Accept", "text/event-stream")
		resp, err := s.httpClient().Do(req)
		if err != nil {
			if ctx.Err() != nil {
				yield(DownloadEvent{}, ctx.Err())
				return
			}
			yield(DownloadEvent{}, fmt.Errorf("download events: %w", err))
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			yield(DownloadEvent{}, streamHTTPError("download events", resp.StatusCode, body))
			return
		}
		err = readModelEvents(resp.Body, func(event DownloadEvent) bool {
			return yield(event, nil)
		})
		if errors.Is(err, errStreamStop) {
			return
		}
		if ctx.Err() != nil {
			yield(DownloadEvent{}, ctx.Err())
			return
		}
		if err != nil {
			yield(DownloadEvent{}, err)
		}
	}
}

func (s *Server) postModel(ctx context.Context, path string, payload []byte, action string) error {
	if err := s.requireAlive(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("%s request: %w", action, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%s: %w", action, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read %s response: %w", action, err)
	}
	return modelActionResult(action, resp.StatusCode, body)
}

func modelActionResult(action string, status int, body []byte) error {
	if status != http.StatusOK {
		return streamHTTPError(action, status, body)
	}
	var decoded struct {
		Success bool `json:"success"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return fmt.Errorf("decode %s response: %w", action, err)
	}
	if decoded.Success {
		return nil
	}
	if decoded.Error.Message != "" {
		return fmt.Errorf("%s: %s", action, decoded.Error.Message)
	}
	return fmt.Errorf("%s failed", action)
}

func validateModelName(name string) error {
	if name == "" {
		return fmt.Errorf("model name is empty")
	}
	if strings.ContainsAny(name, "\r\n") {
		return fmt.Errorf("invalid model name")
	}
	return nil
}

func validateSidecars(sidecars []string) error {
	for _, kind := range sidecars {
		switch kind {
		case SidecarMMProj, SidecarMTP, SidecarDFlash, SidecarEagle3, SidecarDSpark:
		default:
			return fmt.Errorf("invalid sidecar %q", kind)
		}
	}
	return nil
}

type fileProgress struct {
	Done  int64 `json:"done"`
	Total int64 `json:"total"`
}

func weightsFrom(files map[string]fileProgress) []Weight {
	if len(files) == 0 {
		return nil
	}
	weights := make([]Weight, 0, len(files))
	for fileURL, progress := range files {
		weights = append(weights, Weight{URL: fileURL, Done: progress.Done, Total: progress.Total})
	}
	slices.SortFunc(weights, func(a, b Weight) int {
		return strings.Compare(a.URL, b.URL)
	})
	return weights
}

func readModelEvents(body io.Reader, yield func(DownloadEvent) bool) error {
	reader := bufio.NewReader(body)
	var sseName string
	for {
		line, _, err := readSSELine(reader)
		if line != "" {
			if name, ok := strings.CutPrefix(line, "event:"); ok {
				sseName = strings.TrimSpace(name)
			} else if event, ok, parseErr := parseModelEvent(line); parseErr != nil {
				return parseErr
			} else if ok {
				if event.Event == "" {
					event.Event = sseName
				}
				sseName = ""
				if !yield(event) {
					return errStreamStop
				}
			}
		} else {
			sseName = ""
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("read model events: %w", err)
		}
	}
}

func parseModelEvent(line string) (DownloadEvent, bool, error) {
	if line == "" || strings.HasPrefix(line, ":") {
		return DownloadEvent{}, false, nil
	}
	payload, ok := strings.CutPrefix(line, "data:")
	if !ok {
		return DownloadEvent{}, false, nil
	}
	payload = strings.TrimSpace(payload)
	if payload == "" || payload == "[DONE]" {
		return DownloadEvent{}, false, nil
	}
	var raw struct {
		Model string          `json:"model"`
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return DownloadEvent{}, false, fmt.Errorf("decode model event: %w", err)
	}
	event := DownloadEvent{Model: raw.Model, Event: raw.Event}
	if len(raw.Data) == 0 || string(raw.Data) == "null" {
		return event, true, nil
	}
	var files map[string]fileProgress
	if json.Unmarshal(raw.Data, &files) == nil && fileMap(raw.Data) {
		event.Weights = weightsFrom(files)
		return event, true, nil
	}
	var status struct {
		Status   string                   `json:"status"`
		Message  string                   `json:"message"`
		Error    struct{ Message string } `json:"error"`
		Progress map[string]fileProgress  `json:"progress"`
	}
	if err := json.Unmarshal(raw.Data, &status); err != nil {
		return DownloadEvent{}, false, fmt.Errorf("decode model event: %w", err)
	}
	event.Status = status.Status
	event.Message = status.Message
	if event.Message == "" {
		event.Message = status.Error.Message
	}
	event.Weights = weightsFrom(status.Progress)
	return event, true, nil
}

// fileMap reports whether data is a JSON object of per-file progress values.
// A status or error object has other fields, so it is not a file map.
func fileMap(data json.RawMessage) bool {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil || len(raw) == 0 {
		return false
	}
	for _, value := range raw {
		var fields map[string]json.RawMessage
		if json.Unmarshal(value, &fields) != nil {
			return false
		}
		_, hasDone := fields["done"]
		_, hasTotal := fields["total"]
		if !hasDone && !hasTotal {
			return false
		}
	}
	return true
}
