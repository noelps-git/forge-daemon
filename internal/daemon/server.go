// Package daemon wires together the HTTP server, IDE handlers, and MCP WebSocket.
package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/forgeapp/forge-daemon/internal/ai"
	"github.com/forgeapp/forge-daemon/internal/config"
	"github.com/forgeapp/forge-daemon/internal/ide"
	"github.com/forgeapp/forge-daemon/internal/mcp"
)

// Server is the top-level daemon struct.
type Server struct {
	cfg      *config.Config
	httpSrv  *http.Server
	handlers *ide.Handlers
	provider ai.Provider
}

// New creates a Server from cfg.
// The AI provider is selected by cfg.AI.DefaultProvider.
func New(cfg *config.Config) *Server {
	providerName := cfg.AI.DefaultProvider
	if cfg.Privacy.LocalMode {
		providerName = "ollama"
	}

	provider, err := ai.New(providerName, cfg)
	if err != nil {
		// Fall back to null provider so the daemon still runs without an API key.
		fmt.Fprintf(os.Stderr, "forge: warning: AI provider %q unavailable (%v), falling back to null\n", providerName, err)
		provider, _ = ai.New("null", cfg)
	}

	handlers := ide.NewHandlers(cfg, provider)
	mux := buildMux(handlers)

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Daemon.Port))
	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &Server{
		cfg:      cfg,
		httpSrv:  httpSrv,
		handlers: handlers,
		provider: provider,
	}
}

// NewWithProvider creates a Server using the given provider. Useful for tests.
func NewWithProvider(cfg *config.Config, provider ai.Provider) *Server {
	handlers := ide.NewHandlers(cfg, provider)
	mux := buildMux(handlers)

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Daemon.Port))
	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &Server{
		cfg:      cfg,
		httpSrv:  httpSrv,
		handlers: handlers,
		provider: provider,
	}
}

// Handler returns the underlying http.Handler for use with httptest.Server.
func (s *Server) Handler() http.Handler { return s.httpSrv.Handler }

// Start binds the port, writes the pid file, and blocks until the server exits.
func (s *Server) Start() error {
	if err := writePidFile(s.cfg.Daemon.PidFile); err != nil {
		fmt.Fprintf(os.Stderr, "forge: warning: cannot write pid file: %v\n", err)
	}

	fmt.Printf("forge daemon listening on http://127.0.0.1:%d\n", s.cfg.Daemon.Port)
	if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("daemon: listen: %w", err)
	}
	return nil
}

// Stop gracefully shuts the server down.
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = os.Remove(s.cfg.Daemon.PidFile)
	return s.httpSrv.Shutdown(ctx)
}

// buildMux registers all routes.
func buildMux(h *ide.Handlers) *http.ServeMux {
	mux := http.NewServeMux()

	// Health check (no auth).
	mux.HandleFunc("/health", h.HealthHandler)

	// IDE REST API.
	mux.HandleFunc("/ide/v1/connect", h.ConnectHandler)
	mux.HandleFunc("/ide/v1/ask", h.AskHandler)
	mux.HandleFunc("/ide/v1/errors", h.ErrorsHandler)
	mux.HandleFunc("/ide/v1/diff", h.DiffHandler)

	// MCP WebSocket.
	mux.HandleFunc("/mcp", mcp.WSHandler)

	return mux
}

// writePidFile writes the current process PID to path.
func writePidFile(path string) error {
	if path == "" {
		return nil
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644)
}
