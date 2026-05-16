// Package ai provides an abstraction over AI inference providers.
package ai

import (
	"context"
	"fmt"

	"github.com/forgeapp/forge-daemon/internal/config"
)

// AskRequest carries everything a Provider needs to answer a question.
type AskRequest struct {
	Instruction string
	Context     []ContextFile
	Stream      bool
}

// ContextFile is a project file included as context for the AI model.
type ContextFile struct {
	Path    string
	Content string
}

// Provider is the common interface for all AI backends.
type Provider interface {
	// Ask sends the request and returns a channel of text chunks.
	// The channel is closed when the response is complete.
	// If Stream is false, exactly one chunk is sent before closing.
	Ask(ctx context.Context, req AskRequest) (<-chan string, error)

	// Model returns the underlying model identifier string.
	Model() string

	// Name returns the provider name ("claude", "ollama", "null").
	Name() string
}

// New creates a Provider for the given name using cfg for configuration.
// Valid names: "claude", "ollama", "null".
func New(name string, cfg *config.Config) (Provider, error) {
	switch name {
	case "claude":
		return newClaude(cfg)
	case "ollama":
		return newOllama(cfg)
	case "null", "mock", "":
		return newNull(cfg), nil
	default:
		return nil, fmt.Errorf("ai: unknown provider %q (valid: claude, ollama, null)", name)
	}
}
