package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/forgeapp/forge-daemon/internal/ai"
	"github.com/forgeapp/forge-daemon/internal/config"
	"github.com/forgeapp/forge-daemon/internal/daemon"
	"github.com/forgeapp/forge-daemon/internal/ide"
)

// ---------- test helpers ----------

func testConfig() *config.Config {
	cfg := config.Default()
	cfg.AI.DefaultProvider = "null"
	cfg.Privacy.SendFileContent = true
	return cfg
}

func nullProvider() ai.Provider {
	p, _ := ai.New("null", testConfig())
	return p
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := testConfig()
	srv := daemon.NewWithProvider(cfg, nullProvider())
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func postJSON(t *testing.T, ts *httptest.Server, path string, body any, token string) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Forge-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

// connect issues a /ide/v1/connect and returns the session token.
func connect(t *testing.T, ts *httptest.Server, ideName, projectPath string) string {
	t.Helper()
	resp := postJSON(t, ts, "/ide/v1/connect", ide.ConnectRequest{
		IDE:         ideName,
		ProjectPath: projectPath,
	}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("connect: expected 200, got %d", resp.StatusCode)
	}
	var cr ide.ConnectResponse
	decodeJSON(t, resp, &cr)
	if cr.Token == "" {
		t.Fatal("connect: empty token")
	}
	return cr.Token
}

// ---------- tests ----------

func TestHealthEndpoint(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var h ide.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if h.Status != "ok" {
		t.Errorf("expected status=ok, got %q", h.Status)
	}
	if h.Version == "" {
		t.Error("expected non-empty version")
	}
	if h.Port <= 0 {
		t.Errorf("expected positive port, got %d", h.Port)
	}
}

func TestConnect_Xcode(t *testing.T) {
	ts := newTestServer(t)
	tmp := t.TempDir()

	resp := postJSON(t, ts, "/ide/v1/connect", ide.ConnectRequest{
		IDE:         "xcode",
		ProjectPath: tmp,
	}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var cr ide.ConnectResponse
	decodeJSON(t, resp, &cr)

	if !strings.HasPrefix(cr.Token, "frg_") {
		t.Errorf("expected token starting with frg_, got %q", cr.Token)
	}
	if !strings.HasPrefix(cr.ContextID, "ctx_") {
		t.Errorf("expected context_id starting with ctx_, got %q", cr.ContextID)
	}
}

func TestConnect_AndroidStudio(t *testing.T) {
	ts := newTestServer(t)
	tmp := t.TempDir()

	resp := postJSON(t, ts, "/ide/v1/connect", ide.ConnectRequest{
		IDE:         "androidstudio",
		ProjectPath: tmp,
	}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var cr ide.ConnectResponse
	decodeJSON(t, resp, &cr)
	if cr.Token == "" {
		t.Error("expected non-empty token")
	}
}

func TestConnect_InvalidIDE(t *testing.T) {
	ts := newTestServer(t)
	tmp := t.TempDir()

	resp := postJSON(t, ts, "/ide/v1/connect", ide.ConnectRequest{
		IDE:         "notepad++",
		ProjectPath: tmp,
	}, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var body map[string]string
	decodeJSON(t, resp, &body)
	if body["error"] == "" {
		t.Error("expected error message in body")
	}
}

func TestAskWithoutToken(t *testing.T) {
	ts := newTestServer(t)

	resp := postJSON(t, ts, "/ide/v1/ask", ide.AskRequest{
		Instruction: "hello",
	}, "") // no token
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAskWithNullProvider(t *testing.T) {
	ts := newTestServer(t)
	tmp := t.TempDir()

	token := connect(t, ts, "xcode", tmp)

	resp := postJSON(t, ts, "/ide/v1/ask", ide.AskRequest{
		ContextID:   "ctx_test",
		Instruction: "What does this code do?",
		Stream:      false,
	}, token)
	if resp.StatusCode != http.StatusOK {
		body := make([]byte, 512)
		n, _ := resp.Body.Read(body)
		resp.Body.Close()
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body[:n])
	}

	var ar ide.AskResponse
	decodeJSON(t, resp, &ar)
	if ar.Response == "" {
		t.Error("expected non-empty response from null provider")
	}
	if ar.Model == "" {
		t.Error("expected non-empty model name")
	}
}

func TestListFiles_Empty(t *testing.T) {
	ts := newTestServer(t)
	tmp := t.TempDir()

	token := connect(t, ts, "xcode", tmp)

	// Give the index watcher a moment to start (if it does).
	time.Sleep(50 * time.Millisecond)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/ide/v1/errors", nil)
	req.Header.Set("X-Forge-Token", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var er ide.ErrorsResponse
	decodeJSON(t, resp, &er)
	if er.Errors == nil {
		t.Error("errors field should be non-nil (empty slice)")
	}
	if len(er.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(er.Errors))
	}
}

func TestDiffApply_Simple(t *testing.T) {
	ts := newTestServer(t)
	tmp := t.TempDir()

	// Create a file in the temp dir.
	filePath := filepath.Join(tmp, "hello.txt")
	original := "line1\nline2\nline3\n"
	if err := os.WriteFile(filePath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	token := connect(t, ts, "xcode", tmp)

	// Give the index time to scan the file.
	time.Sleep(150 * time.Millisecond)

	// A unified diff that replaces "line2" with "line2-modified".
	diff := `--- hello.txt
+++ hello.txt
@@ -1,3 +1,3 @@
 line1
-line2
+line2-modified
 line3
`

	resp := postJSON(t, ts, "/ide/v1/diff", ide.DiffRequest{
		ContextID: "ctx_test",
		FilePath:  "hello.txt",
		Diff:      diff,
	}, token)
	if resp.StatusCode != http.StatusOK {
		body := make([]byte, 512)
		n, _ := resp.Body.Read(body)
		resp.Body.Close()
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body[:n])
	}

	var dr ide.DiffResponse
	decodeJSON(t, resp, &dr)
	if !dr.Applied {
		t.Errorf("expected applied=true, error: %s", dr.Error)
	}

	// Verify the file on disk was updated.
	updated, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "line2-modified") {
		t.Errorf("file was not updated on disk: %q", string(updated))
	}
}

// ---------- MCP WebSocket tests ----------

// dialMCP returns a WebSocket connection to the test server's /mcp endpoint.
func dialMCP(t *testing.T, ts *httptest.Server) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/mcp"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial MCP: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

type rpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

func sendRPC(t *testing.T, conn *websocket.Conn, id int, method string, params any) {
	t.Helper()
	msg := rpcMsg{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	if err := conn.WriteJSON(msg); err != nil {
		t.Fatalf("sendRPC: %v", err)
	}
}

func readRPC(t *testing.T, conn *websocket.Conn) rpcMsg {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("readRPC: %v", err)
	}
	var msg rpcMsg
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("readRPC unmarshal: %v", err)
	}
	return msg
}

func TestMCPWebSocket_Initialize(t *testing.T) {
	ts := newTestServer(t)
	conn := dialMCP(t, ts)

	sendRPC(t, conn, 1, "initialize", map[string]string{"project_path": t.TempDir()})
	resp := readRPC(t, conn)

	if resp.Error != nil {
		t.Fatalf("initialize error: %s", resp.Error)
	}
	if resp.Result == nil {
		t.Fatal("expected result, got nil")
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if _, ok := result["serverInfo"]; !ok {
		t.Error("expected serverInfo in result")
	}
	if _, ok := result["capabilities"]; !ok {
		t.Error("expected capabilities in result")
	}
}

func TestMCPWebSocket_ToolsList(t *testing.T) {
	ts := newTestServer(t)
	conn := dialMCP(t, ts)

	sendRPC(t, conn, 2, "tools/list", nil)
	resp := readRPC(t, conn)

	if resp.Error != nil {
		t.Fatalf("tools/list error: %s", resp.Error)
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	tools, ok := result["tools"]
	if !ok {
		t.Fatal("expected tools in result")
	}
	toolList, ok := tools.([]any)
	if !ok {
		t.Fatalf("tools should be array, got %T", tools)
	}
	if len(toolList) < 5 {
		t.Errorf("expected at least 5 tools, got %d", len(toolList))
	}

	// Verify tool names.
	wantTools := map[string]bool{
		"read_file": false, "list_files": false, "apply_diff": false,
		"get_git_diff": false, "get_errors": false,
	}
	for _, tl := range toolList {
		if m, ok := tl.(map[string]any); ok {
			if name, ok := m["name"].(string); ok {
				wantTools[name] = true
			}
		}
	}
	for name, found := range wantTools {
		if !found {
			t.Errorf("tool %q not found in tools/list response", name)
		}
	}
}

func TestMCPWebSocket_ReadFile(t *testing.T) {
	ts := newTestServer(t)
	tmp := t.TempDir()

	// Write a file for the MCP server to read.
	content := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	conn := dialMCP(t, ts)

	// Initialize with the project path.
	sendRPC(t, conn, 1, "initialize", map[string]string{"project_path": tmp})
	readRPC(t, conn) // consume initialize response

	// Give the index time to scan.
	time.Sleep(200 * time.Millisecond)

	sendRPC(t, conn, 2, "tools/call", map[string]any{
		"name":      "read_file",
		"arguments": map[string]string{"path": "main.go"},
	})
	resp := readRPC(t, conn)

	if resp.Error != nil {
		t.Fatalf("read_file error: %s", resp.Error)
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	contentArr, ok := result["content"].([]any)
	if !ok || len(contentArr) == 0 {
		t.Fatal("expected content array in result")
	}
	block, ok := contentArr[0].(map[string]any)
	if !ok {
		t.Fatal("expected content[0] to be object")
	}
	text, _ := block["text"].(string)
	if !strings.Contains(text, "func main") {
		t.Errorf("expected file content in response, got: %q", text)
	}
}

func TestMCPWebSocket_GetGitDiff(t *testing.T) {
	ts := newTestServer(t)
	tmp := t.TempDir()

	conn := dialMCP(t, ts)

	// Initialize with a project path (git may or may not be present).
	sendRPC(t, conn, 1, "initialize", map[string]string{"project_path": tmp})
	readRPC(t, conn)

	sendRPC(t, conn, 2, "tools/call", map[string]any{
		"name":      "get_git_diff",
		"arguments": map[string]any{},
	})
	resp := readRPC(t, conn)

	// get_git_diff should always return a result (even if git fails, it returns an error string).
	// We don't require success here — just that the call doesn't hang.
	if resp.ID.(float64) != 2 {
		t.Errorf("unexpected response ID: %v", resp.ID)
	}
	// Either result or error is acceptable — git may not be installed in CI.
}
