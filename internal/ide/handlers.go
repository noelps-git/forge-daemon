package ide

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/forgeapp/forge-daemon/internal/ai"
	"github.com/forgeapp/forge-daemon/internal/config"
	"github.com/forgeapp/forge-daemon/internal/index"
)

// Version is injected at link time via -ldflags.
var Version = "0.1.0"

// validIDEs is the set of IDE names the daemon recognises.
var validIDEs = map[string]bool{
	"xcode":         true,
	"androidstudio": true,
	"intellij":      true,
	"vscode":        true,
	"other":         true,
}

// Session holds the per-IDE-connection state.
type Session struct {
	Token       string
	ContextID   string
	IDE         string
	ProjectPath string
	Index       *index.Index
}

// Handlers bundles the mutable state shared by all HTTP handlers.
type Handlers struct {
	mu       sync.RWMutex
	sessions map[string]*Session // keyed by token
	cfg      *config.Config
	provider ai.Provider
}

// NewHandlers builds a Handlers from a config.
// The provider is created once and shared across all sessions.
func NewHandlers(cfg *config.Config, provider ai.Provider) *Handlers {
	return &Handlers{
		sessions: make(map[string]*Session),
		cfg:      cfg,
		provider: provider,
	}
}

// HealthHandler handles GET /health.
func (h *Handlers) HealthHandler(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Status:  "ok",
		Version: Version,
		AIReady: h.provider != nil,
		Port:    h.cfg.Daemon.Port,
	}
	writeJSON(w, http.StatusOK, resp)
}

// ConnectHandler handles POST /ide/v1/connect.
func (h *Handlers) ConnectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ConnectRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	ide := strings.ToLower(strings.TrimSpace(req.IDE))
	if !validIDEs[ide] {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown IDE %q (valid: xcode, androidstudio, intellij, vscode, other)", req.IDE))
		return
	}
	if req.ProjectPath == "" {
		writeError(w, http.StatusBadRequest, "project_path is required")
		return
	}

	token := "frg_" + randomHex(16)
	contextID := "ctx_" + randomHex(8)

	// Create a project index for this session.
	idx, err := index.New(req.ProjectPath)
	if err != nil {
		// Non-fatal: daemon works without a valid index (e.g. path doesn't exist yet in tests).
		idx = nil
	} else {
		// Start the watcher in the background; ignore errors so connect always succeeds.
		go idx.Start() //nolint:errcheck
	}

	sess := &Session{
		Token:       token,
		ContextID:   contextID,
		IDE:         ide,
		ProjectPath: req.ProjectPath,
		Index:       idx,
	}

	h.mu.Lock()
	h.sessions[token] = sess
	h.mu.Unlock()

	writeJSON(w, http.StatusOK, ConnectResponse{
		Token:     token,
		ContextID: contextID,
	})
}

// AskHandler handles POST /ide/v1/ask.
func (h *Handlers) AskHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sess, ok := h.requireToken(w, r)
	if !ok {
		return
	}

	var req AskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Instruction == "" {
		writeError(w, http.StatusBadRequest, "instruction is required")
		return
	}

	// Build context files.
	var ctxFiles []ai.ContextFile
	if sess.Index != nil && h.cfg.Privacy.SendFileContent {
		if req.FilePath != "" {
			if content, err := sess.Index.ReadFile(req.FilePath); err == nil {
				ctxFiles = append(ctxFiles, ai.ContextFile{Path: req.FilePath, Content: content})
			}
		}
	}
	if req.Selection != "" {
		ctxFiles = append(ctxFiles, ai.ContextFile{
			Path:    "selection",
			Content: req.Selection,
		})
	}

	provReq := ai.AskRequest{
		Instruction: req.Instruction,
		Context:     ctxFiles,
		Stream:      req.Stream,
	}

	ch, err := h.provider.Ask(r.Context(), provReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "AI error: "+err.Error())
		return
	}

	if req.Stream {
		// SSE streaming response.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher, canFlush := w.(http.Flusher)
		for chunk := range ch {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			if canFlush {
				flusher.Flush()
			}
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if canFlush {
			flusher.Flush()
		}
		return
	}

	// Non-streaming: collect all tokens.
	var sb strings.Builder
	for chunk := range ch {
		sb.WriteString(chunk)
	}
	writeJSON(w, http.StatusOK, AskResponse{
		Response: sb.String(),
		Model:    h.provider.Model(),
	})
}

// ErrorsHandler handles GET /ide/v1/errors.
func (h *Handlers) ErrorsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := h.requireToken(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, ErrorsResponse{Errors: []BuildError{}})
}

// DiffHandler handles POST /ide/v1/diff.
func (h *Handlers) DiffHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sess, ok := h.requireToken(w, r)
	if !ok {
		return
	}

	var req DiffRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.FilePath == "" {
		writeError(w, http.StatusBadRequest, "file_path is required")
		return
	}
	if req.Diff == "" {
		writeError(w, http.StatusBadRequest, "diff is required")
		return
	}

	if sess.Index == nil {
		writeJSON(w, http.StatusOK, DiffResponse{Applied: false, Error: "no project index for this session"})
		return
	}

	if err := sess.Index.ApplyDiff(req.FilePath, req.Diff); err != nil {
		writeJSON(w, http.StatusOK, DiffResponse{Applied: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, DiffResponse{Applied: true})
}

// ---------- helpers ----------

func (h *Handlers) requireToken(w http.ResponseWriter, r *http.Request) (*Session, bool) {
	token := r.Header.Get("X-Forge-Token")
	if token == "" {
		writeError(w, http.StatusUnauthorized, "X-Forge-Token header required")
		return nil, false
	}
	h.mu.RLock()
	sess, ok := h.sessions[token]
	h.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid or expired token")
		return nil, false
	}
	return sess, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MiB limit
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}
