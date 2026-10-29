package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const reuseProbe = time.Second

func reuseListening(ctx context.Context, opts Options, router bool, host string, port int) (*Server, bool, error) {
	up, err := portListening(ctx, host, port)
	if err != nil {
		return nil, true, err
	}
	if !up {
		return nil, false, nil
	}
	probe, cancel := context.WithTimeout(ctx, reuseProbe)
	defer cancel()
	base := "http://" + net.JoinHostPort(host, strconv.Itoa(port))
	client := &http.Client{}
	ready, err := healthReady(probe, client, base)
	if ctx.Err() != nil {
		return nil, true, ctx.Err()
	}
	if err != nil || !ready {
		return nil, true, portBusy(port)
	}
	ids, err := modelIDs(probe, client, base)
	if ctx.Err() != nil {
		return nil, true, ctx.Err()
	}
	if err != nil {
		return nil, true, portBusy(port)
	}
	var unloadID string
	if router {
		if len(ids) != 0 {
			return nil, true, portBusy(port)
		}
	} else {
		id, ok := matchingModel(ids, opts.Model, opts.Alias)
		if !ok {
			return nil, true, portBusy(port)
		}
		unloadID = id
	}
	return &Server{
		baseURL:     base,
		client:      client,
		done:        make(chan struct{}),
		adoptedStop: &sync.Once{},
		listenPort:  port,
		unloadID:    unloadID,
	}, true, nil
}

func portListening(ctx context.Context, host string, port int) (bool, error) {
	dialer := &net.Dialer{Timeout: 200 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, nil
	}
	conn.Close()
	return true, nil
}

func portBusy(port int) error {
	return fmt.Errorf("port %d is already in use", port)
}

func modelIDs(ctx context.Context, client *http.Client, baseURL string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models status %d", resp.StatusCode)
	}
	var decoded struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(decoded.Data))
	for _, item := range decoded.Data {
		if item.ID != "" {
			ids = append(ids, item.ID)
		}
	}
	return ids, nil
}

func matchingModel(ids []string, modelPath, alias string) (string, bool) {
	alias = strings.TrimSpace(alias)
	base := filepath.Base(modelPath)
	for _, id := range ids {
		if id == "" {
			continue
		}
		if alias != "" && id == alias {
			return id, true
		}
		if id == modelPath || id == base || filepath.Clean(id) == filepath.Clean(modelPath) {
			return id, true
		}
	}
	return "", false
}

func (s *Server) releaseAdopted() {
	if s.unloadID != "" && s.baseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		payload, err := json.Marshal(struct {
			Model string `json:"model"`
		}{Model: s.unloadID})
		if err == nil {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/models/unload", bytes.NewReader(payload))
			if err == nil {
				req.Header.Set("Content-Type", "application/json")
				resp, err := s.httpClient().Do(req)
				if err == nil {
					_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
					resp.Body.Close()
				}
			}
		}
	}
	stopListening(s.listenPort)
}
