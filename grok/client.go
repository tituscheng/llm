// Package grok calls the Grok chat completions API at https://api.x.ai/v1.
package grok

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

const (
	// Model is the model New sends. WithModel selects another id.
	Model = "grok-4.7"

	// EffortLow, EffortHigh, and EffortXHigh are the reasoning_effort values
	// sent on a chat completion. Empty Turn.Effort omits the field.
	EffortLow   = "low"
	EffortHigh  = "high"
	EffortXHigh = "xhigh"

	defaultBaseURL = "https://api.x.ai/v1"
	userAgent      = "llm"
)

// Client calls POST /v1/chat/completions.
// The zero Client is not usable. Use New.
type Client struct {
	baseURL    string
	key        string
	model      string
	httpClient *http.Client
}

// New returns a client for the Grok chat completions API.
// key is sent as a bearer token. An empty key is rejected.
// The client uses https://api.x.ai/v1/chat/completions and the model grok-4.7.
// It does not set a timeout. Cancel the request context to stop a call.
func New(key string) (*Client, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	return &Client{
		baseURL:    defaultBaseURL,
		key:        key,
		model:      Model,
		httpClient: newHTTPClient(),
	}, nil
}

// WithModel returns a client that sends name as the model.
// The name must be non-empty and must not contain a space or a newline.
func (c *Client) WithModel(name string) (*Client, error) {
	if c == nil {
		return nil, fmt.Errorf("nil client")
	}
	if err := validateModel(name); err != nil {
		return nil, err
	}
	next := *c
	next.model = name
	return &next, nil
}

// WithBaseURL returns a client that posts to raw.
// raw is an absolute http or https URL with no user, query, or fragment.
// Chat joins the chat completions path onto that URL.
func (c *Client) WithBaseURL(raw string) (*Client, error) {
	if c == nil {
		return nil, fmt.Errorf("nil client")
	}
	base, err := validateBaseURL(raw)
	if err != nil {
		return nil, err
	}
	next := *c
	next.baseURL = base
	return &next, nil
}

// WithHTTPClient returns a client that uses httpClient for requests.
// A nil client is rejected. Redirects follow httpClient's own policy.
func (c *Client) WithHTTPClient(httpClient *http.Client) (*Client, error) {
	if c == nil {
		return nil, fmt.Errorf("nil client")
	}
	if httpClient == nil {
		return nil, fmt.Errorf("nil http client")
	}
	next := *c
	next.httpClient = httpClient
	return &next, nil
}

func (c *Client) httpDo() *http.Client {
	if c.httpClient != nil {
		return c.httpClient
	}
	return newHTTPClient()
}

func newHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
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

func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("api key is empty")
	}
	if strings.ContainsFunc(key, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) {
		return fmt.Errorf("invalid api key")
	}
	return nil
}

func validateModel(name string) error {
	if name == "" {
		return fmt.Errorf("model name is empty")
	}
	if strings.TrimSpace(name) != name || strings.ContainsFunc(name, unicode.IsSpace) {
		return fmt.Errorf("invalid model name")
	}
	return nil
}

func validateBaseURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("invalid base url")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("invalid base url")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}
