package huggingface

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	hubBaseURL    = "https://huggingface.co"
	userAgent     = "llm"
	searchLimit   = 20
	maxTreePages  = 20
	maxJSONBytes  = 32 << 20
	headerTimeout = 60 * time.Second
)

// defaultHub is the client used by Search and by Model.Download when the
// model was not produced by a test client.
var defaultHub = &hub{
	baseURL: hubBaseURL,
	http:    newHTTPClient(),
}

type hub struct {
	baseURL string
	http    *http.Client
	token   string
}

func newHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = headerTimeout
	return &http.Client{
		Transport:     transport,
		CheckRedirect: dropAuthOnCrossHostRedirect,
	}
}

func dropAuthOnCrossHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	if len(via) > 0 && !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
		req.Header.Del("Authorization")
	}
	return nil
}

func (h *hub) httpClient() *http.Client {
	if h.http != nil {
		return h.http
	}
	return defaultHub.http
}

func (h *hub) tokenValue() string {
	if h.token != "" {
		return h.token
	}
	if value := os.Getenv("HF_TOKEN"); value != "" {
		return value
	}
	return os.Getenv("HUGGING_FACE_HUB_TOKEN")
}

func (h *hub) get(ctx context.Context, rawURL string) (*http.Response, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if token := h.tokenValue(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := h.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("huggingface request: %w", err)
	}
	return resp, nil
}

func statusError(code int) error {
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("huggingface access denied")
	case http.StatusNotFound:
		return fmt.Errorf("huggingface model not found")
	default:
		return fmt.Errorf("huggingface status %d", code)
	}
}

func drain(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 4096))
	body.Close()
}

func joinURL(base string, elem ...string) (string, error) {
	raw, err := url.JoinPath(base, elem...)
	if err != nil {
		return "", fmt.Errorf("hub url: %w", err)
	}
	return raw, nil
}

func withQuery(raw string, values url.Values) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("hub url: %w", err)
	}
	u.RawQuery = values.Encode()
	return u.String(), nil
}

func sameHostNext(current, next string) (string, error) {
	cur, err := url.Parse(current)
	if err != nil {
		return "", fmt.Errorf("hub url: %w", err)
	}
	ref, err := url.Parse(next)
	if err != nil {
		return "", fmt.Errorf("hub page link: %w", err)
	}
	resolved := cur.ResolveReference(ref)
	if !strings.EqualFold(resolved.Host, cur.Host) {
		return "", fmt.Errorf("huggingface page link left the hub")
	}
	return resolved.String(), nil
}

func nextLink(header http.Header) string {
	for _, part := range strings.Split(header.Get("Link"), ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) && !strings.Contains(part, "rel=next") {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start >= 0 && end > start {
			return part[start+1 : end]
		}
	}
	return ""
}
