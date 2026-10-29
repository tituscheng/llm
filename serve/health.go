package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func healthReady(ctx context.Context, client *http.Client, baseURL string) (bool, error) {
	if ctx == nil {
		return false, fmt.Errorf("nil context")
	}
	if client == nil {
		client = &http.Client{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+healthPath, nil)
	if err != nil {
		return false, fmt.Errorf("health request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, nil
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return true, nil
	}
	var payload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return true, nil
	}
	if payload.Status == "" || strings.EqualFold(payload.Status, "ok") {
		return true, nil
	}
	return false, nil
}
