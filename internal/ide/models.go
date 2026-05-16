// Package ide contains HTTP handlers and request/response models for the
// IDE REST API exposed at /ide/v1/...
package ide

// ConnectRequest is sent by an IDE when it first connects to the daemon.
type ConnectRequest struct {
	IDE         string `json:"ide"`          // e.g. "xcode", "androidstudio"
	ProjectPath string `json:"project_path"` // absolute path on disk
}

// ConnectResponse is returned on a successful /ide/v1/connect call.
type ConnectResponse struct {
	Token     string `json:"token"`      // X-Forge-Token value for subsequent calls
	ContextID string `json:"context_id"` // opaque ID for this session
}

// AskRequest is sent to /ide/v1/ask.
type AskRequest struct {
	ContextID   string `json:"context_id"`
	Instruction string `json:"instruction"`
	Stream      bool   `json:"stream"`
	Selection   string `json:"selection,omitempty"` // editor selection, if any
	FilePath    string `json:"file_path,omitempty"` // currently open file
}

// AskResponse is returned from /ide/v1/ask when Stream is false.
type AskResponse struct {
	Response string `json:"response"`
	Model    string `json:"model"`
}

// ErrorsResponse is returned from GET /ide/v1/errors.
type ErrorsResponse struct {
	Errors []BuildError `json:"errors"`
}

// BuildError represents a single compiler or linter diagnostic.
type BuildError struct {
	File     string `json:"file"`
	Message  string `json:"message"`
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	Severity string `json:"severity"` // "error" | "warning" | "note"
}

// DiffRequest asks the daemon to apply a unified diff to a project file.
type DiffRequest struct {
	ContextID string `json:"context_id"`
	FilePath  string `json:"file_path"`
	Diff      string `json:"diff"`
}

// DiffResponse reports whether the diff was applied successfully.
type DiffResponse struct {
	Applied bool   `json:"applied"`
	Error   string `json:"error,omitempty"`
}

// HealthResponse is returned from GET /health.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	AIReady bool   `json:"ai_ready"`
	Port    int    `json:"port"`
}
