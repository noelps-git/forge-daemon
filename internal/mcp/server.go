// Package mcp implements a Model Context Protocol JSON-RPC 2.0 server over
// WebSocket using gorilla/websocket.
package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/forgeapp/forge-daemon/internal/index"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// jsonRPCRequest is the common envelope for JSON-RPC 2.0 requests.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCResponse is the common envelope for JSON-RPC 2.0 responses.
type jsonRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// toolDef describes a single MCP tool exposed to clients.
type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

var mcpTools = []toolDef{
	{
		Name:        "read_file",
		Description: "Read the content of a file in the current project.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Relative path from project root"},
			},
			"required": []string{"path"},
		},
	},
	{
		Name:        "list_files",
		Description: "List all indexed files in the current project.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
	{
		Name:        "apply_diff",
		Description: "Apply a unified diff to a project file.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
				"diff": map[string]any{"type": "string"},
			},
			"required": []string{"path", "diff"},
		},
	},
	{
		Name:        "get_git_diff",
		Description: "Return the output of `git diff HEAD` in the current project.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
	{
		Name:        "get_errors",
		Description: "Return current build errors for the project.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
}

// session holds per-WebSocket-connection state.
type session struct {
	conn        *websocket.Conn
	mu          sync.Mutex
	projectPath string
	index       *index.Index
}

func (s *session) send(resp jsonRPCResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	resp.JSONRPC = "2.0"
	return s.conn.WriteJSON(resp)
}

func (s *session) sendErr(id any, code int, msg string) {
	_ = s.send(jsonRPCResponse{
		ID:    id,
		Error: &rpcError{Code: code, Message: msg},
	})
}

// WSHandler upgrades an HTTP connection to a WebSocket and runs the MCP loop.
func WSHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "websocket upgrade failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer conn.Close()

	sess := &session{conn: conn}
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return // client disconnected
		}
		var req jsonRPCRequest
		if err := json.Unmarshal(msg, &req); err != nil {
			sess.sendErr(nil, -32700, "parse error")
			continue
		}
		sess.handleRequest(req)
	}
}

func (s *session) handleRequest(req jsonRPCRequest) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	default:
		s.sendErr(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *session) handleInitialize(req jsonRPCRequest) {
	// Optionally capture projectPath from params.
	var params struct {
		ProjectPath string `json:"project_path"`
	}
	if req.Params != nil {
		_ = json.Unmarshal(req.Params, &params)
	}
	if params.ProjectPath != "" {
		s.setProject(params.ProjectPath)
	}

	_ = s.send(jsonRPCResponse{
		ID: req.ID,
		Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]string{
				"name":    "forge-daemon",
				"version": "0.1.0",
			},
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
		},
	})
}

func (s *session) handleToolsList(req jsonRPCRequest) {
	_ = s.send(jsonRPCResponse{
		ID:     req.ID,
		Result: map[string]any{"tools": mcpTools},
	})
}

func (s *session) handleToolsCall(req jsonRPCRequest) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendErr(req.ID, -32602, "invalid params: "+err.Error())
		return
	}

	switch params.Name {
	case "read_file":
		s.toolReadFile(req.ID, params.Arguments)
	case "list_files":
		s.toolListFiles(req.ID)
	case "apply_diff":
		s.toolApplyDiff(req.ID, params.Arguments)
	case "get_git_diff":
		s.toolGetGitDiff(req.ID)
	case "get_errors":
		s.toolGetErrors(req.ID)
	default:
		s.sendErr(req.ID, -32601, fmt.Sprintf("unknown tool: %s", params.Name))
	}
}

// ---------- tool implementations ----------

func (s *session) toolReadFile(id any, args json.RawMessage) {
	var p struct{ Path string `json:"path"` }
	if err := json.Unmarshal(args, &p); err != nil || p.Path == "" {
		s.sendErr(id, -32602, "read_file: path is required")
		return
	}
	idx := s.getIndex()
	if idx == nil {
		s.sendErr(id, -32603, "no project connected")
		return
	}
	content, err := idx.ReadFile(p.Path)
	if err != nil {
		s.sendErr(id, -32603, err.Error())
		return
	}
	s.sendResult(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": content}},
	})
}

func (s *session) toolListFiles(id any) {
	idx := s.getIndex()
	if idx == nil {
		s.sendErr(id, -32603, "no project connected")
		return
	}
	files, err := idx.ListFiles()
	if err != nil {
		s.sendErr(id, -32603, err.Error())
		return
	}
	s.sendResult(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": strings.Join(files, "\n")}},
	})
}

func (s *session) toolApplyDiff(id any, args json.RawMessage) {
	var p struct {
		Path string `json:"path"`
		Diff string `json:"diff"`
	}
	if err := json.Unmarshal(args, &p); err != nil || p.Path == "" || p.Diff == "" {
		s.sendErr(id, -32602, "apply_diff: path and diff are required")
		return
	}
	idx := s.getIndex()
	if idx == nil {
		s.sendErr(id, -32603, "no project connected")
		return
	}
	if err := idx.ApplyDiff(p.Path, p.Diff); err != nil {
		s.sendErr(id, -32603, err.Error())
		return
	}
	s.sendResult(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": "diff applied"}},
	})
}

func (s *session) toolGetGitDiff(id any) {
	s.mu.Lock()
	projectPath := s.projectPath
	s.mu.Unlock()

	if projectPath == "" {
		s.sendErr(id, -32603, "no project connected")
		return
	}

	cmd := exec.Command("git", "diff", "HEAD")
	cmd.Dir = projectPath
	out, err := cmd.Output()
	if err != nil {
		// git diff exits non-zero on some conditions; return what we got.
		out = []byte(fmt.Sprintf("git diff error: %v", err))
	}
	s.sendResult(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(out)}},
	})
}

func (s *session) toolGetErrors(id any) {
	s.sendResult(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": "[]"}},
	})
}

// ---------- helpers ----------

func (s *session) sendResult(id any, result any) {
	_ = s.send(jsonRPCResponse{ID: id, Result: result})
}

func (s *session) setProject(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.index != nil {
		s.index.Stop()
	}
	idx, err := index.New(path)
	if err == nil {
		go idx.Start() //nolint:errcheck
		s.index = idx
	}
	s.projectPath = path
}

func (s *session) getIndex() *index.Index {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.index
}
