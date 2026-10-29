// Package huggingface searches and downloads models from the Hugging Face Hub.
package huggingface

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// Model is a Hugging Face model repository.
type Model struct {
	ID          string   `json:"id"`
	Author      string   `json:"author,omitempty"`
	Downloads   int64    `json:"downloads,omitempty"`
	Likes       int64    `json:"likes,omitempty"`
	PipelineTag string   `json:"pipeline_tag,omitempty"`
	LibraryName string   `json:"library_name,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Private     bool     `json:"private,omitempty"`
	Gated       bool     `json:"gated,omitempty"`
	// Revision is the branch, tag, or commit to download. Empty means main.
	Revision string `json:"revision,omitempty"`

	hub *hub
}

type apiModel struct {
	ID          string    `json:"id"`
	Author      string    `json:"author"`
	Downloads   int64     `json:"downloads"`
	Likes       int64     `json:"likes"`
	PipelineTag string    `json:"pipeline_tag"`
	LibraryName string    `json:"library_name"`
	Tags        []string  `json:"tags"`
	Private     bool      `json:"private"`
	Gated       gatedFlag `json:"gated"`
}

type gatedFlag bool

func (g *gatedFlag) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case "false", "null":
		*g = false
		return nil
	case "true":
		*g = true
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("gated flag: %w", err)
	}
	*g = text != "" && text != "false"
	return nil
}

// Search returns Hugging Face models that match query.
// It returns at most 20 models in the order the Hub ranks them.
// query may be free text or a repository id such as "LiquidAI/LFM3-1.7B".
func Search(ctx context.Context, query string) ([]Model, error) {
	return search(ctx, defaultHub, query)
}

func search(ctx context.Context, h *hub, query string) ([]Model, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("model search query is empty")
	}
	rawURL, err := h.searchURL(query)
	if err != nil {
		return nil, err
	}
	resp, err := h.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		err := statusError(resp.StatusCode)
		drain(resp.Body)
		return nil, fmt.Errorf("search models: %w", err)
	}
	defer resp.Body.Close()

	var payload []apiModel
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode model search: %w", err)
	}
	models := make([]Model, 0, len(payload))
	for _, item := range payload {
		if item.ID == "" {
			continue
		}
		models = append(models, item.model(h))
	}
	return models, nil
}

func (h *hub) searchURL(query string) (string, error) {
	raw, err := joinURL(h.baseURL, "api", "models")
	if err != nil {
		return "", err
	}
	values := url.Values{}
	values.Set("search", query)
	values.Set("limit", fmt.Sprintf("%d", searchLimit))
	return withQuery(raw, values)
}

func (a apiModel) model(h *hub) Model {
	tags := append([]string(nil), a.Tags...)
	model := Model{
		ID:          a.ID,
		Author:      a.Author,
		Downloads:   a.Downloads,
		Likes:       a.Likes,
		PipelineTag: a.PipelineTag,
		LibraryName: a.LibraryName,
		Tags:        tags,
		Private:     a.Private,
		Gated:       bool(a.Gated),
		hub:         h,
	}
	if model.Author == "" {
		author, _, err := parseRepoID(model.ID)
		if err == nil {
			model.Author = author
		}
	}
	return model
}

func (m Model) client() *hub {
	if m.hub != nil {
		return m.hub
	}
	return defaultHub
}

func (m Model) revision() string {
	if m.Revision != "" {
		return m.Revision
	}
	return "main"
}
