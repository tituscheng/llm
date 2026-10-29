// Package serve runs llama-server and sends prompts to its OpenAI-compatible API.
// Options.Model must be a GGUF file. A safetensors snapshot is the wrong input.
// ChatWith and Stream send reasoning options, including realtime reasoning control.
// Sampling sets per-request token limits. Nil fields leave llama-server's defaults.
// WithModel names which loaded model answers a chat request.
// Keep, Reuse, and Shutdown use llama-server resumable streams.
// StartRouter runs the server with no model so Download can fetch weights.
package serve

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"llm/install"
)

const (
	defaultHost   = "127.0.0.1"
	healthPath    = "/health"
	pollInterval  = 50 * time.Millisecond
	stderrLimit   = 4096
	toolLlamaName = "llama-server"
)

// Options configures a llama-server process.
type Options struct {
	// Model is the path to a GGUF file.
	// StartRouter leaves this empty and does not load a file.
	Model string
	// Bin is the llama-server executable. Empty uses ~/.llm/tools/llama-server.
	Bin string
	// Host is the listen address. Empty uses 127.0.0.1.
	Host string
	// Port is the listen port. 0 chooses a free localhost port.
	Port int
	// Context is the context size passed to llama-server. 0 leaves its default.
	Context int
	// Parallel is the KV cache slot count, passed as --parallel.
	// 0 leaves llama-server's default. A 4-bit 12B model uses 1 so it keeps a single slot.
	Parallel int
	// ReasoningBudget is the thinking token cap, passed as --reasoning-budget.
	// 0 leaves the server default. Qwen3.5-4B uses 1024.
	// Per-request reasoning format and enable_thinking stay on the chat request.
	ReasoningBudget int
	// NoMMProj passes --no-mmproj, so a local model does not load a projector.
	NoMMProj bool
	// Alias is the model id passed as --alias when Start launches the process.
	// Empty omits the flag. A server already listening is reused when its model
	// id equals Alias, the model path, or the file name.
	Alias string
}

// Server is a running llama-server process.
type Server struct {
	baseURL string
	cmd     *exec.Cmd
	client  *http.Client
	cancel  context.CancelFunc
	done    chan struct{}
	stderr  *tailBuf

	stopOnce sync.Once
	mu       sync.Mutex
	waitErr  error

	// model is the chat request model field. Empty omits it.
	model string

	// adoptedStop releases a server that was already listening. Handles from
	// WithModel share it. Nil when Start launched the process.
	adoptedStop *sync.Once
	listenPort  int
	unloadID    string
}

// WithModel returns a handle that sends name as the model field on chat requests.
// The router uses that field to choose which loaded model answers.
// Two callers can each keep a handle, such as Gemma 4 12B and Qwen3.5-4B.
// An empty name omits the field, which matches a server started with one GGUF file.
// The handle shares the process. Stop on either handle stops it.
// A name that contains a newline is rejected.
func (s *Server) WithModel(name string) (*Server, error) {
	if s == nil {
		return nil, fmt.Errorf("nil server")
	}
	if err := validateChatModel(name); err != nil {
		return nil, err
	}
	return &Server{
		baseURL:     s.baseURL,
		cmd:         s.cmd,
		client:      s.client,
		cancel:      s.cancel,
		done:        s.done,
		stderr:      s.stderr,
		model:       name,
		adoptedStop: s.adoptedStop,
		listenPort:  s.listenPort,
		unloadID:    s.unloadID,
	}, nil
}

// Start launches llama-server and returns after /health reports ready.
// When Port is set and that address is already serving this model, Start uses
// the running process. Cancelling ctx stops a process Start launched.
// Call Stop when the server is no longer needed. Stop also stops a server
// that was already listening.
func Start(ctx context.Context, opts Options) (*Server, error) {
	return launch(ctx, opts, false)
}

// StartRouter launches llama-server with no model loaded.
// The web UI downloads weights through this process.
// Options.Model must be empty.
// When Port is set and that address is already a llama-server with no model
// loaded, StartRouter uses that process.
// Cancelling ctx stops a process StartRouter launched.
func StartRouter(ctx context.Context, opts Options) (*Server, error) {
	return launch(ctx, opts, true)
}

func launch(ctx context.Context, opts Options, router bool) (*Server, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := validateOptions(opts, router); err != nil {
		return nil, err
	}
	if opts.Port != 0 {
		host := opts.Host
		if host == "" {
			host = defaultHost
		}
		srv, reused, err := reuseListening(ctx, opts, router, host, opts.Port)
		if err != nil || reused {
			return srv, err
		}
	}
	return spawn(ctx, opts, router)
}

func spawn(ctx context.Context, opts Options, router bool) (*Server, error) {
	bin, model, host, port, err := prepare(opts, router)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	args := []string{
		"--host", host,
		"--port", strconv.Itoa(port),
	}
	if !router {
		args = append(args, "-m", model)
		if alias := strings.TrimSpace(opts.Alias); alias != "" {
			args = append(args, "--alias", alias)
		}
	}
	if opts.Context > 0 {
		args = append(args, "--ctx-size", strconv.Itoa(opts.Context))
	}
	if opts.Parallel > 0 {
		args = append(args, "--parallel", strconv.Itoa(opts.Parallel))
	}
	if opts.ReasoningBudget > 0 {
		args = append(args, "--reasoning-budget", strconv.Itoa(opts.ReasoningBudget))
	}
	if opts.NoMMProj {
		args = append(args, "--no-mmproj")
	}
	cmd := exec.CommandContext(runCtx, bin, args...)
	logs := &tailBuf{max: stderrLimit}
	cmd.Stdout = io.Discard
	cmd.Stderr = logs
	configureCommand(cmd)

	srv := &Server{
		baseURL: "http://" + net.JoinHostPort(host, strconv.Itoa(port)),
		cmd:     cmd,
		client:  &http.Client{},
		cancel:  cancel,
		done:    make(chan struct{}),
		stderr:  logs,
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start llama-server: %w", err)
	}
	go srv.wait()

	if err := srv.waitReady(runCtx); err != nil {
		srv.Stop()
		return nil, err
	}
	return srv, nil
}

// Stop kills llama-server and its child processes. A second call is a no-op.
// A server that was already listening is unloaded and stopped too.
func (s *Server) Stop() error {
	if s.adoptedStop != nil {
		s.adoptedStop.Do(func() {
			s.releaseAdopted()
			close(s.done)
		})
	} else if s.cancel != nil {
		s.stopOnce.Do(func() { s.cancel() })
	}
	if s.done != nil {
		<-s.done
	}
	return nil
}

// Close calls Stop so Server implements io.Closer.
func (s *Server) Close() error {
	return s.Stop()
}

// URL returns the server's HTTP base URL.
func (s *Server) URL() string {
	return s.baseURL
}

// IsAlive reports whether the process is running and GET /health says the model is ready.
// A stopped server or a model that is still loading returns false.
// IsAlive does not start or stop the server.
func (s *Server) IsAlive(ctx context.Context) (bool, error) {
	if ctx == nil {
		return false, fmt.Errorf("nil context")
	}
	if s.done != nil {
		select {
		case <-s.done:
			return false, nil
		default:
		}
	}
	return healthReady(ctx, s.httpClient(), s.baseURL)
}

func (s *Server) httpClient() *http.Client {
	if s.client != nil {
		return s.client
	}
	return &http.Client{}
}

func (s *Server) wait() {
	err := s.cmd.Wait()
	s.mu.Lock()
	s.waitErr = err
	s.mu.Unlock()
	close(s.done)
}

func (s *Server) waitReady(ctx context.Context) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		ready, err := healthReady(ctx, s.httpClient(), s.baseURL)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return s.exitError()
		case <-ticker.C:
		}
	}
}

func (s *Server) exitError() error {
	s.mu.Lock()
	err := s.waitErr
	s.mu.Unlock()
	text := ""
	if s.stderr != nil {
		text = s.stderr.String()
	}
	if err == nil {
		err = fmt.Errorf("exit 0")
	}
	if text == "" {
		return fmt.Errorf("llama-server exited: %w", err)
	}
	return fmt.Errorf("llama-server exited: %w: %s", err, text)
}

func validateOptions(opts Options, router bool) error {
	if router {
		if opts.Model != "" {
			return fmt.Errorf("router mode does not load a model")
		}
	} else if opts.Model == "" {
		return fmt.Errorf("model path is empty")
	}
	if opts.Port < 0 || opts.Port > 65535 {
		return fmt.Errorf("invalid port %d", opts.Port)
	}
	if opts.Context < 0 {
		return fmt.Errorf("invalid context size %d", opts.Context)
	}
	if opts.Parallel < 0 {
		return fmt.Errorf("invalid parallel %d", opts.Parallel)
	}
	if opts.ReasoningBudget < 0 {
		return fmt.Errorf("invalid reasoning budget %d", opts.ReasoningBudget)
	}
	if strings.ContainsAny(opts.Alias, "\r\n") {
		return fmt.Errorf("invalid alias")
	}
	return nil
}

func prepare(opts Options, router bool) (bin, model, host string, port int, err error) {
	if err := validateOptions(opts, router); err != nil {
		return "", "", "", 0, err
	}
	if !router {
		info, err := os.Stat(opts.Model)
		if err != nil {
			return "", "", "", 0, fmt.Errorf("model: %w", err)
		}
		if !info.Mode().IsRegular() {
			return "", "", "", 0, fmt.Errorf("model %q is not a file", opts.Model)
		}
	}
	bin, err = resolveBin(opts.Bin)
	if err != nil {
		return "", "", "", 0, err
	}
	host = opts.Host
	if host == "" {
		host = defaultHost
	}
	port = opts.Port
	if port == 0 {
		port, err = freePort(host)
		if err != nil {
			return "", "", "", 0, err
		}
	}
	return bin, opts.Model, host, port, nil
}

func resolveBin(path string) (string, error) {
	if path != "" {
		info, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("llama-server: %w", err)
		}
		if info.IsDir() || info.Mode()&0o111 == 0 {
			return "", fmt.Errorf("llama-server %q is not executable", path)
		}
		return path, nil
	}
	ok, err := install.IsInstalled(toolLlamaName)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("llama-server is not installed")
	}
	dir, err := install.ToolsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, toolLlamaName), nil
}

func freePort(host string) (int, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return 0, fmt.Errorf("listen: %w", err)
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("listen address is not tcp")
	}
	return addr.Port, nil
}

type tailBuf struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (t *tailBuf) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-t.max:]...)
	}
	return len(p), nil
}

func (t *tailBuf) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
