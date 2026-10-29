package huggingface

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type treeEntry struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	LFS  *struct {
		Size int64 `json:"size"`
	} `json:"lfs"`
}

func (e treeEntry) byteSize() int64 {
	if e.LFS != nil {
		return e.LFS.Size
	}
	return e.Size
}

type treeFile struct {
	Path string
	Size int64
}

// IsDownloaded reports whether every file in the model repository is already
// present in dest at the size published by the Hub.
// An empty dest uses $HOME/.llm/models/<author>/<name>.
// The directory is not created. A missing or partial snapshot returns false.
func (m Model) IsDownloaded(ctx context.Context, dest string) (bool, error) {
	if ctx == nil {
		return false, fmt.Errorf("nil context")
	}
	author, name, revision, dir, err := m.resolve(dest)
	if err != nil {
		return false, err
	}
	files, err := m.client().listFiles(ctx, author, name, revision)
	if err != nil {
		return false, err
	}
	for _, file := range files {
		target := filepath.Join(dir, filepath.FromSlash(file.Path))
		if err := withinDir(dir, target); err != nil {
			return false, err
		}
		present, err := alreadyPresent(target, file.Size)
		if err != nil || !present {
			return false, err
		}
	}
	return true, nil
}

// Download fetches every file in the model repository into dest.
// An empty dest uses $HOME/.llm/models/<author>/<name>.
// A file already in dest with the expected size is left in place, so a
// second call continues a partial download.
// The returned path is the directory that holds the files.
func (m Model) Download(ctx context.Context, dest string) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("nil context")
	}
	author, name, revision, dest, err := m.resolve(dest)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", fmt.Errorf("create model directory: %w", err)
	}

	h := m.client()
	files, err := h.listFiles(ctx, author, name, revision)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		if err := h.downloadFile(ctx, author, name, revision, dest, file); err != nil {
			return "", err
		}
	}
	return dest, nil
}

func (m Model) resolve(dest string) (author, name, revision, dir string, err error) {
	author, name, err = parseRepoID(m.ID)
	if err != nil {
		return "", "", "", "", err
	}
	revision = m.revision()
	if !validRevision(revision) {
		return "", "", "", "", fmt.Errorf("invalid revision %q", revision)
	}
	if dest == "" {
		root, err := ModelsDir()
		if err != nil {
			return "", "", "", "", err
		}
		dest = filepath.Join(root, author, name)
	}
	return author, name, revision, dest, nil
}

// ModelsDir returns $HOME/.llm/models. The directory is not created.
func ModelsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".llm", "models"), nil
}

func (h *hub) listFiles(ctx context.Context, author, name, revision string) ([]treeFile, error) {
	pageURL, err := h.treeURL(author, name, revision)
	if err != nil {
		return nil, err
	}
	files := []treeFile{}
	seen := map[string]bool{}
	for page := 0; page < maxTreePages; page++ {
		resp, err := h.get(ctx, pageURL)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			err := statusError(resp.StatusCode)
			drain(resp.Body)
			return nil, fmt.Errorf("list %s/%s: %w", author, name, err)
		}
		var entries []treeEntry
		decodeErr := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(&entries)
		next := nextLink(resp.Header)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("decode model files: %w", decodeErr)
		}
		for _, entry := range entries {
			if entry.Type != "file" {
				continue
			}
			rel, err := safeRel(entry.Path)
			if err != nil {
				return nil, err
			}
			if seen[rel] {
				continue
			}
			seen[rel] = true
			files = append(files, treeFile{Path: rel, Size: entry.byteSize()})
		}
		if next == "" {
			return files, nil
		}
		pageURL, err = sameHostNext(pageURL, next)
		if err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("model file list exceeded %d pages", maxTreePages)
}

func (h *hub) treeURL(author, name, revision string) (string, error) {
	raw, err := joinURL(h.baseURL, "api", "models", author, name, "tree", revision)
	if err != nil {
		return "", err
	}
	values := url.Values{}
	values.Set("recursive", "true")
	return withQuery(raw, values)
}

func (h *hub) downloadFile(ctx context.Context, author, name, revision, dest string, file treeFile) error {
	target := filepath.Join(dest, filepath.FromSlash(file.Path))
	if err := withinDir(dest, target); err != nil {
		return err
	}
	present, err := alreadyPresent(target, file.Size)
	if err != nil {
		return err
	}
	if present {
		return nil
	}
	elems := append([]string{author, name, "resolve", revision}, strings.Split(file.Path, "/")...)
	rawURL, err := joinURL(h.baseURL, elems...)
	if err != nil {
		return err
	}
	resp, err := h.get(ctx, rawURL)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		err := statusError(resp.StatusCode)
		drain(resp.Body)
		return fmt.Errorf("download %s: %w", file.Path, err)
	}
	defer resp.Body.Close()
	if err := save(resp.Body, target, file.Size); err != nil {
		return fmt.Errorf("download %s: %w", file.Path, err)
	}
	return nil
}

func alreadyPresent(target string, size int64) (bool, error) {
	info, err := os.Lstat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat model file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("model path %s is not a file", filepath.Base(target))
	}
	return info.Size() == size, nil
}

func save(body io.Reader, target string, size int64) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create model directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".partial-*")
	if err != nil {
		return fmt.Errorf("create model file: %w", err)
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		if !keep {
			os.Remove(tmpName)
		}
	}()

	written, err := io.Copy(tmp, body)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("write model file: %w", err)
	}
	if size > 0 && written != size {
		tmp.Close()
		return fmt.Errorf("got %d bytes, want %d", written, size)
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod model file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close model file: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("install model file: %w", err)
	}
	keep = true
	return nil
}

func withinDir(root, target string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("model directory: %w", err)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("model file: %w", err)
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return fmt.Errorf("model file: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("model file escapes destination")
	}
	return nil
}

func safeRel(name string) (string, error) {
	if name == "" || strings.Contains(name, `\`) || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("invalid model file path %q", name)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("invalid model file path %q", name)
	}
	return clean, nil
}

func parseRepoID(id string) (string, string, error) {
	author, name, ok := strings.Cut(id, "/")
	if !ok || strings.Contains(name, "/") || !validRepoPart(author) || !validRepoPart(name) {
		return "", "", fmt.Errorf("invalid model id %q", id)
	}
	return author, name, nil
}

func validRepoPart(value string) bool {
	if value == "" || len(value) > 96 {
		return false
	}
	for i, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-':
			if i == 0 || i == len(value)-1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validRevision(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}
